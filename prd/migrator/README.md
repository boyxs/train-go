# webook-migrator 文档

> 数据迁移框架的产品 / 架构 / 实操 / 运维文档。
> 入口船长：按目标找文件，不按目录列文件。

## 我想干嘛 → 读哪个

| 我想... | 读这个 | 预计 |
|---------|--------|------|
| 知道这是什么 / 业务背景 | [01-product](./01-product.md) | 10min |
| 理解整体架构 / 状态机 / 接口签名 | [02-architecture](./02-architecture.md) ★ 权威 | 45min |
| 看任务模块 vertical slice 实现蓝图 | [02b-task-module-design](./02b-task-module-design.md) | 30min |
| 深入原理 / 读代码 / 自己复现一份 | [03-walkthrough](./03-walkthrough.md) | 1-2h |
| 接入业务 SDK（DualWriter / SwitchReader） | [03-walkthrough §5.1 §6.5](./03-walkthrough.md) | 30min |
| 部署 / 切流 / 上线（D-3 → D14 全流程） | [04-cutover-playbook](./04-cutover-playbook.md) | 操作中 |
| cutover 申请前核对硬门槛 | [04b-cutover-checklist](./04b-cutover-checklist.md) | 紧急 |
| oncall 收到告警 → runbook | [runbooks/](./runbooks/README.md) | 紧急 |
| 看历史架构决策为什么 | [adr/](./adr/README.md) | 按需 |
| cutover 前演练 / 事件复盘 | [retros/](./retros/README.md) | 按需 |
| 控制库 DDL / Prometheus 告警 yml | [assets/](./assets/) | 工件 |

