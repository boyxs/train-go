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
	fair            bool          // 公平锁：FIFO 排队获取，防抢占饿死
	ownerId         string        // 显式持有者身份（WithReentrant）；空则每次获取用随机 token

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

// WithFair 公平锁：按开始等待的先后 FIFO 排队获取，杜绝抢占式下早等者被后来者反复插队饿死。
// 仅在阻塞路径（Lock / TryLock+WaitTime）有意义；实现见 fair.go / §3.5。
func WithFair() Options {
	return func(c *lockConfig) { c.fair = true }
}

// WithReentrant 显式持有者身份 ownerId：同一 ownerId 的多次获取即重入（计数 +1），
// 而非被自己挡住。Go 无稳定 goroutine id（ADR-2），故重入身份必须显式传：跨 goroutine
// 共享同一临界区时各方传同一 ownerId 即可重入。空 ownerId（默认）每次获取用随机 token，
// 天然不可重入。释放需与获取次数相等才真正释放（release.lua 计数归零才 del）。
func WithReentrant(ownerId string) Options {
	return func(c *lockConfig) { c.ownerId = ownerId }
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

// heartbeatMs 公平锁等待者的逐出 deadline 心跳（= 3×retryInterval）：活等待者每次获取尝试
// （间隔 ≤ retryInterval）都刷新 deadline，3× 余量下不会被误逐；崩溃者停止刷新 →
// 约 3×retryInterval 后被队头清理逐出（§3.5 死等待者逐出）。
func (c *lockConfig) heartbeatMs() int64 {
	return (3 * c.retryInterval).Milliseconds()
}

// validate 组合校验：fair 与 fencing 暂不支持同时启用（需专门 fair+fence 原子脚本，P4 未做）。
func (c *lockConfig) validate() error {
	if c.fair && c.fencing {
		return ErrFairFencingUnsupported
	}
	return nil
}
