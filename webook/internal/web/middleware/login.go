package middleware

import (
	"encoding/gob"
	"net/http"
	"time"

	"gitee.com/train-cloud/geektime-basic-go/internal/consts"
	"gitee.com/train-cloud/geektime-basic-go/pkg/logger"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

type LoginMiddlewareBuilder struct {
	// 使用切片存储放行路径列表
	ignorePaths map[string]struct{}
	l           logger.LoggerX
}

func NewLoginMiddlewareBuilder(l logger.LoggerX) *LoginMiddlewareBuilder {
	return &LoginMiddlewareBuilder{
		ignorePaths: make(map[string]struct{}),
		l:           l,
	}
}

func (l *LoginMiddlewareBuilder) IgnorePaths(paths ...string) *LoginMiddlewareBuilder {
	for _, path := range paths {
		l.ignorePaths[path] = struct{}{}
	}
	return l
}

func (l *LoginMiddlewareBuilder) Build() gin.HandlerFunc {
	// &errors.errorString{s:"gob: type not registered for interface: time.Time"}
	gob.Register(time.Now())
	return func(ctx *gin.Context) {
		// 需要放行的路径列表
		if _, ok := l.ignorePaths[ctx.Request.URL.Path]; ok {
			return
		}

		session := sessions.Default(ctx)
		userid := session.Get("userid")
		if userid == nil {
			ctx.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		// redis-cli monitor 监控指令执行
		now := time.Now()
		updateTimeKey := "updateTime"
		val := session.Get(updateTimeKey)
		lastUpdateTime, ok := val.(time.Time)
		if val == nil || !ok || now.Sub(lastUpdateTime) > consts.Interval {
			session.Set("userid", userid)
			session.Set(updateTimeKey, now)
			session.Options(sessions.Options{
				Path:     "/",
				MaxAge:   10 * 60,
				Secure:   true,
				HttpOnly: true,
			})
			err := session.Save()
			if err != nil {
				l.l.Error("session保存失败", logger.Error(err))
				return
			}
		}
	}
}
