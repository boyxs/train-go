# 通用标签系统 + 搜索服务 — 架构设计（服务拆分版）

> 配套：`prd/tag/PRD.md` · 原型 `prd/tag/prototypes/01~06-*.png`
> 决策基线（已确认）：**tag / search 各自拆为独立 gRPC 服务；core 退为 HTTP 网关(BFF)；跨服务同步 gRPC 调用（非事件驱动）**
> 原则：严格落地 6 屏原型 · KISS · 复用现有 gRPC 服务拆分范式（comment/interaction/relation）+ 现成 `api/proto/search/v1`

---

## 1. 需求摘要

把内容检索与标签能力从 core 剥离为两个独立可部署服务：**search**（ES + embedding，从 core 抽出）与 **tag**（MySQL `tag`/`tagging`，新建）。core 保留全部 HTTP 路由作 BFF，服务层改调二者 gRPC。做完 = 6 屏原型交互全部有真实接口支撑，且 ES/标签能力部署隔离、独立伸缩。article 表与 core 现有发布/阅读链路结构不动。

---

## 2. 服务拓扑与边界

| 服务 | 端口 HTTP/gRPC | 独占数据 | 对外（gRPC） | 依赖 |
|------|---------------|---------|-------------|------|
| **search**（抽出） | `8080/8081` | ES `article_v1` + embedding client | IndexArticle · RemoveArticle · SearchArticles · RecommendTags | ES、embedding provider |
| **tag**（新建） | `8070/8071` | MySQL `tag`+`tagging` | Suggest · SyncTags · Detail · BatchBySlug · BatchByBiz · BizIdsByTag | MySQL |
| **core**（BFF） | 8010/8011（不变） | MySQL `article`（不动） | 保留全部 HTTP；服务层持 tag/search/interaction gRPC client | tag/search/interaction/comment gRPC |

- **服务发现**：tag/search 注册 etcd，core 经 etcd 解析（沿用项目 gRPC 服务发现；tag/search 纯静态本地配置 + etcd 仅做发现，配置变更靠重启，与 worker 同档）。
- **端口铁律**：占用推进到 tag `8070/71`、search `8080/81`（下一个新服务 `8090/91`）。
- **embedding 迁移**：`internal/service/ai/embedding` 随 search 迁走（core 不再依赖 embedding）。
- **Metric 命名**：统一 `webook_<subsystem>_*`（`webook_es_*`/`webook_db_*`/`webook_grpc_*`），**禁止** `webook_tag_*`/`webook_search_*`，服务区分靠 prometheus 注入的 `job` label。

### 2.1 跨服务调用链（core BFF 编排）

```
① 发文章 (core Publish)         存 article(core DB)
                                  → tag.SyncTags(biz=article,bizId,names,source)         [gRPC]
                                  → search.IndexArticle(article + resolved tags)          [gRPC]
   ↳ 三库无分布式事务；tag/search 失败非致命(记日志，最终一致)——与现有 ES 索引 goroutine 同款降级

①b 下架/删除 (core Withdraw/Delete)  → tag.SyncTags(bizId, 空) 清该文关联(ref_count 连带-1)   [gRPC]
                                  → search.RemoveArticle(id)                               [gRPC]
   ↳ 保证 BizIdsByTag 只剩在架文章（tag 侧不 join article、不知 status，靠 core 清关联维持）

② 标签页 /tag/:slug             tag.Detail(slug)                                          [gRPC]
   标签页文章 /tag/:slug/articles
                                  tag.BizIdsByTag(slug,窗口) → ids(按 tagging.created_at DESC)
                                  → core 查 article(自库) + interaction.FindByBizIds(ids)  [gRPC]
                                  + tag.BatchByBiz(article,ids) 补每篇标签                  [gRPC]

③ 搜索 /search/article          search.SearchArticles(query, filterTags, page)            [gRPC]
                                  → hits(含 category/tags) + facets(slug+count)
                                  → core + interaction 计数 + tag.BatchBySlug 补 facet 名   [gRPC]

④ AI 荐标签 /tag/recommend      search.RecommendTags(title,content,k)                     [gRPC]
                                  → tag slug+count（kNN 相似文章标签聚合）
                                  → core + tag.BatchBySlug 补名                            [gRPC]

⑤ typeahead /tag/suggest       tag.Suggest(prefix,limit)                                 [gRPC]
```

