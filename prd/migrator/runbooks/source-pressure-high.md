# Runbook: 源库压力告警

> 触发：`MySQLSourceLoadHigh` (threads_running > 100 持续 3min) / 业务方反馈源库慢
> 严重度：**P2**（业务受影响升 P1）
> 预期解决时间：**< 30 min**
> 关联：`zero-downtime-playbook.md` §4.5 / §15.1 风险

---

## 症状

- 源库 `mysql_global_status_threads_running` > 100
- 源库慢查询数飙升
- 业务侧 P99 RT 上升（如果业务和迁移共用主库）
- 全量阶段：扫描 SQL 拖慢业务

## 立即动作（5 分钟内）

1. **降全量限速**（如果在全量阶段）

   ```bash
   curl -X POST http://migrator.internal:8030/migrator/tasks/$TASK_ID/throttle \
     -H "Authorization: Bearer $ADMIN_TOKEN" \
     -d '{"qps": 1000, "shard_workers": 4}'
   # 默认 16 worker × 5k qps = 80k；降到 4 × 1k = 4k
   ```

2. **确认全量是否走 ReadReplica**

   ```bash
   # 任务配置里应该是 readonly DSN
   curl http://migrator.internal:8030/migrator/tasks/$TASK_ID | jq '.data.task.sourceDsnRef'
   # 应该指向 readonly endpoint，不是 master
   ```

3. **如果指向 master**：紧急切到 readonly + 重启全量 worker

## 诊断（10 分钟内）

```bash
# 1. 源库压力来源
mysql -h source -e "SHOW PROCESSLIST" | head -30
# 关注：state=Sending data, time>10 的 SELECT

# 2. 当前哪些 SQL 在跑
mysql -h source -e "SELECT * FROM information_schema.processlist WHERE COMMAND != 'Sleep' AND TIME > 5\G"

# 3. webook-migrator 全量进度推进速度（API 查 checkpoint）
curl -s http://migrator.internal:8030/migrator/tasks/$TASK_ID | jq '.data.checkpoints[].progress_percent'

# 4. 主从延迟
mysql -h replica -e "SHOW SLAVE STATUS\G" | grep Seconds_Behind_Master
```

## 永久修复

| 根因 | 处理 |
|------|------|
| 全量 qps 太高 | 限速 + 减 worker；监控源库阈值 |
| 全量未走 readonly | 改 task.sourceDsnRef 指 readonly；重启 worker |
| 业务洪峰叠加迁移 | 暂停全量 / incr，等业务洪峰过去再启动 |
| 源库本来就紧张 | 先扩源库容量再做迁移；不能一边救火一边迁移 |
| 大事务 + binlog 解析压力 | Canal 拉 binlog 也消耗 IO；考虑专用从库给 Canal |
| 慢 SQL（业务侧） | 与本迁移无关，走业务慢查询治理 |

## 验证

- threads_running 回落 < 50
- 业务 P99 RT 恢复
- 全量进度仍在递增（虽然慢了）

## 紧急停全量

如果限速无效，业务受严重影响：

```bash
curl -X POST http://migrator.internal:8030/migrator/tasks/$TASK_ID/pause \
# task 暂停，全量 worker 退出，checkpoint 持久化
# 业务恢复后 POST /start {phase: full} 续传
```

## 事后

- [ ] 全量任务必须强制走 ReadReplica（架构改硬）
- [ ] 加源库压力联动：threads_running > 阈值自动限速
- [ ] 业务洪峰窗口与迁移窗口分开（如夜间跑全量）
- [ ] postmortem
