# Webook

本文件为 Claude Code 提供项目开发指引。

## 模块

```
gitee.com/train-cloud/geektime-basic-go
```

## 核心依赖

| 包 | 用途 |
|---|---|
| `github.com/gin-gonic/gin` | HTTP 框架 |
| `gorm.io/gorm` + `gorm.io/driver/mysql` | ORM + MySQL 驱动 |
| `github.com/redis/go-redis/v9` | Redis 客户端 |
| `github.com/golang-jwt/jwt/v5` | JWT 令牌 |
| `github.com/google/wire` | 依赖注入 |
| `go.uber.org/mock/mockgen` | Mock 生成 |
| `github.com/tencentcloud/tencentcloud-sdk-go` | 腾讯云短信 |
| `github.com/dlclark/regexp2` | 正则（支持前瞻 `(?=`） |
| `golang.org/x/crypto` | 密码哈希（bcrypt） |

## 常用命令

### 构建
```bash
# 本地开发（Windows）
make -f win.mk build

# Docker + K8s 部署
make docker

# 清理 K8s 部署
make clean
```

### 测试
```bash
# 运行全部测试
go test ./...

# 运行单个测试
go test ./internal/service/... -run TestXxx

# 运行集成测试（需要 MySQL + Redis）
go test ./internal/integration/...
```

### 生成 Mock
```bash
make -f win.mk mockgen
```

### 生成 Wire 依赖注入
```bash
wire ./...
# 集成测试的 wire
wire ./internal/integration/setup/...
```

### 基础设施（K8s）
```bash
make -f infra.mk all      # 部署 MySQL + Redis
make -f infra.mk mysql    # 仅部署 MySQL
make -f infra.mk redis    # 仅部署 Redis
make -f infra.mk status   # 查看 Pod 状态
make -f infra.mk clean    # 清理基础设施
```

## 架构

Go Web 后端（Gin + GORM），严格分层架构：

```
Handler (web) → Service → Repository → DAO/Cache
```

### 分层说明

- **`internal/web/`** — HTTP 处理层。每个 Handler 内嵌 `jwt.JwtHandler`，通过 `RegisterRoutes(*gin.Engine)` 注册路由。
- **`internal/service/`** — 业务逻辑层。包含 `UserService`、`CodeService`、`oauth2/` 微信登录等。
- **`internal/repository/`** — 数据访问抽象层。协调 DAO（MySQL）+ Cache（Redis）的读写。
- **`internal/repository/dao/`** — GORM 模型和数据库查询。
- **`internal/repository/cache/`** — Redis 缓存实现。
- **`ioc/`** — Wire 依赖注入提供者（DB、Redis、SMS、中间件、Web 服务器）。
- **`config/`** — 基于构建标签的配置：`dev.go`（`!k8s`）和 `k8s.go`（`k8s`）。
- **`pkg/`** — 共享工具：限流器（`pkg/ratelimit/`）、Gin 限流中间件（`pkg/ginx/middleware/ratelimit/`）。
- **`internal/consts/`** — 共享常量（JWT 密钥、Header、Cookie 名称）。

### 依赖注入

