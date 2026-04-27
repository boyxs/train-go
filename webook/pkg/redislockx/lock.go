package redislockx

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/bsm/redislock"
)

// redisLock 包装 bsm Lock，加 Watchdog goroutine。
// stop chan 同时承担"已 Unlock"信号；sync.Once 保证 Unlock 重入只关一次 chan。
type redisLock struct {
	inner *redislock.Lock
	ttl   time.Duration // Watchdog 续约用，Refresh 后会更新

	stop     chan struct{}
	stopOnce sync.Once

	onLost    func(key string, err error)
	onRefresh func(key string)
}

func newRedisLock(inner *redislock.Lock, ttl time.Duration, cfg *lockConfig) *redisLock {
	l := &redisLock{
		inner:     inner,
		ttl:       ttl,
		stop:      make(chan struct{}),
		onLost:    cfg.onLost,
		onRefresh: cfg.onRefresh,
	}
	if cfg.watchdogInterval > 0 {
		go l.runWatchdog(cfg.watchdogInterval)
	}
	return l
}

func (l *redisLock) Key() string   { return l.inner.Key() }
func (l *redisLock) Token() string { return l.inner.Token() }

// safeOnLost / safeOnRefresh 包 recover：用户回调里 panic（指标库重复注册 /
// 关闭的 chan 写入等）不应把 watchdog goroutine 拖崩进程。回调失败静默吞。
func (l *redisLock) safeOnLost(err error) {
	if l.onLost == nil {
		return
	}
	defer func() { _ = recover() }()
	l.onLost(l.inner.Key(), err)
}

func (l *redisLock) safeOnRefresh() {
	if l.onRefresh == nil {
		return
	}
	defer func() { _ = recover() }()
	l.onRefresh(l.inner.Key())
}

func (l *redisLock) Refresh(ctx context.Context, ttl time.Duration) error {
	err := l.inner.Refresh(ctx, ttl, nil)
	if errors.Is(err, redislock.ErrNotObtained) {
		return ErrLockNotHeld
	}
	if err != nil {
		return err
	}
	l.ttl = ttl
	return nil
}

func (l *redisLock) Unlock(ctx context.Context) error {
	// 先停 watchdog，避免 Unlock 后 ticker 又把 key 续回来
	l.stopOnce.Do(func() { close(l.stop) })
	err := l.inner.Release(ctx)
	if errors.Is(err, redislock.ErrLockNotHeld) {
		return ErrLockNotHeld
	}
	return err
}

// runWatchdog 周期性 Refresh，直到 Unlock 关 stop。
// Redisson 招牌特性：bsm 不带，由我们这层提供。
// 续约失败（锁丢了 / Redis 抖动）静默退出 watchdog；调用方挂 OnLost 可感知。
// pkg 层不依赖项目 logger，可观测靠回调。
func (l *redisLock) runWatchdog(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-l.stop:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), interval)
			err := l.inner.Refresh(ctx, l.ttl, nil)
			cancel()
			if errors.Is(err, redislock.ErrNotObtained) {
				l.safeOnLost(ErrLockNotHeld)
				return
			}
			if err == nil {
				l.safeOnRefresh()
			}
		}
	}
}
