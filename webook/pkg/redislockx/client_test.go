package redislockx

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 公共 fixture
func newTestClient(t *testing.T) (Client, *miniredis.Miniredis, redis.Cmdable) {
	s := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	t.Cleanup(func() { rdb.Close() })
	return NewClient(rdb), s, rdb
}

// 不传任何 opts → watchdog 默认 ON（对齐 Redisson）。
// 用 OnLost+偷 token 作探针：watchdog 真在 tick 才会触发 OnLost；
// 仅验"默认开了 watchdog"，不验具体 interval 值（间隔精度由 TestTryLock_OnRefresh 覆盖）。
func TestTryLock_DefaultWatchdog(t *testing.T) {
	mutex, s, _ := newTestClient(t)
	ctx := context.Background()

	var hit int32
	lock, ok, err := mutex.TryLock(ctx, "k1", 1*time.Second,
		WithOnLost(func(string, error) { atomic.AddInt32(&hit, 1) }))
	require.NoError(t, err)
	require.True(t, ok)
	t.Cleanup(func() { _ = lock.Unlock(context.Background()) })

	require.NoError(t, s.Set("k1", "stolen-token"))

	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&hit) >= 1
	}, 2*time.Second, 200*time.Millisecond,
		"默认 watchdog 应自动启动并触发 OnLost（默认 interval=ttl/3=333ms）")
}

func TestTryLock_OnRefresh(t *testing.T) {
	mutex, _, _ := newTestClient(t)
	ctx := context.Background()

	var hit int32
	lock, ok, err := mutex.TryLock(ctx, "k1", 1*time.Second,
		WithOnRefresh(func(string) { atomic.AddInt32(&hit, 1) }))
	require.NoError(t, err)
	require.True(t, ok)
	t.Cleanup(func() { _ = lock.Unlock(context.Background()) })

	//实际时序
	//
	//ticker.NewTicker(333ms) 标称时间：
	//tick 1: t=333.33ms
	//tick 2: t=666.67ms
	//tick 3: t=1000.00ms
	//tick 4: t=1333.33ms
	//tick 5: t=1666.67ms     ← hit=5，余量 333ms
	//tick 6: t=2000.00ms     ← hit=6 与 Eventually 截止同时发生 ⚠️
	//
	//Eventually waitFor=2s 截止：t=2000ms
	//tick=200ms 决定 poll 时刻：200, 400, ..., 1800, 2000
	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&hit) >= 5
	}, 1801*time.Millisecond, 200*time.Millisecond,
		"默认 watchdog 应自动启动并触发 OnRefresh（默认 interval=ttl/3=333ms）")
}

// 显式关闭 watchdog：偷 token 后 OnLost 永远不该被调（因为 watchdog 根本没跑）。
func TestTryLock_WithoutWatchdog(t *testing.T) {
	mutex, s, _ := newTestClient(t)
	ctx := context.Background()

	var hit int32
	lock, ok, err := mutex.TryLock(ctx, "k1", 1*time.Second,
		WithoutWatchdog(),
		WithOnLost(func(string, error) { atomic.AddInt32(&hit, 1) }))
	require.NoError(t, err)
	require.True(t, ok)
	t.Cleanup(func() { _ = lock.Unlock(context.Background()) })

	require.NoError(t, s.Set("k1", "stolen-token"))

	time.Sleep(2 * time.Second)
	assert.Equal(t, int32(0), atomic.LoadInt32(&hit),
		"WithoutWatchdog 后 watchdog 不该跑 → OnLost 不该触发")
}

func TestTryLock_Success(t *testing.T) {
	mutex, s, _ := newTestClient(t)
	ctx := context.Background()

	lock, ok, err := mutex.TryLock(ctx, "k1", 5*time.Second)

	require.NoError(t, err)
	assert.True(t, ok, "首次抢锁应该成功")
	require.NotNil(t, lock)
	assert.Equal(t, "k1", lock.Key())
	assert.NotEmpty(t, lock.Token(), "token 必须非空（用于校验所有权）")
	assert.True(t, s.Exists("k1"), "Redis 应已写入 lock key")
}

func TestTryLock_Busy(t *testing.T) {
	mutex, _, _ := newTestClient(t)
	ctx := context.Background()

	first, ok1, err := mutex.TryLock(ctx, "k1", 5*time.Second)
	require.NoError(t, err)
	require.True(t, ok1)

	second, ok2, err := mutex.TryLock(ctx, "k1", 5*time.Second)
	require.NoError(t, err, "被占应返 ok=false 而非 error")
	assert.False(t, ok2, "已被占用，第二次应抢不到")
	assert.Nil(t, second)
	assert.NotNil(t, first)
}