---

## 3. 数据

### 3.1 tag 服务独占 — 两张新表（DDL 落 `webook/tag/` 建表脚本；article 表不动）

**`tag`**：id · name varchar(30) · **slug varchar(30)** · **type varchar(16) 'topic'** · description varchar(255) · **ref_count bigint** · created_at/updated_at(bigint 毫秒)
- `uni_tag_slug_type(slug,type)` · `idx_tag_type_refcount(type,ref_count)`

**`tagging`**：id · **tag_id** · **biz varchar(32)** · **biz_id bigint** · **source varchar(16) 'author'** · created_at/updated_at(bigint 毫秒)
- `uk_tagging_dedup(biz,biz_id,tag_id)` · `idx_tagging_tag_biz(tag_id,biz)` 反查 · `idx_tagging_target(biz,biz_id)` 正查

> DDL 铁律：单数表名 · 每列 COMMENT · bigint 毫秒 · utf8mb4_0900_ai_ci · 索引带表前缀。
> **硬删风格（无 soft_delete）**——untag = 物理删（同 relation `BlockRelation`；tag MVP 无删除路径）。TDD 中据 relation edge/计数表惯例定；`uk` 保活跃行唯一、无软删幽灵，重打标签直接 insert、无需 reactivate。（已实现 + 15 集成测试验证）
> **ref_count 在 tag 服务本地事务内** 随 SyncByBiz 增删 +1/-1（`GREATEST(0,...)` 防负；发文低频同步维护，不异步）。

### 3.2 search 服务独占 — ES `article_v1` 增量（非破坏，不 reindex）

mapping 加 `"tags": {"type":"keyword"}` + `"category": {"type":"keyword"}`；`ArticleESDoc` 加 `Tags []string`(slug) + `Category string`。
- 建索引后追加幂等 `PUT _mapping`（加已存在字段 no-op）。
- 存量 doc 无 tags/category → 编辑重发补；一次性 backfill（任务见 §5）。

### 3.3 core — `article` 表结构不动

标签不落 article 表（归一化在 tag 服务）；category 已有列不变。

### 3.4 缓存（KISS）

MVP 各服务均不加缓存（tag 详情单索引查、facet/搜索 ES 直出、标签下文章索引 join）。**P3+ 已给 tag 详情热点读补 Cache-Aside（`tag:detail:{slug}`，见 §F5）**；search facet/标签下文章、tag typeahead/list 仍 P1。

---

## 4. 接口

### 4.1 gRPC 契约（服务间，proto 单一真相源）

**`api/proto/search/v1/search.proto`（扩展现有）**
```proto
service SearchService {
  rpc SearchArticles(SearchReq) returns (SearchResp);          // 扩展：+filter_tags, +facets
  rpc IndexArticle(ArticleDoc) returns (google.protobuf.Empty); // 新增（core 发布调用，search 内部 embed+写 ES）
  rpc RemoveArticle(RemoveReq) returns (google.protobuf.Empty); // 新增
  rpc RecommendTags(RecommendReq) returns (RecommendResp);      // 新增（kNN 相似文章标签聚合）
}
message SearchReq   { string query=1; repeated string filter_tags=2; int32 page=3; int32 size=4; }
message SearchResp  { repeated ArticleCard cards=1; int64 total=2; repeated TagCount facets=3; }
message ArticleCard { int64 id=1; string title=2; string abstract=3; int64 author_id=4;
                      string author_name=5; string category=6; repeated string tags=7; int64 created_at=8; }
message ArticleDoc  { int64 id=1; string title=2; string abstract=3; int64 author_id=4; string author_name=5;
                      uint32 status=6; string category=7; int64 created_at=8; repeated string tags=9; }
message TagCount    { string slug=1; int64 count=2; }
message RecommendReq{ string title=1; string content=2; int32 k=3; }
message RecommendResp{ repeated TagCount tags=1; }
message RemoveReq   { int64 id=1; }
```

