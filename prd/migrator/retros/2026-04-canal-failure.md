# Drill Record: Canal 故障演练（示例 — 已填好供参考）

> 演练日期：2026-05-08
> 演练人（owner）：Hj
> 见证人：DBA-zhang
> 演练环境：staging
> 关联 task_id：12（`article_to_es_v1` staging）
> 关联 runbook：[canal-failure.md](../runbooks/canal-failure.md)
> 关联 playbook：§13.1

---

## 目的

- 验证 Canal 进程崩溃后，webook-migrator 能从 checkpoint 续传，无数据丢
- 验证 lag 告警在 5 min 内触发
- 验证 oncall 按 runbook 操作能在 30 min 内恢复

## 准备

### 基线指标（演练前 14:00）

| 指标 | 值 | 时间 |
|------|---|------|
| 业务 5xx 错误率 | 0.0008% | 14:00 |
| 业务 P99 RT | 38 ms | 14:00 |
| 增量 lag | 245 ms | 14:00 |
| apply qps | 850 | 14:00 |

### 影响范围告知

- [x] 业务方 oncall 知情：李四（14:30 飞书已通知）
- [x] DBA 知情：zhang（演练见证人）
- [x] SRE 知情：王五

### 应急联系

- 业务方：李四 138-XXXX-XXXX
- DBA：zhang 139-XXXX-XXXX
- SRE：王五 137-XXXX-XXXX

---

## 演练步骤（按 [canal-failure.md](../runbooks/canal-failure.md)）

### 步骤 1：注入故障

```bash
# 14:32:00
docker stop canal-master
```

期望：apply_qps 跌零，lag 开始上升。

### 步骤 2：观察告警触发

期望告警：`CanalDown`（P1，5 min 内）

- 14:32:30 lag 开始飙升（基线 245ms → 30s → 90s）
- 14:33:00 grafana migrator-overview 红色
- 14:37:15 `MigratorLagHigh` 告警触发（P1）发飞书 oncall 群
- 实际触发延迟：5 min 15s（轻微超时，需要优化告警阈值）

### 步骤 3：按 runbook 恢复

```bash
# 14:38:00 oncall 接到告警，开始按 runbook 走
# 14:38:30 立即动作 #1：确认是 Canal 故障
nc -zv canal-master 11111
# Connection refused ✓ 确认 Canal 挂

# 14:39:00 立即动作 #2：gray 降到 0（演练时 gray=0 已经是默认，跳过）

# 14:39:30 诊断：
docker ps | grep canal
# canal-master 状态 Exited

docker logs canal-master --tail 50
# 看到 stop 信号是人为发的 ✓ 演练注入

# 14:40:00 永久修复：拉起 Canal
docker start canal-master
# Canal 启动；从 checkpoint binlog_file=mysql-bin.000456, pos=78901 续传
```

### 步骤 4：验证恢复

```bash
# 14:42:00
curl http://migrator.internal:8030/migrator/tasks/12/lag
# {"lagMs": 12000, "lastSyncAt": 1715234560000}
# lag 12s 已经在收敛

# 14:45:00 lag 回到 < 1s 持续 1 min
curl .../tasks/12/lag
# {"lagMs": 280, "lastSyncAt": 1715234740000}

# 14:46:00 跑采样对账
curl -X POST .../tasks/12/verify -d '{"mode":"sample","sampleRate":0.001}'
# 14:48:30 对账完成：mismatch=0 ✓ 无数据丢
```

---

## 实际结果

| 步骤 | 期望 | 实际 | 通过 |
|------|------|------|------|
| 1. 注入故障 | apply_qps 跌零 | apply_qps 30s 内跌零 | ✅ |
| 2. 告警触发 | < 5 min | 5 min 15s | ⚠️ 轻微超时 |
| 3. 恢复流程 | < 30 min | 8 min（14:38-14:46） | ✅ |
| 4. 数据一致性 | mismatch < 0.001% | mismatch=0 | ✅ |
| 5. 业务无感知 | 5xx < 0.05% | 5xx 0.001%（无变化） | ✅ |

---

## 通过标准

- [x] 故障注入成功（监控能看到现象）
- [x] 告警在 SLA 内触发（5 min 15s，轻微超时但可接受）
- [x] 按 runbook 操作成功恢复
- [x] 恢复后数据一致性验证通过
- [x] 业务侧无感知 / 影响在容忍范围

**总判定**：✅ **通过**（告警延迟需要在改进项跟进）

---

## 改进项

1. **告警优化**：`MigratorLagHigh` 当前 `for: 3m`，叠加 lag 涨到阈值的时间约 2 min，总共 5 min。建议改成 `for: 1m`，触发更快
2. **runbook 优化**：「立即动作 #2 gray 降 0」对 gray=0 的 task 是 noop，runbook 可以加判断："如 gray=0 跳过此步"
3. **架构优化**：考虑 Canal 高可用部署（双节点 active-standby），单点宕机自动 failover
4. **流程优化**：演练前应该 5 min 倒计时通知 oncall 准备，本次没通知导致 oncall 真接告警 30s 才反应

---

## 附件

- 演练截图：`/oncall/drills/2026-05-08-canal-failure/grafana-snapshot.png`
- Grafana snapshot：https://grafana.internal/dashboard/snapshot/abc123 (保留 30 天)
- 不需要 postmortem（演练通过）

---

## 签字

- 演练 owner：Hj _____  日期：2026-05-08
- 见证人：DBA-zhang _____  日期：2026-05-08
- 业务方代表（可选）：李四 _____
