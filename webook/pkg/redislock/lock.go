package redislock

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Lock RedisLock 句柄实现。innerMu 串行化续约 / 释放脚本执行（§3.2），
// stop chan + sync.Once 停 watchdog（Unlock 先停再释放）。构造后 key/token/
// leaseMs/fence 均不变，故只有脚本执行需串行、无其它共享可变状态。
type Lock struct {
	client  *RedisClient
	key     string
	token   string
	leaseMs int64
	fence   int64 // WithFencing 时 >0，否则 0

	innerMu  sync.Mutex
	stop     chan struct{}
	stopOnce sync.Once

	onLost    func(key string, err error)
	onRefresh func(key string)
}

func newLock(c *RedisClient, key, token string, fence int64, cfg *lockConfig) *Lock {
	l := &Lock{
		client:    c,
		key:       key,
		token:     token,
		leaseMs:   cfg.leaseMs(),
		fence:     fence,
		stop:      make(chan struct{}),
		onLost:    cfg.onLost,
		onRefresh: cfg.onRefresh,
	}
	if cfg.watchdogEnabled() {
		go l.runWatchdog(cfg.watchdogInterval())
	}
	return l
}

func (l *Lock) Key() string   { return l.key }
func (l *Lock) Token() string { return l.token }
func (l *Lock) Fence() int64  { return l.fence }

func (l *Lock) Unlock(ctx context.Context) error {
	// 先停 watchdog，避免释放后 ticker 又把锁续回来
	l.stopOnce.Do(func() { close(l.stop) })

	l.innerMu.Lock()
	res, err := releaseScript.Run(ctx, l.client.cmd,
		[]string{lockKey(l.key), channelKey(l.key)},
		l.leaseMs, l.token, unlockMsg).Int64()
	l.innerMu.Unlock()

	if err != nil {
		return err
	}
	if res == -1 {
		return ErrLockNotHeld
	}
	return nil
}

func (l *Lock) ForceUnlock(ctx context.Context) (bool, error) {
	l.stopOnce.Do(func() { close(l.stop) })
	n, err := l.client.cmd.Del(ctx, lockKey(l.key)).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (l *Lock) Refresh(ctx context.Context) error {
	ok, err := l.doRefresh(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return ErrLockNotHeld
	}
	return nil
}

// doRefresh 跑续约脚本；true=续约成功，false=不在自己手里。innerMu 串行化。
func (l *Lock) doRefresh(ctx context.Context) (bool, error) {
	l.innerMu.Lock()
	defer l.innerMu.Unlock()
	res, err := refreshScript.Run(ctx, l.client.cmd, []string{lockKey(l.key)}, l.leaseMs, l.token).Int64()
	if err != nil {
		return false, err
	}
	return res == 1, nil
}

func (l *Lock) IsLocked(ctx context.Context) (bool, error) {
	n, err := l.client.cmd.Exists(ctx, lockKey(l.key)).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (l *Lock) IsHeldByMe(ctx context.Context) (bool, error) {
	return l.client.cmd.HExists(ctx, lockKey(l.key), l.token).Result()
}

func (l *Lock) HoldCount(ctx context.Context) (int, error) {
	n, err := l.client.cmd.HGet(ctx, lockKey(l.key), l.token).Int()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return n, nil
}

func (l *Lock) TTL(ctx context.Context) (time.Duration, error) {
	d, err := l.client.cmd.PTTL(ctx, lockKey(l.key)).Result()
	if err != nil {
		return 0, err
	}
	if d < 0 { // -1 无过期，-2 无 key
		return 0, nil
	}
	return d, nil
}

// runWatchdog 周期续约，直到 Unlock 关 stop。P0 三分支（务必保留）：
// 成功 → onRefresh；token 不匹配（干净丢锁）→ onLost 并退出；
// 网络错误 → 静默重试，但距上次成功 >= 租约时长时视同丢锁 → onLost 并退出，
// 杜绝幻觉持锁与续约长期失败时 goroutine 永不退出的泄漏。
func (l *Lock) runWatchdog(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	lastOK := time.Now() // 锁刚获取，视为上次成功续约时刻
	leaseDur := time.Duration(l.leaseMs) * time.Millisecond
	for {
		select {
		case <-l.stop:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), interval)
			ok, err := l.doRefresh(ctx)
			cancel()
			switch {
			case err == nil && ok:
				lastOK = time.Now()
				l.fireOnRefresh()
			case err == nil && !ok:
				l.fireOnLost(ErrLockNotHeld) // 干净丢锁：token 已不是自己
				return
			default:
				// 网络抖动 / 超时：距上次成功 >= 租约时，key 早该过期，
				// 继续自认持锁就是幻觉 → 告警并退出（同时避免 goroutine 泄漏）。
				if time.Since(lastOK) >= leaseDur {
					l.fireOnLost(err)
					return
				}
			}
		}
	}
}

// fireOnLost / fireOnRefresh 包 recover：回调 panic（指标库重复注册 / 写关闭的 chan 等）
// 不应把 watchdog goroutine 拖崩进程。回调失败静默吞。
func (l *Lock) fireOnLost(err error) {
	if l.onLost == nil {
		return
	}
	defer func() { _ = recover() }()
	l.onLost(l.key, err)
}

func (l *Lock) fireOnRefresh() {
	if l.onRefresh == nil {
		return
	}
	defer func() { _ = recover() }()
	l.onRefresh(l.key)
}