**`api/proto/tag/v1/tag.proto`（新建）**
```proto
service TagService {
  rpc Suggest(SuggestReq) returns (TagList);                    // typeahead 前缀
  rpc SyncTags(SyncReq) returns (TagList);                      // find-or-create + diff + ref_count（本地事务），返回已解析
  rpc Detail(DetailReq) returns (Tag);                          // 不存在 → codes.NotFound
  rpc BatchBySlug(SlugsReq) returns (TagList);                  // facet/推荐补名
  rpc BatchByBiz(BizIdsReq) returns (BizTagsResp);              // 列表每项标签（消 N+1）
  rpc BizIdsByTag(BizIdsReq) returns (BizIdsResp); // 候选窗口 ids + total（core 再聚合）
}
message Tag       { int64 id=1; string name=2; string slug=3; string type=4; string description=5; int64 ref_count=6; }
message TagList   { repeated Tag tags=1; }
message SuggestReq{ string prefix=1; int32 limit=2; }
message SyncReq   { string biz=1; int64 biz_id=2; repeated string names=3; string source=4; }
message DetailReq { string slug=1; }
message SlugsReq  { repeated string slugs=1; }
message BizIdsReq { string biz=1; repeated int64 biz_ids=2; }
message BizTagsResp { map<int64, TagList> tags=1; }             // bizId → 标签
message BizIdsReq { string slug=1; string biz=2; int32 limit=3; }  // 通用：biz 入参，窗口封顶(≤500)
message BizIdsResp{ repeated int64 ids=1; int64 total=2; } // ids 按 created_at DESC
```

### 4.2 HTTP 契约（core BFF，形状与非拆分版一致，仅后端换 gRPC）

| Method | Path | 请求 | 响应 data | 认证 | 中间件 |
|--------|------|------|-----------|------|--------|
| GET | `/tag/suggest` | `q`,`limit`≤10 | `[{name,slug,refCount}]` | JWT | — |
| POST | `/tag/recommend` | `{title,content}` | `[{name,slug}]`≤8 | JWT | 超时豁免 |
| GET | `/tag/:slug` | path slug | `{name,slug,description,refCount}` | 公开 | IgnoredPaths |
| POST | `/tag/:slug/articles` | `{page,size≤50,sort:new\|hot}` | `{list:[ArticleVO],total}` | 公开 | IgnoredPaths |
| POST | `/article`（扩展） | +`tags:[]string`≤5 | `{id}` | JWT | 原样 |
| POST | `/search/article`（扩展） | +`filter:{tags:[]string}` | `{list:[ArticleVO],total,facets:[{name,slug,count}]}` | JWT | 原样 |

- **ArticleVO**：`{id,title,abstract,author,category,tags:[{name,slug}],readCnt,likeCnt,collectCnt,createdAt}`。
- **错误码→用户消息**（core 侧 sentinel `internal/errs/tag.go`；gRPC 侧 tag 服务用 `codes.NotFound/InvalidArgument`，core 映射为 `*errs.Error`）：`TAG_LIMIT_EXCEEDED`(400 每篇≤5) · `TAG_NOT_FOUND`(404) · `TAG_NAME_INVALID`(400)。
- 空 `filter.tags` 时 `/search/article` 行为与现状**完全一致**（无回归）。
- **AI 推荐无 Prompt 契约**：走 embedding + ES kNN（search 服务内），非 LLM 文本生成。

### 4.3 搜索 facet — search 服务内单条 ES 查询

