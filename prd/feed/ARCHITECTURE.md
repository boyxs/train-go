# Feed 关注流架构设计

> 配套需求见同目录 `prd.md`，原型见 `prototypes/*.png`。
> 服务：**webook-feed**（新独立服务，纯同步 gRPC，`:8100/:8101`）+ **webook-core**（事件生产 + HTTP BFF 聚合）+ **webook-worker**（Kafka 消费派发）+ 前端。
> 三条项目铁律决定拓扑：① relation 铁律「事件由 core 生产、消费归 worker/feed」；② worker 铁律「所有异步消费收拢 worker，handler 只调 owner gRPC、不写业务」；③ 服务边界「不直连别服务私有库」。

## 0. 调用拓扑

```
写路径（异步扩散）：
  发布/撤回文章 → core ArticleAuthorService（写库成功）
      └─▶ 生产 Kafka: article_events（key=authorId，失败记日志不阻断）
              └─▶ worker FeedArticleConsumer 消费 ──gRPC──▶ feed.FanoutArticle / RemoveArticle
  关注/取关/拉黑 → core RelationService（已有）
      └─▶ Kafka: relation_events（已有，key=followerId）
              └─▶ worker FeedRelationConsumer 消费 ──gRPC──▶ feed.InvalidateInboxes（失效重建模式）

读路径（同步）：
  前端 /feed?tab=follow → core HTTP POST /feed/list（JWT）
      └─▶ core GRPCFeedService ──gRPC──▶ feed.ListFeed（收件箱 + 大V发件箱归并，miss 全量重建）
              │                              └─（重建/outbox miss 时）──gRPC──▶ core article.ListAuthorArticles
              └─ BFF 聚合：article FindByIds（含撤回过滤）+ user 昵称 + interaction 计数 + comment 计数 + tag 标签 → VO
```

**为什么 feed 是独立服务**：relation 拆分时已预留（`relation/CLAUDE.md`：feed 未来独立服务订阅 relation_events）；扩散写入是突发大批量 Redis 写，与 core 请求路径隔离；core/未来 chat 都可作平等 client。
**为什么 feed 无 MySQL**：feed 的数据本质是「published_article × relation 的缓存投影」，源头分别在 core 与 relation；直查别人库破坏边界 → 收件箱/发件箱全 Redis，源数据经 gRPC 拉取。丢失可重建，不需要持久化兜底。
**为什么消费者在 worker 不在 feed**：worker 铁律"收拢所有异步"；feed 保持与 relation 同构的「纯同步 gRPC」形态（部署/观测/配置全对齐现有模板）。

## 1. 需求摘要

已登录用户在广场页「关注」Tab 按时间倒序看到关注作者的已发布文章（游标无限滚动，卡片含计数+标签）；发布后分钟级可见、撤回/取关/拉黑后不可见。做完的标志 = PRD `prd.md` §7 九条验收全过。

## 2. 数据 + 接口

### 2.1 数据（全 Redis，无新 MySQL 表）

| Key（feed 服务 consts 定义） | 类型 | 内容 | 生命周期 |
|------|------|------|---------|
| `feed:inbox:{uid}` | ZSET | member=articleId，score=publishedAt（事件 ts，毫秒）| cap 2000（ZREMRANGEBYRANK），TTL 7d+jitter |
| `feed:inbox:built:{uid}` | String | "1"，收件箱已完整重建的标记（区分「未建」vs「建了但空」，避免空关注用户每次读都全量重建） | TTL 与 inbox 同批设置 |
| `feed:bigv:{uid}` | SET | 该用户关注中的大 V uid 集合（重建时算出） | TTL 与 inbox 同批 |
| `feed:outbox:{authorId}` | ZSET | 作者最近文章投影，member=articleId，score=publishedAt | cap 100，TTL 1h+jitter，cache-aside（miss 回源 core article gRPC） |

