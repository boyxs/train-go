# Cutover Checklist（一页纸硬门槛）

> 用途：cutover 前**逐条勾选**，任意一条 ❌ 系统拒绝 `{stage:DST_ONLY, action:propose}`
> 关联：`zero-downtime-playbook.md` §8.1 / `architecture.md` §11
> 强制：API 在 `{stage:DST_ONLY, action:propose}` 时校验本 checklist；勾选信息记入 `audit_log`

---

## 状态门槛（系统自动校验）

- [ ] `task.status = incr_running` 且持续 ≥ 24 h
- [ ] `task.gray_percent = 100` 且持续 ≥ 24 h
- [ ] `GET /tasks/:id/lag` 的 dstLagMs P99 < 30s 持续 30 min
- [ ] 最近一次 `verify mode=full` 在 24h 内完成
- [ ] `mismatch_rate < 0.001%` 且 `validate_log WHERE repaired=0` < 100 行
- [ ] `dead_letter` 表行数 1h 内 0 增长（mysql 查）
- [ ] webook-migrator 服务多实例部署（≥ 3 实例 up）

## 演练门槛（系统自动校验）

最近 7 天内 `drill-records/` 必须包含全部 6 项演练记录，且每份「总判定 = 通过」：

- [ ] Canal 故障演练
- [ ] Kafka 故障演练（仅 `sinkType=kafka` 任务；CDC 传输不经 Kafka）
- [ ] Sink 故障演练
- [ ] webook-migrator 进程崩溃演练
- [ ] 切读回滚演练
- [ ] cutover 回滚演练（**最关键**，必须在 staging 真实跑通）

## 业务门槛（人工确认）

- [ ] 业务方 oncall 知情且在岗（姓名 + 电话）
- [ ] 业务方书面确认非高峰窗口（飞书 / 邮件，截图归档）
- [ ] 当前是工作时间（周一 ~ 周四，10:00 ~ 16:00；禁止周五 / 周末 / 节假日 / 凌晨）
- [ ] 业务方近 7 天无大促 / 大流量计划
- [ ] 业务方近 24 h 无紧急发版计划

## 应急门槛（人工确认）

- [ ] DBA 在岗（姓名 + 电话）
- [ ] SRE 在岗（姓名 + 电话）
- [ ] 业务后端 owner 在岗（姓名 + 电话）
- [ ] 应急通道开通（飞书群 / 电话会议链接）
- [ ] cutover-rollback runbook 已二次 review

## 数据门槛（DBA 确认）

- [ ] OLD 库 binlog 保留 ≥ 30 天（rollback 兜底窗口）
- [ ] OLD 库与 NEW 库时钟同步偏差 < 1s（NTP 校准）
- [ ] OLD / NEW 双侧最近备份 < 24 h（极端 rollback 兜底）

## 权限门槛（双人复核）

- [ ] 操作人 A：执行 `{stage:DST_ONLY, action:propose}`（姓名 + 工号）
- [ ] 操作人 B：执行 `{stage:DST_ONLY, action:approve}`（姓名 + 工号，必须与 A 不同）
- [ ] 双人都有 migrator 写权限（v1 走运维侧账号权限管控；接 webook-core SSO 后挂 `migrator:switch` RBAC scope 校验，见 `02-architecture.md` §11）

---

## API 校验示例

```bash
# 系统在收到切流申请时自动校验
curl -X POST http://migrator.internal:8030/migrator/tasks/$TASK_ID/switch \
  -H "Authorization: Bearer $ADMIN_TOKEN_A" \
  -d '{
    "stage": "DST_ONLY",
    "action": "propose",
    "checklist_confirmed": true,
    "business_owner": "李四",
    "dba": "zhang",
    "sre": "王五",
    "low_traffic_window_evidence_url": "https://feishu.cn/abc..."
  }'

# 任意自动校验项失败：
# {
#   "code": 409,
#   "msg": "MIGRATOR_CUTOVER_PRECONDITION_FAILED",
#   "data": {
#     "failed_checks": ["lag_p99_30min", "drill_canal_failure_missing"]
#   }
# }
```
