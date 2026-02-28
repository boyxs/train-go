package ratelimit

import (
	"context"
	_ "embed"
	"time"

	"github.com/redis/go-redis/v9"
)

//go:embed sliding_window.lua
var luaScript string

//redis-cli > monitor 监控指令

type RedisSlidingWindowLimiter struct {
	cmd      redis.Cmdable
	interval time.Duration
	rate     int
}

func NewRedisSlidingWindowLimiter(cmd redis.Cmdable, interval time.Duration, rate int) Limiter {
	return &RedisSlidingWindowLimiter{
		cmd:      cmd,
		interval: interval,
		rate:     rate,
	}
}

func (r *RedisSlidingWindowLimiter) Limit(ctx context.Context, key string) (bool, error) {
	return r.cmd.Eval(
		ctx,
		luaScript,
		[]string{key},
		r.interval.Milliseconds(),
		r.rate,
		time.Now().UnixMilli(),
	).Bool()
}
