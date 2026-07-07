# webook-worker（调度器服务）

收拢所有异步 / 定时任务的独立服务（端口 `:8050`，**无 DB**）：cron 定时任务 + Kafka 消费者，
**全部经 gRPC 派发给业务服务，自身零业务数据 / 零业务逻辑**（对齐 bolee-task 的 XxlJob + StreamListener 模式）。

## 为什么拆 + 铁律

把散在各服务的异步处理统一进一个调度器，避免每个业务进程各自起 cron / 消费者带来的多副本重复执行、观测分散。

**铁律**（让「统一」不破坏限界上下文）：worker 是纯调度 / 派发——
- 每个 handler 只能**调被调服务的 gRPC API**，**绝不直连别的服务的私有库**，自己也无库可写。
- 业务逻辑留在 owner 服务（ranking 留 core、read 计数落 interaction）。
- 违反即破坏边界，review 直接打回。（可用 `grep -rn 'gorm\|dao' worker/` 自查，应为空。）

## 住户

- `job/`：cron 触发 core `RankingJobService`（榜单重算 / 归档）；spec→fn 表，锁 / 指标 / date 注入 / panic recover 由 `cronx.Wrapper` 统一。
- `consumer/`：消费 `interaction_events`，按 `(biz,biz_id)` 聚合一批 → 调 interaction `BatchIncrReadCount`；自管连接、无限退避重连。
  - `consumer/event/`：消费侧事件线格式契约（`InteractionEvent` + topic 常量，纯数据、**无生产者/消费者运行时**）。**契约 = topic + JSON，两端各自定义、不共享代码**；单数 `event` 刻意区别 core 复数 `events`（那是生产 + 消息 plumbing 全家桶）；漂移由 `worker/consumer/event/contract_test.go` 守护。

## 关键决策 / 注意点

- **cron 分布式锁**：`redislockx` watchdog（类 Redisson），`InitCronWrapper` 显式 `WithLockTTL(30s)` 作 crash 让贤窗口
  （**勿吃隐式默认值**）；丢锁由 prometheus 装饰器记 `webook_lock_watchdog_lost_total`（告警见
  `deploy/grafana/provisioning/alerting/webook-worker.yml`），**勿在 cronx 再传 `WithOnLost`，会覆盖该指标**。
- **静态配置**：worker 只 `LoadLocal`、不 `WatchRemote`，无 etcd 配置热更（区别于 core / chat / interaction）；
  etcd 仅供 gRPC 服务发现。配置变更靠重启。
- **优雅停机**：`main` 等 cron drain + 消费者 goroutine 退出（有界）再 `cleanup()`，避免关 gRPC 连接撞在途 RPC。

## 接入

下游 gRPC client 经 etcd 发现：`webook-core`（ranking）+ `webook-interaction`（read 计数），共享同一 metrics builder。
新增异步任务 = 在 `job/` 或 `consumer/` 加住户 + 调对应 owner 的 gRPC，**不在此写业务**。
