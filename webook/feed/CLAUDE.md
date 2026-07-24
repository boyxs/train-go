# webook-feed（关注流服务）

把「关注作者的已发布文章」按发布时间倒序聚合成信息流的独立 gRPC 微服务（HTTP `:8100` 仅
metrics/health，gRPC `:8101`）。与 `relation/`、`tag/` 同构平铺布局（不套 `internal/`）。**无 MySQL**。

## 为什么拆

关注流是「published_article × relation」的缓存投影，扩散写入是突发大批量 Redis 写，与 core 请求路径
隔离更稳；relation 拆分时已预留（feed 未来独立服务订阅 relation_events）。core / 未来 chat 都作平等
client 经 etcd 调用——与 relation/interaction 被多方消费同构。

## 边界铁律

- **纯同步 gRPC**：HTTP `:8100` 只暴露 `/metrics` + `/health`，业务全走 gRPC `:8101`。
- **无 MySQL**：收件箱/发件箱全 Redis 投影；源数据（文章/关系）经 gRPC 拉 core(article) + relation，
  **绝不直连别服务私有库**。丢失可重建，不做持久化兜底。
- **不碰 Kafka**：异步消费（article_events / relation_events）统一归 `webook-worker`，worker 消费后经
  gRPC 调本服务 `FanoutArticle`/`RemoveArticle`/`InvalidateInboxes`（对齐 relation/interaction 边界）。
- **鉴权留 core 网关**：uid 由 core BFF 从 JWT 注入，本服务 gRPC 只做 id 非空校验。

## 接入方

- **core**：`internal/web/feed.go`（HTTP 网关 `POST /feed/list`）经 `internal/ioc/grpc.go` 的 `FeedConn`
  调 `ListFeed`；`service.GRPCFeedService` 五源聚合（feed + article + user + interaction + comment + tag）填 VO。
- **worker**：`consumer/feed_article.go` 消费 `article_events` → `FanoutArticle`/`RemoveArticle`；
  `consumer/feed_relation.go` 消费 `relation_events` → 批内聚合去重 → `InvalidateInboxes`。
- **下游**：本服务回源调 relation（`ListFollowers`/`ListFollowees`/`GetStats`/`BatchGetStats`）+
  core article（`ListAuthorArticles`）。

## 关键决策

- **推拉结合**：普通作者发文写扩散进粉丝 inbox（ZADD 幂等）；大 V（粉丝数 ≥ `big_v_threshold`，yaml 可调）
  不扩散，读时归并其 outbox。挡住「大 V 百万粉丝」写放大。
- **关系变更 = 失效重建**：follow/unfollow/block 一律 DEL inbox+bigv+built，下次读全量重建（一招覆盖三事件，
  含拉黑级联解除双向关注）；代价是变更后首读慢。
- **撤回 = 读时过滤**：inbox 不摘除，DEL 作者 outbox；BFF `BatchDetail` 查线上库天然滤掉已撤回文章。
  **新内容计数同口径**：`NewCount` 只返回收件箱候选 id（`InboxSince`，capped `new_count_max`），BFF 用 `CountByIds`（COUNT + 软删过滤）算可见数——否则撤回文章仍占收件箱会被当新文章反复计数。
- **outbox「存在才追加」Lua 原子**：EXISTS→ZADD+trim+PEXPIRE，防事件把不存在的 outbox 建成只含 1 条的假全量。
  inbox 无条件 ZADD 安全（无 built 标记的部分收件箱读时被重建覆盖）。
- **游标 = publishedAt**：inbox/outbox 均 ZSET（member=articleId，score=publishedAt），
  ZREVRANGEBYSCORE 开区间翻页；同 score 由 service 归并时按 articleId DESC 决胜。
- **TTL 带 jitter**（0~5min）防雪崩；多步 Redis 用 pipeline。

## 分层

`grpc/`（适配 pb↔domain + `ValidateUnaryInterceptor`）→ `service/`（三路径：Fanout/Remove/Invalidate 写、
ListFeed+rebuild 读；持 relation + article gRPC client）→ `repository/`（薄委托，**无 DAO**）→ `cache/`
（唯一碰 Redis 的层：Lua + pipeline + jitter）。构造函数返回接口；domain↔pb 单条 `toPb` + `slicex.Map` 批量；
时间全 `int64` 毫秒。

## 部署

`config/{local,dev,staging,prod,test}.yaml` 5 份同构 + 差异点（otel.env / sample_ratio / 8100·8101 /
`feed` 业务段 / `client.grpc.webook-relation`+`webook-core`）。内部服务，**不入 nginx**（gRPC 经 etcd 发现）。基建：

- prometheus job `webook-feed`（`deploy/prometheus/prometheus.yml`，target `webook-feed:8100`）
- grafana 告警 `deploy/grafana/provisioning/alerting/webook-feed.yml`（镜像 relation：up / grpc-error-rate /
  grpc-p99 / goroutines，纯 gRPC 无 5xx/P99 HTTP）
- docker-compose 服务块 + healthcheck(`:8100/health`) + depends redis/etcd（无 mysql）；
  `docker-compose.local.yaml` build override（context=`webook/`）
- `.env.{local,dev,staging,prod}.example` 的 `FEED_IMAGE_TAG` / `FEED_APP_ENV`（MEM 用 compose 默认 256m）
- CI `.github/workflows/webook-feed-ci.yml`（paths 与 9 兄弟服务互斥）+ 9 兄弟 CI 的 `paths-ignore` 已各加 `webook/feed/**`
- `feed/Dockerfile`（多阶段，context=`webook/` 仓根，GOWORK=off）

无需改动（服务无关 / 内部服务）：`deploy.sh`（服务名透传）· grafana 看板（`$service` 变量自动纳入）·
prometheus 录制规则（feed 无 cron/lock）· nginx（gRPC 经 etcd，不直连前端）。

## Metric 命名

统一 `webook_grpc_*`（复用 pkg 共享 builder），**禁止** `webook_feed_*`；service 区分靠 prometheus 注入的 `job` 标签。
