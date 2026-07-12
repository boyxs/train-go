# 通用标签系统 + 搜索服务 — HANDOVER（续作指南）

> 用途：本次超长 session 已完成两服务的**全部业务逻辑 + gRPC 契约**（都在磁盘、测试全绿）；剩余是**管道/接线/部署**大体量工作，建议在新会话（满血上下文）续做。
> 配套：`prd/tag/PRD.md`（需求 + 6 原型）· `prd/tag/ARCHITECTURE.md`（双服务拆分架构，唯一真相源）

## 一、已完成（✅ 全绿，勿重做）

**设计/架构**：PRD + 6 屏原型（`prd/tag/prototypes/01~06`）· ARCHITECTURE.md（双服务拆分：core BFF + 同步 gRPC）。

**tag 服务**（`webook/tag/`，端口规划 8070/8071，MySQL）——**32 测试绿**
- `domain/tag.go`：`Tag`、`NormalizeSlug`（小写/trim/空白折叠/剥`/?#%&`/CJK 原样，免拼音库）、`IsValidTagName`、`MaxTagsPerBiz=5`、`TagTypeTopic`/`TagSourceAuthor/AI`。`domain/tag_test.go` 8 例。
- `repository/dao/tag.go`：`TagDAO`（**`UpsertTags`** 批量/FindBySlugs/FindByIds/Suggest）+ `Tag` model（**无 soft_delete**，硬删风格同 relation）。批量 Upsert 见 ARCHITECTURE F4。
- `repository/dao/tagging.go`：`TaggingDAO`（**`SyncByBiz`** 事务 diff+ref_count GREATEST防负 / ListByBiz / BatchByBiz / **`BizIdsByTag`**〔通用，非 ArticleIdsByTag〕）+ `Tagging` model。
- `repository/tag.go`：`InternalTagRepository`（协调两 DAO + toDomain）。
- `service/tag.go`：`InternalTagService`（`SyncTags` 校验≤5/归一/去重/批量Upsert/SyncBiz、Detail/Suggest/TagsBySlugs/TagsByBiz/BizIdsByTag）。
- `errs/error.go`：`ErrTagLimitExceeded`/`ErrTagNameInvalid`/`ErrTagNotFound`。
- `grpc/tag.go`：`TagServer`（pb↔domain，实现 `tagv1.TagServiceServer`）。
- 测试：`integration/{tag_dao_test,tagging_dao_test,tag_service_test}.go` + `setup/db.go` + `main_test.go` + `config/test.yaml`。真库 `webook_test`（root/13520@localhost:3306）。

