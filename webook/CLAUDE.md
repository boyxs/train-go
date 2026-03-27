# Webook 后端

Go Web 后端（Gin + GORM + Redis），严格分层架构 + Wire 依赖注入。

模块路径: `gitee.com/train-cloud/geektime-basic-go`

## 核心依赖

| 包 | 用途 |
|---|---|
| `gin` | HTTP 框架 |
| `gorm` + `mysql` | ORM + MySQL |
| `go-redis/v9` | Redis |
| `golang-jwt/v5` | JWT 双 Token |
| `google/wire` | 编译时依赖注入 |
| `go.uber.org/mock` | Mock 生成 |
| `go.uber.org/zap` | 日志 |
| `spf13/viper` + etcd | 配置（本地 YAML + 远程热更新） |
| `dlclark/regexp2` | 正则（支持前瞻） |
| `golang.org/x/crypto` | bcrypt 密码哈希 |
| `tencentcloud-sdk-go` | 腾讯云短信 |

## 常用命令

```bash
go build ./...                        # 编译
go vet ./...                          # 静态分析
go test ./...                         # 全量测试
go test ./internal/integration/...    # 集成测试（需要 MySQL + Redis）
make -f win.mk mockgen                # 重新生成 Mock
make -f win.mk build                  # 本地构建（Windows）
wire ./...                            # 重新生成 Wire
wire ./internal/integration/setup/... # 集成测试 Wire
make docker                           # Docker 构建
make -f infra.mk all                  # K8s 部署 MySQL + Redis
```

## 架构

```
Handler (web) → Service → Repository → DAO (MySQL) / Cache (Redis)
                             ↑               ↑
                           domain          domain（DAO 除外）
```

- 依赖方向严格单向，**禁止跨层调用**
- `domain` 是最内层，所有层可依赖它，但 **DAO 不可依赖 domain**
- 每层通过接口解耦，Wire 负责注入实现

## 分层规则

| 层 | 位置 | 职责 | 关键模式 |
|---|---|---|---|
| Handler | `internal/web/` | 路由注册（`RegisterRoutes`）、参数绑定、`Result{Code,Msg,Data}` 响应 | 内嵌 `jwt.JwtHandler` |
| Service | `internal/service/` | 业务逻辑，不感知数据库细节 | 接口定义 + 内部实现 |
| Repository | `internal/repository/` | 协调 DAO + Cache | Cache-Aside 模式 |
| DAO | `internal/repository/dao/` | GORM 模型和查询 | 参数化防注入，事务在此层 |
| Cache | `internal/repository/cache/` | Redis 缓存 | 原子操作用 Lua 脚本（`//go:embed`） |
| IoC | `ioc/` | Wire Provider | DB、Redis、SMS、中间件、Web 服务器 |
| Config | `config/` | 环境配置 | build tag `dev`(默认) / `k8s` |
| Domain | `internal/domain/` | 领域模型 + 状态常量 | 最内层，不依赖任何其他层 |
| Consts | `internal/consts/` | 共享常量 | JWT 密钥、Header、Cookie 名称 |
| Pkg | `pkg/` | 共享工具 | 限流器、Gin 限流中间件 |

## 核心接口

| 模块 | 接口 | 位置 | 实现 |
|---|---|---|---|
| 用户 | `UserHandler` | `web/user.go` | `InternalUserHandler` |
| 用户 | `UserService` | `service/user.go` | `InternalUserService` |
| 用户 | `UserRepository` | `repository/user.go` | `RedisUserRepository` |
| 用户 | `UserDAO` | `dao/user.go` | `GormUserDAO` |
| 用户 | `UserCache` | `cache/user.go` | `RedisUserCache` |
| 文章 | `ArticleHandler` | `web/article.go` | `InternalArticleHandler`（Edit/Publish/Withdraw/Detail/List） |
| 文章 | `ArticleService` | `service/article.go` | `InternalArticleService`（Edit/Publish/Withdraw/Detail/List） |
| 文章 | `ArticleAuthorRepository` | `repository/article_author.go` | `CacheArticleAuthorRepository`（CRUD + Publish/Withdraw） |
| 文章 | `ArticleAuthorDAO` | `dao/article_author.go` | `GormArticleAuthorDAO`（含双库事务） |
| 认证 | `JwtHandler` | `web/jwt/types.go` | `RedisJWTHandler` |
| 验证码 | `CodeService` | `service/code.go` | `SmsCodeService` |
| 短信 | `SmsService` | `service/sms/types.go` | memory / tencent / failover / ratelimit / auth |
| OAuth2 | `OAuth2Service` | `service/oauth2/types.go` | `WechatService` |
| 限流 | `Limiter` | `pkg/ratelimit/types.go` | `RedisSlidingWindowLimiter` |