func TestTryLock_AfterUnlock(t *testing.T) {
	mutex, _, _ := newTestClient(t)
	ctx := context.Background()

	first, ok, err := mutex.TryLock(ctx, "k1", 5*time.Second)
	require.NoError(t, err)
	require.True(t, ok)

	require.NoError(t, first.Unlock(ctx))

	second, ok, err := mutex.TryLock(ctx, "k1", 5*time.Second)
	require.NoError(t, err)
	assert.True(t, ok, "Unlock 后另一方应能抢到")
	assert.NotNil(t, second)
}

func TestUnlock_TokenMismatch(t *testing.T) {
	mutex, _, rdb := newTestClient(t)
	ctx := context.Background()

	owned, ok, err := mutex.TryLock(ctx, "k1", 5*time.Second)
	require.NoError(t, err)
	require.True(t, ok)

	// 模拟"我以为我有锁，其实是别人的"：直接覆写 redis value 改 token
	rdb.Set(ctx, "k1", "someone-else-token", 5*time.Second)

	err = owned.Unlock(ctx)
	assert.ErrorIs(t, err, ErrLockNotHeld, "token 不匹配必须返 ErrLockNotHeld")

	// 锁还在（不能被误删）
	got, _ := rdb.Get(ctx, "k1").Result()
	assert.Equal(t, "someone-else-token", got)
}

func TestRefresh_Success(t *testing.T) {
	mutex, s, _ := newTestClient(t)
	ctx := context.Background()

	lock, ok, err := mutex.TryLock(ctx, "k1", 1*time.Second)
	require.NoError(t, err)
	require.True(t, ok)

	// 推进 800ms，TTL 剩 200ms
	s.FastForward(800 * time.Millisecond)
	assert.True(t, s.Exists("k1"))

	require.NoError(t, lock.Refresh(ctx, 5*time.Second))

	// 再推进 1s，原 TTL 早过期；如果 Refresh 生效 key 应仍存在
	s.FastForward(1 * time.Second)
	assert.True(t, s.Exists("k1"), "Refresh 后 TTL 应被重置")
}

func TestRefresh_LostLock(t *testing.T) {
	mutex, _, rdb := newTestClient(t)
	ctx := context.Background()

	lock, ok, err := mutex.TryLock(ctx, "k1", 5*time.Second)
	require.NoError(t, err)
	require.True(t, ok)

	// 模拟锁被别人接管
	rdb.Set(ctx, "k1", "stolen-token", 5*time.Second)

	err = lock.Refresh(ctx, 5*time.Second)
	assert.ErrorIs(t, err, ErrLockNotHeld)
}

// 用 OnRefresh 探针真验 stopOnce + close(stop) 链路：
//  1. watchdog 必须先 tick 至少一次（refreshes >= 1）→ 证明 goroutine 在跑
//  2. Unlock 后 refreshes 不再增长 → 证明 stop 信号生效
//  3. bsm Release 删 key
//
// 任意一步坏掉（注释 go runWatchdog / 删 stopOnce / 跳过 Release）测试必红。
func TestWatchdog_StopOnUnlock(t *testing.T) {
	mutex, s, _ := newTestClient(t)
	ctx := context.Background()

	var refreshes int32
	lock, ok, err := mutex.TryLock(ctx, "k1", 3*time.Second,
		WithOnRefresh(func(string) { atomic.AddInt32(&refreshes, 1) }))
	require.NoError(t, err)
	require.True(t, ok)

	// 默认 watchdog 间隔 = ttl/3 = 1s；等到至少续约一次，确认 goroutine 真在跑
	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&refreshes) >= 1
	}, 5*time.Second, 200*time.Millisecond, "watchdog 应至少续约一次")

	require.NoError(t, lock.Unlock(ctx))
	assert.False(t, s.Exists("k1"), "Unlock 后 key 应消失（bsm Release）")
	snap := atomic.LoadInt32(&refreshes)

	// 等 2.5 秒（>2 个 watchdog 间隔）；stop 链路坏了 watchdog 还在 tick → refreshes 必增
	time.Sleep(2500 * time.Millisecond)
	assert.Equal(t, snap, atomic.LoadInt32(&refreshes),
		"Unlock 后 watchdog 应停掉，OnRefresh 不该再触发")
}

func TestLock_BlockingThenCtxCancel(t *testing.T) {
	mutex, _, _ := newTestClient(t)
	bg := context.Background()

	first, ok, err := mutex.TryLock(bg, "k1", 5*time.Second)
	require.NoError(t, err)
	require.True(t, ok)
	t.Cleanup(func() { _ = first.Unlock(bg) })

	ctx, cancel := context.WithTimeout(bg, 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	got, err := mutex.Lock(ctx, "k1", 5*time.Second,
		WithRetryInterval(50*time.Millisecond))
	elapsed := time.Since(start)

	assert.Nil(t, got)
	assert.True(t, errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled),
		"ctx 超时应返 ctx.Err，实际: %v", err)
	assert.GreaterOrEqual(t, elapsed, 150*time.Millisecond, "应阻塞重试到 ctx 取消")
}
