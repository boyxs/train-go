# Runbook: 死信队列在增长

> 触发：oncall 巡检 / mysql 查 `dead_letter` 表行数持续增长
> 严重度：**P2**
> 预期解决时间：**< 4 h**
> 关联：`zero-downtime-playbook.md` §3.2 / §11.1 异步双写

---

## 症状

- 控制库 `dead_letter` 表行数持续增长（mysql 查）
- `GET /tasks/:id` detail 死信计数上升

## 立即动作（5 分钟内）

1. **确认增长速率**（区分一过性 vs 持续）

   ```bash
   # 隔 1 min 两次查行数，看增速
   mysql webook_migrator -e "SELECT COUNT(*) FROM dead_letter WHERE task_id=$TASK_ID"
   ```

2. **如果增长持续 > 100/s**：升级 P1，因为说明 NEW 持续不可用，业务双写大量失败

## 诊断（30 分钟内）

```bash
# 1. 死信里都是什么 op
mysql webook_migrator -e "
  SELECT op, COUNT(*) FROM dead_letter
  WHERE task_id=$TASK_ID AND replayed=0
  GROUP BY op"

# 2. 抽样几条看 error 信息
mysql webook_migrator -e "
  SELECT biz_id, op, last_error, retry_count
  FROM dead_letter WHERE task_id=$TASK_ID AND replayed=0 LIMIT 10"

# 3. NEW 健康
# 按 task 的 sink_type 查健康
curl -s sink-host/health
```

## 永久修复

| 根因 | 处理 |
|------|------|
| Sink 短暂不可用（已恢复） | 触发死信重放：`POST /tasks/$ID/replay-dl` |
| Sink 持续不可用 | 走 [sink-unreachable.md](./sink-unreachable.md) |
| Transform 函数 bug 导致写入失败 | 修代码；不能直接重放（重放还是会失败）；走 repair |
| NEW schema 不匹配 | 修 NEW schema；重放 |
| 业务双写时 OLD 已删除但 NEW 没有（race） | 数据一致性问题，走 verify + repair |
| 凭据失效 | 更新 Vault；重放 |

## 重放死信

```bash
# 单批重放（默认 1000 条）
curl -X POST http://migrator.internal:8030/migrator/tasks/$TASK_ID/replay-dl \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{"limit": 1000}'

# 重放进度
mysql webook_migrator -e "
  SELECT COUNT(*) AS unreplayed FROM dead_letter
  WHERE task_id=$TASK_ID AND replayed=0"
```

## 验证

- 死信表 `replayed=0` 行数收敛 < 100
- 重放后 `replay_failed` 行数 < 5（剩下的进人工处理列表）

## 事后

- [ ] 死信增长在 NEW 健康时不应该发生：审计是哪些 corner case 导致
- [ ] 加死信表容量告警（避免吃满磁盘）
- [ ] 死信重放定期任务（每天凌晨自动跑）
- [ ] postmortem
