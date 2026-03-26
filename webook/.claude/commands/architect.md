# 架构设计

> **铁律**: 没有批准的设计，不写一行代码。
> 没有例外。没有"太简单不需要设计"。没有"边写边想"。

对当前需求做一次完整的架构设计，输出设计方案等确认后再编码。

## Checklist

### 1. 需求分析
- 用一句话描述这个功能要解决什么问题
- 列出核心用户场景（谁在什么情况下做什么）
- 识别涉及的分层模块（web / service / repository / dao / cache / ioc）

### 2. 数据设计
- 涉及哪些 MySQL 表（GORM 模型定义在 `internal/repository/dao/`）
- 需要新增/修改哪些字段，字段类型和约束
- 表关系设计（外键 / 关联查询）
- 需要的索引（`gorm:"index"` / `gorm:"uniqueIndex"`）
- 缓存策略（哪些数据需要 Redis 缓存，key 设计，TTL）

### 3. 接口设计
- API 路径、Method、请求参数、响应格式
- 遵循统一响应格式 `Result{Code, Msg, Data}`
- 认证要求（哪些接口需要 JWT 认证，走 `x-access-token` header）
- 限流策略（是否需要 Redis 滑动窗口限流）

### 4. 分层设计
按 Handler → Service → Repository → DAO/Cache 逐层设计：
- **Handler**（`internal/web/`）— 路由注册（`RegisterRoutes`）、Gin 参数绑定、`Result` 响应构造
- **Service**（`internal/service/`）— 业务逻辑、接口定义
- **Repository**（`internal/repository/`）— Cache-Aside 策略，协调 DAO + Cache
- **DAO**（`internal/repository/dao/`）— GORM 模型和查询，参数化防注入
- **Cache**（`internal/repository/cache/`）— Redis 缓存，需要原子性的用 Lua 脚本

每层列出接口签名，先查现有接口是否可复用。

### 5. 风险评估
- **性能**: 数据量预估，是否需要分页 / 缓存 / 批量查询
- **并发**: Redis 操作是否需要原子性（Lua 脚本），FindOrCreate 是否有竞态
- **安全**: SQL 注入（GORM 参数化）、未鉴权接口、敏感数据泄露
- **回归**: 改动是否影响现有功能
- **依赖**: 是否引入新依赖包（需说明原因和替代方案）

### 6. Wire 依赖注入
- 新增的 Provider 函数（构造函数）
- 需要修改 `wire.go` 和 `wire_gen.go`
- 集成测试是否需要更新 `internal/integration/setup/wire.go`

### 7. 任务拆分
- 按分层拆分为可独立开发的子任务
- 每个任务 2-5 分钟粒度：写测试 → 跑失败 → 实现 → 跑通过 → 提交
- 标注依赖关系（DAO 先于 Repository，Repository 先于 Service）

## 输出格式

```
## 功能名称

### 需求摘要
一句话描述

### 数据设计
MySQL 表 / GORM 模型 / 字段 / 索引 / Redis 缓存策略

### 接口设计
API 列表 + 请求响应格式（Result{Code, Msg, Data}）

### 分层设计
Handler → Service → Repository → DAO/Cache 每层接口签名

### Wire 变更
新增 Provider / wire.go 修改点

### 风险点
性能 / 并发 / 安全 / 回归

### 任务拆分
子任务列表（bite-sized）+ 依赖关系
```

## Red Flags

如果你正在想以下任何一条，停下来：
- "太简单了，不需要设计" → 未经审视的假设造成最多返工
- "先写代码，设计自然就出来了" → 边写边想 = 边拆边建
- "和之前做过的差不多" → 差不多 ≠ 一样，差异点正是风险点

## 下一步

设计确认后 → `/tdd` 开始测试驱动开发
