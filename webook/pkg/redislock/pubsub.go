package redislock

import (
	"context"
	"time"
)

// blockingAcquire 阻塞获取（§3.4）：先订阅释放通道（订阅先于试获取，堵住"释放信号在订阅前
// 发出"的丢失窗口）→ 循环试获取；被占则 select{释放消息唤醒 / min(pttl,retryInterval) 兜底
// 轮询 / ctx.Done}。所有退出路径经 defer 退订，防连接 / goroutine 泄漏。
//
// deadline 非零 = TryLock 的 WaitTime 上限，到点软失败返 (nil,false,nil)；
// deadline 零 = Lock，仅 ctx 作上限，ctx.Done 返 (nil,false,ctx.Err())。
//
// 集群后续增强：7.0+ 可用 sharded SSUBSCRIBE（channel 与锁同 hash-tag、同 slot）省广播开销；
// 当前用广播 SUBSCRIBE，单机 / 集群均正确（集群下退化为广播），留待有集群消费者时优化。
func (c *RedisClient) blockingAcquire(ctx context.Context, key, token string, cfg *lockConfig, deadline time.Time) (RedisLock, bool, error) {
	pubsub := c.cmd.Subscribe(ctx, channelKey(key))
	defer func() { _ = pubsub.Close() }()
	ch := pubsub.Channel()

	for {
		lock, pttlMs, err := c.tryAcquire(ctx, key, token, cfg)
		if err != nil {
			return nil, false, err
		}
		if lock != nil {
			return lock, true, nil
		}

		wait := blockWait(pttlMs, cfg.retryInterval, deadline)
		if wait <= 0 {
			// WaitTime 到点仍被占，软失败。公平锁：优雅退出队列，不占位堵后面的人。
			if cfg.fair {
				c.dequeueFair(key, token)
			}
			return nil, false, nil
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			if cfg.fair {
				c.dequeueFair(key, token)
			}
			return nil, false, ctx.Err()
		case <-ch:
			timer.Stop() // 收到释放通知，立即重试
		case <-timer.C: // 兜底轮询到点，重试
		}
	}
}

// blockWait 本轮兜底等待 = min(pttl, retryInterval)，且不超过 deadline 剩余。
// pttl 兜底保证持有者崩溃不 publish 时，也能在锁自然过期附近及时重试。
// 返回 <=0 表示 deadline 已到（调用方据此软失败）；deadline 零表示无 WaitTime 上限。
func blockWait(pttlMs int64, retryInterval time.Duration, deadline time.Time) time.Duration {
	wait := retryInterval
	if pttlMs > 0 {
		if p := time.Duration(pttlMs) * time.Millisecond; p < wait {
			wait = p
		}
	}
	if !deadline.IsZero() {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return 0
		}
		if remaining < wait {
			wait = remaining
		}
	}
	return wait
}
