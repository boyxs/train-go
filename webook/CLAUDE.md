# Webook 后端

Go + Gin + GORM + Redis + Wire（DI）

模块路径: `gitee.com/train-cloud/geektime-basic-go`

## 常用命令

```bash
go build ./...                        # 编译
go vet ./...                          # 静态分析
go test ./...                         # 全量测试
go test ./internal/integration/...    # 集成测试（需 MySQL + Redis）
wire ./...                            # 重新生成主 wire_gen.go
wire ./internal/integration/setup/... # 重新生成集成测试 wire_gen.go
make -f mk/mock.mk mockgen           # 重新生成 Mock
make -f mk/es.mk help                # ES 管理命令
make -f mk/infra.mk help             # 基础设施管理
```

## 导航

```
Handler (web/) → Service (service/) → Repository (repository/) → DAO (dao/) / Cache (cache/)
```

| 目录 | 职责 | 找什么来这里 |
|------|------|------------|
| `internal/web/` | 路由、参数绑定、响应 | API 入口、请求/响应结构 |
| `internal/service/` | 业务逻辑 | 业务规则、校验 |
| `internal/repository/` | 协调 DAO + Cache | 数据访问逻辑 |
| `internal/repository/dao/` | GORM 模型和查询 | 表结构、SQL |
| `internal/repository/cache/` | Redis 缓存 | 缓存键、TTL |
| `internal/domain/` | 领域模型 | 业务实体定义 |
| `internal/consts/` | 共享常量 | Token Key、TTL、Redis 键模式 |
| `pkg/` | 跨项目工具 | 限流器、日志接口、Gin 中间件 |
| `ioc/` | Wire Provider | 基础设施初始化 |
| `config/` | 环境配置 | YAML 配置项 |
| `wire.go` | 主入口 DI | 依赖注入全景 |

## 分层规则

- 依赖方向严格单向：Handler → Service → Repository → DAO/Cache，**禁止跨层调用**
- `domain` 是最内层，所有层可依赖它，但 **DAO 不可依赖 domain**
- 每层通过接口解耦，Wire 负责注入实现
- Repository 层实现 Cache-Aside：查询先缓存 → miss 回源 DAO → 回填；写入后清缓存
- 事务在 DAO 层（`db.Transaction()`），Service 层不感知数据库细节

## 命名规范

| 维度 | 规则 | 示例 |
|------|------|------|
| 接口 | `[实体][业务角色][层]` | `ArticleAuthorDAO` |
| 实现 | `[技术限定][实体][业务角色][层]` | `GormArticleAuthorDAO` |
| 文件 | 按业务角色，不按技术 | `article_author.go`（非 `article_dao.go`） |
| receiver | 类型首字母小写 | `func (s *chatService) Send()` |
| 查询方法 | `Find`(单条) `Page`(分页) `List`(列表) | `FindByXx` / `PageXxs` / `ListXxs` |
| 写入方法 | 按业务动作 | `Publish` / `Withdraw`（非 `Process`） |

## 复用已有工具包（禁止重复造轮子）

新模块开发前必须检查以下已有封装，**直接复用，不要手写替代实现**：

| 能力 | 位置 | 用法 |
|------|------|------|
| **限流** | `pkg/ratelimit/` | 注入 `Limiter` 接口，ioc 层配置窗口和阈值 |
| **Gin 限流中间件** | `pkg/ginx/middleware/ratelimit/` | IP 级别限流 |
| **日志** | `pkg/logger/` | 注入 `LoggerX` 接口，结构化字段 |
| **JWT 认证** | `internal/web/jwt/` | 双 Token，Handler 内嵌 `JwtHandler` |
| **统一响应** | `internal/web/result.go` | `Result{Code, Msg, Data}` |
| **共享常量** | `internal/consts/` | Token Key、TTL、Redis 键模式、时间格式 |

**规则：**
1. Handler 层不直接操作 Redis — 限流、缓存等通过注入的接口完成
2. Repository 层有 Cache 接口就必须用 — 不能绕过缓存直连 DAO
3. 新增 Redis 键在 `internal/consts/` 定义 Pattern，不散写 `fmt.Sprintf`
4. 通用工具函数提取到 `pkg/` 或 `internal/domain/`

## 编码约束

- 日志用注入的 `LoggerX`，禁止 `fmt.Println`
- 错误必须处理，禁止 `_ = err`
- 测试假数据用 `// ===== TODO: 测试假数据 START/END =====` 包裹
- 大数据量查询优先在数据库内完成过滤和聚合，避免全量加载到内存