- **写扩散只进普通作者粉丝的 inbox**；大 V（粉丝数 ≥ 阈值）不扩散，读时经 outbox 拉取归并——「大 V 的百万粉丝」问题的推拉结合解。
- **关系变更 = 失效重建**：follow/unfollow/block 一律 DEL inbox+bigv+built，下次读全量重建。一招覆盖三种事件（含拉黑级联解除双向关注），无按作者摘除的复杂度；代价是变更后首次读慢（见风险）。
- **撤回 = 读时过滤**：inbox 不做撤回摘除；BFF `FindByIds` 查线上库天然滤掉已撤回/已删文章。feed 侧仅 DEL 作者 outbox。
- **outbox 追加必须「存在才追加」**（Lua：EXISTS→ZADD+trim+EXPIRE 原子）——否则事件把不存在的 outbox 创建成只含 1 条的假全量。inbox 无条件 ZADD 安全：无 built 标记的「部分收件箱」会在读时被重建覆盖。
- `published_article` 无表结构改动；outbox 回源查询 `WHERE author_id=? AND status=published ORDER BY updated_at DESC LIMIT 100`（走现有 author_id 索引，Select 排除 Content BLOB），发布时间以 updated_at 近似（编辑重发置顶，微博同语义）。

### 2.2 事件契约（新增 article_events）

```jsonc
// topic: article_events，key = authorId（同作者 publish→withdraw 有序）
// 生产：core InternalArticleAuthorService.Publish / Withdraw 成功后（复用 internal/events 通用 Producer；失败记日志不阻断主流程）
// 消费：worker consumer/event/article.go 定义同构副本（topic+JSON 契约，两端不共享代码，contract_test 守护）
{ "type": "published" | "withdrawn", "articleId": 1001, "authorId": 7, "ts": 1770000000000 }
```

relation_events 消费映射（worker FeedRelationConsumer，批内聚合 uid 去重后一次 InvalidateInboxes）：

| 事件 | 失效对象 | 理由 |
|------|---------|------|
| follow / unfollow | followerId | 关注集变了，收件箱重建 |
| block | followerId + followeeId | 拉黑级联解除双向关注，两边收件箱都要重建 |
| unblock | 跳过 | 不恢复关注，无 feed 影响 |

### 2.3 gRPC 接口

**新增 `api/proto/feed/v1/feed.proto`**（模块路径/生成方式对齐现有 7 个 proto）：

```proto
service FeedService {
  // 读（core BFF 调）
  rpc ListFeed(ListFeedRequest) returns (ListFeedResponse);
  rpc NewCount(NewCountRequest) returns (NewCountResponse);              // P1
  // 写（worker 消费事件后派发）
  rpc FanoutArticle(FanoutArticleRequest) returns (FanoutArticleResponse);
  rpc RemoveArticle(RemoveArticleRequest) returns (RemoveArticleResponse);
  rpc InvalidateInboxes(InvalidateInboxesRequest) returns (InvalidateInboxesResponse);
}
message FeedItem            { int64 article_id = 1; int64 published_at = 2; }
message ListFeedRequest     { int64 uid = 1; int64 cursor = 2; int32 limit = 3; }  // cursor<=0 首页
message ListFeedResponse    { repeated FeedItem items = 1; int64 next_cursor = 2; bool has_more = 3; }
message NewCountRequest     { int64 uid = 1; int64 since_cursor = 2; }
message NewCountResponse    { int64 count = 1; }
message FanoutArticleRequest  { int64 article_id = 1; int64 author_id = 2; int64 published_at = 3; }
message RemoveArticleRequest  { int64 article_id = 1; int64 author_id = 2; }
message InvalidateInboxesRequest { repeated int64 uids = 1; }
// 各 Response 预留空 message，字段后续按需加
```

**article.proto 增量**（core 实现，feed 回源用；只回轻量字段避免 Content）：

```proto
rpc ListAuthorArticles(ListAuthorArticlesRequest) returns (ListAuthorArticlesResponse);
message ListAuthorArticlesRequest  { int64 author_id = 1; int32 limit = 2; }
message ListAuthorArticlesResponse { repeated FeedArticleBrief items = 1; }
message FeedArticleBrief           { int64 id = 1; int64 published_at = 2; }  // published_at=updated_at
```

