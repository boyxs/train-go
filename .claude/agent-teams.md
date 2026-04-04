---
description: 多 agent 团队协作规则
---

# Agent Teams（多 agent 协作）

| 角色名 | 担任者 | 职责 |
|--------|--------|------|
| **Boss** | 我（人） | 定需求、做决策、最终拍板 |
| **PM** | Leader agent | 产品经理兼设计师：需求分析、原型设计、交互规范、验收标准 |
| **Arch** | Leader agent | 方案设计、接口定义、拆分任务、集成验收、Code Review |
| **FE** | Teammate agent | 负责 webook-fe/ 下的前端模块 |
| **BE** | Teammate agent | 负责 webook/ 下的后端模块 |
| **QA** | Teammate agent | 测试：编写/执行测试用例、覆盖率检查、回归验证 |

**何时使用:** 一个功能涉及前端 + 后端，需要并行推进；模块之间有接口契约或数据依赖
**何时不使用:** 改动范围小（单模块 bug 修复、UI 调整）；前后端由同一人完成

**使用模式:** 一个长期团队持续复用，Arch 保持上下文，FE/BE 每次任务干净启动由 Arch 传必要上下文。长期记忆存文件：`CLAUDE.md`（规范）+ `memory/`（经验）+ `CHANGELOG.md`（变更历史）

**工作流程:**
1. Boss 向 PM 描述需求
2. PM 完成需求分析 + 原型/交互设计（`/workflow:design`），等 Boss 确认
3. Arch 基于 PM 产出完成技术设计（`/workflow:architect`）+ 接口契约，等 Boss 确认
4. FE + BE 并行编码 + 自测（`/workflow:tdd`）
5. FE + BE 完成后向 Arch 汇报
6. QA 编写并执行测试用例（`/workflow:test`），覆盖核心路径 + 边界条件
7. Arch 做整体 Review（`/workflow:review`），检查模块间集成
8. Arch 完成文档更新 + 提交（`/workflow:done`），向 Boss 汇报

**FE / BE 规则:** 只操作分配的模块目录，不交叉修改；遵循对应模块的 `.claude/rules/` 规范
**QA 规则:** 后端集成测试放 `webook/internal/integration/`；测试必须覆盖正常路径、边界条件和错误场景；发现问题提给 Arch 而非直接改业务代码
