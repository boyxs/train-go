# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Module

```
gitee.com/train-cloud/geektime-basic-go
```

## Key Dependencies

| Package | Purpose |
|---|---|
| `github.com/gin-gonic/gin` | HTTP framework |
| `gorm.io/gorm` + `gorm.io/driver/mysql` | ORM + MySQL driver |
| `github.com/redis/go-redis/v9` | Redis client |
| `github.com/golang-jwt/jwt/v5` | JWT tokens |
| `github.com/google/wire` | Dependency injection |
| `go.uber.org/mock/mockgen` | Mock generation |
| `github.com/tencentcloud/tencentcloud-sdk-go` | Tencent Cloud SMS |
| `github.com/dlclark/regexp2` | Regex (supports lookahead `(?=`) |
| `golang.org/x/crypto` | Password hashing (bcrypt) |

## Commands

### Build
```bash
# Local development (Windows)
make -f win.mk build

# Docker + K8s deployment
make docker

# Clean K8s deployment
make clean
```

### Test
```bash
# Run all tests
go test ./...

# Run a single test file
go test ./internal/service/... -run TestXxx

# Run integration tests (requires MySQL + Redis running)
go test ./internal/integration/...
```

### Generate mocks
```bash
make -f win.mk mockgen
```

### Generate wire dependency injection
```bash
wire ./...
# or for integration tests
wire ./internal/integration/setup/...
```

### Infrastructure (K8s)
```bash
make -f infra.mk all      # Deploy MySQL + Redis
make -f infra.mk mysql    # Deploy MySQL only
make -f infra.mk redis    # Deploy Redis only
make -f infra.mk status   # Check pod status
make -f infra.mk clean    # Tear down infra
```

## Architecture

This is a Go web backend (Gin + GORM) following a strict layered architecture:

```
Handler (web) → Service → Repository → DAO/Cache
```

### Layers

- **`internal/web/`** — HTTP handlers. Each handler embeds `jwt.JwtHandler` for token ops and calls service methods. Handlers register routes via `RegisterRoutes(*gin.Engine)`.
- **`internal/service/`** — Business logic. Contains `UserService`, `CodeService`, and `oauth2/` for WeChat OAuth2.
- **`internal/repository/`** — Data access abstraction. Coordinates DAO (MySQL) + Cache (Redis) reads/writes.
- **`internal/repository/dao/`** — GORM models and raw DB queries.
- **`internal/repository/cache/`** — Redis cache implementations.
- **`ioc/`** — Wire providers for all infrastructure and service wiring (DB, Redis, SMS, middlewares, web server).
- **`config/`** — Build-tag-based config: `dev.go` (`!k8s`) and `k8s.go` (`k8s`).
- **`pkg/`** — Shared utilities: rate limiter (`pkg/ratelimit/`) and Gin middleware builder (`pkg/ginx/middleware/ratelimit/`).
- **`internal/consts/`** — Shared constants (JWT keys, headers, cookie names).

### Dependency Injection

