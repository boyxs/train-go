package redislockx

import "time"

type lockConfig struct {
	watchdogInterval time.Duration               // 0 表示关闭 watchdog
	retryInterval    time.Duration               // Lock 阻塞模式的退避间隔
	onLost           func(key string, err error) // watchdog 续约失败回调（锁中途丢失）
	onRefresh        func(key string)            // watchdog 每次成功续约回调（可挂 refresh_total 指标）
}

type Options func(*lockConfig)

// WithWatchdog 覆盖 watchdog 续约间隔。默认 ttl/3（对齐 Redisson）；想关掉用 WithoutWatchdog。
// 续约失败（锁丢了 / Redis 抖动）静默退出 watchdog；想感知挂 OnLost。
func WithWatchdog(interval time.Duration) Options {
	return func(c *lockConfig) {
		c.watchdogInterval = interval
	}
}

// WithoutWatchdog 显式关闭自动续约。短临界区 / 不想要后台 goroutine 时用。
func WithoutWatchdog() Options {
	return func(c *lockConfig) {
		c.watchdogInterval = 0
	}
}

// WithRetryInterval 设置阻塞 Lock 的重试间隔。默认 100ms。
func WithRetryInterval(d time.Duration) Options {
	return func(c *lockConfig) {
		c.retryInterval = d
	}
}

// WithOnLost 当 watchdog 续约失败（锁被抢 / Redis 抖动）触发；可挂指标或告警。
// 回调在 watchdog goroutine 内同步调用，需自行短平快退出。
func WithOnLost(fn func(key string, err error)) Options {
	return func(c *lockConfig) {
		c.onLost = fn
	}
}

// WithOnRefresh watchdog 每次成功续约触发；可挂 refresh_total 指标，
// 也是测试 "watchdog 在 tick" 的唯一可靠探针（不依赖 miniredis 虚拟时钟）。
// 回调在 watchdog goroutine 内同步调用，需自行短平快退出。
func WithOnRefresh(fn func(key string)) Options {
	return func(c *lockConfig) {
		c.onRefresh = fn
	}
}

// applyOptions 合并默认值和调用方 opts，消除 TryLock / Lock 两份重复。
// watchdog 默认 ttl/3，对齐 Redisson lockWatchdogTimeout/3 行为。
func applyOptions(opts []Options, ttl time.Duration) *lockConfig {
	cfg := &lockConfig{
		retryInterval:    100 * time.Millisecond,
		watchdogInterval: ttl / 3,
	}
	for _, o := range opts {
		o(cfg)
	}
	return cfg
}
