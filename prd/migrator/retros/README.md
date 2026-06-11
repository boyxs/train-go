# retros — 演练记录 + 事件复盘

> 演练（drill）+ 复盘（postmortem）合并目录。
> drill: cutover 前必跑 7 项故障演练，每项产出一份记录归档此目录。
> postmortem: P0/P1 事件 + 演练"不通过"必出复盘报告。
> 模板：[_drill-template](./_drill-template.md) | [_postmortem-template](./_postmortem-template.md) | 示例：[2026-04-canal-failure](./2026-04-canal-failure.md)

---

## 6 项必跑演练

按 `zero-downtime-playbook.md` §13 顺序：

| # | 演练 | 关联 runbook | 通过标准 |
|---|------|------------|---------|
| 1 | Canal 故障演练 | [canal-failure.md](../runbooks/canal-failure.md) | lag 5 min 内恢复，无数据丢 |
| 2 | Kafka 故障演练（仅 sinkType=kafka）| [kafka-broker-down.md](../runbooks/kafka-broker-down.md) | leader 重选 < 30s + 死信重放无丢 |
| 3 | Sink 故障演练 | [sink-unreachable.md](../runbooks/sink-unreachable.md) | 死信自动重放，无数据丢 |
| 4 | webook-migrator 进程崩溃 | [migrator-service-down.md](../runbooks/migrator-service-down.md) | 拉起后 checkpoint 续传 |
| 5 | 切读回滚 | （无单独 runbook，直接 gray=0） | 业务侧 ChooseSide 立即返回 SideOld |
| 6 | cutover 回滚（**最关键**） | [cutover-rollback.md](../runbooks/cutover-rollback.md) | 双写期 rollback：gray=0 后业务读全回 OLD，OLD/NEW 一致 |

---

## 命名规则

```
YYYY-MM-DD-<scenario>-<env>.md

示例：
  2026-05-09-canal-failure-staging.md
  2026-05-10-cutover-rollback-staging.md
```

约定：
- `staging`：在 staging 环境演练（推荐）
- `prod`：禁止在 prod 跑「故障注入」类演练（cutover-rollback 例外，用真实 task 但选低峰期 + 业务方知情）
- `dev`：仅做最初验证

---

## 用法

1. **演练前**：

   - copy `_drill-template.md` 为 `YYYY-MM-DD-<scenario>-<env>.md`
   - 填写「目的」「准备」「关联 task_id」
   - **业务方书面知情**（即使 staging）

2. **演练中**：

   - 严格按 `runbooks/` 步骤走
   - 实时填写「实际结果」表格
   - 注入故障 → 确认告警触发 → 走恢复流程

3. **演练后**：

   - 填写「通过标准」勾选
   - 「改进项」必填（即使没问题，也写"演练流程优化"）
   - 演练 owner + 见证人签字
   - 提 PR 归档（review 中可以指出"通过标准没勾完"等问题）

---

## 准入门槛

cutover 申请被系统验证时会检查：

```bash
# 系统在切流申请（{stage:DST_ONLY, action:propose}）时校验
ls retros/$(date -d '7 days ago' +%Y-%m-%d 2>/dev/null || date -v-7d +%Y-%m-%d)-*-staging.md 2>/dev/null
# 期望：6 类演练记录都有 + 都在 7 天内
```

7 天内未跑完 6 项 → cutover 拒绝。

---

## Postmortem 部分

### 何时必做

| 严重度 | 必做 | SLA |
|--------|------|-----|
| **P0**（业务影响 / 数据丢失 / 不可 rollback）| 必做 | 解决后 5 工作日内出报告 |
| **P1**（迁移流程中断 / 恢复 > 30 min）| 必做 | 解决后 10 工作日内出报告 |
| **P2** | 推荐（runbook 更新即可，可不出独立报告）| 不强制 |

演练"总判定 = 不通过"也必须出 postmortem（虚拟事件复盘）。

### 命名

```
YYYY-MM-DD-<scenario>-<severity>.md
例：2026-06-15-cutover-rollback-P0.md
```

### blameless 准则

- 不写"张三误操作导致……" → 写"系统未阻止该操作 → Action: 加 confirm 弹窗"
- 不写"李四没看告警" → 写"告警发到不再值守的群 → Action: 改告警渠道"

目标：**找机制 bug，不是找人 bug**。任何指责性语言 review 一律打回。

### Action Items 格式

每份 postmortem 末尾必须有：

| # | Action | Owner | 期限 | 类型 | 状态 |
|---|--------|-------|------|------|------|

类型枚举：prevention / detection / mitigation / recovery / process。

30 天内所有 Action Items 必须落地或明确 reschedule 理由。