## 领域模型

- **User**: Id, Email, Password, Nickname, Birthday, AboutMe, Phone, WechatAuth, CreatedAt, UpdatedAt
- **Article**: Id, Title, Content, Author{Id,Name}, Status(Unknown/Unpublished/Published/Private), CreatedAt, UpdatedAt
- **PublishedArticle**: 线上库，和 Article 同构，独立表 `published_article`

## 业务规则

### 文章双库设计

- **制作库**（`article` 表）— 作者视角，存所有文章（草稿、已发布、已撤回）
- **线上库**（`published_article` 表）— 读者视角，只存已发布文章
- **发布** = 制作库 upsert（状态→Published）+ 线上库 upsert（同一事务）
- **撤回** = 幂等设计：只有 Published→Private，Unpublished/Private 不改状态，线上库 DELETE 幂等
- **DAO 层不依赖 domain**：Withdraw 的状态判断通过 `fromStatus`/`toStatus` 参数传入，Publish 的状态随实体字段传入
- **事务在 DAO 层**：`ArticleAuthorDAO.Publish/Withdraw` 用 `db.Transaction()` 保证原子性

### 文章列表

- 分页参数校验在 Service 层：page≤0 默认 1，pageSize≤0 或 >100 默认 10
- 列表返回 `ArticleVO`（不含 Content），详情返回完整 `Article`
- 列表按 `id DESC` 排序

## 认证

双 Token JWT（`internal/web/jwt/`）：
- Access token（30min）→ `x-access-token` header
- Refresh token（7天）→ `x-refresh-token` header
- 登出 → Redis 存 `ssid` 失效会话

## 短信服务

`internal/service/sms/` 装饰器链：
```
Memory/Tencent → FailoverSms → TimeoutFailoverSms → RateLimitSms → AuthSms
```
活跃实现在 `ioc/sms.go` 中选择。

## 依赖注入

Wire 编译时 DI：
- 主入口: `wire.go` → `wire_gen.go`
- 集成测试: `internal/integration/setup/wire.go`
- 接口变更后需重新 `wire ./...`

## 测试

- 单元测试：`testing` + `testify/assert` + `go.uber.org/mock`（gomock 表驱动）
- 集成测试：`internal/integration/`，`testify/suite`，真实 MySQL + Redis
- Mock 包命名：`*mocks/`，变更后 `make -f win.mk mockgen`

## 命名规范

### 方法命名
- 查询：`FindXx` / `FindByXx`（单条）、`PageXxs`（分页）、`ListXxs`（列表）、`CountXx`（计数）
- 写入：`CreateXx` / `UpdateXx` / `DeleteXx` / `Upsert`
- 业务：`Publish` / `Withdraw`（按业务动作命名，不用泛化的 `Process` / `Handle`）
- 不用模糊命名（`FetchList` / `GetData`），按业务简洁表达

### 类型和文件命名
- receiver：类型首字母小写（`func (s *Service) Run()`）
- 文件按**业务角色**命名，不按技术实现：`article_author.go`（不是 `article_dao.go`）
- 接口按 `[实体][业务角色][层]` 命名：`ArticleAuthorDAO`、`ArticleAuthorRepository`
- 实现按 `[技术限定][实体][业务角色][层]` 命名：`GormArticleAuthorDAO`、`CacheArticleAuthorRepository`
- 业务角色是语义概念（author/reader），技术限定是实现细节（Gorm/Cache/Redis）

### 命名体系示例

```
接口（业务语义）          实现（技术限定）              文件
ArticleAuthorDAO       → GormArticleAuthorDAO       → dao/article_author.go
ArticleAuthorRepository → CacheArticleAuthorRepository → repository/article_author.go
ArticleReaderDAO       → GormArticleReaderDAO       → dao/article_reader.go（待实现）
```

| 维度 | 位置 | 示例 | 含义 |
|------|------|------|------|
| 实体 | 接口名第一段 | `Article`AuthorDAO | 操作的领域对象 |
| 业务角色 | 接口名第二段 | Article`Author`DAO | 区分制作库/线上库 |
| 层 | 接口名第三段 | ArticleAuthor`DAO` | 所在架构层 |
| 技术限定 | 实现名前缀 | `Gorm`ArticleAuthorDAO | 区分不同技术实现 |

## 编码约束

- 日志用 `zap`，禁止 `fmt.Println`
- 错误必须处理，禁止 `_ = err`
- 测试假数据用 `// ===== TODO: 测试假数据 START/END =====` 包裹
- 大数据量查询优先在数据库内完成过滤和聚合，避免全量加载到内存
