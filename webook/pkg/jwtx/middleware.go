package jwtx

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	stdjwt "github.com/golang-jwt/jwt/v5"
)

// 验签失败原因码，写进 401 响应 reason 供前端区分处理。
const (
	ReasonAccessTokenExpired = "ACCESS_TOKEN_EXPIRED" // 过期，可刷新
	ReasonTokenInvalid       = "TOKEN_INVALID"        // 无效/登出，去登录
)

// MiddlewareBuilder 验签中间件构造器。
// 只验证 token，不签发；签发由各服务自己的 jwt handler 负责。
type MiddlewareBuilder struct {
	cfg           MiddlewareConfig
	ignoredPaths  map[string]struct{}
	optionalPaths map[string]struct{}
}

func NewMiddlewareBuilder(cfg MiddlewareConfig) *MiddlewareBuilder {
	return &MiddlewareBuilder{
		cfg:           cfg,
		ignoredPaths:  make(map[string]struct{}),
		optionalPaths: make(map[string]struct{}),
	}
}

// IgnoredPaths 完全放行（不验签、不写 UserClaims）
func (b *MiddlewareBuilder) IgnoredPaths(paths ...string) *MiddlewareBuilder {
	for _, p := range paths {
		b.ignoredPaths[p] = struct{}{}
	}
	return b
}

// OptionalPaths 验签可选：成功 → 写 UserClaims；失败 → 仍放行（不抛 401）
func (b *MiddlewareBuilder) OptionalPaths(paths ...string) *MiddlewareBuilder {
	for _, p := range paths {
		b.optionalPaths[p] = struct{}{}
	}
	return b
}

func (b *MiddlewareBuilder) Build() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		path := ctx.Request.URL.Path
		if _, ok := b.ignoredPaths[path]; ok {
			return
		}
		uc, reason := b.parseWithReason(ctx)
		if _, optional := b.optionalPaths[path]; optional {
			if reason == "" {
				ctx.Set(b.cfg.UserKey, uc)
			}
			return
		}
		if reason != "" {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":   http.StatusUnauthorized,
				"reason": reason,
				"msg":    "登录已过期或无效，请重新登录",
				"data":   nil,
			})
			return
		}
		ctx.Set(b.cfg.UserKey, uc)
	}
}

// Parse 验签，成功返回 (claims, true)。
func (b *MiddlewareBuilder) Parse(ctx *gin.Context) (UserClaims, bool) {
	uc, reason := b.parseWithReason(ctx)
	return uc, reason == ""
}

// parseWithReason 验签，返回失败原因码（"" = 成功）。
func (b *MiddlewareBuilder) parseWithReason(ctx *gin.Context) (UserClaims, string) {
	tokenStr := ExtractBearer(ctx)
	if tokenStr == "" {
		return UserClaims{}, ReasonTokenInvalid
	}
	var uc UserClaims
	token, err := stdjwt.ParseWithClaims(tokenStr, &uc, func(_ *stdjwt.Token) (any, error) {
		return b.cfg.AccessKey, nil
	})
	if err != nil {
		// 过期可刷新，与其他失败区分
		if errors.Is(err, stdjwt.ErrTokenExpired) {
			return UserClaims{}, ReasonAccessTokenExpired
		}
		return UserClaims{}, ReasonTokenInvalid
	}
	if token == nil || !token.Valid {
		return UserClaims{}, ReasonTokenInvalid
	}
	if uc.UserAgent != ctx.GetHeader(HeaderUserAgent) {
		return UserClaims{}, ReasonTokenInvalid
	}
	if b.cfg.Cmd != nil && ssidLoggedOut(ctx, b.cfg.Cmd, b.cfg.SsidKeyPattern, uc.Ssid) {
		return UserClaims{}, ReasonTokenInvalid
	}
	return uc, ""
}

// ExtractBearer 从 Authorization: Bearer <token> 头解析 access token。
// 前端 axios `setAuthorization(bearer(...))` 用此格式发送。
func ExtractBearer(ctx *gin.Context) string {
	authorization := ctx.GetHeader(HeaderAuthorization)
	if authorization == "" {
		return ""
	}
	prefix, token, found := strings.Cut(authorization, " ")
	if !found || !strings.EqualFold(prefix, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}
