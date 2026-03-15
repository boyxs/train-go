# DEVLOG

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

---

<!-- 新功能追加在此 -->
