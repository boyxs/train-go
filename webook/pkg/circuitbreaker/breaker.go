package circuitbreaker

import (
	"sync/atomic"
	"time"
)

// Breaker 基于连续失败次数的熔断器
// 三态：Closed（正常）→ Open（熔断）→ HalfOpen（试探）
type Breaker struct {
	failCount int64 // 连续失败次数
	lastFail  int64 // 上次失败时间戳（UnixMilli）
	threshold int64 // 连续失败多少次触发熔断
	cooldown  int64 // 熔断冷却时间（毫秒）
}

// NewBreaker 创建熔断器
// threshold: 连续失败多少次触发熔断
// cooldown: 熔断后多久允许一次试探
func NewBreaker(threshold int, cooldown time.Duration) CircuitBreaker {
	return &Breaker{
		threshold: int64(threshold),
		cooldown:  cooldown.Milliseconds(),
	}
}

func (b *Breaker) Allow() bool {
	cnt := atomic.LoadInt64(&b.failCount)
	if cnt < b.threshold {
		// Closed：正常放行
		return true
	}
	// Open：检查是否到冷却时间，到了则 HalfOpen 放一次
	last := atomic.LoadInt64(&b.lastFail)
	return time.Now().UnixMilli()-last > b.cooldown
}

func (b *Breaker) Success() {
	atomic.StoreInt64(&b.failCount, 0)
}

func (b *Breaker) Fail() {
	atomic.AddInt64(&b.failCount, 1)
	atomic.StoreInt64(&b.lastFail, time.Now().UnixMilli())
}
