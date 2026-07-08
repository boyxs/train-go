package redislock

import "time"

const (
	// defaultWatchdogTimeout watchdog 模式默认租约；续约每 /3（对齐现状 Redisson 风格）。
	defaultWatchdogTimeout = 30 * time.Second
	// defaultRetryInterval 阻塞路径兜底轮询间隔（pub/sub 之外）。
	defaultRetryInterval = 100 * time.Millisecond
)

// lockConfig 一次获取的全部参数，由 Options 填充。
type lockConfig struct {
	leaseTime       time.Duration // >0：固定租约、关 watchdog；0：走 watchdog
	watchdogTimeout time.Duration // watchdog 模式的租约，续约每 /3
	waitTime        time.Duration // TryLock 拿不到时最多阻塞该时长
	retryInterval   time.Duration // 阻塞路径兜底轮询间隔
	fencing         bool          // 启用 fencing，Fence() 返回单调令牌

	onLost    func(key string, err error) // watchdog 续约失败（丢锁）回调
	onRefresh func(key string)            // watchdog 每次成功续约回调
}

// Options functional option，全部参数经此交付。
type Options func(*lockConfig)

// WithLeaseTime 固定租约 d、关闭 watchdog。短临界区 / 不想要后台 goroutine 时用。
func WithLeaseTime(d time.Duration) Options {
	return func(c *lockConfig) { c.leaseTime = d }
}

// WithWatchdogTimeout watchdog 模式的租约（续约每 d/3）。默认 30s。
func WithWatchdogTimeout(d time.Duration) Options {
	return func(c *lockConfig) { c.watchdogTimeout = d }
}

// WithWaitTime TryLock 拿不到时最多阻塞 d 再返回 false。默认 0（立即返回）。
func WithWaitTime(d time.Duration) Options {
	return func(c *lockConfig) { c.waitTime = d }
}

// WithRetryInterval 阻塞路径（Lock / TryLock+WaitTime）的兜底轮询间隔。默认 100ms。
func WithRetryInterval(d time.Duration) Options {
	return func(c *lockConfig) { c.retryInterval = d }
}

// WithFencing 启用 fencing，全新获取时 INCR 单调计数器，Fence() 返回令牌。
// 真安全须资源侧配合校验（见 fence.go / §3.3）。
func WithFencing() Options {
	return func(c *lockConfig) { c.fencing = true }
}

// WithOnLost watchdog 续约失败（丢锁）回调；挂指标 / 告警。回调在 watchdog goroutine 内
// 同步调用，需短平快退出。
func WithOnLost(fn func(key string, err error)) Options {
	return func(c *lockConfig) { c.onLost = fn }
}

// WithOnRefresh watchdog 每次成功续约回调；挂 refresh_total 指标，也是测试
// "watchdog 在 tick" 的可靠探针（不依赖虚拟时钟）。
func WithOnRefresh(fn func(key string)) Options {
	return func(c *lockConfig) { c.onRefresh = fn }
}

// applyOptions 合并默认值与调用方 opts。
func applyOptions(opts []Options) *lockConfig {
	c := &lockConfig{
		watchdogTimeout: defaultWatchdogTimeout,
		retryInterval:   defaultRetryInterval,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// leaseMs 本次租约（毫秒）：固定租约优先，否则 watchdog 租约。
func (c *lockConfig) leaseMs() int64 {
	if c.leaseTime > 0 {
		return c.leaseTime.Milliseconds()
	}
	return c.watchdogTimeout.Milliseconds()
}

// watchdogEnabled 未设固定租约即走 watchdog。
func (c *lockConfig) watchdogEnabled() bool {
	return c.leaseTime <= 0
}

// watchdogInterval watchdog 续约间隔 = 租约 / 3。
func (c *lockConfig) watchdogInterval() time.Duration {
	return c.watchdogTimeout / 3
}
