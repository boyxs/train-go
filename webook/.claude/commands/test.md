# 测试补充

> **铁律**: 没有运行结果，不声称已覆盖。
> 写了测试不跑 = 没写。跑了不看输出 = 没跑。

为已有代码补充单元测试和集成测试，确保关键路径有覆盖。

## Checklist

### 1. 测试现状评估
- 运行 `go test ./...` 记录当前状态（通过数、失败数）
- 运行 `go test -cover ./...` 查看各包覆盖率
- 识别测试覆盖的空白区域：
  - 哪些模块/文件没有测试
  - 哪些公开接口没有测试
  - 哪些边界条件没有覆盖
- **输出评估结果，等确认优先级后再继续**

### 2. 单元测试

对每个需要覆盖的模块：

**识别测试目标**
- 公开函数/方法的正常路径
- 错误路径（无效输入、MySQL/Redis 依赖失败）
- 边界条件（nil、零值、重复数据、并发）

**编写测试**
- 框架：`testing` + `testify/assert` + `go.uber.org/mock`（gomock）
- 表驱动测试（table-driven）覆盖多场景
- 命名：`TestXxxService_Method_Scenario`
- 用 mockgen 生成的 mock 隔离外部依赖
- mock 变更后运行 `make -f win.mk mockgen`
- 测试假数据用 `// ===== TODO: 测试假数据 START/END =====` 包裹

**运行验证**
- 每写完一组测试：`go test ./internal/xxx/... -v`
- 确认无副作用（测试之间互不干扰）

### 3. 集成测试

集成测试放 `internal/integration/`，用 `testify/suite`：

**识别测试目标**
- 核心 API 的请求→响应完整链路
- 跨模块的数据流转（Handler → Service → Repository → DAO）
- MySQL 读写 + Redis 缓存的真实行为

**编写测试**
- 使用真实 MySQL + Redis，不用 mock
- Wire 配置在 `internal/integration/setup/`
- `TearDownTest()` 中清理测试数据（`TRUNCATE TABLE`）
- 验证完整链路：HTTP 请求 → Gin Handler → 数据落库 → 响应

**运行验证**
- `go test ./internal/integration/... -v`
- 确认不影响已有单元测试

### 4. 覆盖率检查
- `go test -cover ./...` — 查看各包覆盖率
- `go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out` — 可视化
- 关注未覆盖的关键路径，不追求数字

### 5. 完成报告
- 运行 `go test ./... -v` 贴出完整输出
- 报告：
  - 新增测试数量（单元 / 集成）
  - 覆盖的模块和场景
  - 仍未覆盖的已知空白（如有）

## Red Flags

| 借口 | 现实 |
|------|------|
| "代码能跑就行，不需要测试" | 能跑 ≠ 对，测试是证明对的唯一方式 |
| "覆盖率够高就行" | 高覆盖率 ≠ 好测试，一个断言都没有的测试覆盖率也是 100% |
| "mock 太多写不动" | mock 多说明耦合重，先考虑重构 |
| "集成测试太慢不值得" | 慢测试抓到的 bug 比快测试多 |
| "这段逻辑太简单不需要测" | 简单逻辑的 bug 最难发现，因为没人会怀疑它 |

## 下一步

测试补充完成后 → `/review` 做代码审查
