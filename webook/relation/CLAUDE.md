# webook-relation（用户关系服务）

关注 / 粉丝 / 拉黑的独立 gRPC 微服务。与 `interaction/`、`comment/` 同构平铺布局（不套 `internal/`）。

## 为什么拆

用户关系是高频写 + 通用能力，且 **feed（未来独立服务）、chat 都要跨服务消费关系数据**。留在 core：
① core 发布部署与关系写入耦合、无法独立扩容；② feed/chat 若经 core 取关系 = 后端服务反向依赖网关（循环/耦合）。
抽成独立 gRPC 服务后，core / chat / 未来 feed 都作平等 client 经 etcd 调用——与 interaction 被 core/comment/chat 消费同构。

## 边界铁律

- **纯同步 gRPC**：HTTP `:8060` 只暴露 `/metrics` + `/health`，业务全走 gRPC `:8061`。
- **内部不碰 Kafka / cron**：本服务只做同步读写；`relation_events` 由 **core 网关**在写成功且 `changed=true` 时生产（对齐 interaction 边界），消费（被关注通知 / feed 扩散）归 worker / 未来 feed。
- 与 interaction/comment **同库不分库**（webook 库）；表 `relation_follow`（关注边 status 翻转）/ `relation_stats`（聚合计数）/ `relation_block`（黑名单）。

## 接入方

- **core**：`internal/web/relation.go`（HTTP 网关 8 endpoint）经 `internal/ioc/grpc.go` 的 `RelationConn` 调本服务；`userSvc.FindByIds` 聚合昵称/简介填 VO；写成功且 changed 经 `internal/events/relation` 产事件。
- **chat**（P1）：私信前调 `GetRelation` 校验被拉黑（发送方是否被接收方拉黑）。
- **feed**（未来独立服务）：订阅 `relation_events` 做写/读扩散；`ListFollowers` 游标拉全量粉丝。

## 关键决策

- **关注边 status 翻转**（非软删）：`Follow/Unfollow` 事务内 `clause.Locking{UPDATE}`（FOR UPDATE）读旧状态，仅真翻转才增减计数（`GREATEST(0, cnt±1)` 防负），否则并发同一对会计数翻倍/漏减。返回 `changed` 供 core 门控事件。
- **拉黑级联**：`Block` 事务内 `INSERT IGNORE` 黑名单 + 复用 `flipFollowTx` 解除双向关注（连带计数）；`Unblock` 物理删、**不恢复**关注。
- **Cache-Aside**：`relation_stats` 计数走 Redis Hash（`relation:stats:{uid}`），写后失效双方（非增量），TTL 带 jitter；关系态点查（isFollowing/isMutual/isBlocked）P0 不缓存（唯一索引点查）。
- **业务校验在 service**：自关注/自拉黑拒绝、Follow 前置双向拉黑门控；gRPC 层只做 id 非空校验，鉴权（uid 注入）留 core 网关。

## 分层

`grpc/`（适配 pb↔domain + `ValidateUnaryInterceptor`）→ `service/`（业务校验 + 关系态组装）→ `repository/`（Cache-Aside）→ `dao/` + `cache/`。
构造函数返回接口；domain↔pb 单条 `toPb<Type>` + `slicex.Map` 批量；repository domain↔dao 用 `toDomain<Type>` 接收者方法；时间全 `int64` 毫秒；DAO 不依赖 domain。

## 部署

`config/{local,dev,staging,prod,test}.yaml` 5 份同构 + 差异点（otel.env / sample_ratio / server.addr `:8060`/`:8061`）。
内部服务，**不入 nginx**（gRPC 经 etcd 发现，不直连前端）。基建已落地：

- prometheus job `webook-relation`（`deploy/prometheus/prometheus.yml`，target `webook-relation:8060`）
- grafana 告警 `deploy/grafana/provisioning/alerting/webook-relation.yml`（镜像 interaction：up / grpc-error-rate / grpc-p99 / goroutines，无 5xx/P99 HTTP 因纯 gRPC）
- docker-compose 服务块 + healthcheck(`:8060/health`) + depends mysql/redis/etcd；`docker-compose.local.yaml` build override（context=`webook/`）
- `.env.{local,dev,staging,prod}{,.example}` 的 `RELATION_IMAGE_TAG` / `RELATION_APP_ENV`
- CI `.github/workflows/webook-relation-ci.yml`（paths 与 6 兄弟服务互斥）+ 6 兄弟 CI 的 `paths-ignore` 已各加 `webook/relation/**`
- `relation/Dockerfile`（多阶段，context=`webook/` 仓根）

无需改动（服务无关 / 内部服务）：`deploy.sh`（服务名透传）· grafana 看板（`$service` 变量 / `label_values(up{job=~"webook-.*"})` 自动纳入）· prometheus 录制规则（relation 无 cron/lock）· nginx（gRPC 经 etcd，不直连前端）。