### 2.4 HTTP 契约（core BFF）

| 字段 | 值 |
|------|-----|
| Method + Path | `POST /feed/list`（core 路由组 `Group("/feed")`，**不带 /api 前缀**；前端经 `/api/feed/list` rewrite） |
| Auth | JWT（`ginx.MustClaims`，受保护路由） |
| Request | `{ "cursor": int64 可选(缺省/0=首页), "limit": int 可选(默认10，后端夹取 1..20) }` |
| Success | `{ code:200, msg:"OK", data: { list: FeedItemVO[], nextCursor: int64, hasMore: bool } }` |
| FeedItemVO | `{ articleId, title, abstract, author:{id, nickname}, publishedAt, likeCnt, collectCnt, commentCnt, tags:[{id,name}] }` |

| 错误 | code/reason | 用户可见消息 |
|------|------------|-------------|
| 未登录 | 401 `UNAUTHENTICATED` | 请先登录 |
| 参数非法（limit 越界自动夹取，不报错；cursor 负数按 0） | — | — |
| feed 服务不可用 | 500（框架转） | 系统繁忙，请稍后重试（前端展示重试按钮） |

P1：`POST /feed/new-count` `{ sinceCursor }` → `{ count }`，同认证。

### 2.5 核心算法（service 层三条路径）

```
FanoutArticle(articleId, authorId, ts):
  followerCnt = relation.GetStats(authorId)
  outboxAppendIfExists(authorId, item)                    # Lua 原子：EXISTS→ZADD+trim+EXPIRE
  if followerCnt >= big_v_threshold: return               # 拉模式，跳过扩散
  for batch in relation.ListFollowers(authorId, 游标, 500):
    pipeline 对每个 follower: ZADD inbox + ZREMRANGEBYRANK 裁 2000 + EXPIRE   # ZADD 幂等→整批重投安全

ListFeed(uid, cursor, limit):
  if !EXISTS inbox:built:{uid}: rebuild(uid)
  normal = ZREVRANGEBYSCORE inbox (cursor→-inf) LIMIT limit
  outs   = errgroup(≤10) [ outboxRead(v, cursor, limit) for v in SMEMBERS bigv:{uid} ]   # 单作者失败→跳过降级
  merged = 归并降序（score DESC，同 score 按 articleId DESC）取前 limit
  return merged, nextCursor=末条 score, hasMore=len(merged)==limit

rebuild(uid):                                             # 并发重建无锁：结果幂等，后写覆盖
  followees = relation.ListFollowees 游标循环（上限 rebuild_max_followees=1000）
  stats     = relation.BatchGetStats（分批 100）
  bigvs / normals = 按 follower_cnt >= threshold 二分
  items = errgroup(≤10) [ outboxRead(a, 0, 100) for a in normals ]   # miss 回源 core.ListAuthorArticles
  DEL inbox/bigv/built → pipeline: ZADD 归并 top 2000 + SADD bigvs + SET built + EXPIRE ×3（同 TTL）
```

### 2.6 配置（feed config yaml `feed` 段，5 份同构）

```yaml
feed:
  big_v_threshold: 1000        # 粉丝数 ≥ 此值不写扩散
  inbox_cap: 2000
  inbox_ttl: 168h
  outbox_size: 100
  outbox_ttl: 1h
  rebuild_max_followees: 1000
  fanout_batch: 500            # ListFollowers 每批
```

服务地址：HTTP `:8100`（仅 /metrics + /health）、gRPC `:8101`——`8090/8091` 已被 permission 设计预定（`prd/permission/ARCHITECTURE.md`），feed 取下一个十位段。内部服务不入 nginx。

### 2.7 前端