`bool{ should:[match title, match abstract, script_score(cosineSim vec)], filter:[status=2], msm:2 }` + `post_filter:{ bool.must:[term tags=slugX ...] }`（多选 AND）+ `aggs:{ tags: terms(field=tags,size=20) }`。hits=标签过滤后 · aggs=关键词命中集（不受已选标签影响，便于增删）。RecommendTags = kNN(content_vec) + `aggs.tags`。

---

## 5. 风险

- **分布式/一致性**：发文跨 core+tag+search 三库无事务 → tag/search 失败非致命降级(日志)，最终一致；重复发布幂等(article.Id>0 update + 标签全量 diff)。
- **可用性**：tag/search 挂 → 发文主流程仍成功（索引/标签补偿靠重发或 backfill）；标签页/搜索页依赖 tag/search，挂则该页降级空态+重试（core BFF 里 gRPC 调用带超时，失败返可感知错误）。
- **性能**：跨服务多一跳 RTT（内网 gRPC，可接受）；"最热"排序互动数在独立服务无法 DB JOIN → tag.BizIdsByTag 返 ≤500 候选窗口，core 拉 interaction 后内存排序分页（深翻页近似，精确热榜 P1）；列表标签 N+1（读侧）→ tag.BatchByBiz 批量；facet agg size 封顶 20。**写侧 N+1**（发文 SyncTags）→ 批量 Upsert + SyncByBiz 增删/计数批量化（详见 F4）：一次 `INSERT ... ON CONFLICT DO NOTHING`+回查解析全部 tagId、`CreateInBatches`/`DELETE...IN`+ref_count 同向分组 UPDATE，消逐行往返、缩短热门 tag 行事务锁持有。
- **并发**：SyncTags 的 tagging 增删 + ref_count 在 tag 服务本地事务原子；find-or-create 靠 `uni(slug,type)` 兜底并发建同名。
- **安全**：标签名/slug/数量服务端强校验；`/tags/*` 写需登录；slug 进 ES term/路由前剥 `#/?` 危险字符（CJK 原样，UTF-8 路由编码，免拼音库）。
- **回归**：article 表 + core 发布/阅读结构不动；search 从 core 抽出属**行为等价迁移**（chat 已消费 search gRPC，契约向后兼容，仅新增 rpc + 扩展字段）；ES 加字段非破坏。

---

## 6. 任务拆分

> 五阶段。粒度 2–5min/项，可独立验证。P1=抽 search，P2=建 tag，P3=core BFF 接线，P4=前端，P5=部署/可观测同步（服务拆分 14 点，见 §D）。

**P1 · 抽取 search 服务**
| # | 任务 | 依赖 | 验收 |
|---|------|------|------|
|1| 建 `webook/search/`（平铺布局）：搬 `article_search.go`(dao/repo/service) + `ioc/es.go` + embedding + main/wire | 无 | `cd search && go build ./...` 绿 |
|2| 扩展 `search.proto`：+IndexArticle/RemoveArticle/RecommendTags，SearchReq+filter_tags、SearchResp+facets，doc+category/tags；regen | 1 | pb 生成，`buf`/protoc 绿 |
|3| ES：mapping+`tags`/`category`；`ArticleESDoc`+字段；`Search` 加 filterTags(post_filter)+facets agg；新增 `RecommendTags`(kNN+agg) | 2 | dao 集成测试：过滤命中+facet 计数+kNN 荐正确 |
|4| search `grpc/` server 实现全部 rpc（Index 内部 embed）；wire+config(5 yaml, 8081) | 2,3 | `cd search && wire ./... && go test` 绿 |

