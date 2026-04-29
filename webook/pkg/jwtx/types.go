package jwtx

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	stdjwt "github.com/golang-jwt/jwt/v5"
)

// UserClaims 标准用户身份载荷（access token payload）。
// 所有服务使用同一份定义，避免跨服务 claims 字段漂移。
type UserClaims struct {
	stdjwt.RegisteredClaims
	Userid    int64
	Ssid      string
	UserAgent string
}

// RefreshClaims refresh token payload
type RefreshClaims struct {
	stdjwt.RegisteredClaims
	Userid int64
	Ssid   string
}

// SessionChecker 校验 ssid 是否被登出黑名单。
// 返回 true = 已登出（拒绝放行），false = 有效。
type SessionChecker func(ctx *gin.Context, ssid string) bool

// MiddlewareConfig 验签中间件配置。
type MiddlewareConfig struct {
	AccessKey []byte
	UserKey   string
	Session   SessionChecker
}

// Handler JWT 处理器接口（签发 + 提取 + 校验 + 登出）。
// webook-core 登录流程用全套；chat 等下游服务只需验签，走 MiddlewareBuilder 不需此接口。
type Handler interface {
	SetLoginToken(ctx *gin.Context, userid int64) error
	SetAccessToken(ctx *gin.Context, userid int64, ssid string) error
	SetRefreshToken(ctx *gin.Context, userid int64, ssid string) error
	ExtractToken(ctx *gin.Context) string
	CheckSession(ctx *gin.Context, ssid string) error
	ClearToken(ctx *gin.Context) error
}

// HandlerConfig RedisHandler 配置（密钥 + 头名 + ssid pattern + TTL）。
// 通过参数注入，pkg/jwtx 不依赖任何业务 consts。
type HandlerConfig struct {
	AccessKey      []byte
	RefreshKey     []byte
	AccessHeader   string        // e.g. "x-access-token"
	RefreshHeader  string        // e.g. "x-refresh-token"
	AccessExpire   time.Duration // e.g. 30min
	RefreshExpire  time.Duration // e.g. 7d
	SsidKeyPattern string        // 登出黑名单 redis key，含 %s，e.g. "user:ssid:%s"
	UserKey        string        // gin.Context 存取 UserClaims 的 key
}

// ErrTokenInvalid CheckSession 检测到 ssid 在登出黑名单时返回
var ErrTokenInvalid = errors.New("jwtx: token invalid")

// 协议头名（跨服务签发/验签共用，避免散落字面量）
const (
	HeaderAuthorization = "Authorization"
	HeaderUserAgent     = "User-Agent"
)
