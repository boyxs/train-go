package redislock

import (
	"context"
	"errors"
	"time"
)

// ErrFairFencingUnsupported 公平锁与 fencing 暂不支持组合（需专门的 fair+fence 原子脚本，留后续）。
// fail-loud：静默丢掉 fencing 会让调用方误以为有真安全，故在获取入口直接报错。
var ErrFairFencingUnsupported = errors.New("redislock: WithFair 与 WithFencing 暂不支持组合")

// acquireFair 公平锁单次获取（§3.5）：跑 fair_acquire.lua——清理死等待者 + 重入 + FIFO 获取 +
// 入队/刷新 deadline，全程原子。成功 (lock, 0, nil)；未获取 (nil, pttlMs, nil)；网络错误 (nil, 0, err)。
// 与非公平共用 blockingAcquire 的订阅唤醒循环；只有队头能在脚本 step 3 成功，从而实现 FIFO。
func (c *RedisClient) acquireFair(ctx context.Context, key, token string, cfg *lockConfig) (RedisLock, int64, error) {
	nowMs := time.Now().UnixMilli()
	res, err := fairAcquireScript.Run(ctx, c.cmd,
		[]string{lockKey(key), queueKey(key), qtsKey(key)},
		cfg.leaseMs(), token, nowMs, cfg.heartbeatMs()).Int64()
	if err != nil {
		return nil, 0, err
	}
	if res != -1 {
		return nil, res, nil // 未获取，res 为锁剩余 pttl
	}
	return newLock(c, key, token, 0, cfg), 0, nil
}

// dequeueFair 放弃排队时把自己从 queue + qts 移除（优雅退出，不占位堵后面的人）。
// 用独立 ctx：调用方多在业务 ctx 已取消 / 超时的收尾路径调它。
func (c *RedisClient) dequeueFair(key, token string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	// 优雅退出队列；失败无害——qts deadline 会兜底逐出（§3.5），故不向上传播。
	_ = fairCancelScript.Run(ctx, c.cmd, []string{queueKey(key), qtsKey(key)}, token).Err()
}
