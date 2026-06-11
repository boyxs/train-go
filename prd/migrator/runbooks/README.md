# Runbooks 索引

> 应急手册按事件类型拆。oncall 接告警先查 runbook，再动手。
> 每份 runbook 严格按「症状 → 立即动作（5 分钟内） → 诊断 → 永久修复 → 验证 → 事后」结构。

---

## 告警 → Runbook 对照表

| Prometheus 告警 | severity | Runbook |
|----------------|---------|---------|
| `MigratorServiceDown` | P0 | [migrator-service-down.md](./migrator-service-down.md) |
| `MigratorLagHigh` | P1 | [lag-spike.md](./lag-spike.md) |
| `MigratorMismatchHigh` | P2 | [mismatch-spike.md](./mismatch-spike.md) |
| `MigratorDeadLetterGrowing` | P2 | [dead-letter-growing.md](./dead-letter-growing.md) |
| `CanalDown` / `CanalLagHigh` | P1 | [canal-failure.md](./canal-failure.md) |
| `KafkaBrokerDown` / `KafkaPartitionUnavailable` | P1 | [kafka-broker-down.md](./kafka-broker-down.md) |
| `SinkUnreachable{type=es\|ck\|mongo}` | P1 | [sink-unreachable.md](./sink-unreachable.md) |
| `MySQLSourceLoadHigh` | P2 | [source-pressure-high.md](./source-pressure-high.md) |
| 业务方反馈切流后异常 | P0 | [cutover-rollback.md](./cutover-rollback.md) |

---

## 严重度定义

| 严重度 | SLA | 处理 |
|--------|-----|------|
| **P0** | 5 分钟响应 / 30 分钟恢复 | call 全员 + 业务方 + leader |
| **P1** | 15 分钟响应 / 2 小时恢复 | oncall + DBA |
| **P2** | 1 小时响应 / 1 工作日恢复 | oncall 自处理 |

---

## Runbook 通用约束

1. **不允许越级**：未经过双人确认不要执行 destructive 命令（drop / truncate / kill 主进程）
2. **回滚优先**：能回滚就先回滚再排查根因
3. **保留现场**：恢复前 dump 一份 binlog pos / checkpoint / Redis 状态
4. **事后必复盘**：P0/P1 必须出 postmortem 进 `postmortems/` 目录（待建）