**P2 · 新建 tag 服务**
| # | 任务 | 依赖 | 验收 |
|---|------|------|------|
|5| 建 `webook/tag/`：建表脚本 `tag`/`tagging` + DAO model + AutoMigrate | 无 | 建表 SQL↔struct 对齐 |
|6| `GormTagDAO`(FindBySlug(s)/UpsertTags 批量/Suggest) + `GormTaggingDAO`(SyncByBiz 批量事务/ListByBiz/BatchByBiz/BizIdsByTag) | 5 | dao_test 全绿（含事务 ref_count 双向） |
|7| `TagRepository` + `TagService`(Suggest/SyncTags/Detail/BatchBySlug/BatchByBiz/BizIdsByTag) | 6 | repo/service_test 全绿 |
|8| `tag.proto`(新建)+regen；`grpc/` server 实现；wire+config(5 yaml, 8071) | 7 | `cd tag && wire ./... && go test` 绿 |

**P3 · core BFF 接线**
| # | 任务 | 依赖 | 验收 |
|---|------|------|------|
|9| core 移除内嵌 search 实现，改注入 `searchv1.SearchServiceClient`；`Publish` 加 tag+search gRPC 调用（非致命）；`article` create/edit req +tags | 4,8 | 发文→tagging 落库+ES 可 facet；不带标签发布回归绿 |
|10| core 新增 `tagv1.TagServiceClient`；BFF service：SearchGateway(search+interaction+tag 聚合)、TagGateway(tag.BizIdsByTag+article+interaction+tag.BatchByBiz) | 8 | service_test（mock client）全绿 |
|11| core web：`TagHandler`(/tag/suggest,/tag/recommend,/tag/:slug,/tag/:slug/articles) + 扩展 `/search/article`(filter.tags+facets)；路由/中间件(IgnoredPaths/超时豁免) | 10 | web_test 全绿；路由无 `/api` |
|12| core `ioc` + wire：tag/search gRPC client(etcd 发现)；`wire ./...` | 9,11 | `make wire`+`make verify`(GOWORK=off) 全绿 |

**P4 · 前端（core HTTP，严格对齐 6 屏原型）**
| # | 任务 | 依赖 | 验收 |
|---|------|------|------|
|13| `api/tag.ts` + `TagInput`(typeahead+chips) + `AiTagSuggest` | 11 | 发文章打标签 5 态齐 |
|14| `/tag/[slug]` 页(TagHeader+SortTabs+列表+分页) + 搜索 `FacetBar` | 11 | 标签页/搜索 facet 5 态齐，移动 facet 横滑 |

**P5 · 部署 + 可观测同步（服务拆分 14 点 ×2，详见 §D）**
| # | 任务 | 依赖 | 验收 |
|---|------|------|------|
|15| 两服务 Dockerfile + docker-compose(定义+healthcheck+depends_on) + deploy.sh/.env(TAG_/SEARCH_IMAGE_TAG) | 4,8 | `./deploy.sh local` 起全栈 |
|16| prometheus 抓取+规则 · grafana 告警+看板(up/5xx/P99/goroutines) · nginx(仅 gRPC 无新 HTTP rewrite) | 15 | 监控见 job=tag/search |
|17| CI workflow ×2(paths 互斥) · 各服务 CLAUDE.md · webook/CLAUDE.md 端口台账 · CHANGELOG · backfill 脚本(存量文章→tagging→ES) | 15 | CI 触发；`grep -rn` 14 点无遗漏 |

---

## B. 分层设计（每服务平铺布局，不套 internal/）

**search 服务**（`webook/search/`）：`grpc/`(SearchServer) → `service/`(ArticleSearchService: Index/Remove/Search/RecommendTags，内部 embed) → `repository/`+`dao/`(ES) ；`ioc/`(es/embedding/grpc/etcd)。构造函数返回接口，实现带技术前缀（`ElasticArticleDAO`）。

**tag 服务**（`webook/tag/`）：`grpc/`(TagServer) → `service/`(TagService) → `repository/`(TagRepository) → `dao/`(`tag.go` GormTagDAO / `tagging.go` GormTaggingDAO)；`ioc/`(mysql/grpc/etcd)。`toDomain`/`toEntity` 单条 + `slicex.Map`。

