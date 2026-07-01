package ratelimit

import (
	_ "embed"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/webook/pkg/ginx"
	"github.com/webook/pkg/logger"
)

type Builder struct {
	prefix   string
	cmd      redis.Cmdable
	l        logger.LoggerX
	interval time.Duration
	// 阈值
	rate int
}

//go:embed sliding_window.lua
var luaScript string

func NewBuilder(cmd redis.Cmdable, interval time.Duration, rate int, l logger.LoggerX) *Builder {
	return &Builder{
		cmd:      cmd,
		prefix:   "ip-limiter",
		interval: interval,
		rate:     rate,
		l:        l,
	}
}

func (b *Builder) Prefix(prefix string) *Builder {
	b.prefix = prefix
	return b
}

func (b *Builder) Build() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		limited, err := b.limit(ctx)
		if err != nil {
			b.l.Error("限流器异常", logger.Error(err))
			// 保守做法：借助 Redis 做限流，Redis 崩溃时为防系统被打垮，直接限流
			ctx.AbortWithStatusJSON(http.StatusInternalServerError, ginx.Internal("服务繁忙，请稍后重试"))
			// 激进做法：虽然 Redis 崩溃了，但是这个时候还是要尽量服务正常的用户，所以不限流
			// ctx.Next()
			return
		}
		if limited {
			b.l.Warn("触发限流", logger.String("ip", ctx.ClientIP()))
			ctx.AbortWithStatusJSON(http.StatusTooManyRequests, ginx.TooManyRequests("请求过于频繁，请稍后重试"))
			return
		}
		ctx.Next()
	}
}

func (b *Builder) limit(ctx *gin.Context) (bool, error) {
	key := fmt.Sprintf("%s:%s", b.prefix, ctx.ClientIP())
	return b.cmd.Eval(
		ctx,
		luaScript,
		[]string{key},
		b.interval.Milliseconds(),
		b.rate,
		time.Now().UnixMilli(),
	).Bool()
}
