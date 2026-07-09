# redislock 设计与代码文档

> 自研分布式锁库 `webook/pkg/redislock` 的文档入口。按你的目标选一篇进。

`webook/pkg/redislock/` 是**纯自研**的分布式锁库：6 段 embed Lua 原子核心 + Go 侧 watchdog / pub-sub 阻塞 / 公平队列 / fencing / prometheus 装饰器。单机与集群同一套实现（靠 key hash tag），多主 quorum 规划中。首个消费者：worker cron leader 选举（`pkg/cronx`）。

## 按目标导航

| 你的目标 | 读这篇 | 类型 |
|---------|-------|------|
| 拍板设计决策 / 查 ADR / 看任务拆分与风险 | [ARCHITECTURE.md](./ARCHITECTURE.md) | 详细设计 spec（权威） |
| 深入原理、沿代码调用链读懂、准备自己复现 | [WALKTHROUGH.md](./WALKTHROUGH.md) | 代码阅读向导 |

冲突时**以 ARCHITECTURE.md 为准**（它是权威 spec；WALKTHROUGH 是「读它 + 读代码」的辅助，讲代码在哪、每行怎么落地、怎么复现）。

## 一句话总览

- **两条安全真相**：fencing token 是唯一真安全；多主 / 集群解决可用性、不解决安全性（详见 ARCHITECTURE §0）。
- **五能力**：可重入 / 公平锁 / pub-sub 阻塞 / fencing / 多主 quorum，各自独立 Options 开关，签名保持 `(ctx, key, opts...)` 干净。
- **当前进度**：P1-P4 已落地（改名去 bsm + 单机/集群 + fencing + 可重入 + 阻塞/公平）；P5 多主 quorum pending。
- **最该先啃的三处反例**（内化正确性）：幻觉持锁（watchdog 三分支）、丢唤醒（订阅先于获取）、双写（fencing + 资源侧校验）——见 WALKTHROUGH §4.6 / §4.7 / §4.9。
