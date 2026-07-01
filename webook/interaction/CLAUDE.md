# webook-interaction（互动服务）

点赞 / 收藏 / 浏览 / 计数的独立 gRPC 微服务。从 core 进程内 service 抽出，与 `comment/` 同构平铺布局。

## 为什么拆

互动是高频写 + 通用能力（article / comment / 未来任意 biz 都要点赞计数）。留在 core 进程内：
① 把 core 的发布部署与互动写入耦合；② 高频计数写无法独立扩容。抽成独立服务后 core 转薄 gRPC 客户端经
etcd 调用；按 `(biz, biz_id)` 通用建模——新增可互动实体 = 新 biz 值，零代码改动。

## 边界铁律

- **纯同步 gRPC**：HTTP `:8040` 只暴露 `/metrics` + `/health`，业务全走 gRPC `:8041`。
- **内部不碰 Kafka / cron**：异步（read 计数攒批、ranking 事件化）统一归 `webook-worker`，本服务只做同步读写。
- 与 comment **同库不分库**（无数据迁移）；表 `interaction`（聚合计数）/ `user_interaction`（用户状态）。

## 接入方

- **core**：`internal/service/interaction.go` 的 `GRPCInteractionService` 适配器经 etcd 调本服务。
- **chat**：`chat/ioc/grpc.go` 的 `InteractionConn` 直连本服务。
- **worker**：消费 read 事件后调 `BatchIncrReadCount`（攒批累加，见下）。

## 关键决策

- **幂等翻转加行锁**：`UpsertLike/UpsertCollect` 事务内读旧状态用 `clause.Locking{UPDATE}`（FOR UPDATE），
  否则并发同用户点赞会都读到旧值各 +1 → 计数翻倍。
- **BatchIncrReadCount**：worker 把一批 read 事件按 `(biz,biz_id)` 聚合成一次 RPC，单事务逐项 upsert；
  取代逐条 N+1，整批一次提交避免「批内部分成功 + 重投」的重复计数。
- **Cache-Aside**：计数 + 用户状态两套 key，写后失效；TTL 带 jitter（防雪崩）；多步 Redis 用 Pipeline。
- 鉴权（uid 注入）与 `allowedBiz` 白名单留 core 网关侧；本服务 gRPC 只做非空校验。

## 分层

`grpc/`（适配 + 校验拦截器）→ `service/` → `repository/`（Cache-Aside）→ `dao/` + `cache/`。
构造函数返回接口；domain↔pb 单条 `toPb` + `slicex.Map` 批量；时间全 `int64` 毫秒；DAO 不依赖 domain。

## 部署

`config/{local,dev,staging,prod,test}.yaml` 5 份同构 + 差异点；prometheus job `webook-interaction`；
grafana 告警 `deploy/grafana/provisioning/alerting/webook-interaction.yml`；CI `.github/workflows/webook-interaction-ci.yml`。
内部服务，**不入 nginx**（gRPC 经 etcd 发现，不直连前端）。
