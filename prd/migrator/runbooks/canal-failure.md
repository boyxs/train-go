# Runbook: Canal 故障

> 触发：`CanalDown` / `CanalLagHigh` / 增量 lag 飙升超 5min
> 严重度：**P1**
> 预期解决时间：**< 30 min**
> 关联：`zero-downtime-playbook.md` §5.4 / §13.1

---

## 症状

- `GET /tasks/:id/lag` 的 srcLagMs > 300_000（5 min）持续上涨
- checkpoint `updated_at`（mysql）不推进
- Canal 节点 `curl canal:11111/metrics` 不通 / 返回异常
- webook-migrator 日志：`canal: connection refused` / `canal: subscribe failed`

## 立即动作（5 分钟内）

1. **确认是 Canal 故障还是网络问题**

   ```bash
   # 任意一个 webook-migrator 实例上执行
   nc -zv canal-master 11111         # 期望 succeeded
   curl -s canal-master:11111/metrics | head -20
   ```

2. **检查 gray，必要时降到 0**（避免业务读到陈旧 NEW 数据）

   ```bash
   curl -X POST http://migrator.internal:8030/migrator/tasks/$TASK_ID/gray \
     -H "Authorization: Bearer $ADMIN_TOKEN" \
     -d '{"percent": 0}'
   ```

3. **通知业务方 oncall**：增量同步暂停，业务侧无影响（仍读写 OLD）

## 诊断（10 分钟内）

```bash
# 1. Canal 进程 / 容器状态
docker ps | grep canal
kubectl get pods -n webook -l app=canal

# 2. Canal 日志（看是 binlog 解析错 / OOM / 连源库失败）
docker logs canal-master --tail 200
# 关注：connection / parser / OOM

# 3. 源库连接状态
mysql -h source-host -e "SHOW PROCESSLIST" | grep canal
# 期望：有 canal 用户的 Binlog Dump 线程

# 4. checkpoint 状态（确认还能从哪个位点恢复）
mysql webook_migrator -e "SELECT phase, cursor_value, updated_at FROM checkpoint WHERE task_id=$TASK_ID AND phase='incr'"
```

## 永久修复

| 根因 | 处理 |
|------|------|
| 进程崩溃 / OOM | k8s 已自动拉起；OOM 调大 limits.memory，重新部署 |
| 网络分区 | 排查网络层，恢复后 Canal 自动从 checkpoint 续传 |
| binlog 已被清理（保留时长不够） | **严重**：不能从原位点续。需要重跑全量 + 重新启动增量。call DBA |
| 源库主从切换 | Canal 重连新 master；如果 GTID 模式自动续传 |
| Canal 解析失败（DDL 不支持） | 跳过该 DDL：`SET @@global.canal.instance.filter.regex` 临时排除该表 |

## 验证

```bash
# 1. lag 恢复
curl http://migrator.internal:8030/migrator/tasks/$TASK_ID/lag
# 期望：lagMs < 5000 持续 5 min

# 2. 跑采样对账，确认无数据丢
curl -X POST http://migrator.internal:8030/migrator/tasks/$TASK_ID/verify \
  -d '{"mode":"sample","sampleRate":0.001}'
# 期望：mismatch 增量 < 100

# 3. lag 稳定后，恢复 gray
curl -X POST http://migrator.internal:8030/migrator/tasks/$TASK_ID/gray \
  -d '{"percent": '$LAST_GRAY'}'
```

## 事后

- [ ] 加 Canal 进程级心跳告警（不依赖 lag 派生）
- [ ] 评估 binlog 保留时长是否够（建议 ≥ 7 天）
- [ ] 写 postmortem
