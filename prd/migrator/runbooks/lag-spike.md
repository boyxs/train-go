# Runbook: 增量 lag 飙升

> 触发：oncall 巡检 `GET /tasks/:id/lag` 发现 dstLagMs > 5min / 业务反馈 NEW 读不到新数据
> 严重度：**P1**
> 预期解决时间：**< 30 min**
> 关联：`zero-downtime-playbook.md` §5.3 / §15.4 风险

---

## 症状

- `GET /tasks/:id/lag` 的 `dstLagMs` / `srcLagMs` > 300_000（5 min）
- checkpoint `updated_at`（mysql）长时间不推进
- 业务方反馈：在 NEW 上读不到刚写的数据（如果已切读）

## 立即动作（5 分钟内）

1. **确认是真 lag 还是计算错误**

   ```bash
   # binlog 事件时间 vs 当前时间
   curl http://migrator.internal:8083/migrator/tasks/$TASK_ID/lag
   # 看 lastSyncAt 是否合理（毫秒戳，应该接近 now()）
   ```

2. **如果已 gray > 0：立即降 gray=0**（避免业务读到滞后数据）

   ```bash
   # 先记录当前 gray，恢复时还要用
   CURRENT_GRAY=$(curl -s .../tasks/$TASK_ID | jq -r '.data.task.gray_percent')
   echo "原 gray=$CURRENT_GRAY，回滚到 0"
   
   curl -X POST .../tasks/$TASK_ID/gray -d '{"percent": 0}'
   ```

3. **通知 oncall + DBA**

## 诊断（10 分钟内）

```bash
# 1. 大事务正在跑？
mysql -h source-host -e "SHOW PROCESSLIST" | grep -E "Update|Delete|Insert" | awk '$6 > 30'
# Time > 30s 的写事务都列出来

# 2. binlog 产出速率（看是不是业务洪峰）
mysql -e "SHOW MASTER STATUS\G"
# 多次执行看 Position 增长速率

# 3. Sink 是否反压（checkpoint 推进慢 / lag 不收敛）
curl -s http://migrator.internal:8083/migrator/tasks/$TASK_ID/lag | jq '.data'
# dstLagMs 持续高 = Sink 写不动

# 4. checkpoint 是否在更新
mysql webook_migrator -e "SELECT cursor_value, updated_at FROM checkpoint WHERE task_id=$TASK_ID AND phase='incr'"
# 多次执行看 updated_at 是否在增长

# 5. 是否热点 PK 拖慢某 partition（v1 进程内 partition，无单独 lag 可观测）
mysql -h<源库> -e "SHOW PROCESSLIST"   # 看源端是否有大事务 / 热点行批量更新
# 单行高频更新会集中到同一 partition worker（FNV(PK) 固定路由），表现为整体 lag 升高
```

## 永久修复

| 根因 | 处理 |
|------|------|
| 业务大事务（批量删除 / 全表 update） | 等业务事务结束自动追平；下次让业务分批 |
| Sink 写入瓶颈 | 调大 `apply_batch_size` / `apply_workers`；扩 Sink 容量 |
| Hot key（单 partition 占大量数据） | 改 partition 策略（PK + biz_field 复合 hash）；需重建 topic |
| binlog 解析失败（卡在某 event） | 看 Canal 日志；必要时跳过该 event（DDL 等） |
| webook-migrator OOM 频繁重启 | 调大 limits.memory；profiling 找内存泄漏 |
| 网络抖动 | 临时容忍，长期排查链路 |

## 验证

```bash
# 1. lag 收敛
curl .../tasks/$TASK_ID/lag
# 期望：lagMs < 30_000 持续 5 min

# 2. 业务侧验证（如果之前 gray > 0）
# 让产品/QA 在已切读的少量用户上验证 read-your-write

# 3. lag 稳定后，恢复 gray
curl -X POST .../tasks/$TASK_ID/gray -d '{"percent": '$CURRENT_GRAY'}'
```

## 事后

- [ ] 加业务大事务监控告警
- [ ] 评估 partition 数是否够，hot key 用复合 hash
- [ ] webook-migrator 资源 limits 是否合理
- [ ] postmortem
