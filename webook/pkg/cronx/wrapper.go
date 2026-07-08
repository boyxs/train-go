package cronx

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/golang-module/carbon/v2"

	cronprom "github.com/boyxs/train-go/webook/pkg/cronx/prometheus"
	"github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/pkg/redislock"
)

// Task 定时任务回调签名。date 由 Wrapper 注入（YYYY-MM-DD），方便对齐归档日期。
type Task func(ctx context.Context, date string) error

// Wrapper 把"抢锁 → 跑业务 → 释放锁 + 4 组指标 + panic recover" 模板封死。
// 多 Job 共享一个实例，task 标签区分。业务无关，搬自原 internal/job/ranking.go。
type Wrapper struct {
	lock       redislock.Client
	metrics    *cronprom.Metrics
	l          logger.LoggerX
	now        func() time.Time
	keyFn      func(name string) string
	lockTTL    time.Duration
	relTimeout time.Duration
}

type WrapperOption func(*Wrapper)

// WithNow 注入 clock，便于测试断言 date 字段精确值。
func WithNow(fn func() time.Time) WrapperOption {
	return func(w *Wrapper) { w.now = fn }
}

// WithLockKeyPrefix 设置锁 key 前缀。默认 "cronx:lock:"。
func WithLockKeyPrefix(prefix string) WrapperOption {
	return func(w *Wrapper) { w.keyFn = func(name string) string { return prefix + name } }
}

// WithLockTTL 设置锁 TTL。默认 30s（实例 crash 后让贤窗口）；映射到 redislock 的
// WithWatchdogTimeout(ttl)——watchdog 每 ttl/3 续约保活。
func WithLockTTL(d time.Duration) WrapperOption {
	return func(w *Wrapper) { w.lockTTL = d }
}

// NewWrapper 业务 Job 注入用：所有 cron callback 都从 Wrap() 得到。
func NewWrapper(lock redislock.Client, m *cronprom.Metrics, l logger.LoggerX, opts ...WrapperOption) *Wrapper {
	w := &Wrapper{
		lock:       lock,
		metrics:    m,
		l:          l,
		now:        time.Now,
		keyFn:      func(name string) string { return "cronx:lock:" + name },
		lockTTL:    30 * time.Second,
		relTimeout: 2 * time.Second,
	}
	for _, o := range opts {
		o(w)
	}
	return w
}

// Wrap 把 (name, timeout, fn) 包装为无参 cron callback。
// 流程：抢锁 → 跑业务 → Unlock；全程埋点 4 组指标 + panic recover。
func (w *Wrapper) Wrap(name string, timeout time.Duration, fn Task) func() {
	lockKey := w.keyFn(name)
	return func() {
		defer w.recover(name)
		w.metrics.InFlight(name).Inc()
		defer w.metrics.InFlight(name).Dec()

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		lock, ok := w.acquire(ctx, name, lockKey)
		if !ok {
			return
		}
		defer w.release(name, lock)

		w.runTask(ctx, name, fn)
	}
}

func (w *Wrapper) acquire(ctx context.Context, name, lockKey string) (redislock.RedisLock, bool) {
	// watchdog 自动开（WithWatchdogTimeout 未设 leaseTime → 走 watchdog，续约 ttl/3）
	lock, ok, err := w.lock.TryLock(ctx, lockKey, redislock.WithWatchdogTimeout(w.lockTTL))
	if err != nil {
		w.metrics.Runs(name, "error").Inc()
		w.l.Error("cronx 抢锁失败",
			logger.String("task", name), logger.String("lockKey", lockKey), logger.Error(err))
		return nil, false
	}
	if !ok {
		w.metrics.Runs(name, "skipped").Inc()
		w.l.Debug("cronx 锁被占，跳过本轮",
			logger.String("task", name), logger.String("lockKey", lockKey))
		return nil, false
	}
	return lock, true
}

func (w *Wrapper) release(name string, lock redislock.RedisLock) {
	ctx, cancel := context.WithTimeout(context.Background(), w.relTimeout)
	defer cancel()
	if err := lock.Unlock(ctx); err != nil && !errors.Is(err, redislock.ErrLockNotHeld) {
		w.l.Warn("cronx 锁释放异常",
			logger.String("task", name), logger.Error(err))
	}
}

func (w *Wrapper) runTask(ctx context.Context, name string, fn Task) {
	date := carbon.CreateFromStdTime(w.now()).ToDateString()
	start := time.Now()
	err := fn(ctx, date)
	w.metrics.Duration(name).Observe(time.Since(start).Seconds())

	if err != nil {
		w.metrics.Runs(name, "failed").Inc()
		w.l.Error("cronx 任务失败",
			logger.String("task", name),
			logger.String("date", date),
			logger.Error(err))
		return
	}
	w.metrics.Runs(name, "success").Inc()
	w.metrics.MarkSuccess(name)
}

func (w *Wrapper) recover(name string) {
	r := recover()
	if r == nil {
		return
	}
	w.metrics.Runs(name, "panic").Inc()
	w.l.Error("cronx 任务 panic",
		logger.String("task", name),
		logger.String("panic", fmt.Sprintf("%v", r)),
		logger.String("stack", string(debug.Stack())))
}
