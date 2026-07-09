package redislock

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// RedisClient 单机 / 集群共用的 Client 实现（redis.UniversalClient + hash-tag key）。
// 与多主 quorum 的 QuorumClient（quorum.go，P5）区分：这里是"单个 client"，无论它底层
// 是单机 *redis.Client 还是集群 *redis.ClusterClient。
type RedisClient struct {
	cmd redis.UniversalClient
}

// newToken 每次获取的 ownerToken：Go 无稳定 goroutine id（ADR-2），用随机 token 标识
// 持有者，Unlock / Refresh 校验所有权防误删他人锁。
func newToken() string { return uuid.NewString() }

// resolveToken 本次获取的 ownerToken：WithReentrant 的显式 ownerId 优先（同一 ownerId
// 可重入、跨 goroutine 共享持有者身份），否则随机 token（每次获取身份独立、天然不可重入）。
func resolveToken(cfg *lockConfig) string {
	if cfg.ownerId != "" {
		return cfg.ownerId
	}
	return newToken()
}

func (c *RedisClient) TryLock(ctx context.Context, key string, opts ...Options) (RedisLock, bool, error) {
	cfg := applyOptions(opts)
	if err := cfg.validate(); err != nil {
		return nil, false, err
	}
	token := resolveToken(cfg)
	// 无 WaitTime：非阻塞单次尝试，被占立即软失败。
	if cfg.waitTime <= 0 {
		lock, _, err := c.tryAcquire(ctx, key, token, cfg)
		if err != nil {
			return nil, false, err
		}
		// 公平锁非阻塞未拿到：脚本已把我入队，但我并不打算等 → 立即退出队列，不占位堵后面的人。
		if lock == nil && cfg.fair {
			c.dequeueFair(key, token)
		}
		return lock, lock != nil, nil
	}
	// 有 WaitTime：阻塞至 deadline，走 pub/sub 唤醒 + 兜底轮询（§3.4）。
	return c.blockingAcquire(ctx, key, token, cfg, time.Now().Add(cfg.waitTime))
}

func (c *RedisClient) Lock(ctx context.Context, key string, opts ...Options) (RedisLock, error) {
	cfg := applyOptions(opts)
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	token := resolveToken(cfg)
	// ctx 即等待上限（deadline 零值 = 无 WaitTime 上限）；pub/sub 唤醒 + 兜底轮询。
	lock, _, err := c.blockingAcquire(ctx, key, token, cfg, time.Time{})
	return lock, err
}

// tryAcquire 单次获取尝试。成功 (lock, 0, nil)；被占 (nil, pttlMs, nil)（pttlMs=剩余租约，
// 供阻塞路径算兜底等待）；网络错误 (nil, 0, err)。公平 / fencing 各走独立变体（fair.go / fence.go）。
func (c *RedisClient) tryAcquire(ctx context.Context, key, token string, cfg *lockConfig) (RedisLock, int64, error) {
	switch {
	case cfg.fair:
		return c.acquireFair(ctx, key, token, cfg)
	case cfg.fencing:
		return c.acquireFencing(ctx, key, token, cfg)
	}
	res, err := acquireScript.Run(ctx, c.cmd, []string{lockKey(key)}, cfg.leaseMs(), token).Int64()
	if err != nil {
		return nil, 0, err
	}
	if res != -1 {
		return nil, res, nil // 被占，res 为剩余 pttl
	}
	return newLock(c, key, token, 0, cfg), 0, nil
}
