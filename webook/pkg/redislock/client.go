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

func (c *RedisClient) TryLock(ctx context.Context, key string, opts ...Options) (RedisLock, bool, error) {
	cfg := applyOptions(opts)
	token := newToken()

	var deadline time.Time
	if cfg.waitTime > 0 {
		deadline = time.Now().Add(cfg.waitTime)
	}
	for {
		lock, ok, err := c.acquire(ctx, key, token, cfg)
		if err != nil {
			return nil, false, err
		}
		if ok {
			return lock, true, nil
		}
		// 被占：无 WaitTime 立即软失败；有 WaitTime 但已超时也软失败
		if cfg.waitTime <= 0 || !time.Now().Before(deadline) {
			return nil, false, nil
		}
		// 阻塞路径 P1 用轮询兜底（pub/sub 阻塞是 P4）
		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		case <-time.After(cfg.retryInterval):
		}
	}
}

func (c *RedisClient) Lock(ctx context.Context, key string, opts ...Options) (RedisLock, error) {
	cfg := applyOptions(opts)
	token := newToken()
	for {
		lock, ok, err := c.acquire(ctx, key, token, cfg)
		if err != nil {
			return nil, err
		}
		if ok {
			return lock, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(cfg.retryInterval):
		}
	}
}

// acquire 单次获取尝试。拿到 (lock, true, nil)；被占 (nil, false, nil)；网络错误 (nil, false, err)。
// fencing 开启时走 fencing 变体（见 fence.go）。
func (c *RedisClient) acquire(ctx context.Context, key, token string, cfg *lockConfig) (RedisLock, bool, error) {
	if cfg.fencing {
		return c.acquireFencing(ctx, key, token, cfg)
	}
	res, err := acquireScript.Run(ctx, c.cmd, []string{lockKey(key)}, cfg.leaseMs(), token).Int64()
	if err != nil {
		return nil, false, err
	}
	if res != -1 {
		// 被占，res 为剩余 pttl
		return nil, false, nil
	}
	return newLock(c, key, token, 0, cfg), true, nil
}
