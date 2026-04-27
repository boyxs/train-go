package redislockx

import (
	"context"
	"errors"
	"time"
)

// ErrLockNotHeld 锁不在自己手里：可能从未拿到、已被释放、TTL 过期被别人拿走。
// Refresh / Unlock 校验 token 不匹配时返回。
var ErrLockNotHeld = errors.New("redislockx: lock not held")

//go:generate mockgen -source=./types.go -package=lockmocks -destination=./mocks/lock_mock.go Client,Lock

// Client 类 Redisson 的分布式锁工厂。多实例部署下用同一个 Redis 抢锁。
// 命名对齐 Redisson 的 RedissonClient.getLock()：Client 是工厂，Lock 是持有锁。
type Client interface {
	// TryLock 非阻塞抢锁。ok=false 不是 error，表示锁被别人占着。
	// ttl 是锁自身过期时间；启用 Watchdog 后由后台 goroutine 续约。
	TryLock(ctx context.Context, key string, ttl time.Duration, opts ...Options) (Lock, bool, error)
	// Lock 阻塞抢锁。按退避策略重试，直到拿到锁或 ctx.Done。
	Lock(ctx context.Context, key string, ttl time.Duration, opts ...Options) (Lock, error)
}

// Lock 已持有的锁句柄。Refresh / Unlock 都校验 token，防止误删别人的锁。
type Lock interface {
	Key() string
	Token() string
	// Refresh 把 TTL 重置到 ttl。锁不在自己手里返回 ErrLockNotHeld。
	Refresh(ctx context.Context, ttl time.Duration) error
	// Unlock 释放锁。锁不在自己手里返回 ErrLockNotHeld（说明已 TTL 过期或被抢）。
	// 调用方应在 defer 里用独立 ctx（context.Background 或 WithTimeout），
	// 业务 ctx 已 cancel 时 Unlock 仍要走。
	Unlock(ctx context.Context) error
}
