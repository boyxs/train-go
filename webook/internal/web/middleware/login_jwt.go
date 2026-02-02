package middleware

import (
	"encoding/gob"
	"fmt"
	"gitee.com/train-cloud/geektime-basic-go/internal/web"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"net/http"
	"strings"
	"time"
)

type LoginJwtMiddlewareBuilder struct {
	// 使用切片存储放行路径列表
	ignorePaths map[string]struct{}
}

func NewLoginJwtMiddlewareBuilder() *LoginJwtMiddlewareBuilder {
	return &LoginJwtMiddlewareBuilder{
		ignorePaths: make(map[string]struct{}),
	}
}

func (l *LoginJwtMiddlewareBuilder) IgnorePaths(paths ...string) *LoginJwtMiddlewareBuilder {
	for _, path := range paths {
		l.ignorePaths[path] = struct{}{}
	}
	return l
}

func (l *LoginJwtMiddlewareBuilder) Build() gin.HandlerFunc {
	// &errors.errorString{s:"gob: type not registered for interface: time.Time"}
	gob.Register(time.Now())
	return func(ctx *gin.Context) {
		// 需要放行的路径列表
		if _, ok := l.ignorePaths[ctx.Request.URL.Path]; ok {
			return
		}
		authorization := ctx.GetHeader(web.Authorization)
		if authorization == "" {
			ctx.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		splits := strings.Split(authorization, " ")
		if len(splits) != 2 || strings.ToLower(splits[0]) != "bearer" {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid authorization header format",
			})
			return
		}
		tokenStr := splits[1]
		var uc web.UserClaims
		token, err := jwt.ParseWithClaims(tokenStr, &uc, func(token *jwt.Token) (any, error) {
			return web.JwtKey, nil
		})
		if err != nil {
			ctx.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		if token == nil || !token.Valid {
			ctx.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		if uc.UserAgent != ctx.GetHeader(web.UserAgent) {
			// 后期我们讲到了监控告警的时候，这个地方要埋点
			// 能够进来这个分支的，大概率是攻击者
			ctx.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		expiresAt := uc.ExpiresAt
		// 每10秒刷新一次
		if expiresAt.Sub(time.Now()) < web.RefreshTime {
			uc.ExpiresAt = jwt.NewNumericDate(time.Now().Add(web.ExpireTime))
			tokenStr, err := token.SignedString(web.JwtKey)
			if err != nil {
				// 这里续约失败，仅需要记录日志
				fmt.Printf("🚀 ~ file: login_jwt.go ~ line 79 ~ err: %#v\n", err)
			}
			ctx.Header(web.JwtHeader, tokenStr)
		}
		ctx.Set(web.UserKey, uc)
	}
}
