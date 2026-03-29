# 团队协作规则

## Agent 选型

- 只读任务（调研/探索）用 Explore agent，禁止用 general-purpose
- 方案设计用 Plan agent，不写代码
- 只有代码实现任务才用有写权限的 agent

## 产出验收

- agent 报告完成后，自己跑 build + lint 验证，不信任 agent 的声明
- 逐文件审查 agent 改动的代码，对照 CLAUDE.md 命名规范检查
- 一个 agent 验收通过后再启动下一个，不批量接受

## 文件冲突

- 多个 agent 不得同时修改同一个文件
- 有文件交叉的任务串行执行
