# DEVLOG

<!-- 新功能前插在此，日期降序 -->

## 2026-03-29

### 点赞/收藏功能（第2期互动模块）

**变更内容**: 新增文章互动功能，包含阅读量上报、点赞、收藏，前后端全栈实现
**影响范围**:
- 后端：domain/interaction.go（新），dao/interaction.go（新，`interaction`+`user_interaction` 两张表），repository/interaction.go（新），service/interaction.go（新），web/interaction.go（新），ioc/web.go，wire.go，wire_gen.go，init_table.go
- 前端：types/interaction.ts（新），api/interaction.ts（新），views/article/read.tsx（更新），types/index.ts（更新）

**新增接口**:
- `POST /interaction/read` — 阅读上报（公开，无需登录）
- `POST /interaction/get` — 获取聚合计数+用户状态（公开，登录后有 liked/collected 状态）
- `POST /interaction/like` — 点赞/取消点赞（需登录）
- `POST /interaction/collect` — 收藏/取消收藏（需登录）

**技术决策**:
- 聚合计数独立表 `interaction`（article_id unique），避免污染 published_article
- 用户操作记录独立表 `user_interaction`（user_id+article_id 联合唯一索引）
- 计数用 `GREATEST(0, count + delta)` 防止下界越零
- 前端乐观更新：点赞/收藏立即更新本地状态，失败时显示错误提示
- 阅读上报在文章内容加载完成后触发，失败静默处理不影响阅读体验
- `/interaction/read` 和 `/interaction/get` 加入 JWT 白名单（允许匿名访问）

**会话**: 260329-like-collect

---

## 2026-03-26

### 文章发布/撤回功能

**变更内容**: 实现文章发布（制作库+线上库双表事务）和撤回（幂等，只对已发布文章改状态）功能
**影响范围**: domain/article.go, dao/article_author.go, dao/article_reader.go, repository/article_author.go, service/article.go, web/article.go, wire.go, integration/article_test.go
**技术决策**:
- 制作库/线上库用 author/reader 命名，发布事务合并到 ArticleAuthorDAO（作者的操作）
- 撤回采用幂等设计：已发布→Private，草稿/已撤回→不改状态，线上库 DELETE 幂等
- DAO 层不依赖 domain 包，状态常量只在 domain 定义一份，通过 Repository 传参
- ArticleReaderDAO 暂时只保留 PublishedArticle 模型，接口和实现待未来读取功能时添加
**待办**: Publish 接口缺少"发布不存在文章"测试用例
**会话**: 260326-article-publish

---

## 2026-03-25

### 学习笔记：第01-03周

生成专业学习笔记，覆盖 Go 基本语法、Gin/GORM 入门、Session/JWT 认证、Kubernetes 部署入门。按知识体系重新组织，补充底层原理、最佳实践、对比表格和面试要点。

**文件:** `notes/_sections/sec_01_03.md`（2400+ 行）

---

## 2026-03-08

### 微信 OAuth2 登录

微信扫码登录完整流程：AuthURL 生成授权链接 → 回调获取 code → 换取 access_token + openid → 查找/创建用户 → 签发双 Token

**文件:** `internal/web/wechat.go` / `internal/service/oauth2/wechat/service.go` / `internal/domain/wechat.go` / `ioc/wechat.go`

### JWT 双 Token 认证（长短 Token）

access token 30min + refresh token 7天，refresh 时 ssid 不变只重签 access token，logout 时 Redis 存 ssid 使会话失效

**文件:** `internal/web/jwt/handler.go` / `internal/web/jwt/types.go` / `internal/web/user.go`

### SMS 装饰器：权限控制 / 限流 / 故障转移

SMS 服务支持多种装饰器组合：JWT 权限校验(`sms/auth/`) / Redis 滑动窗口限流(`sms/ratelimit/`) / 轮询故障转移(`sms/failover/`) / 连续超时切换(`sms/failover/timeout_failover.go`)

### 集成测试框架

Wire 独立 setup + MySQL/Redis 容器化测试环境，覆盖用户注册/登录/微信登录等核心流程