| 项 | 设计 |
|----|------|
| 路由 | `/feed?tab=follow / discover`（默认 discover 省略参数），`router.replace` 切换；发现 Tab 保留现有分页参数 |
| 组件 | 广场视图加双 Tab 壳；新增 `views/feed/` 关注流组件（卡片复用/对齐广场卡片 + 计数 + 标签 chips） |
| API | `api/feed.ts`：`listFeed({cursor?, limit?})`；必须 catch，失败态可重试 |
| Hook | `hooks/useInfiniteScroll.ts`：IntersectionObserver 哨兵触底 + loading/hasMore 门控（防重复触发）；≥2 列表可复用（关注流 + 未来评论流） |
| 5 态 | 未登录引导 / 首屏 Skeleton + 翻页底部 spinner / 错误+重试 / 空态引导 / 内容流（对应原型 01~03） |
| 游标 | 组件 state 持有，不进 URL（PRD 已定）；切走再切回 Tab 重置为首页 |

## 3. 风险

- **性能**：rebuild 是读路径最重操作（关注 1000 × outbox）——上限夹取 + errgroup 限 10 + outbox 缓存吸收；触发频率低（关系变更/TTL 过期后首读）。
- **性能**：写扩散大批量 Redis 写——ListFollowers 分批 500 + pipeline；大 V 判定挡住百万粉丝场景；worker 批量消费天然削峰。
- **并发**：Kafka 整批重投 → ZADD/DEL 幂等，安全重放；并发 rebuild 同 uid 双算 → 结果幂等后写覆盖，不加锁。
- **并发**：outbox「存在才追加」必须 Lua 原子，否则产生假全量缓存（§2.1）。
- **一致性**：事件生产失败仅记日志 → 该文章漏扩散，靠 inbox TTL 过期重建最终补齐（7 天窗口）；同 score 游标开区间可能跳/重同毫秒条目，毫秒粒度概率可忽略——两处都是学习项目可接受的取舍，文档留痕。
- **依赖环**：core BFF→feed，feed→core(article gRPC) 双向调用——feed 调 core 仅限 rebuild/outbox miss，带超时（1s）+ 单作者失败跳过降级，不会形成请求环（不同 RPC 链路）。
- **安全**：uid 一律取自 JWT claims（BFF 注入），feed gRPC 信任内网调用方（对齐 relation：鉴权留 core 网关）。
- **回归**：广场页改双 Tab——发现 Tab 的分页/URL 行为必须原样保留（前端回归重点）；`article.proto` 加 rpc 后需重跑 buf 生成 + `make verify`（GOWORK=off 抓漂移）。
- **待确认（tdd 前）**：tag `TagsByBiz` 是否支持批量 bizIds（否则 BFF N+1，需给 tag.proto 补批量）；article.proto 现有 message 是否可复用替代新增 `FeedArticleBrief`；Kafka 新 topic 建法对齐现有（broker 自动建 vs 部署脚本）。

## 4. 任务拆分

