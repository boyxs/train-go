package jwtx

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	stdjwt "github.com/golang-jwt/jwt/v5"
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
		uc, ok := b.Parse(ctx)
		if _, optional := b.optionalPaths[path]; optional {
			if ok {
				ctx.Set(b.cfg.UserKey, uc)
			}
			return
		}
		if !ok {
			ctx.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		ctx.Set(b.cfg.UserKey, uc)
	}
}

func (b *MiddlewareBuilder) Parse(ctx *gin.Context) (UserClaims, bool) {
	tokenStr := ExtractBearer(ctx)
	if tokenStr == "" {
		return UserClaims{}, false
	}
	var uc UserClaims
	token, err := stdjwt.ParseWithClaims(tokenStr, &uc, func(_ *stdjwt.Token) (any, error) {
		return b.cfg.AccessKey, nil
	})
	if err != nil || token == nil || !token.Valid {
		return UserClaims{}, false
	}
	if uc.UserAgent != ctx.GetHeader(HeaderUserAgent) {
		return UserClaims{}, false
	}
	if b.cfg.Cmd != nil && ssidLoggedOut(ctx, b.cfg.Cmd, b.cfg.SsidKeyPattern, uc.Ssid) {
		return UserClaims{}, false
	}
	return uc, true
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