**文件:** `internal/integration/` / `internal/integration/setup/`

## 2026-03-15

### Docker Compose 本地开发环境

从 K8s deployment 文件中提取 MySQL 8.0、Redis 7.0、etcd 3.5.17 三个基础设施服务，生成 `docker-compose.yaml`，方便本地 `docker compose up -d` 一键启动开发依赖

**文件:** `docker-compose.yaml`

### Logger 中间件

请求日志中间件，Builder 模式构建，记录 Path/Query/Method/ClientIP/UserAgent/ReqBody/ResBody/Status/Duration，ReqBody 和 ResBody 截断 2048 字节

**文件:** `internal/web/middleware/logger.go`

### Logger 配置模块

基于 viper 配置文件路径判断环境（dev → DevelopmentConfig，k8s → ProductionConfig），通过 `LoggerConfig` 中间结构体将 yaml 配置逐字段覆盖到 `zap.Config`，解决 `viper.UnmarshalKey` 无法反序列化 `AtomicLevel`/`EncoderConfig` 的问题

**文件:** `ioc/logger.go` / `config/dev.yaml` / `config/k8s.yaml`

### Docker Compose webook 服务

webook 服务加入 docker-compose，传递 `APP_ENV=config/k8s.yaml` 环境变量，Dockerfile 增加 COPY config 目录

**文件:** `docker-compose.yaml` / `Dockerfile`

### 全局日志替换

项目中所有 `log.Printf`/`log.Println`/`fmt.Printf` 调试打印替换为注入式 `logger.LoggerX`，通过构造函数注入而非全局变量，涉及 user handler、failover SMS、memory SMS、ratelimit builder、login middleware。补充 `logger.Int32`/`Uint64`/`Strings` 辅助函数

**文件:** `internal/web/user.go` / `internal/service/sms/failover/` / `internal/service/sms/memory/` / `pkg/ginx/middleware/ratelimit/builder.go` / `internal/web/middleware/login.go` / `pkg/logger/fields.go`

### 集成测试 viper 初始化

添加 `TestMain` 入口加载 `config/dev.yaml`，修复集成测试无法连接数据库的问题

**文件:** `internal/integration/init_test.go` / `internal/integration/setup/wire.go`

## 2026-03-17

### 时间类型改造：int64 → time.Time

DAO 层和 Domain 层的 `Birthday`/`CreatedAt`/`UpdatedAt` 从 `int64` 毫秒时间戳改为 `time.Time`，MySQL 存储从 `BIGINT` 改为 `DATETIME`，和 MongoDB BSON Date 行为一致

**策略:** 全链路 UTC 存储 + carbon 东八区查询

**配置:**
- DSN: `parseTime=true&loc=UTC` — MySQL 驱动按 UTC 解析 datetime
- `ioc/time.go`: `carbon.SetTimezone("Asia/Shanghai")` — carbon 全局东八区
- `main.go`: 启动时调用 `ioc.InitTimezone()`

**时区处理规则:**
- 存储: 前端传毫秒时间戳 → `time.UnixMilli(ts).UTC()` → 存入 MySQL datetime（UTC）
- 查询: `carbon.Now().StartOfDay().StdTime()` 自动按东八区算一天的开始/结束
- 展示: 前端 `dayjs()` 自动将 UTC 转本地时区显示

**踩坑:**
1. `time.Time` JSON 反序列化只认 RFC3339，前端传时间戳必须用 `int64` 接收
2. `dayjs(value, 'YYYY-MM-DD')` 带格式参数会丢时区信息导致日期少一天，应用 `dayjs(value)`
3. carbon 默认用 UTC，必须 `SetTimezone` 后 `StartOfDay()` 才是东八区零点

**文件:** `ioc/time.go` / `ioc/db.go` / `config/dev.yaml` / `config/prd.yaml` / `internal/repository/dao/user.go` / `internal/domain/user.go` / `internal/domain/article.go` / `internal/repository/user.go` / `internal/web/user.go`