| # | 任务 | 依赖 | 验收 |
|---|------|------|------|
| **P0-A 契约** | | | |
| 1 | `api/proto/feed/v1/feed.proto` 新增 + buf 生成 | 无 | api 模块编译过 |
| 2 | `article.proto` 加 `ListAuthorArticles` + 重新生成 | 无 | 同上 |
| **P0-B feed 服务**（拷 relation 骨架） | | | |
| 3 | 脚手架：`feed/` 平铺目录 + config×5 + main + ioc（redisx/etcd/otel/metrics/grpc server）+ wire | #1 | 服务能起、/health 200 |
| 4 | consts + cache 层：inbox/outbox/bigv/built 全部 Redis 操作（含 Lua 条件追加、pipeline+jitter TTL） | #3 | cache 单测（miniredis 或集成）全绿 |
| 5 | repository 层：FeedRepository 协调 cache（无 DAO，写明理由注释） | #4 | repo 单测全绿 |
| 6 | service 层：Fanout / Remove / InvalidateInboxes（mock relation client） | #5 | service 单测全绿 |
| 7 | service 层：ListFeed + rebuild + 归并（mock relation + core article client） | #5 | 单测覆盖：miss 重建/大V归并/游标翻页/降级 |
| 8 | grpc server 层：pb↔domain + validate 拦截器 | #6,#7 | grpc 层薄壳自测 |
| 9 | integration：`integration/setup` wire 装配 + main.go 锚 + 核心链路集成测试 | #8 | `go test ./integration/...` 绿 |
| **P0-C core 侧** | | | |
| 10 | `internal/events/article/`：event + producer（镜像 relation events） | 无 | 单测 + 契约字段冻结 |
| 11 | `ArticleAuthorService.Publish/Withdraw` 注入 producer 产事件（失败记日志） | #10 | article 单测更新全绿 |
| 12 | core article gRPC server 实现 `ListAuthorArticles`（DAO 加 ListBriefByAuthor，Select 排除 Content） | #2 | dao/grpc 单测绿 |
| 13 | BFF `service/feed.go`：五源聚合（feed+article+user+interaction+comment+tag） | #1 | service 单测（mock 全下游）绿 |
| 14 | BFF `web/feed.go`：`Group("/feed")` + WrapReq + MustClaims + wire + `ioc/grpc.go` FeedConn | #13 | 路由自测 200/401 |
| **P0-D worker 侧** | | | |
| 15 | `consumer/event/article.go` 契约副本 + contract_test 扩展 | #10 | 契约测试绿 |
| 16 | FeedArticleConsumer / FeedRelationConsumer（批量聚合 + 独立 group + 退避重连，镜像 interaction）+ wire + config | #15,#1 | 消费单测绿 |
| **P0-E 前端** | | | |
| 17 | `api/feed.ts` + `types/feed.ts` | #14 | lint/build 过 |
| 18 | `hooks/useInfiniteScroll.ts` | 无 | hook 单测或页面自测 |
| 19 | 广场页双 Tab 壳（URL 状态、发现 Tab 回归不变） | 无 | 手测切换+刷新保持 |
| 20 | `views/feed/` 关注流列表 + 卡片（计数+标签）+ 空态/未登录/错误态 | #17,#18,#19 | 5 态齐全，对齐原型 pen 值 |
| **P0-F 部署（14 项清单逐一过）** | | | |
| 21 | compose 服务块 + healthcheck + prometheus job + grafana alerting webook-feed.yml + `.env×4+example`（FEED_*）+ CI workflow + 7 兄弟 paths-ignore + Dockerfile + CHANGELOG + feed/CLAUDE.md | #9 | `grep -rn webook-feed` 全仓核对清单 |
| **P1** | | | |
| 22 | feed.NewCount + BFF /feed/new-count + 前端提示条（30s 轮询） | P0 全部 | PRD 验收第 6 条过 |

## A. 前瞻性设计

| 维度 | 问题 | 方案 |
|------|------|------|
| 扩展性 | 第二种内容（如「想法/短文」）接入 feed 改动多大？ | inbox member=articleId 是单业务假设——**刻意保留**（KISS：当前仅文章一个 caller，不预抽 biz 维度）；将来接入 = member 改 `{biz}:{bizId}` + 事件加 biz 字段，改动集中 feed 服务内，接口出入参不破坏 |
| 扩展性 | 推荐流（P2）接入？ | 双 Tab 前端壳已就位；推荐流走独立数据源（另一个 rpc），不复用 inbox——两者只共享 BFF 聚合段 |
| 可用性 | Redis 挂 → ListFeed 全挂？ | 是（feed 数据本体就是 Redis），BFF 收错误 → 前端错误态+重试；**不做**降级直查 DB（feed 非核心阅读路径，广场 Tab 仍可用） |
| 可用性 | relation / core article gRPC 挂？ | rebuild 失败 → ListFeed 返错（首读）；outbox 单作者回源失败 → 跳过该作者降级出流（记日志）；fanout 失败 → Kafka 整批重投 |
| 可用性 | Kafka 挂？ | 生产侧记日志不阻断发布主流程；恢复后新事件正常，漏扩散靠 TTL 重建窗口补齐 |
| 容错性 | 重复消费/重投？ | ZADD/DEL/SADD 全幂等 ✓ |
| 容错性 | 数值边界？ | limit 夹取 1..20；rebuild 关注数上限 1000；inbox cap 2000、outbox cap 100 硬裁剪 |
| 容错性 | 并发 rebuild？ | 无锁双算，结果幂等后写覆盖（都基于同一源数据） |
| 可观测性 | 5 分钟定位？ | 指标：`webook_feed_fanout_total{result}`、`webook_feed_rebuild_duration`（histogram）、`webook_feed_outbox_total{hit\|miss}`、消费 lag（现有 kafka 面板）；日志：fanout/rebuild/降级分支全带 uid/articleId/authorId 上下文；grafana 告警镜像 relation 模板（up/grpc-error/p99/goroutines） |

