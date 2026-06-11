# Runbook: Sink 不可达

> 触发：webook-migrator 日志 `sink apply failed` 飙升 / mysql `dead_letter` 累积
> 严重度：**P1**
> 预期解决时间：**< 30 min**
> 关联：`zero-downtime-playbook.md` §5.4 / §13.3

---

## 症状

- webook-migrator 日志：`sink apply failed: connection refused / timeout / 503` 持续刷
- 控制库 `dead_letter` 开始累积（mysql 查）
- `GET /tasks/:id/lag` 的 dstLagMs 飙升（写不动）

## 立即动作（5 分钟内）

1. **确认是哪个 Sink 出问题**（task 可能挂多个 sink）

   ```bash
   # 看服务日志失败的 sink 类型 + 死信按操作分布
   docker logs webook-migrator --tail 200 | grep "sink apply failed"
   mysql webook_migrator -e "SELECT op, COUNT(*) FROM dead_letter WHERE task_id=$TASK_ID GROUP BY op"
   ```

2. **确认是 Sink 全挂还是部分节点**

   ```bash
   # ES
   curl -s http://es-cluster:9200/_cluster/health | jq
   # 期望 status=green/yellow，红色严重

   # ClickHouse
   echo "SELECT 1" | clickhouse-client --host ck-cluster

   # MySQL Sink
   mysql -h sink-host -e "SELECT 1"
   ```

3. **不要紧急切流回滚**：Sink 故障期间业务侧仍正常（写 OLD 不受影响）。让 dead_letter 累积是正常应对，恢复后自动重放。

## 诊断（10 分钟内）

```bash
# 1. Sink 集群状态详情
# ES
curl -s es-cluster:9200/_cat/nodes?v
curl -s es-cluster:9200/_cat/indices?v | grep article_v1

# ClickHouse
echo "SHOW TABLES" | clickhouse-client
echo "SELECT count() FROM system.replicas WHERE is_readonly" | clickhouse-client

# 2. webook-migrator → Sink 网络
docker exec webook-migrator nslookup sink-host
docker exec webook-migrator nc -zv sink-host 9200

# 3. 死信队列大小
mysql webook_migrator -e "SELECT COUNT(*) FROM dead_letter WHERE task_id=$TASK_ID AND replayed=0"
```

## 永久修复

| 根因 | 处理 |
|------|------|
| Sink 单节点宕机 | 等集群自愈；webook-migrator 自动从健康节点重试 |
| Sink 集群全挂 | 紧急恢复 Sink；恢复后 `POST /tasks/$ID/replay-dl` 重放死信 |
| 网络分区 | 排查网络；恢复后自动重试 |
| Sink 容量满 / 写不进 | 紧急扩容 / 清理；ES 看磁盘 watermark |
| 凭据失效（API key 过期） | 更新 Vault / Secret，webook-migrator 自动 reload |
| Schema 不兼容（ES mapping 冲突） | 临时停 sink，修 mapping，重建索引或新建别名 |

## 验证

```bash
# 1. Sink 恢复
curl -s es-cluster:9200/_cluster/health | jq '.status'  # green / yellow

# 2. 死信不再新增（apply 恢复）
mysql webook_migrator -e "SELECT COUNT(*) FROM dead_letter WHERE task_id=$TASK_ID"
# 隔 1 min 两次，期望行数不再增长

# 3. 死信重放
curl -X POST http://migrator.internal:8083/migrator/tasks/$TASK_ID/replay-dl \

# 4. 跑 verify 确认对账
curl -X POST .../tasks/$TASK_ID/verify -d '{"mode":"sample","sampleRate":0.001}'
```

## 事后

- [ ] Sink 集群至少 3 节点 + 副本
- [ ] 加 Sink 健康检查（不依赖 webook-migrator 探测）
- [ ] 死信队列限额 + 持久化（避免无限增长压垮控制库）
- [ ] postmortem