Uses [google/wire](https://github.com/google/wire). The wiring entry point is `wire.go` (build tag `wireinject`); the generated file is `wire_gen.go`. Integration tests have their own wire setup under `internal/integration/setup/`.

### Authentication

Dual-token JWT strategy via `internal/web/jwt/`:
- **Access token** (30min, sent in `x-access-token` header)
- **Refresh token** (7 days, sent in `x-refresh-token` header)
- Logout invalidates the session by storing the `ssid` key in Redis

### SMS Service

Located in `internal/service/sms/`. Supports:
- **Tencent Cloud SMS** (`sms/tencent/`)
- **In-memory** (`sms/memory/`) for local testing
- **Failover** (`sms/failover/`) — round-robin across providers with atomic counter
- **Timeout-based failover** (`sms/failover/timeout_failover.go`) — switches provider after consecutive timeouts
- **Rate-limited decorator** (`sms/ratelimit/`) — wraps any provider with Redis sliding-window limiter
- **Auth decorator** (`sms/auth/`) — JWT-based permission control for SMS sending

The active SMS implementation is selected in `ioc/sms.go`.

### Build Tags

- `dev` (default) — local config, Windows binary
- `k8s` — K8s config with cluster-internal service addresses

### Mocks

All interfaces have generated mocks using `go.uber.org/mock/mockgen`. Mock packages follow the pattern `*mocks/` alongside the source. Run `make -f win.mk mockgen` to regenerate after interface changes.

## Project File Structure

```
webook/
├── main.go                          # Entry point
├── wire.go                          # Wire providers (build tag: wireinject)
├── wire_gen.go                      # Wire generated file
├── config/
│   ├── types.go                     # Config struct
│   ├── dev.go                       # Local dev config (!k8s)
│   └── k8s.go                       # K8s config (k8s)
├── internal/
│   ├── consts/
│   │   ├── user.go                  # JWT keys, cookie/header names
│   │   └── wechat.go                # WeChat OAuth2 constants
│   ├── domain/
│   │   ├── user.go                  # User domain model
│   │   └── wechat.go                # WechatAuth domain model
│   ├── web/
│   │   ├── user.go                  # UserHandler (register/login/profile/SMS login)
│   │   ├── wechat.go                # WechatHandler (OAuth2 callback)
│   │   ├── result.go                # Unified response struct
│   │   ├── jwt/
│   │   │   ├── types.go             # JwtHandler interface + UserClaims
│   │   │   ├── handler.go           # JWT impl (set/extract/clear tokens)
│   │   │   └── mocks/jwt_mock.go    # JwtHandler mock
│   │   └── middleware/
│   │       ├── login.go             # Session-based auth middleware
│   │       └── login_jwt.go         # JWT-based auth middleware
│   ├── service/
│   │   ├── user.go                  # UserService
│   │   ├── code.go                  # CodeService (SMS verification)
│   │   ├── oauth2/
│   │   │   ├── types.go             # OAuth2Service interface
│   │   │   └── wechat/service.go    # WeChat OAuth2 implementation
│   │   ├── sms/
│   │   │   ├── types.go             # SMSService interface
│   │   │   ├── memory/service.go    # In-memory (dev/test)
│   │   │   ├── tencent/service.go   # Tencent Cloud SMS
│   │   │   ├── failover/
│   │   │   │   ├── failover.go      # Round-robin failover
│   │   │   │   └── timeout_failover.go # Consecutive-timeout failover
│   │   │   ├── ratelimit/limiter.go # Rate-limit decorator
│   │   │   └── auth/service.go      # JWT auth decorator
│   │   └── mocks/                   # Service mocks
│   ├── repository/
│   │   ├── user.go                  # UserRepository
│   │   ├── code.go                  # CodeRepository
│   │   ├── dao/
│   │   │   ├── init.go              # GORM AutoMigrate
│   │   │   └── user.go              # GormUserDAO (User GORM model)
│   │   └── cache/
│   │       ├── user.go              # UserCache (Redis)
│   │       └── code.go              # CodeCache (Redis, TTL-based)
│   └── integration/
│       ├── user_test.go             # User integration tests
│       ├── wechat_test.go           # WeChat integration tests
│       └── setup/
│           ├── db.go                # Test DB init
│           ├── redis.go             # Test Redis init
│           ├── wire.go              # Integration test wire (wireinject)
│           └── wire_gen.go          # Integration test wire generated
├── ioc/
│   ├── db.go                        # GORM DB provider
│   ├── redis.go                     # Redis client provider
│   ├── sms.go                       # SMS service selection
│   ├── web.go                       # Gin engine + middleware setup
│   └── wechat.go                    # WeChat OAuth2 service provider
└── pkg/
    ├── ratelimit/
    │   ├── types.go                 # Limiter interface
    │   └── redis_sliding_window.go  # Redis sliding window impl
    └── ginx/middleware/ratelimit/
        └── builder.go               # Gin rate-limit middleware builder
```

## Key Interfaces

| Interface | Location | Implementations |
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

## Domain Models

### User (`internal/domain/user.go`)
```go
type User struct {
    Id, Email, Password, Nickname, Birthday, AboutMe, Phone string/int64
    WechatAuth WechatAuth
    CreatedAt, UpdatedAt string
}
```

### WechatAuth (`internal/domain/wechat.go`)
OpenId + UnionId from WeChat OAuth2.

## 工作规则

- 完成功能后自动追加到 `DEVLOG.md`，同一天归类到同一个日期标题下
- `CLAUDE.md` 只放规则和约定，不放功能记录
- 用中文沟通，代码注释可中可英但需要简洁
- 发现问题、踩过的坑、更好的做法，主动记录到 `memory/` 目录（feedback 类型），并同步更新 `MEMORY.md` 索引
- 每次对话结束前，检查 `CLAUDE.md` 和 `MEMORY.md` 是否需要更新