## B. 分层设计（关键签名）

```go
// feed 服务（平铺，无 internal/；repository 无 DAO——数据源经 gRPC，本地仅缓存投影）
grpc/feed.go        FeedServiceServer                                  // validate + pb↔domain + slicex.Map(toPb)
service/feed.go     FeedService { ListFeed(ctx, uid, cursor int64, limit int) ([]domain.FeedItem, int64, bool, error)
                                  Fanout(ctx, domain.FeedArticle) error
                                  Remove(ctx, articleId, authorId int64) error
                                  InvalidateInboxes(ctx, uids []int64) error
                                  NewCount(ctx, uid, since int64) (int64, error) }   // 持 relationClient + articleClient + repo
repository/feed.go  FeedRepository { InboxBuilt / ReadInbox / SaveInbox(items, bigvs) / AppendInbox(uids, item) /
                                     ReadOutbox / FillOutbox / AppendOutboxIfExists / DelOutbox / Invalidate(uids) }
repository/cache/feed.go  RedisFeedCache（唯一碰 Redis 的层；Lua + pipeline + jitter 都在这）

// core
internal/events/article/     ArticleEvent + SaramaArticleEventProducer
internal/service/feed.go     GRPCFeedService { List(ctx, uid, cursor int64, limit int) ([]domain.FeedItem, int64, bool, error) }
internal/web/feed.go         InternalFeedHandler（薄壳：WrapReq + MustClaims + slicex.Map(toFeedVO)）

// worker
consumer/event/article.go    ArticleEvent 契约副本 + topic 常量
consumer/feed_article.go     FeedArticleConsumer   → feed.FanoutArticle / RemoveArticle
consumer/feed_relation.go    FeedRelationConsumer  → feed.InvalidateInboxes（批内聚合去重）
```

## C. Wire / 部署变更清单（服务拆分 14 项映射）

| 维度 | 动作 |
|------|------|
| 应用配置 | `feed/config/{local,dev,staging,prod,test}.yaml`（8100/8101、otel.service_name=webook-feed、`feed` 业务段） |
| Wire | feed 全新 wire.go；core wire 加 FeedConn/producer/BFF；worker wire 加两 consumer；`make wire` 全绿 |
| Prometheus | `job_name: webook-feed`，target `webook-feed:8100` |
| 录制规则 | 无 cron/lock，不加 |
| Grafana 告警 | `webook-feed.yml`（up/grpc-error-rate/grpc-p99/goroutines，镜像 relation） |
| 看板 | `$service` 变量自动纳入，无需改 |
| Compose | 服务块 + healthcheck(:8100/health) + depends redis/etcd/kafka；local build override |
| Nginx | **不入**（内部服务，经 core BFF） |
| deploy.sh | 服务名透传，无需改 |
| 部署变量 | `.env×4 + example`：`FEED_IMAGE_TAG` / `FEED_APP_ENV` |
| CI | `webook-feed-ci.yml` + 7 个兄弟 workflow 的 paths-ignore 互加 |
| Dockerfile | `feed/Dockerfile`（多阶段，context=webook/） |
| Metric 命名 | `webook_feed_*`（业务域前缀，服务区分靠 job label） |
| 文档 | CHANGELOG + `feed/CLAUDE.md`（拆分原因/边界/接入方） |
