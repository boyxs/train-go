package middleware

import (
	"encoding/gob"
	"net/http"
	"time"

	"gitee.com/train-cloud/geektime-basic-go/internal/consts"
	"gitee.com/train-cloud/geektime-basic-go/internal/web"
	myJwt "gitee.com/train-cloud/geektime-basic-go/internal/web/jwt"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type LoginJwtMiddlewareBuilder struct {
	myJwt.JwtHandler
	ignoredPaths  map[string]struct{}
	optionalPaths map[string]struct{}
}

func NewLoginJwtMiddlewareBuilder(hdl myJwt.JwtHandler) *LoginJwtMiddlewareBuilder {
	return &LoginJwtMiddlewareBuilder{
		JwtHandler:    hdl,
		ignoredPaths:  make(map[string]struct{}),
		optionalPaths: make(map[string]struct{}),
	}
}

func (l *LoginJwtMiddlewareBuilder) IgnoredPaths(paths ...string) *LoginJwtMiddlewareBuilder {
	for _, path := range paths {
		l.ignoredPaths[path] = struct{}{}
	}
	return l
}

func (l *LoginJwtMiddlewareBuilder) OptionalPaths(paths ...string) *LoginJwtMiddlewareBuilder {
	for _, path := range paths {
		l.optionalPaths[path] = struct{}{}
	}
	return l
}

func (l *LoginJwtMiddlewareBuilder) Build() gin.HandlerFunc {
	gob.Register(time.Now())
	return func(ctx *gin.Context) {
		path := ctx.Request.URL.Path
		// 1. 公开路径——完全放行
		if _, ok := l.ignoredPaths[path]; ok {
			return
		}
		// 2. 尝试解析 token
		uc, ok := l.tryParseToken(ctx)
		// 3. 可选认证——成功设置 UserKey，失败放行
		if _, optional := l.optionalPaths[path]; optional {
			if ok {
				ctx.Set(consts.UserKey, uc)
			}
			return
		}
		// 4. 必须认证——失败 401
		if !ok {
			ctx.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		ctx.Set(consts.UserKey, uc)
	}
}

func (l *LoginJwtMiddlewareBuilder) tryParseToken(ctx *gin.Context) (web.UserClaims, bool) {
	tokenStr := l.ExtractToken(ctx)
	if tokenStr == "" {
		return web.UserClaims{}, false
	}
	var uc web.UserClaims
	token, err := jwt.ParseWithClaims(tokenStr, &uc, func(token *jwt.Token) (any, error) {
		return consts.AccessKey, nil
	})
	if err != nil || token == nil || !token.Valid {
		return web.UserClaims{}, false
	}
	if uc.UserAgent != ctx.GetHeader(consts.UserAgent) {
		return web.UserClaims{}, false
	}
	if err = l.CheckSession(ctx, uc.Ssid); err != nil {
		return web.UserClaims{}, false
	}
	return uc, true
}