**search 服务**（`webook/search/`，端口规划 8080/8081，ES）——**9 测试绿**
- `domain/article.go`：`Article`/`TagCount`/`SearchResult`。
- `repository/dao/article_search.go`：`ArticleSearchDAO`（Upsert/Delete/**Search**〔BM25+向量 script_score+status过滤+`post_filter`标签AND+tags terms facet〕/**RecommendTags**〔kNN近邻+Go侧标签聚合〕）+ `ArticleESDoc`（+Tags/+Category）+ `article_index.json`（mapping +tags/+category keyword）+ `ErrESDocNotFound`。
- `repository/article_search.go`：`ESArticleSearchRepository`（domain↔dao）。
- `service/article_search.go`：`InternalArticleSearchService`（`Embedder` 接口自声明；Search 校验/分页/embed 编排、Index/Remove/RecommendTags；embed 失败降级）。
- `errs/error.go`：`ErrSearchQueryEmpty`/`TooLong`/`DocNotFound`。
- `grpc/search.go`：`SearchServer`（pb↔domain，实现 `searchv1.SearchServiceServer`）。
- 测试：`integration/{article_search_dao_test,article_search_service_test}.go`（真 ES 9.3.2，elastic/elastic；每测试独占索引；stub embedder 固定向量）+ `setup/es.go` + `main_test.go` + `config/test.yaml`。

**proto**（`api/proto/`，已 `protoc` 生成到 `api/gen/`）
- `tag/v1/tag.proto`（新）：`TagService`（Suggest/SyncTags/Detail/BatchBySlugs/TagsByBiz/BizIdsByTag）。
- `search/v1/search.proto`（**向后兼容扩展**：+filter_tags/+facets/ArticleCard补字段/+IndexArticle/RemoveArticle/RecommendTags/ArticleDoc）——chat 现有 `SearchArticles` 消费不破。
- 生成命令：`cd api && make gen`（全量）或仅两个：`protoc --proto_path=proto --go_out=gen --go_opt=paths=source_relative --go-grpc_out=gen --go-grpc_opt=paths=source_relative proto/tag/v1/tag.proto proto/search/v1/search.proto` + `goimports -w gen/tag gen/search`。

**go.work**：已加 `./tag`、`./search`。两 go.mod 已 tidy（含 `replace ../api ../pkg ../shared`）。

## 二、验证现状

```
cd webook/tag    && go test ./...   # 32 绿（需 MySQL 3306 + webook_test 库）
cd webook/search && go test ./...   # 9  绿（需 ES 9200，elastic/elastic）
# 两模块 go vet + goimports 均干净
```
基础设施：MySQL 3306 ✅ · Redis 6379 ✅ · ES 9200 ✅（xpack.security elastic/elastic，v9.3.2）· docker CLI 不可用（用本机原生服务）。

## 三、剩余路线（管道/接线，未做）

按依赖顺序：

1. ✅ **两服务可启动（已完成，全绿）**——镜像 `relation/` 的 ioc+wire+main+config：
   - tag `ioc/{config,db,etcd,grpc,logger,otel,time}.go` + wire.go + main.go + config/{local,dev,staging,prod}.yaml（`:8070/:8071`，otel=webook-tag，**MySQL only 无 redis**：从 test.yaml 也剥了 vestigial redis 块，5 份同构）。
   - search `ioc/{config,es,embedding,redis,etcd,grpc,logger,otel}.go` + wire.go + main.go + config/{local,dev,staging,prod}.yaml（`:8080/:8081`，otel=webook-search，data.es+embedding+ollama+redis）。**无 time.go**（search 全链路 int64 毫秒、无 carbon，`InitTimezone` 会成 unused provider）。ES DAO 需 index 入参 → `ioc.InitArticleSearchDAO` 读 `data.es.index`（默认 article_v1）构造，避免裸 string 进 wire graph。embedding 用 `wire.Bind(new(service.Embedder), new(embedding.Client))`。
   - **embedding 已抽到 `pkg/embedding`**（用户拍板，非留 core）：types/openai/ollama/failover + `CachedClient`（内联 cache key，原 `internal/consts.EmbeddingCachePattern` 已删）+ 3 个 test + mocks。core 4 处 re-point（ioc/es.go、service/article_search.go[_test]），删 `internal/service/embedding/` + `internal/repository/cache/embedding.go`。`mk/mock.mk` embedding mock 挪到 pkg 段。
   - **依赖对齐**：tag/search `go mod tidy` 后 otel/grpc/go-redis 会漂到 latest（v1.44/v1.81/v9.21），已 `go get @` 钉回 relation 同版（otel v1.43.0、grpc v1.79.3、go-redis v9.18.0，否则 workspace 拉高 → core 的 `redismocks.MockCmdable` 缺 ARCount 编译炸）。
   - **Makefile**：`MODULES` + `wire` loop 已加 `tag search`（原缺 → verify 盲区）。`make verify` 12 模块 workspace + GOWORK=off **全绿**。
   - ✅ **`make wire` 已修**（本次）：改为逐模块只对「含 wireinject 的目录」跑 `wire gen`（模块根 + 部分 integration/setup），不再用 `wire ./...`——后者会扫到只含 `_test.go` 的 `integration` 包、报 "no files to derive output directory from" 而整体退 1（wire_gen.go 其实已写出）。现 `make wire` 全绿退 0，逐模块 `cd <svc> && wire ./...` 亦仍可用（照旧有那行 benign 噪声）。
2. ✅ **core BFF（已完成，全绿）**——core 退为 tag/search 的 gRPC client，彻底去掉 ES/embedding：
   - ioc/grpc.go：`SearchConn`/`TagConn` + `InitSearchConn/Client`、`InitTagConn/Client`（etcd 发现，镜像 comment/interaction）；`InitGRPCServer` 去掉 searchSrv 参 + 停注册 search server。core 5 份 config 加 `client.grpc.webook-search`/`webook-tag`。
   - service BFF（**折叠版**，同构 `GRPCCommentService`；无 `_gateway`——曾拆「适配层+网关层」两层，已折叠，见 ARCHITECTURE §F1）：`service/search.go`（`SearchService`/`GRPCSearchService`，持 searchCli+tagCli+intrSvc：`IndexArticle`/`RemoveArticle` + `Search` 聚合〔interaction 计数+tag 名+facet〕）+ `service/tag.go`（`TagService`/`GRPCTagService`，持 tagCli+searchCli+readerSvc+intrSvc：`SyncTags`/`ClearTags`/`Suggest`/`Recommend`/`Detail`/`TagArticles`〔窗口 200→reader BatchDetail+interaction+TagsByBiz，内存 hot 排序+分页〕）。单一 pb↔domain 映射点 + 共享 `toTaggedArticle`/`batchArticleInteraction`/`resolveTagNames`；两 impl 互持对方**裸 client**无环。
   - web：`web/tag.go`（`TagHandler` + 共享 VO）+ 重写 `web/article_search.go` 用 `SearchService`（+filter.tags/+facets）；`article` editReq/domain.Article + `Tags []string`。
   - ioc/web.go：注册 TagHandler；超时豁免加 `/tags/recommend`；**新增 `jwtx.IgnoredPrefixes("/tag/")`**（`/tag/:slug`、`/tag/:slug/articles` 带路径参数公开——`IgnoredPaths` 是精确匹配挡不住动态段，故给 `pkg/jwtx` 加了 `IgnoredPrefixes` 前缀放行；`/tags/*` 不匹配该前缀仍需登录）。
   - Publish 接线（`service/article.go`）：后台 goroutine `tag.SyncTags(names)→resolved slugs→search.IndexArticle(带 tags)`；Withdraw/Delete→`tag.ClearTags + search.RemoveArticle`，均非致命降级。
   - **移除**：`internal/grpc/search_server.go(+test)`、`service/article_search.go(+test)`、`repository/article_search.go`、`repository/dao/article_search.go`、`ioc/es.go`、旧 search/repo mock；wire.go `searchProviderSet`→`tagSearchProviderSet`。
   - **chat 重指向**：`chat/ioc/grpc.go` 加 `SearchConn` 直连 `webook-search`（原走共享 CoreConn，因 CoreConn 还带 article-reader 不能整体改），chat 5 份 config 加 `client.grpc.webook-search`，wire 加 `InitSearchConn`。
   - **集成测试桩**：`internal/integration/setup/fake_search.go`（`FakeSearchService`/`FakeTagService` no-op），发布链路后台调用空转。
   - ✅ **搜索定案（本轮）**：不做 generic，per-biz 打字化 + 共享基础设施（ARCHITECTURE §F2，`webook/search/CLAUDE.md` 有「加可搜 biz 配方」）；ES **索引别名+版本化**（§F3：别名 `article`→物理 `article_v1`，`ensureIndex` 幂等挂别名，零停机 reindex；`mk/es.mk` 已对齐读 search json）。
   - ✅ **tag 测试补齐（本会话）**：`GRPCTagService` service_test（`internal/service/tag_test.go`，新增 `web/grpcmocks/tag_mock.go` + `mk/mock.mk` 一行——Detail 聚合/FollowStatus 降级/未登录跳过/错误传播 + Follow/Unfollow，5 例全绿）；`TagHandler` web_test（`internal/web/tag_test.go`，Follow/Unfollow/Detail 登录+匿名，4 例全绿）。
   - ⚠ **遗留 follow-up（tag 之外，未做）**：`GRPCSearchService` service_test + 搜索 handler web_test；`jwtx.IgnoredPrefixes` 单测（该方法已被 07-11 `@Public` 路由改造取代、趋于废弃）。集成测试环境依赖：core 全量需 Redis 单机/集群、search 需 ES——本会话已实跑 tag e2e（真 MySQL）；`make verify` 12 模块 build/vet+GOWORK=off 双绿。
3. ✅ **前端 P4（本会话完成，`next build` 全绿）**：`api/tag.ts`（suggest/recommend/findTag/pageTagArticles）+ `types/tag.ts` · 发文章 `components/tag/TagInput`（antd Select tags：typeahead 带「n 篇」+ 创建项 + teal chips + ≤5）+`AiTagSuggest`（紫色候选）接入 `views/article/edit.tsx` · `app/(auth)/tag/[slug]` 公开页 + `views/tag/detail`（TagHeader + 最新/最热 + 分页）+ 共享 `components/tag/TaggedArticleCard` · 搜索 `components/tag/FacetBar` + 重做 `views/search/index`（filter.tags + facets）。**路由已改名 `/tags/*`→`/tag/*`**（不再靠单复数区分公开；后端 `server.Public.GET` 自声明，见 CHANGELOG 07-11「鉴权路由自声明」）。复用 `utils/format.formatCount`。

   **续作（本会话进行中，用户「全部 + 严格对齐 6 原型」）—— 6 件套逐件推进，进度记于此**：
   - **①a 回显·编辑器 ✅**（TDD RED→GREEN）：后端 `service.TagService.TagsByBiz` 暴露 + `ArticleAuthorService.Detail` 聚合标签名（tag 失败降级不阻断）+ `AuthorDetailVO.tags` + handler + `fake_search.go` 桩 + mock 重生成；FE `Article.tags` + 编辑器 `setFieldsValue({tags})`。测试 `internal/service/article_test.go`（2 例）。build+vet+test+tsc 绿。
   - **①b 回显·阅读页 ✅**：`/article/reader/detail` 加 tags。**改为 reader HANDLER 聚合**（`readerSvc.Detail` 另有 gRPC server 调用方 `internal/grpc/article_reader_server.go`、内部调用不需标签 → 不改 Detail 签名、不加 tagCli 到 service，而在 reader handler 已有 errgroup 里加 `tagSvc.TagsByBiz`，与 article+interaction 聚合一致）：handler 加 `service.TagService` + `ReaderDetailVO.tags []tagVO`（`slicex.Map(tags, toTagVO)`）+ wire 重生成（`InitArticleReaderHandler` 补 `newFakeTagService` 桩，否则 `wire ./...` generate failed）。FE 新增 `types.ReaderArticleDetail`（tags `{name,slug}[]`，与作者端 `Article.tags` 名字数组**区分**）+ `findPublishedArticle` 返回它 + `read.tsx` 展示 chip 链 `/tag/:slug`。build+vet+test+tsc 绿。
   - **② 视觉细节 ✅**：`components/tag/TaggedArticleCard` 加 `activeTags?: string[]` prop，命中当前搜索筛选的 tag 卡内 teal 实心高亮（bg=colorPrimary/白字）；`views/search` 传 `filterTags`。tsc 绿。
   - **③ 分区 category ✅**（**补齐了半成品**）：原 category 是半成品——两表有列、reader 读路径映射、搜索/标签卡展示，但**作者 repo `toDomain`/`toEntity` + reader `Upsert` 写路径 + `editReq` 都不带 category → 实际恒空**。已补齐全链路：`article_author.go` `toDomain`/`toEntity` + `article_reader.go` `Upsert` 三处映射 `Category`、`editReq`+`Category`、Edit/Publish handler 设 `article.Category`、`AuthorDetailVO.category`（回显）；FE `Article.category`/`EditArticleReq.category` + `constants/article.ts ARTICLE_CATEGORIES`（`技术/生活/职场/阅读/其他`，**FE 常量小集，无后端枚举，可调**）+ 编辑器 `分区` `Select` + 回显。backend build+vet+test + FE eslint+tsc 绿。
   - **④ 公开页 App 头 ✅**（本会话核实：`PublicHeader` 已接入全部公开内容页 `article/feed`·`article/read`·`tag/detail`·`user/detail`，login/register 保持极简无头——原 handover「未做」为陈旧标注）。
   - **⑤ 标签关注订阅子系统 ✅**（本会话完成，5 阶段全绿，见 `CHANGELOG.md [2026-07-11] 标签关注订阅子系统`）：
     - P1 契约+DDL：proto `Tag.follow_count` + `Follow`/`Unfollow`/`FollowStatus` RPC（regen 向后兼容）；`tag` 表 +`follow_count` 列 + 新表 `tag_follow`（status 翻转，镜像 relation_follow）+ `TagFollowDAO`（FOR UPDATE 翻转 + GREATEST 防负 + 回读计数）；InitTable 加 TagFollow。
     - P2 tag 服务：repo（slug→tag `findBySlug` 复用）+ service + grpc 实现；集成 e2e +4（真 MySQL，翻转/幂等/多用户累计/not-found）；wire 重生成。
     - P3 core BFF：`domain.Tag`+FollowCount；`TagService.Detail(slug,viewerId)` errgroup 聚合 Detail+FollowStatus（降级 false）+ Follow/Unfollow 委托；`web` POST/DELETE `/tag/:slug/follow`（需登录）+ VO；`GET /tag/:slug` Public→**Optional**；fake 桩 + mock 重生成 + web handler 单测 +4。
     - P4 前端：`api/tag.ts` follow/unfollow + `types.TagDetail`+followCount/isFollowing + `FollowResult` + 新组件 `TagFollowButton`（2 态镜像 relation FollowButton）+ `views/tag/detail` 关注按钮/「N 人关注」（乐观覆盖按 slug 隔离，避开 set-state-in-effect）。
     - P5 收尾：CHANGELOG + 本 handover；`make verify` 12 模块双绿、goimports 干净、前端 eslint+tsc+build 绿。
     - 视觉收口（本轮 review）：原型 `02-标签浏览页` **本就有关注按钮 + 关注数**（此前 handover 误记「未含」，已订正）；移除 tag 详情页冗余面包屑（.pen 已无、重导出 02 PNG 同步旧图）+ 统一标签 chip 浅 teal 配色 `#F0FDFA`/`#0D9488`（`TaggedArticleCard`/`TagInput` 去 antd `colorPrimaryBg` 偏灰，对齐 read/user 页 + 原型）。
     - ⚠ **遗留**：关注数缓存层仍架构 P1；原型 meta 有「本周新增 12 篇」= ⑥ 未实现。
   - **⑥ 本周新增统计 ✅**（本会话完成，见 `CHANGELOG.md [2026-07-11] 标签「本周新增 X 篇」统计`）：proto `Tag.weekly_new_count`（仅 Detail 算）；tag `TaggingDAO.CountRecentByTag(tagId,since)` + `repo.Detail(slug,since)` + `service.Detail` 算 `now-7d`（滚动窗口，非日历周）+ grpc toPb + e2e（含 8 天前旧关联排除）；core `domain.Tag`+WeeklyNewCount + toDomainTag + VO + handler 单测；FE `types`+weeklyNewCount + `views/tag/detail` meta「· 本周新增 Z 篇」(Z>0)。跨 biz 口径同 ref_count；计算而非存储（滚动窗口无法增量维护）。**至此 §3 前端 6 件套 ①~⑥ 全部完成。**
4. ✅ **部署（本轮完成，镜像 relation）**：`search`/`tag` 各 **Dockerfile**（多阶段 context=webook/）· docker-compose 服务块+healthcheck+depends（+`docker-compose.local.yaml` build override）· `.env{,.example}` ×8 加 `TAG_/SEARCH_ IMAGE_TAG/APP_ENV/HOST_PORT` · prometheus job(`:8070`/`:8080`) · grafana 告警 `webook-{tag,search}.yml` · CI `webook-{tag,search}-ci.yml`（paths 互斥）+ 7 兄弟 CI paths-ignore 补 tag/search · CLAUDE.md 端口台账更新。依约定**不改** nginx（内部 gRPC 不入）/deploy.sh（服务名透传）/grafana 看板（`$service` 变量自动纳入）。所有 YAML 过 `python yaml` 校验。⚠ **docker CLI 未装 → compose/镜像未实跑**（静态+YAML 级）。
5. ✅ **收尾（07-11 续作完成大部）**：CHANGELOG 已追加（见 `CHANGELOG.md [2026-07-11]`）· backfill 命令 `internal/cmd/backfill-search/`（wire + 4 单测，`make backfill-search`）已建。**另修一处部署遗漏**：P3 core BFF 去 ES/embedding 未同步到 core 自身部署——已改 compose `webook-core`（去 stale `webook-es` depends + 死 `ES_PASS`/`QIANFAN_API_KEY` env）、`webook-chat`（加 `webook-search` depends）、core 5 份 config 删死块 `data.es`/`embedding`/`ollama` + `.env.example` 去 QIANFAN。`make wire && make verify` 双绿。**仍需**：backfill/compose 实跑（docker CLI + ES/search gRPC 未起）· core BFF service_test/web_test（§六）。

## 四、关键决策/踩坑（勿推翻）

- **命名铁律**（用户强调 + 已入 CLAUDE.md）：insert-or-ensure 用 **`Upsert`**（禁 FindOrCreate）；通用模块方法名禁硬编码 biz，用 **`BizIdsByTag`**（禁 ArticleIdsByTag）——biz 是入参。
- **通用标签模型**：`tag`（本体，`type` 命名空间）+ `tagging`（`biz`+`biz_id`+`source` 多态），零业务耦合。硬删（untag 物理删，同 relation BlockRelation），`ref_count` 事务内 GREATEST 防负同步维护。
- **BizIdsByTag 解耦**：按 `tagging.created_at` 排序、**不 join article**；下架时 core 调 `SyncByBiz(bizId,空)` 清关联保只剩在架内容。
- **AI 推荐**：复用 `content_vec` + ES kNN 聚合相似文章标签，**零 LLM / 零 Prompt 契约**。冷启动候选少可接受。
- **facet**：单条 ES 查询 `post_filter`（标签 AND 收窄 hits）+ `aggs`（tags terms，恒基于关键词命中集）。`TypedKeys(true)` + `*types.StringTermsAggregate` 解析。
- **Pencil 渲染坑**：文件大时远离画布原点(y≈0)的 frame 不进渲染缓存、`export_nodes` 导出空白 body；改法移到原点附近 + `open_document` 刷新后导出。
- **ES 测试**：每测试独占索引（`t.Name()` 生成）避免删建竞态；seed 后必 `Indices.Refresh`；service 测试用手写 stub embedder（固定 1024 维 one-hot 向量）不依赖外部 API。
- **端口**：tag 8070/8071、search 8080/8081（下一个新服务 8090/91）。metric 统一 `webook_<subsystem>_*` 靠 job label 区分。

## 五、workflow 状态

design ✅ · architect ✅ · tdd 进行中（tag+search 业务+gRPC + ioc/wire/main + embedding 抽 pkg + **P3 core BFF 全绿** + BFF 折叠去 `_gateway`〔§F1〕 + 搜索 per-biz 打字化不 generic〔§F2〕 + ES 别名+版本化〔§F3〕 + **P5 部署全套**〔剩余路线 §4〕 + **集成测 e2e 化 + xhigh code review 全修**〔见 §六〕 + **④公开页头核实 + ⑤标签关注订阅全链路**〔见 §3.续作〕；`make verify` 12 模块 workspace+GOWORK=off 双绿）。**下一步**：§3 前端 6 件套 ①~⑥ **全部完成** + tag **详情缓存 Cache-Aside 已落地**（架构 §F5 + CHANGELOG，引入 Redis、精确写失效、真 MySQL+Redis e2e 绿）。**⑥ 之后又交付**：P1 tag 详情缓存 + AI 推荐修复 + 全站配色 token 收口 + tag SQL 脚本/服务文档 + 注释精简（见 §七）。剩余架构 P1/P2 项：① search facet/标签下文章 + tag typeahead/list 缓存（P1）② 精确全量热榜深翻页（P1，需跨服务反范式化）③ 标签治理 / 广场（P2，需先 design）④ Tailwind 内置类收口 @theme（可选）。

## 六、本轮续作记录（集成测 e2e 化 + code review 全修 + 部署）

> 承 P3 之后本轮做完三件事，均 `make verify` 12 模块（workspace + GOWORK=off）双绿、goimports 干净。

**A. 集成测 e2e 化（tag/search/relation，interaction 风格）** —— 用户拍板「转 interaction 风格 e2e」：把三服务的集成测从「分层测（dao/repo/service 各 SetupSuite 手动装配）」重写为「整服务 wire + bufconn 发真实 gRPC 请求」。
- 各服务 `integration/setup/wire.go` 出 `InitXxxServer()`（`InitTagServer`/`InitSearchServer`/`InitRelationServer`，wire 装配真 dao/repo/service→server）；`wire gen ./integration/setup` 生成。
- 重写 `tag/integration/tag_server_test.go`、`search/integration/article_search_test.go`、`relation/integration/relation_server_test.go`（bufconn + errconv〔relation 另 +Validate〕拦截器 + `XxxServiceClient` 驱动），删旧分层测文件。
- **`main.go` 锚文件铁律**：每个 test-only `integration/` 目录放 `main.go`（`package integration` + 包 doc），否则 `wire ./...` 在 `detectOutputDir` 阶段（injector 检测**之前**）对空 `GoFiles` 目录报 `"no files to derive output directory from"` 整体退 1。已写进 **webook/CLAUDE.md「集成测试规范」**（含原理 + 「setup 必须 wire」）。`make wire` 回归惯用 `wire ./...`、退 0。
- search e2e 关键手法：`setup/embedding.go` 的**文本相关 stub embedder**（含 "rust"→pos2、否则 pos1），令经 gRPC IndexArticle 入库的向量分 Go/Rust 两簇、保住 kNN 判别；`SetupTest` 按物理索引名 `<别名>_v1`（`dao.IndexVersionSuffix`）删建。
- **验证**：`tag` e2e 真 MySQL **12 子测全过**；`search`/`relation` **仅编译过、未跑**（见「唯一未闭环」）。

**B. xhigh code review 全修（19 findings 全处理）**：
- 正确性：#2 `/tag/:slug/articles` 大 page 切片 panic（web `normalizePage` clamp page≤10000 + `pageTagged` `offset<0` 兜底）· #3 `/search/article` page int32 截断（同 clamp）· #4 tag dao `Suggest` LIKE 通配转义（`escapeLike`）· #5 `search` Index 写 ES 失败也降级 return nil（补 `TestIndex_WriteFail_Degrade`）· #10 `jwtx.IgnoredPrefixes("/tag/")` 加命名约定警示注释。
- 清理：#12 5 个 `InitXxxConn`→抽 `dialDownstream` · #13 Withdraw/Delete 重复 goroutine→抽 `clearTagAndIndexAsync` · #14 reader `batchInteraction`→复用共享 · #15 Search/TagArticles 无依赖下游调用→`errgroup` 并发。
- 补测：**新增 `search/service/article_search_test.go`**（stub embedder + stub repo，覆盖校验/clamp/降级/幂等，**不依赖 ES、CI 可跑**，已绿）；relation e2e 补 `TestUnblock`（不恢复关注）/`TestUnfollow_CountDecrementAndFloor`（GREATEST(0) 不为负）/`TestStats_CacheAside`（写后失效）。
- 被驳回不改：前端契约（fe 只读保留字段）、`slicex.Map`、`limit` 解析吞错（=0 落默认）。

**C. 部署 P5**：见剩余路线 §4（已完成）。

### 集成 e2e 闭环进度（2026-07-11 全部实跑通过）
- ✅ **search e2e**（ES 9.3.2 elastic/elastic）：`cd webook/search && go test ./integration/...`——`TestSearchServer` 5 子测（IndexAndRemove/RecommendTags/Search_KeywordAndFacets/Search_TagFilter/Search_Validation）全绿。顺带验证了 backfill 依赖的 `IndexArticle→ES` 写路径。
- ✅ **relation e2e**（MySQL 3306 + 单机 Redis 6379）：`cd webook/relation && go test ./integration/...` 绿（relation test.yaml 本就单机）。
- ✅ **core e2e**（MySQL + 单机 Redis 6379）：`cd webook/internal && go test ./integration/...` 全绿。**本轮为跑通改了两处（均非生产逻辑）**：
  1. `internal/config/test.yaml` redis 由 `mode: cluster`(7001-7003) 切 `mode: single`(6379)——为方便本地测试；**cluster 支持仍在**（`redisx.Config.Mode`，代码不变），yaml 里保留 cluster 块注释、未来起 3 主集群后切回即可。
  2. `internal/integration/setup/lock.go` 新增测试版 `InitLockClient`（锁指标走独立 `prometheus.NewRegistry()`），替掉 `infraSvcProvider` 里的生产 `ioc.InitLockClient`——否则每个用例调 `InitWebServer` 重建全套会在 `DefaultRegisterer` 上重复注册 lock collector 而 panic（`provideTestMiddlewares` 早有同款隔离，lock 是漏网的最后一个；纯 test-infra，不动生产）。
- ⏳ **backfill e2e / 部署 compose**：docker CLI 未装；backfill 实跑另需 etcd + search/tag gRPC server + MySQL 同时在（ES 单起不够）。search e2e 已覆盖其下游写路径。

## 七、⑥ 之后交付（P1 缓存 + AI 推荐修复 + 配色收口 + SQL/文档，07-11~07-12）

> 均已 `make verify` 12 模块双绿 + 相关 e2e/build 绿；详见 CHANGELOG 对应条目。

- **tag 详情缓存 Cache-Aside**（P1，架构 §F5）：Redis 引入 tag（原 MySQL-only）；`tag:detail:{slug}` JSON + jitter TTL 10min；`Follow/Unfollow` 真翻转 + `SyncByBiz` 返 affected tagIds **精确失效**；`isFollowing`/typeahead/list 不缓存。go-redis 钉 v9.18.0 对齐 core/relation。真 MySQL+Redis e2e 绿（命中不查库 + 写后失效）。
- **AI 推荐标签修复**（debug）：根因 = `search.RecommendTags` kNN **无相似度阈值** + 小语料 → 恒推 go/claude 不相关。修：dao kNN 按 `_score≥0.75`（cosine≈0.5）过滤远邻；proto `ArticleDoc +content` → core 发布下发正文 → search `Index` embed `title+content`（与 Recommend 同口径，无正文回退 abstract）。复现测试 FAIL→PASS + `TestIndex_EmbedsContent` 锁测。⚠ 阈值 0.75 按测试桩标定，**生产真实向量需实测微调**；存量文旧向量（title+abstract）需重发布才升级为 title+content。
- **全站配色 token 收口**：`constants/theme.ts` `PALETTE`（21 token）+ `app/globals.css @theme` 同值双写；~320 处 inline hex / Tailwind `[#hex]` → token（32 文件，4 subagent 并行）；修 antd Select 默认从主色推导的灰绿选中底色。零 in-mapping hex 残留、17 自定义工具类全命中 @theme。**未收**：Tailwind 内置类（`bg-white`/`text-gray-400` 等）仍在，属另一轮。
- **tag SQL 脚本 + 服务文档**：新增 `tag/scripts/tag.sql`（`tag`/`tagging`/`tag_follow` 三表 DDL 严格对齐 GORM model，throwaway DB 校验过）+ `tag/CLAUDE.md`（补拆分服务文档）；`webook/CLAUDE.md` 数据表规范 #10 对齐「按服务落点」（core→webook.sql、拆分服务→`<svc>/scripts/<svc>.sql`、ES 服务无 SQL）。
- **注释精简**：清本会话产出的关联性/填充注释（`见 §F`/`同 relation`/`镜像`/`KISS` 等），保留功能注释（幂等/降级/GREATEST/FOR UPDATE/窗口/索引语义）。
