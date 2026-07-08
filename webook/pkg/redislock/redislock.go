package redislock

import (
	"context"
	"errors"
	"time"
)

// ErrLockNotHeld 锁不在自己手里：从未拿到 / 已释放 / TTL 过期被别人拿走。
// Unlock / Refresh 校验 ownerToken 不匹配时返回。
var ErrLockNotHeld = errors.New("redislock: lock not held")

//go:generate mockgen -source=./redislock.go -package=lockmocks -destination=./mocks/lock_mock.go

// Client 自研分布式锁工厂。多副本部署下用同一 Redis 抢锁。
// 单机 / 集群同一构造 NewClient(redis.UniversalClient)，多主用 NewQuorumClient。
// 参数（waitTime / leaseTime / watchdog / fencing…）全部经 Options 交付，
// 签名保持 (ctx, key, opts...) 干净。
type Client interface {
	// TryLock 软获取：拿到 (lock, true, nil)；被占 (nil, false, nil)（非 error）。
	// WithWaitTime>0 时最多阻塞该时长再放弃仍返回 (nil,false,nil)。
	// 网络错误 / ctx 取消返回 (nil, false, err)。
	TryLock(ctx context.Context, key string, opts ...Options) (RedisLock, bool, error)
	// Lock 阻塞获取：ctx 即等待上限，拿到或 ctx.Done。失败返 error。
	Lock(ctx context.Context, key string, opts ...Options) (RedisLock, error)
}

// RedisLock 已持有的锁句柄（自研领域名，非裸 Lock，避免与 Client.Lock() 方法同词）。
// Unlock / Refresh 校验本句柄 ownerToken，防误删他人锁。
type RedisLock interface {
	Key() string
	Token() string // 本句柄 ownerToken
	// Unlock 重入减计数，归零才真释放 + 通知等待者。不在自己手里返 ErrLockNotHeld。
	// 调用方应在 defer 里用独立 ctx（context.Background / WithTimeout），
	// 业务 ctx 已 cancel 时 Unlock 仍要走。
	Unlock(ctx context.Context) error
	// ForceUnlock 强制删锁，不校验持有者（管理 / 兜底用）。返回是否真的删掉了。
	ForceUnlock(ctx context.Context) (bool, error)
	// Refresh 手动续约一个租约周期（watchdog 之外）。不在自己手里返 ErrLockNotHeld。
	Refresh(ctx context.Context) error

	// 状态查询
	IsLocked(ctx context.Context) (bool, error)   // 锁是否被任何人持有
	IsHeldByMe(ctx context.Context) (bool, error) // 是否被本句柄 ownerToken 持有
	HoldCount(ctx context.Context) (int, error)   // 本句柄重入深度（未持有=0）
	TTL(ctx context.Context) (time.Duration, error)

	// Fence 本次持有的 fencing 令牌（WithFencing 时 >0，否则 0）。
	Fence() int64
}
