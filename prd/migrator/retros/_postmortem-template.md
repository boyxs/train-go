# Postmortem: <事件标题>

> 事件 ID：YYYY-MM-DD-<scenario>-<severity>
> 严重度：P0 / P1
> 影响时长：HH:MM ~ HH:MM（共 N min）
> 影响范围：<受影响业务 / 用户量 / 数据量>
> 归因类型：infra / code / config / process / external
> 复盘 owner：<姓名>
> 复盘日期：YYYY-MM-DD

---

## 一句话总结

<一句话讲清楚发生了什么 + 影响 + 根因。读者只看这一句话也能知道核心>

---

## 时间线

| 时间 | 事件 | 操作人 | 备注 |
|------|------|--------|------|
| HH:MM | 异常开始 / 告警触发 | 系统 | <监控指标快照> |
| HH:MM | oncall 接到告警 | <name> | |
| HH:MM | 走 runbook：<runbook-name> | <name> | |
| HH:MM | 临时缓解 | <name> | <如：gray=0> |
| HH:MM | 找到根因 | <name> | |
| HH:MM | 永久修复上线 | <name> | |
| HH:MM | 业务恢复 / 验证通过 | <name> | |
| HH:MM | 复盘小组成立 | | |

**关键时间间隔**：

- 异常开始 → 告警触发：N min（**MTTD**, 越短越好）
- 告警 → oncall 响应：N min
- oncall 响应 → 临时缓解：N min（**MTTR-缓解**）
- 临时缓解 → 根因定位：N min
- 根因 → 永久修复：N min（**MTTR-修复**）

---

## 影响

| 维度 | 影响 |
|------|------|
| 用户量 | N 用户 / X% 流量 |
| 业务功能 | <如：文章发布失败 / 搜索结果错乱> |
| 数据 | 是否有数据丢失 / 数据不一致 / 修复方案 |
| SLO 违约 | 是 / 否（哪些 SLO 被打破） |
| 客户 / 业务方反馈 | <投诉数 / 渠道> |

---

## 根因（5 Whys）

**现象**：<一句话描述显性问题>

1. **Why?** <第一层原因>
2. **Why?** <第二层>
3. **Why?** <第三层>
4. **Why?** <第四层>
5. **Why?** <根本原因>

**根本原因**：<最终归结到的机制 bug，不是人的错>

**触发条件**（什么情况下会再次发生）：
- 条件 1
- 条件 2

---

## 处置过程评估

| 阶段 | 做得好 | 做得不好 |
|------|--------|---------|
| 检测 | 告警在 5 min 触发 | 告警渠道不对，oncall 30 min 才看到 |
| 诊断 | runbook 步骤清晰，5 min 定位 | 缺少日志关键字段 |
| 缓解 | gray=0 立即降影响 | / |
| 修复 | hotfix 按计划上线 | 无灰度，新 bug 风险高 |
| 沟通 | 业务方 5 min 内通知 | 没同步用户层公告 |

---

## Action Items

| # | Action | Owner | 期限 | 类型 | 状态 |
|---|--------|-------|------|------|------|
| 1 | <具体行动> | <name> | YYYY-MM-DD | prevention / detection / mitigation / recovery / process | TODO / DOING / DONE |
| 2 | | | | | |

类型必填，期限不允许「TBD」，owner 必须是具体人不是部门。

---

## 经验沉淀

### 修改了哪些文档

- [ ] runbooks/<xxx>.md：补充诊断步骤
- [ ] zero-downtime-playbook.md：§X 补充边界条件
- [ ] architecture.md：§X 设计修正

### 修改了哪些代码 / 配置

- PR 链接 / commit hash

### 增加了哪些演练

- drill-records/<新增演练>

---

## blameless 提醒

复盘讨论中若出现"是某某操作错"的归因，重新表达为"系统/流程/文档没阻止/没引导该操作"。本 postmortem 用于改进，不用于追责。

---

## 签字

- 复盘 owner：<name> ____  日期：YYYY-MM-DD
- 业务方代表：<name> ____  日期：YYYY-MM-DD
- 见证人：<name> ____  日期：YYYY-MM-DD
- Leader 知情：<name> ____  日期：YYYY-MM-DD
