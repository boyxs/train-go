# Runbook: Kafka Broker 宕机（仅 sinkType=kafka）

> 触发：`sinkType=kafka` 任务的 `apply_qps` 跌零 + `dead_letter` 增长
> 严重度：**P1**
> 预期解决时间：**< 30 min**
> 关联：`04-cutover-playbook.md` §5.4 / §13.2
>
> ⚠️ **适用范围**：仅当 `sinkType=kafka`（Kafka 作迁移**目标**）。v1 CDC 传输是 canal → 进程内 channel partition，**不经 Kafka**——MySQL→MySQL / ES / Mongo 任务不会触发本 runbook。webook-migrator 对 Kafka 是 **producer**（`KafkaSink` 用 sarama SyncProducer，key=PK 保序），不是 consumer。

---

## 症状

- `apply_qps` 跌零（`KafkaSink.Apply` 写不进去）
- `dead_letter` 行数累积（写失败的行入死信兜底）
- webook-migrator 日志：`kafka: client has run out of available brokers` / `sink apply failed`
- 目标 topic 部分 partition `LeaderNotAvailableException`

## 立即动作（5 分钟内）

1. **确认 Kafka 集群剩余可用节点**

   ```bash
   kafka-topics.sh --bootstrap-server kafka:9092 --describe --topic <目标 topic>
   # 关注 Leader / ISR 列；某些 partition 没有 Leader = 宕机影响到该 partition
   ```

2. **检查副本因子**

   ```bash
   kafka-topics.sh --bootstrap-server kafka:9092 --describe --topic <目标 topic> | grep ReplicationFactor
   # 期望 ≥ 3；如果 = 1，单节点宕机就丢 partition
   ```

3. **如果是 Leader 宕机**：等 Kafka 自动选举（< 30s），`KafkaSink.Apply` 自动恢复；否则进入诊断

## 诊断（10 分钟内）

```bash
# 1. Kafka broker 状态
docker ps | grep kafka

# 2. 控制器节点（KRaft）
kafka-metadata-quorum.sh --bootstrap-server kafka:9092 describe --status

# 3. 各 broker 日志
docker logs kafka-broker-0 --tail 200

# 4. 死信积累（宕机期间写失败的行）
mysql webook_migrator -e "SELECT COUNT(*) FROM dead_letter WHERE task_id=$TASK_ID AND replayed=0"
```

## 永久修复

| 根因 | 处理 |
|------|------|
| 单节点宕机（副本足） | 等 Kafka 自动 leader 重选（< 30s）；`KafkaSink.Apply` 自动恢复 |
| 多节点宕机（超副本数） | **严重**：部分 partition 不可写。重启失败节点；评估是否 reassign |
| 磁盘满 | 紧急扩容 / 删过期 segment；调短 `retention.ms` |
| 副本因子 = 1 | 立即 reassign：`kafka-reassign-partitions.sh` 把 partition 分散到多节点 |

## 验证 + 死信重放

```bash
# 1. 所有 partition 都有 Leader
kafka-topics.sh --describe --topic <目标 topic> | awk '{print $6}' | grep -c "none"
# 期望：0

# 2. broker 恢复后，重放宕机期间积累的死信
curl -sS -X POST http://migrator.internal:8083/migrator/tasks/$TASK_ID/replay-dl

# 3. 死信清零 + lag 收敛
mysql webook_migrator -e "SELECT COUNT(*) FROM dead_letter WHERE task_id=$TASK_ID AND replayed=0"  # 期望趋 0
curl -s http://migrator.internal:8083/migrator/tasks/$TASK_ID/lag | jq '.data.dstLagMs'                  # 期望下降
```

## 事后

- [ ] 副本因子统一 ≥ 3，min.insync.replicas = 2
- [ ] 加磁盘容量告警（80% 提前预警）
- [ ] kafka rack-aware 配置，避免同 rack 多节点宕机
- [ ] postmortem