使用 [google/wire](https://github.com/google/wire)。入口为 `wire.go`（构建标签 `wireinject`），生成文件为 `wire_gen.go`。集成测试有独立的 wire 配置在 `internal/integration/setup/`。

### 认证

双 Token JWT 策略，位于 `internal/web/jwt/`：
- **Access token**（30min，通过 `x-access-token` header 传递）
- **Refresh token**（7 天，通过 `x-refresh-token` header 传递）
- 登出通过在 Redis 中存储 `ssid` 键来失效会话

### 短信服务

位于 `internal/service/sms/`，支持：
- **腾讯云短信**（`sms/tencent/`）
- **内存实现**（`sms/memory/`）用于本地测试
- **故障转移**（`sms/failover/`）— 轮询多个服务商，原子计数器
- **超时故障转移**（`sms/failover/timeout_failover.go`）— 连续超时后切换服务商
- **限流装饰器**（`sms/ratelimit/`）— Redis 滑动窗口限流
- **鉴权装饰器**（`sms/auth/`）— JWT 权限控制

活跃的短信实现在 `ioc/sms.go` 中选择。

### 构建标签

- `dev`（默认）— 本地配置，Windows 二进制
- `k8s` — K8s 配置，集群内部服务地址

### Mock

所有接口使用 `go.uber.org/mock/mockgen` 生成 Mock。Mock 包命名模式为 `*mocks/`。接口变更后运行 `make -f win.mk mockgen` 重新生成。

## 项目文件结构

```
webook/
├── main.go                          # 入口
├── wire.go                          # Wire 提供者（构建标签: wireinject）
├── wire_gen.go                      # Wire 生成文件
├── config/
│   ├── types.go                     # 配置结构体
│   ├── dev.go                       # 本地开发配置（!k8s）
│   └── k8s.go                       # K8s 配置（k8s）
├── internal/
│   ├── consts/
│   │   ├── user.go                  # JWT 密钥、Cookie/Header 名称
│   │   └── wechat.go                # 微信 OAuth2 常量
│   ├── domain/
│   │   ├── user.go                  # 用户领域模型
│   │   └── wechat.go                # 微信认证领域模型
│   ├── web/
│   │   ├── user.go                  # 用户 Handler（注册/登录/个人信息/短信登录）
│   │   ├── wechat.go                # 微信 Handler（OAuth2 回调）
│   │   ├── result.go                # 统一响应结构体
│   │   ├── jwt/
│   │   │   ├── types.go             # JwtHandler 接口 + UserClaims
│   │   │   ├── handler.go           # JWT 实现（设置/提取/清除 token）
│   │   │   └── mocks/jwt_mock.go    # JwtHandler Mock
│   │   └── middleware/
│   │       ├── login.go             # Session 认证中间件
│   │       └── login_jwt.go         # JWT 认证中间件
│   ├── service/
│   │   ├── user.go                  # 用户服务
│   │   ├── code.go                  # 验证码服务（短信验证）
│   │   ├── oauth2/
│   │   │   ├── types.go             # OAuth2Service 接口
│   │   │   └── wechat/service.go    # 微信 OAuth2 实现
│   │   ├── sms/
│   │   │   ├── types.go             # SMSService 接口
│   │   │   ├── memory/service.go    # 内存实现（开发/测试）
│   │   │   ├── tencent/service.go   # 腾讯云短信
│   │   │   ├── failover/
│   │   │   │   ├── failover.go      # 轮询故障转移
│   │   │   │   └── timeout_failover.go # 超时故障转移
│   │   │   ├── ratelimit/limiter.go # 限流装饰器
│   │   │   └── auth/service.go      # JWT 鉴权装饰器
│   │   └── mocks/                   # 服务 Mock
│   ├── repository/
│   │   ├── user.go                  # 用户仓储
│   │   ├── code.go                  # 验证码仓储
│   │   ├── dao/
│   │   │   ├── init.go              # GORM 自动建表
│   │   │   └── user.go              # 用户 DAO（GORM 模型）
│   │   └── cache/
│   │       ├── user.go              # 用户缓存（Redis）
│   │       └── code.go              # 验证码缓存（Redis，TTL）
│   └── integration/
│       ├── user_test.go             # 用户集成测试
│       ├── wechat_test.go           # 微信集成测试
│       └── setup/
│           ├── db.go                # 测试数据库初始化
│           ├── redis.go             # 测试 Redis 初始化
│           ├── wire.go              # 集成测试 wire（wireinject）
│           └── wire_gen.go          # 集成测试 wire 生成文件
├── ioc/
│   ├── db.go                        # GORM DB 提供者
│   ├── redis.go                     # Redis 客户端提供者
│   ├── sms.go                       # 短信服务选择
│   ├── web.go                       # Gin 引擎 + 中间件配置
│   └── wechat.go                    # 微信 OAuth2 服务提供者
└── pkg/
    ├── ratelimit/
    │   ├── types.go                 # Limiter 接口
    │   └── redis_sliding_window.go  # Redis 滑动窗口实现
    └── ginx/middleware/ratelimit/
        └── builder.go               # Gin 限流中间件构建器
```

## 核心接口

| 接口 | 位置 | 实现 |
|---|---|---|
| `UserHandler` | `internal/web/user.go` | `UserHandlerImpl` |
| `JwtHandler` | `internal/web/jwt/types.go` | `RedisJWTHandler` |
| `UserService` | `internal/service/user.go` | `UserServiceImpl` |
| `CodeService` | `internal/service/code.go` | `CodeServiceImpl` |
| `OAuth2Service` | `internal/service/oauth2/types.go` | `WechatService` |
| `SMSService` | `internal/service/sms/types.go` | memory/tencent/failover/ratelimit/auth |
| `UserRepository` | `internal/repository/user.go` | `UserRepositoryImpl` |
| `UserDAO` | `internal/repository/dao/user.go` | `GormUserDAO` |
| `UserCache` | `internal/repository/cache/user.go` | `RedisUserCache` |
| `Limiter` | `pkg/ratelimit/types.go` | `RedisSlidingWindowLimiter` |

## 领域模型

### User（`internal/domain/user.go`）
```go
type User struct {
    Id, Email, Password, Nickname, Birthday, AboutMe, Phone string/int64
    WechatAuth WechatAuth
    CreatedAt, UpdatedAt string
}
```

### WechatAuth（`internal/domain/wechat.go`）
微信 OAuth2 的 OpenId + UnionId。

## 开发流程

> **每个任务按顺序执行以下阶段，不可跳过。**

### 阶段一：理解（编码前）

- 先阅读相关模块的现有代码，理解上下文和现有模式
- **新功能:** 先输出简要设计方案（接口定义、数据结构、分层边界、影响范围），等确认后再写代码
- **Bug 修复:** 先定位根因并说明原因，等确认修复方案后再改
- **重构:** 先列出变更计划（哪些文件、改什么、为什么），等确认后再动手
- 不确定的地方主动问，不要猜

### 阶段二：编码

- 每次只做一件事，不要跨层大范围改动
- 新增代码遵循所在层的现有模式和风格
- 不要自作主张修改文件组织结构，变更目录/文件结构前先确认
- 不要引入新依赖包而不说明原因和替代方案

### 阶段三：自测

编码完成后必须执行：

- **编译检查:** `go build ./...` 确认无编译错误
- **Lint 检查:** `go vet ./...` 确认无明显问题
- **基本验证:** 写出核心路径的手动验证步骤
- **边界条件:** 逐项检查以下场景
  - 空数据 / nil / 零值处理
  - 并发场景（Redis 操作、缓存一致性）
  - 错误传播是否正确（error wrapping）
  - 大数据量场景是否有性能问题

### 阶段四：自我 Review

对自己的改动做一次审查，逐项检查：

- **硬编码:** 有没有应该提取为配置/常量的值
- **错误处理:** error 是否完整处理，数据库连接失败、Redis 超时、第三方 API 异常是否处理
- **安全:** 有没有 SQL 注入、XSS、未鉴权接口的风险
- **回归:** 是否影响现有功能，是否破坏其他层的调用
- **性能:** 有没有 N+1 查询、全表扫描、缓存穿透的问题
- **可读性:** 命名是否语义明确，逻辑是否清晰

### 阶段五：文档更新

- 新增/修改 API 接口 → 更新接口文档或相关注释
- 改了数据结构 → 更新 domain model 和相关注释
- 完成功能/修复 → 追加到 `DEVLOG.md`（格式见下方）

### 阶段六：提交

- **Commit 格式:**

  ```
  type(scope): description
  ```

  - type: feat / fix / refactor / docs / chore / perf / test
  - scope: web / service / repository / dao / cache / ioc / config / pkg / integration
  - 一个 commit 只做一件事，不混合不相关改动

- 目前仅提供简短的 commit message，暂时不需要自动提交

## DEVLOG.md 记录格式

```
## [日期] 功能/修复名称

**变更内容**: 一句话描述做了什么
**影响范围**: 涉及哪些模块/文件
**技术决策**: 为什么这样做（如果有取舍）
**待办**: 后续需要跟进的事项（如果有）
**会话**: 会话名称
```

同一天归类到同一个日期标题下，日期按降序排列（最新的在最前面）。

## 会话管理

- 每次开始新功能时用 `/rename` 命名会话，格式：`YYMMDD-模块-功能中文`
- DEVLOG 中每条记录带 **会话** 字段，方便后续恢复上下文

## 工作规则

### 沟通

- 用中文沟通，代码注释可中可英但需要简洁
- 不确定的事情主动问，不要猜测后自行决定
- 不要一次性输出超过 300 行代码而不分段解释

### 代码质量

- 严格遵循分层架构：Handler → Service → Repository → DAO/Cache，不允许跨层调用
- 写代码要考虑数据量、性能、可用性、扩展性：大数据量查询优先在数据库内完成过滤和聚合，避免全量加载到内存
- 不要用 `fmt.Println` 作为正式日志，用项目的日志模块（`go.uber.org/zap`）
- 错误处理：不忽略 error，必须处理或显式标注
- 测试假数据必须用 `// ===== TODO: 测试假数据，正式环境删除 START/END =====` 注释块包裹，方便一次性删除

### 命名规范

- **方法命名要语义明确**：查询用 `FindXx`，分页用 `PageXxs`，列表用 `ListXxs`，新增用 `CreateXx`，更新用 `UpdateXx`，删除用 `DeleteXx`
- 不要用模糊的 `FetchList`、`GetData` 等命名，要根据具体业务简洁表达
- receiver 命名用类型首字母小写（如 `func (s *Service) Run()`）
- 不用不必要的类型参数

### 记录与更新

- 完成功能后自动追加到 `DEVLOG.md`（格式见开发流程章节）
- 同一天归类到同一个日期标题下，日期按降序排列
- `CLAUDE.md` 只放规则和约定，不放功能记录
- 发现问题、踩过的坑、更好的做法，主动记录到 `memory/` 目录（feedback 类型），并同步更新 `MEMORY.md` 索引
- 每次对话结束前，检查 `CLAUDE.md` 和 `MEMORY.md` 是否需要更新

### 禁止事项

- 不要自作主张拆分或合并文件结构，改动文件组织前先确认
- 不要引入新的依赖包而不说明原因
- 不要在 PR 描述中写废话，只写有信息量的内容
