# ADR-0001: Redis 作为路由决策的唯一真相源

> 状态：**接受**
> 日期：2026-04（初版）
> 关联：[02-architecture §5](../02-architecture.md) / [03-walkthrough §6.4 §10](../03-walkthrough.md)

## 背景

迁移期间业务 DAO 需要根据当前 stage（SRC_ONLY / SRC_FIRST / DST_FIRST / DST_ONLY）和 gray% 决定读写 OLD 表还是 NEW 表。这个路由决策每次业务请求都要查一次。

可选的存储位置：
- MySQL `task.status` + `gray_percent` 字段（已有）
- Redis 独立 key `migrator:stage:{taskName}` + `migrator:gray:{taskName}`（按 taskName 而非 taskId，与 SDK [`internal/migratorsdk/redis.go`] 对齐——业务侧只从 yaml 拿 taskName）
- 配置中心（如 etcd / Nacos）
- 业务进程本地缓存 + 推送更新

## 备选方案

### A. MySQL 为路由真相源

业务每次请求查 `task` 表的 status / gray 字段决定路由。

- 优点：单一存储，与任务状态强一致
- 缺点：每个业务请求 +1 SQL；MySQL 故障时业务路由全瘫；task 表读压力激增

### B. Redis 为路由真相源（本方案）

业务每次请求读 Redis 两个 key（stage / gray）决定路由。MySQL `task` 字段冗余持久化（task 列表页展示用），不在请求路径。

- 优点：毫秒级 RTT；Redis 不可达时业务降级 `SideOld`（仍走 OLD），不阻塞；migrator 服务挂掉不影响业务（业务只读 Redis 不调 migrator gRPC）
- 缺点：双存储一致性弱（Redis 与 MySQL 可能短时不一致，但路由决策只读 Redis，不一致只影响 task 列表展示）

### C. 配置中心（etcd / Nacos）

- 优点：天然支持 watch 推送，业务不需要轮询
- 缺点：引入新基础设施；公司内 etcd 已用于 K8s，混用风险高；切流频率低（一天数次），不值得为推送加复杂性

## 选择

**B. Redis 为路由真相源**。

## 理由

1. **业务可用性优先于强一致**：迁移期间业务请求 QPS > 100，每次 +1 SQL 不可接受（A 方案）；引入新基础设施风险大（C 方案）
2. **降级路径明确**：Redis 不可达 → 业务自动走 SideOld（保守降级，回到迁移前状态）
3. **migrator 服务解耦**：业务只依赖 Redis，不依赖 migrator gRPC，故障域隔离
4. **量化依据**：Redis GET RTT P99 < 1ms；MySQL SELECT P99 5-10ms；切流频率 < 1 次/秒（路由读频率 > 1000 次/秒）

## 后果

### 正面
- 业务请求路径只增加 1 次 Redis GET（已有 cache 链路，几乎免费）
- migrator 服务故障域隔离
- Redis 不可达时业务降级清晰

### 负面（必须承认）
- **双存储弱一致**：Redis stage 与 MySQL `task.status` 短时不同步（写 Redis 后异步写 MySQL）；task 列表页显示可能滞后几秒
- **Redis 单点风险**：本项目 Redis 是单主，挂掉业务全部降级 SideOld（虽不阻塞但路由不再灵活）
- **缓存击穿风险**：批量任务启动时若 Redis 同时失效，业务侧瞬间 fallback 到 SideOld。缓解：业务侧本地缓存 stage 30s（fail-safe）

### 中性
- 引入 Redis key 命名约定（`migrator:` 前缀），需要在 [02-architecture §5](../02-architecture.md) 维护

## 验证

- 集成测试：杀 Redis 验证业务 200 + 走 SideOld（已在 `webook/internal/repository/article_reader_integration_test.go` 覆盖）
- 监控：`migrator_redis_unavailable_total` counter；告警阈值 > 10/min
