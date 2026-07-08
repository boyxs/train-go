package redislock

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// steal 模拟"锁被别人接管"：清掉本 token、装上另一持有者，制造 token 不匹配。
func steal(t *testing.T, rdb *redis.Client, key, myToken string) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, rdb.HDel(ctx, lockKey(key), myToken).Err())
	require.NoError(t, rdb.HSet(ctx, lockKey(key), "stolen-token", 1).Err())
}

func TestUnlock_FullRelease(t *testing.T) {
	cli, s, _ := newTestClient(t)
	ctx := context.Background()

	lock, ok, err := cli.TryLock(ctx, "k1", WithLeaseTime(5*time.Second))
	require.NoError(t, err)
	require.True(t, ok)

	require.NoError(t, lock.Unlock(ctx))
	assert.False(t, s.Exists(lockKey("k1")), "完全释放后 lock key 应消失")
}

func TestUnlock_TokenMismatch(t *testing.T) {
	cli, _, rdb := newTestClient(t)
	ctx := context.Background()

	lock, ok, err := cli.TryLock(ctx, "k1", WithLeaseTime(5*time.Second))
	require.NoError(t, err)
	require.True(t, ok)

	steal(t, rdb, "k1", lock.Token())

	err = lock.Unlock(ctx)
	assert.ErrorIs(t, err, ErrLockNotHeld, "token 不匹配必须返 ErrLockNotHeld")

	// 不能误删别人的锁
	held, err := rdb.HGet(ctx, lockKey("k1"), "stolen-token").Int()
	require.NoError(t, err)
	assert.Equal(t, 1, held)
}

func TestForceUnlock(t *testing.T) {
	cli, s, _ := newTestClient(t)
	ctx := context.Background()

	lock, ok, err := cli.TryLock(ctx, "k1", WithLeaseTime(5*time.Second))
	require.NoError(t, err)
	require.True(t, ok)

	deleted, err := lock.ForceUnlock(ctx)
	require.NoError(t, err)
	assert.True(t, deleted, "ForceUnlock 应真的删掉锁")
	assert.False(t, s.Exists(lockKey("k1")))

	// 再次强删已不存在的锁返回 false
	deleted, err = lock.ForceUnlock(ctx)
	require.NoError(t, err)
	assert.False(t, deleted)
}

func TestRefresh_Success(t *testing.T) {
	cli, s, _ := newTestClient(t)
	ctx := context.Background()

	lock, ok, err := cli.TryLock(ctx, "k1", WithLeaseTime(1*time.Second))
	require.NoError(t, err)
	require.True(t, ok)

	s.FastForward(800 * time.Millisecond)
	assert.True(t, s.Exists(lockKey("k1")))

	require.NoError(t, lock.Refresh(ctx))

	s.FastForward(900 * time.Millisecond) // 原 1s 租约早过期；Refresh 生效则仍在
	assert.True(t, s.Exists(lockKey("k1")), "Refresh 后 TTL 应被重置")
}

func TestRefresh_LostLock(t *testing.T) {
	cli, _, rdb := newTestClient(t)
	ctx := context.Background()

	lock, ok, err := cli.TryLock(ctx, "k1", WithLeaseTime(5*time.Second))
	require.NoError(t, err)
	require.True(t, ok)

	steal(t, rdb, "k1", lock.Token())

	assert.ErrorIs(t, lock.Refresh(ctx), ErrLockNotHeld)
}

func TestQueries(t *testing.T) {
	cli, _, _ := newTestClient(t)
	ctx := context.Background()

	lock, ok, err := cli.TryLock(ctx, "k1", WithLeaseTime(5*time.Second))
	require.NoError(t, err)
	require.True(t, ok)

	locked, err := lock.IsLocked(ctx)
	require.NoError(t, err)
	assert.True(t, locked)

	mine, err := lock.IsHeldByMe(ctx)
	require.NoError(t, err)
	assert.True(t, mine)

	cnt, err := lock.HoldCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, cnt)

	ttl, err := lock.TTL(ctx)
	require.NoError(t, err)
	assert.Greater(t, ttl, time.Duration(0))
	assert.LessOrEqual(t, ttl, 5*time.Second)

	require.NoError(t, lock.Unlock(ctx))

	locked, err = lock.IsLocked(ctx)
	require.NoError(t, err)
	assert.False(t, locked, "释放后 IsLocked=false")
	cnt, err = lock.HoldCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, cnt, "释放后 HoldCount=0")
}

// 不设 WithLeaseTime → watchdog 默认 ON。偷 token 后 OnLost 应被触发（证明 watchdog 在 tick）。
func TestWatchdog_DefaultOn(t *testing.T) {
	cli, _, rdb := newTestClient(t)
	ctx := context.Background()

	var hit int32
	lock, ok, err := cli.TryLock(ctx, "k1",
		WithWatchdogTimeout(180*time.Millisecond), // 续约间隔 60ms
		WithOnLost(func(string, error) { atomic.AddInt32(&hit, 1) }))
	require.NoError(t, err)
	require.True(t, ok)
	t.Cleanup(func() { _ = lock.Unlock(context.Background()) })

	steal(t, rdb, "k1", lock.Token())

	require.Eventually(t, func() bool { return atomic.LoadInt32(&hit) >= 1 },
		2*time.Second, 20*time.Millisecond, "默认 watchdog 应自动启动并触发 OnLost")
}

