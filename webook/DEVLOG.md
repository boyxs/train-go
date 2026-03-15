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

---

<!-- 新功能追加在此 -->