**core**（BFF）：`web/`(TagHandler + 扩展 article_search) → `service/`(SearchGatewayService / TagGatewayService，持下游 gRPC client 做聚合，镜像现有 `GRPCCommentService`) → 下游 tag/search/interaction gRPC。`Publish` 注入 search+tag client。**移除** core 内嵌 search service/repo/dao（迁至 search 服务）。

---

## C. Wire / DI 变更

- **search 服务** wire：ES client + embedding + ArticleSearchService + SearchServer + grpcx server（`searchProviderSet` 从 core 迁入本服务）。
- **tag 服务** wire：mysql + TagDAO/TaggingDAO + TagRepository + TagService + TagServer（`tagProviderSet`）。
- **core** wire：新增 `searchv1.SearchServiceClient`、`tagv1.TagServiceClient`（etcd 发现，镜像现有 comment/interaction client 注入）；`SearchGatewayService`/`TagGatewayService`/`TagHandler` provider；`Publish` 依赖加两 client。移除 core 的 search 本地 provider。各服务 `wire ./...`；`make wire`+`make verify`。

---

## D. 服务拆分 14 点检查表（tag & search 各一份，收尾 `grep -rn` 核对）

| 维度 | 文件 | tag(8070/71) | search(8080/81) |
|------|------|-------------|-----------------|
| 应用配置 | `<svc>/config/{local,dev,staging,prod,test}.yaml` | mysql + grpc addr :8071 + otel.service_name=tag | es + embedding + grpc :8081 + otel=search |
| Wire | `<svc>/wire.go` + regen | tagProviderSet | searchProviderSet(迁入) |
| Prometheus 抓取 | `deploy/prometheus/prometheus.yml` | job=tag targets :8070 | job=search :8080 |
| Prometheus 规则 | `deploy/prometheus/rules/*.rules.yml` | grpc/db 模式 | grpc/es 模式 |
| Grafana 告警 | `deploy/grafana/provisioning/alerting/<svc>.yml` | up/5xx/P99/goroutines `{job="tag"}` | `{job="search"}` |
| Grafana 看板 | `dashboards/*.json` | services-overview 加 tag | 加 search |
| Docker compose | `deploy/docker-compose.yaml` | 服务定义+healthcheck+depends_on(mysql/etcd) | +depends_on(es/etcd)，core depends_on tag/search |
| Nginx | `deploy/nginx/conf.d/default.conf` | 无新 HTTP（core BFF）—仅确认不需 upstream | 同（HTTP 仍走 core） |
| 部署脚本 | `deploy/deploy.sh` | logs/restart 认 tag | 认 search |
| 部署变量 | `deploy/.env.*(.example)` | `TAG_IMAGE_TAG`/APP_ENV | `SEARCH_IMAGE_TAG`/`ES_*` |
| CI | `.github/workflows/<svc>-ci.yml` | paths=webook/tag/** 互斥 | webook/search/**（+api/proto/search） |
| Dockerfile | `<svc>/Dockerfile` | 多阶段(context=webook/) | 同 |
| Metric 命名 | 应用 builder | `webook_db_*`/`webook_grpc_*`（禁 `webook_tag_*`） | `webook_es_*`/`webook_grpc_*` |
| 文档 | `<svc>/CLAUDE.md` + CHANGELOG + 本 PRD | 拆分原因/边界/接入 | 抽出原因/契约兼容 |

> 发版：新增 tag `webook-tag-v*`、search `webook-search-v*`，`deploy/.env.prod.example` 同步 `TAG_IMAGE_TAG`/`SEARCH_IMAGE_TAG`。

---

## E. 前端（core HTTP，不变）

沿用非拆分版：`TagInput`+`AiTagSuggest`（发文章）· `/tag/[slug]`（TagHeader+SortTabs+列表+分页）· 搜索 `FacetBar`（多选 URL 驱动）。5 态齐全，标签 pill 复用 `CategoryTag`，移动 facet 横滑。**前端无感知服务拆分**——仍调 core `/tags/*`、`/tag/*`、`/search/article`。

---

## 附：本轮不做（KISS 边界）

标签订阅/广场(**P1，已做**：关注订阅子系统 + 本周新增统计，见 CHANGELOG 2026-07-11) · 用户/对话标签(P2，`biz` 已预留) · 标签治理(P2) · 缓存层(**tag 详情已做，见 §F5**；search facet/list、typeahead 仍 P1) · 精确全量热榜深翻页(P1) · 事件驱动同步(现同步 gRPC) · category 收编为 tag(未来)。

---

## F. 决策记录（P3+ 迭代，唯一真相源补充）

### F1. core BFF 命名：折叠成 `GRPC{X}Service`，去掉 `_gateway`
core 侧 tag/search 的 BFF 从「适配层 + `SearchGatewayService`/`TagGatewayService` 聚合层」两层**折叠成一个 `GRPCSearchService`/`GRPCTagService`**，与既有 `GRPCCommentService`/`GRPCRelationService` 同构（持下游 client + aux svc、service 层聚合、单一 pb↔domain 映射点）。
- **为何**：本仓 BFF 网关无 `Gateway` 命名先例，标准是 `GRPC{领域}Service`；`GRPCCommentService` 本身也硬钉 `biz="article"`——article 耦合在 core BFF 层是常态，不需另起模式。
- **无环**：两个 impl 互相持对方的**裸 gRPC client**（非对方 Service 接口），如 `GRPCCommentService` 持 `intrSvc`，wire 构造无循环。
- **article 耦合合规**：coding-rules #8「单业务服务（如 search 专搜 article_v1）可用业务名」。

### F2. 搜索**不做 generic**：per-biz 打字化 + 共享基础设施
「用户/对话搜索要做」**不等于**把 search 做成「归一化 doc + 通用 query + 配置」的强 generic——那是漏抽象。能归一的是存储形状，漏的是**相关性/查询语义**（article 向量 kNN、user 前缀 typeahead、comment 全文各不相同；user 硬套向量还白烧 embedding）。
- **标准形态**：search 服务 = **检索基础设施之家**（共享 ES/embedding/脚手架）+ N 个 **per-biz 打字化搜索能力**（各自 typed RPC + mapping + query）。**通用基础设施，不通用契约。**
- 与「comment/tag 能通用（实体统一、`biz` 只是命名空间）、search 不能（相关性不统一）」同一根线。
- 加可搜 biz 的标准配方见 `webook/search/CLAUDE.md`。当前仅 `article`。

### F3. ES 索引「别名 + 版本化」
app/查询只认稳定别名（`article`），物理索引版本化（`article_v1`）；`ensureIndex` 幂等确保 `别名→物理`（存量补挂别名、数据不动）。改 mapping = 建 `v2`→reindex→原子切别名→删旧，**零停机**。mapping 单一真相源 = `search/repository/dao/article_index.json`（app `//go:embed` 与 `mk/es.mk` 同读一份）。

### F4. tag 写路径批量化（消写侧 N+1）
发文/改标签的 tag 写链路原为逐条往返，批量化后固定几次查询完成，语义完全等价、gRPC 契约与缓存策略（仍 P1）不变。对齐 relation/interaction 既有 `clause.OnConflict{DoNothing}` + `GREATEST(0, cnt±?)` 惯例。
- **SyncTags**（`service/tag.go` + `dao.UpsertTags`）：原对每个标签名循环 `Upsert`（各 1 SELECT + 可能 1 INSERT，5 标签 → 5~10 次**串行**往返）；改一次 `CreateInBatches(OnConflict{DoNothing})` 只建缺失 + 一次 `SELECT WHERE type=? AND slug IN(...)` 回取全部真实 id（含并发新建），**5~10 → 2 查询**。DoNothing + 回查天然兜底 `uni(slug,type)` 并发建同名，比原循环更稳；repo 层按输入 slug 顺序重排，返回顺序=入参顺序不变。
- **SyncByBiz**（`dao/tagging.go`）：原事务内逐行 `Create`/`Delete` + 逐行 ref_count UPDATE（5 变更 → ~11 语句）；改 `CreateInBatches` 批量插关联 + 一次 `DELETE ... WHERE (biz,biz_id,tag_id IN)` 批量删 + ref_count 按同向（新增 +1 / 删除 -1）各一条 `GREATEST(0, ref_count±1)` UPDATE，**事务内语句数与标签数解耦（~4 条）**，缩短热门 `tag` 行锁持有、降并发争用。
- **无回归**：由 tag 现有集成测试（ref_count 双向增减 / 超 5 拒绝不落库 / 重打标签幂等 / 并发 uni 兜底）验证；`go build`+`vet`+`go test ./...`+`GOWORK=off` 五重绿。

### F5. tag 详情缓存（Cache-Aside，把 Redis 引入 tag 服务）

tag 服务原为 MySQL-only（拆分时刻意剥了 vestigial redis 块）。P3+ 给**详情热点读**（`GET /tag/:slug` 每次加载都打）补 Cache-Aside，镜像 relation 的 `RedisRelationCache` + `pkg/redisx` + `consts/cache.go` 三件套。**只缓存 Detail**，`isFollowing`（per-viewer 点查、唯一索引）与 typeahead/list/facet 不缓存（KISS，同 relation「关系态 P0 不缓存」）。

- **key / 值 / TTL**：`tag:detail:{slug}` → JSON(`domain.Tag`：name/slug/type/description/**refCount/followCount/weeklyNewCount**)，TTL `10min + jitter(0~5min)`。`tag/consts/cache.go` 定义 `TagDetailPattern`/`TagDetailTTL`。
- **TTL 为何 10min（而非 relation stats 的 24h）**：relation stats 非时窗、每次 follow 写精确失效 → 24h 安全；tag 详情含 `weeklyNewCount`（7 天滚动窗，**时间流逝即漂移、无写触发失效**），短 TTL 把漂移兜到 ≤15min（对 7 天窗 <0.15%）。refCount/followCount 靠写失效保持准。
- **Cache-Aside**（`repository/tag.go`，`InternalTagRepository` +`cache`+`logger`）：`Detail` 先 `GetDetail` 命中即返 → miss(`redis.Nil`)/缓存故障回源 DB(`findBySlug`+`CountRecentByTag`) → `SetDetail` 回填（失败记日志不阻断）。**Detail 对外签名/行为不变，上层透明**。
- **写失效**（对齐编码规则 6「写后必清缓存」）：`Follow`/`Unfollow(slug)` 真翻转时 → `DelDetail(slug)`；`SyncByBiz` 返回 ref_count 变化的标签 id（**新增 ∪ 删除**）→ repo `FindByIds` 解析 slugs → `DelDetail`，新增与移除标签**都精确失效**（含清空关联 `tagIds=[]` 的场景，被移除标签的 refCount 立即反映，不靠 TTL）。
- **取舍**：① 缓存-DB 竞态（reader 回填晚于 writer DEL）与 relation 同款、短 TTL 兜底可接受；② Redis 故障读回源、写失效失败记日志不阻断（脏缓存至 TTL）；③ 引入 Redis 依赖 = tag 从 MySQL-only 变依赖 Redis（Redis 挂时 Detail 仍可用），集成测试从「只需 MySQL」变「需 MySQL+Redis:6379」。
- **基础设施同步**：`ioc/redis.go`（镜像 relation：`redisx.NewClient`+prometheus hook+otel）· 5 config + test.yaml 加 `data.redis`（local/test 明文 6379、dev/staging/prod `${REDIS_PASS}`）· `wire.go`+`integration/setup/wire.go`(+`setup/redis.go`) 加 provider · `docker-compose` tag 服务 `depends_on: redis{healthy}` + `REDIS_PASS` env。
