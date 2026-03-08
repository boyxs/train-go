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
	// 使用切片存储放行路径列表
	ignorePaths map[string]struct{}
}

func NewLoginJwtMiddlewareBuilder(hdl myJwt.JwtHandler) *LoginJwtMiddlewareBuilder {
	return &LoginJwtMiddlewareBuilder{
		ignorePaths: make(map[string]struct{}),
		JwtHandler:  hdl,
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
		tokenStr := l.ExtractToken(ctx)
		if tokenStr == "" {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		var uc web.UserClaims
		token, err := jwt.ParseWithClaims(tokenStr, &uc, func(token *jwt.Token) (any, error) {
			return consts.AccessKey, nil
		})
		if err != nil {
			ctx.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		if token == nil || !token.Valid {
			ctx.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		if uc.UserAgent != ctx.GetHeader(consts.UserAgent) {
			// 后期我们讲到了监控告警的时候，这个地方要埋点
			// 能够进来这个分支的，大概率是攻击者
			ctx.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		if err = l.CheckSession(ctx, uc.Ssid); err != nil {
			ctx.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		//expiresAt := uc.ExpiresAt
		// 每10秒刷新一次
		//if expiresAt.Sub(time.Now()) < consts.RefreshThreshold {
		//	uc.ExpiresAt = jwt.NewNumericDate(time.Now().Add(consts.ExpireTime))
		//	tokenStr, err := token.SignedString(consts.AccessKey)
		//	if err != nil {
		//		// 这里续约失败，仅需要记录日志
		//		zap.L().Error("续约 access token 失败", zap.Error(err))
		//	}
		//	ctx.Header(consts.AccessHeader, tokenStr)
		//}
		ctx.Set(consts.UserKey, uc)
	}
}