func TestWatchdog_OnRefresh(t *testing.T) {
	cli, _, _ := newTestClient(t)
	ctx := context.Background()

	var hit int32
	lock, ok, err := cli.TryLock(ctx, "k1",
		WithWatchdogTimeout(150*time.Millisecond), // 续约间隔 50ms
		WithOnRefresh(func(string) { atomic.AddInt32(&hit, 1) }))
	require.NoError(t, err)
	require.True(t, ok)
	t.Cleanup(func() { _ = lock.Unlock(context.Background()) })

	// 50ms 间隔反复续约：2s 内应 ≥5 次（留足余量抗 CI 抖动）
	require.Eventually(t, func() bool { return atomic.LoadInt32(&hit) >= 5 },
		2*time.Second, 50*time.Millisecond, "watchdog 应按 租约/3 间隔反复续约")
}

// 设了 WithLeaseTime → 固定租约、watchdog OFF：偷 token 后 OnLost 永不触发。
func TestLeaseTime_NoWatchdog(t *testing.T) {
	cli, _, rdb := newTestClient(t)
	ctx := context.Background()

	var hit int32
	lock, ok, err := cli.TryLock(ctx, "k1",
		WithLeaseTime(1*time.Second),
		WithOnLost(func(string, error) { atomic.AddInt32(&hit, 1) }))
	require.NoError(t, err)
	require.True(t, ok)
	t.Cleanup(func() { _ = lock.Unlock(context.Background()) })

	steal(t, rdb, "k1", lock.Token())

	time.Sleep(500 * time.Millisecond)
	assert.Equal(t, int32(0), atomic.LoadInt32(&hit), "固定租约模式 watchdog 不该跑")
}

// P0 分支①：token 不匹配（干净丢锁）→ OnLost 一次并透传 key。
func TestWatchdog_OnLost_Invoked(t *testing.T) {
	cli, _, rdb := newTestClient(t)
	ctx := context.Background()

	var (
		lostCount int32
		mu        sync.Mutex
		lostKey   string
	)
	lock, ok, err := cli.TryLock(ctx, "k1",
		WithWatchdogTimeout(150*time.Millisecond),
		WithOnLost(func(key string, _ error) {
			atomic.AddInt32(&lostCount, 1)
			mu.Lock()
			lostKey = key
			mu.Unlock()
		}))
	require.NoError(t, err)
	require.True(t, ok)
	t.Cleanup(func() { _ = lock.Unlock(context.Background()) })

	steal(t, rdb, "k1", lock.Token())

	require.Eventually(t, func() bool { return atomic.LoadInt32(&lostCount) >= 1 },
		2*time.Second, 20*time.Millisecond, "OnLost 应被调用至少一次")

	mu.Lock()
	got := lostKey
	mu.Unlock()
	assert.Equal(t, "k1", got)
}

// P0 分支②（核心）：网络错误持续超租约 → 视同丢锁，OnLost 一次并退出，不无限空转。
func TestWatchdog_OnLost_OnPersistentRefreshError(t *testing.T) {
	cli, s, _ := newTestClient(t)
	ctx := context.Background()

	var lostCount int32
	lock, ok, err := cli.TryLock(ctx, "k1",
		WithWatchdogTimeout(180*time.Millisecond), // 租约 180ms、间隔 60ms
		WithOnLost(func(string, error) { atomic.AddInt32(&lostCount, 1) }))
	require.NoError(t, err)
	require.True(t, ok)
	t.Cleanup(func() { _ = lock.Unlock(context.Background()) })

	s.Close() // 关 Redis：后续每次续约都网络错误（非 token 不匹配）

	require.Eventually(t, func() bool { return atomic.LoadInt32(&lostCount) >= 1 },
		2*time.Second, 20*time.Millisecond,
		"网络错误持续超租约后应视同丢锁并触发 OnLost")

	// 触发后 watchdog 应已退出：OnLost 不再重复
	snap := atomic.LoadInt32(&lostCount)
	time.Sleep(400 * time.Millisecond)
	assert.Equal(t, snap, atomic.LoadInt32(&lostCount), "视同丢锁后 watchdog 应退出")
}

// Unlock 后 watchdog 必须停：OnRefresh 不再增长（stopOnce + close(stop) 链路）。
func TestWatchdog_StopOnUnlock(t *testing.T) {
	cli, s, _ := newTestClient(t)
	ctx := context.Background()

	var refreshes int32
	lock, ok, err := cli.TryLock(ctx, "k1",
		WithWatchdogTimeout(300*time.Millisecond), // 间隔 100ms
		WithOnRefresh(func(string) { atomic.AddInt32(&refreshes, 1) }))
	require.NoError(t, err)
	require.True(t, ok)

	require.Eventually(t, func() bool { return atomic.LoadInt32(&refreshes) >= 1 },
		3*time.Second, 50*time.Millisecond, "watchdog 应至少续约一次")

	require.NoError(t, lock.Unlock(ctx))
	assert.False(t, s.Exists(lockKey("k1")), "Unlock 后 key 应消失")
	snap := atomic.LoadInt32(&refreshes)

	time.Sleep(400 * time.Millisecond) // >2 个续约间隔
	assert.Equal(t, snap, atomic.LoadInt32(&refreshes), "Unlock 后 watchdog 应停")
}
