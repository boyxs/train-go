# Runbook: 切流期间紧急回滚（DST_ONLY 前）

> 触发：切流灰度 / 双写期业务方反馈异常 / 5xx 飙升 / 用户投诉
> 严重度：**P0**
> 预期解决时间：**< 5 min 决策 / < 5 min 完成**
> 关联：`zero-downtime-playbook.md` §8.5 / §13.6
> ⚠️ **适用范围**：仅适用于**进入 DST_ONLY 之前**（SRC_FIRST / DST_FIRST 双写期）。一旦切到 `DST_ONLY` 单写，OLD 停止更新、**不可回滚**——这是 point of no return，故 cutover checklist 要求进 DST_ONLY 前充分对账。

---

## 前置变量

操作前 `export`：

- `TASK_ID=<数字>`：URL 路径 / 控制库 `task.id`
- `TASK_NAME=<task name>`：Redis stage/gray/cutover_propose key 用 taskName 而非 ID（v1 SwitchService 与 SDK 共享约定）

## 症状

- 业务方紧急反馈：灰度 / 切流期间用户操作异常
- `webook_core_http_requests_total{status=~"5.."}` 飙升
- 业务 P99 RT 异常上涨
- 数据展示错乱 / 字段缺失 / 内容不对

## 立即动作（5 分钟内）

**关键决策：双写期能 rollback 就 rollback，不要先排查根因。** 双写期 OLD 一直在被同步写、保有全量数据，切回安全。

1. **触发 rollback**

   ```bash
   curl -X POST http://migrator.internal:8083/migrator/tasks/$TASK_ID/switch \
     -H "Authorization: Bearer $ADMIN_TOKEN_A" \
     -d '{"stage": "SRC_FIRST", "action": "rollback"}'
   ```

2. **同时通知**：业务 oncall + DBA + leader（P0 流程）

3. **保留现场**：dump 控制库 task / checkpoint / 业务侧 SDK 配置

   ```bash
   mysql webook_migrator -e "SELECT * FROM task WHERE id=$TASK_ID\G" > /tmp/cutover-rollback-$(date +%Y%m%d-%H%M).log
   redis-cli get "migrator:stage:$TASK_NAME" >> /tmp/cutover-rollback-$(date +%Y%m%d-%H%M).log
   ```

## rollback 内部时序（系统自动）

```
T-0    收到 rollback 指令
       switch_stage = SRC_FIRST，gray 自动归零
       业务侧 SDK 下次 ChooseSide 立即返回 SideOld（读全切回 OLD）

T-0    立即生效
       双写期 OLD 一直被同步写、保有全量数据，无需"追平"，秒级一致
```

## 验证（rollback 完成后）

```bash
# 1. switch_stage 已回到 SRC_FIRST
redis-cli get "migrator:stage:$TASK_NAME"
# 期望：SRC_FIRST

# 2. gray 已归零（业务读全回 OLD）
redis-cli get "migrator:gray:$TASK_NAME"
# 期望：0

# 3. 业务侧 5xx 恢复
curl -G http://prom:9090/api/v1/query --data-urlencode \
  'query=rate(webook_core_http_requests_total{status=~"5.."}[1m])'
# 期望：回到正常水位

# 4. 跑全量 verify 确认 OLD/NEW 双写一致
spark-submit ... full-verify --task-id $TASK_ID
# 期望：mismatch_rate < 0.0001%
```

## 永久修复

rollback 不是终点，必须查清楚切流失败根因后再决定下一步：

| 根因 | 后续动作 |
|------|---------|
| NEW 容量 / 性能不足 | 扩容 NEW，重测后再 cutover |
| 业务代码读 NEW 字段错 | 修业务代码，重新发布，重测后再 cutover |
| Transform bug 导致数据错 | 修 transform，重做全量 + 增量，重 verify 通过后再 cutover |
| Sink 持续不稳定 | 评估是否换 sink；同样需要重做全量 |
| 灰度阶段没暴露的边缘 case | 加测试用例 → 修代码 → 走影子流量验证后再 cutover |

## 注意事项

- **仅双写期可回滚**：SRC_FIRST / DST_FIRST 双写期 OLD 有全量数据，rollback 秒级切回；进入 `DST_ONLY` 单写后 OLD 停滞、**不可回滚**（point of no return）
- **rollback 幂等**：重复触发安全，最终 stage = SRC_FIRST
- **rollback 期间 OLD 仍在双写**：业务侧 SDK 切回读 OLD，写仍双写 OLD+NEW
- **rollback 后再 cutover**：必须重新走 verify + 双人复核，不允许直接 cutover

## 事后

- [ ] 强制出 P0 postmortem
- [ ] 复盘灰度阶段为何没发现问题
- [ ] cutover checklist 补充本次缺失项
- [ ] 演练补充类似场景
