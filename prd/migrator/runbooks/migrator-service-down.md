# Runbook: webook-migrator 服务宕机

> 触发：`MigratorServiceDown` (up == 0 持续 1 min)
> 严重度：**P0**
> 预期解决时间：**< 15 min**
> 关联：`zero-downtime-playbook.md` §13.4 / FAQ Q14

---

## 症状

- Prometheus `up{job="webook-migrator"}` = 0
- `/health` 端点不通
- Grafana migrator-overview 整体没数据
- 控制台 API 全部 502 / connection refused

## 立即动作（5 分钟内）

1. **确认业务影响**：webook-migrator 挂不影响 webook-core / webook-chat 主流程
   - 业务侧 SDK 默认 safe：Redis 读不到 stage 走 SideOld
   - 业务读写都正常
   - 仅迁移流程暂停（lag 会上升）

2. **拉起服务**

   ```bash
   # docker-compose
   cd /opt/webook/deploy
   ./deploy.sh prod restart webook-migrator
   
   # k8s
   kubectl rollout restart deployment webook-migrator -n webook
   ```

3. **同时**：通知业务方 + DBA（P0 流程）

## 诊断（10 分钟内）

```bash
# 1. 服务状态
docker ps -a | grep webook-migrator
kubectl get pods -n webook -l app=webook-migrator

# 2. 最近日志（找 panic / OOM)
docker logs webook-migrator --tail 500 | grep -E "panic|fatal|killed|OOM"
kubectl logs -n webook -l app=webook-migrator --tail 500

# 3. 系统资源
docker stats webook-migrator --no-stream
kubectl top pods -n webook -l app=webook-migrator

# 4. 控制库连接
mysql -h db-host -e "SHOW PROCESSLIST" | grep migrator

# 5. Redis / Kafka / Canal 连通
redis-cli ping
nc -zv kafka-broker-0 9092
nc -zv canal-master 11111
```

## 永久修复

| 根因 | 处理 |
|------|------|
| 进程 panic | 看 stack trace 定位代码 bug；hotfix |
| OOM | 调大 limits.memory；profiling 找泄漏；扩实例 |
| 控制库连接断 | 排查 DB；增加连接重试退避 |
| Redis / Kafka / Canal 全挂 | 走对应 runbook |
| 配置错（昨天发布 bug） | rollback 到上一版本镜像 |
| 磁盘满（日志爆） | 紧急清理 + 加日志轮转 |

## 启动后验证

```bash
# 1. 健康检查
curl http://migrator.internal:8030/health
# {"status":"ok","service":"migrator"}

# 2. 正在运行的 task 状态
curl http://migrator.internal:8030/migrator/tasks?status=incr_running

# 3. lag 是否在恢复
curl http://migrator.internal:8030/migrator/tasks/$TASK_ID/lag
# 期望：从高位逐步下降

# 4. 业务侧 SDK 是否正确读到 Redis
# 看业务侧日志：DualWriter / SwitchReader 调用是否正常
```

## 自动恢复检查

webook-migrator 设计为**无状态**：所有进度在 checkpoint 表（binlog 位点 / 全量分片游标）。重启后自动从断点续传，无数据丢失。

如果重启后发现数据丢，说明 checkpoint 损坏 → 走 [checkpoint-corrupted.md] 流程（待补 runbook）。

## 事后

- [ ] 多实例部署（至少 3 节点）+ Leader Election，单实例宕不影响
- [ ] 加进程级心跳（Pushgateway）独立于服务自身
- [ ] postmortem
