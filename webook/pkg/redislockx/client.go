package redislockx

import (
	"context"
	"errors"
	"time"

	"github.com/bsm/redislock"
	"github.com/redis/go-redis/v9"
)

// redisClient 基于 bsm/redislock 的分布式锁工厂。
//
// bsm/redislock 提供了 Redisson 的核心三件：
//   - SETNX + 随机 token 抢锁（Obtain）
//   - Lua 校验 token 续约（Refresh）/ 释放（Release）
//   - RetryStrategy 阻塞重试（LinearBackoff）
//
// 但 bsm 唯独没做 Watchdog 自动续约（Redisson 招牌特性），所以我们在 redisLock
// 上自己起 goroutine 周期 Refresh。这层胶水不引第三方依赖。
//
// 错误语义统一映射到本包的 ErrLockNotHeld，调用方不感知底层库。
type redisClient struct {
	inner *redislock.Client
}

// NewClient 用给定的 redis.Cmdable 构造分布式锁工厂。
func NewClient(cmd redis.Cmdable) Client {
	return &redisClient{inner: redislock.New(cmd)}
}

func (c *redisClient) TryLock(ctx context.Context, key string, ttl time.Duration, opts ...Options) (Lock, bool, error) {
	cfg := applyOptions(opts, ttl)
	bsmLock, err := c.inner.Obtain(ctx, key, ttl, &redislock.Options{
		RetryStrategy: redislock.NoRetry(),
	})
	if errors.Is(err, redislock.ErrNotObtained) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return newRedisLock(bsmLock, ttl, cfg), true, nil
}

func (c *redisClient) Lock(ctx context.Context, key string, ttl time.Duration, opts ...Options) (Lock, error) {
	cfg := applyOptions(opts, ttl)
	// LinearBackoff 永久重试，直到拿到锁或 ctx.Done；bsm 内部 select ctx.Done() 返 ctx.Err()
	bsmLock, err := c.inner.Obtain(ctx, key, ttl, &redislock.Options{
		RetryStrategy: redislock.LinearBackoff(cfg.retryInterval),
	})
	if err != nil {
		return nil, err
	}
	return newRedisLock(bsmLock, ttl, cfg), nil
}
