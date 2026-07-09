package redislock

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestClient 包级共享 fixture：miniredis + go-redis 单机客户端。
func newTestClient(t *testing.T) (Client, *miniredis.Miniredis, *redis.Client) {
	s := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return NewClient(rdb), s, rdb
}

// 空 key 必须被拒：空 hash tag `{}` 在 Redis 集群退化为整键哈希 → 同一把锁多个 key 落不同
// slot → 多 key Lua（release/fencing/fair）CROSSSLOT。获取入口 fail-fast。
func TestTryLock_EmptyKeyRejected(t *testing.T) {
	cli, _, _ := newTestClient(t)
	ctx := context.Background()

	_, ok, err := cli.TryLock(ctx, "", WithLeaseTime(time.Second))
	assert.ErrorIs(t, err, ErrEmptyKey, "TryLock 空 key 应返回 ErrEmptyKey")
	assert.False(t, ok)

	// Lock 用有界 ctx：未加守卫时空 key 会阻塞（miniredis 不按 wall-clock 过期），
	// 200ms 超时兜底；加守卫后应立即返回 ErrEmptyKey（ErrorIs 区分两者）。
	lockCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	_, err = cli.Lock(lockCtx, "", WithLeaseTime(time.Second))
	assert.ErrorIs(t, err, ErrEmptyKey, "Lock 空 key 应返回 ErrEmptyKey")
}

func TestTryLock_Success(t *testing.T) {
	cli, s, rdb := newTestClient(t)
	ctx := context.Background()

	lock, ok, err := cli.TryLock(ctx, "k1", WithLeaseTime(5*time.Second))
	require.NoError(t, err)
	require.True(t, ok, "首次抢锁应成功")
	require.NotNil(t, lock)
	assert.Equal(t, "k1", lock.Key())
	assert.NotEmpty(t, lock.Token(), "token 必须非空（校验所有权）")

	// hash 存储模型：redislock:{k1}:lock 应有 field=token value=1
	lk := lockKey("k1")
	assert.True(t, s.Exists(lk), "lock hash key 应存在")
	cnt, err := rdb.HGet(ctx, lk, lock.Token()).Int()
	require.NoError(t, err)
	assert.Equal(t, 1, cnt, "首次持有计数应为 1")
}

func TestTryLock_Busy(t *testing.T) {
	cli, _, _ := newTestClient(t)
	ctx := context.Background()

	first, ok1, err := cli.TryLock(ctx, "k1", WithLeaseTime(5*time.Second))
	require.NoError(t, err)
	require.True(t, ok1)

	second, ok2, err := cli.TryLock(ctx, "k1", WithLeaseTime(5*time.Second))
	require.NoError(t, err, "被占应返 ok=false 而非 error")
	assert.False(t, ok2, "已被占用，第二次应抢不到")
	assert.Nil(t, second)
	require.NotNil(t, first)
}

func TestTryLock_AfterUnlock(t *testing.T) {
	cli, _, _ := newTestClient(t)
	ctx := context.Background()

	first, ok, err := cli.TryLock(ctx, "k1", WithLeaseTime(5*time.Second))
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, first.Unlock(ctx))

	second, ok, err := cli.TryLock(ctx, "k1", WithLeaseTime(5*time.Second))
	require.NoError(t, err)
	assert.True(t, ok, "Unlock 后另一方应能抢到")
	require.NotNil(t, second)
}

// WithWaitTime>0：被占时阻塞轮询，等对方释放后拿到（P1 用轮询兜底，pub/sub 是 P4）。
func TestTryLock_WaitTime_AcquiresAfterRelease(t *testing.T) {
	cli, _, _ := newTestClient(t)
	bg := context.Background()

	first, ok, err := cli.TryLock(bg, "k1", WithLeaseTime(5*time.Second))
	require.NoError(t, err)
	require.True(t, ok)

	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = first.Unlock(bg)
	}()

	start := time.Now()
	second, ok, err := cli.TryLock(bg, "k1",
		WithLeaseTime(5*time.Second), WithWaitTime(2*time.Second), WithRetryInterval(20*time.Millisecond))
	require.NoError(t, err)
	assert.True(t, ok, "释放后应在 WaitTime 内拿到")
	require.NotNil(t, second)
	assert.Less(t, time.Since(start), 2*time.Second)
}

// WithWaitTime 超时是软失败（false, nil），非 error。
func TestTryLock_WaitTime_Timeout(t *testing.T) {
	cli, _, _ := newTestClient(t)
	ctx := context.Background()

	first, ok, err := cli.TryLock(ctx, "k1", WithLeaseTime(5*time.Second))
	require.NoError(t, err)
	require.True(t, ok)
	t.Cleanup(func() { _ = first.Unlock(ctx) })

	start := time.Now()
	second, ok, err := cli.TryLock(ctx, "k1",
		WithLeaseTime(5*time.Second), WithWaitTime(200*time.Millisecond), WithRetryInterval(20*time.Millisecond))
	require.NoError(t, err, "WaitTime 超时是软失败，非 error")
	assert.False(t, ok, "一直被占，WaitTime 内应拿不到")
	assert.Nil(t, second)
	assert.GreaterOrEqual(t, time.Since(start), 200*time.Millisecond, "应阻塞到 WaitTime")
}

// Lock 阻塞获取：ctx 即等待上限，一直被占则阻塞到 ctx 取消返 ctx.Err。
func TestLock_BlockingThenCtxCancel(t *testing.T) {
	cli, _, _ := newTestClient(t)
	bg := context.Background()

	first, ok, err := cli.TryLock(bg, "k1", WithLeaseTime(5*time.Second))
	require.NoError(t, err)
	require.True(t, ok)
	t.Cleanup(func() { _ = first.Unlock(bg) })

	ctx, cancel := context.WithTimeout(bg, 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	got, err := cli.Lock(ctx, "k1", WithLeaseTime(5*time.Second), WithRetryInterval(50*time.Millisecond))
	elapsed := time.Since(start)

	assert.Nil(t, got)
	assert.True(t, errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled),
		"ctx 超时应返 ctx.Err，实际: %v", err)
	assert.GreaterOrEqual(t, elapsed, 150*time.Millisecond, "应阻塞重试到 ctx 取消")
}

// pub/sub 阻塞（P4-9）：holder 释放后，阻塞的 Lock 应经 publish 近即时唤醒拿到，而非等到
// 轮询间隔。故意把 retryInterval 设很大（5s），只有 pub/sub 能让它快速拿到——证明 §3.4 收益。
func TestBlockingLock_PubSubWakesOnRelease(t *testing.T) {
	cli, _, _ := newTestClient(t)
	bg := context.Background()

	first, ok, err := cli.TryLock(bg, "k1", WithLeaseTime(30*time.Second))
	require.NoError(t, err)
	require.True(t, ok)

	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = first.Unlock(bg)
	}()

	ctx, cancel := context.WithTimeout(bg, 10*time.Second)
	defer cancel()
	start := time.Now()
	second, err := cli.Lock(ctx, "k1", WithLeaseTime(30*time.Second), WithRetryInterval(5*time.Second))
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.NotNil(t, second)
	t.Cleanup(func() { _ = second.Unlock(bg) })
	assert.Less(t, elapsed, 1*time.Second, "应经 pub/sub 唤醒近即时拿到（~100ms），而非等 5s 轮询间隔")
}

// pub/sub 阻塞不泄漏：多轮阻塞获取后订阅应全部退订，goroutine 数不随轮数增长。
// 真泄漏（漏 pubsub.Close）每轮 +~2 个 goroutine，30 轮会 +~60；无泄漏应基本持平。
func TestBlockingLock_NoSubscriptionLeak(t *testing.T) {
	cli, _, _ := newTestClient(t)
	bg := context.Background()

	// 预热一轮让连接池 / 后台 goroutine 稳定，再取基线
	warm, ok, _ := cli.TryLock(bg, "warm", WithLeaseTime(time.Second))
	require.True(t, ok)
	_ = warm.Unlock(bg)
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	before := runtime.NumGoroutine()

	const rounds = 30
	for i := 0; i < rounds; i++ {
		h, ok, err := cli.TryLock(bg, "k1", WithLeaseTime(30*time.Second))
		require.NoError(t, err)
		require.True(t, ok)
		go func() {
			time.Sleep(10 * time.Millisecond)
			_ = h.Unlock(bg)
		}()
		// Lock 阻塞：订阅 → 被 publish 唤醒 → 拿到 → defer 退订
		l, err := cli.Lock(bg, "k1", WithLeaseTime(30*time.Second), WithRetryInterval(time.Second))
		require.NoError(t, err)
		require.NoError(t, l.Unlock(bg))
	}
	time.Sleep(300 * time.Millisecond) // 等所有 pubsub 后台 goroutine 退出
	runtime.GC()
	after := runtime.NumGoroutine()

	assert.LessOrEqual(t, after, before+5,
		"阻塞获取每轮应退订，不泄漏订阅 goroutine（before=%d after=%d）", before, after)
}

// ── P3 可重入（WithReentrant 显式 ownerId 共享持有者身份，跨调用 / 跨 goroutine）──────────

// 同 owner 再次获取即重入（计数 +1），而非被自己挡住 → HoldCount=2。
func TestReentrant_SameOwnerReenters(t *testing.T) {
	cli, _, rdb := newTestClient(t)
	ctx := context.Background()

	first, ok, err := cli.TryLock(ctx, "k1", WithReentrant("owner-A"), WithLeaseTime(5*time.Second))
	require.NoError(t, err)
	require.True(t, ok, "首次获取应成功")
	require.NotNil(t, first)

	second, ok, err := cli.TryLock(ctx, "k1", WithReentrant("owner-A"), WithLeaseTime(5*time.Second))
	require.NoError(t, err)
	require.True(t, ok, "同 owner 再次获取应重入成功，而非被自己占住")
	require.NotNil(t, second)

	cnt, err := rdb.HGet(ctx, lockKey("k1"), "owner-A").Int()
	require.NoError(t, err)
	assert.Equal(t, 2, cnt, "重入两次，hash 计数应为 2")

	hc, err := second.HoldCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, hc, "HoldCount 应反映重入深度")
}

// 重入 N 次须释放 N 次才真正释放（release.lua 计数归零才 del）。
func TestReentrant_ReleaseNTimesToFullyRelease(t *testing.T) {
	cli, _, _ := newTestClient(t)
	ctx := context.Background()

	first, ok, err := cli.TryLock(ctx, "k1", WithReentrant("owner-A"), WithLeaseTime(5*time.Second))
	require.NoError(t, err)
	require.True(t, ok)
	second, ok, err := cli.TryLock(ctx, "k1", WithReentrant("owner-A"), WithLeaseTime(5*time.Second))
	require.NoError(t, err)
	require.True(t, ok)

	// 第一次 Unlock：计数 2→1，仍持有
	require.NoError(t, first.Unlock(ctx))
	locked, err := second.IsLocked(ctx)
	require.NoError(t, err)
	assert.True(t, locked, "重入未归零，锁仍应被持有")
	hc, err := second.HoldCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, hc, "释放一次后重入深度应为 1")

	// 第二次 Unlock：计数 1→0，真正释放
	require.NoError(t, second.Unlock(ctx))
	locked, err = second.IsLocked(ctx)
	require.NoError(t, err)
	assert.False(t, locked, "释放次数等于获取次数后锁应真正释放")
}

// 持有者身份即传入的 ownerId（非随机 UUID），供跨 goroutine 显式共享。
func TestReentrant_TokenEqualsOwnerID(t *testing.T) {
	cli, _, _ := newTestClient(t)
	ctx := context.Background()

	lock, ok, err := cli.TryLock(ctx, "k1", WithReentrant("owner-A"), WithLeaseTime(5*time.Second))
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "owner-A", lock.Token(), "WithReentrant 时 token 应为显式 ownerId")
}

// 跨 owner 互斥——A 持锁时 B 抢不到；A 全部释放后 B 能拿。
func TestReentrant_CrossOwnerMutualExclusion(t *testing.T) {
	cli, _, _ := newTestClient(t)
	ctx := context.Background()

	a, ok, err := cli.TryLock(ctx, "k1", WithReentrant("owner-A"), WithLeaseTime(5*time.Second))
	require.NoError(t, err)
	require.True(t, ok)

	b, ok, err := cli.TryLock(ctx, "k1", WithReentrant("owner-B"), WithLeaseTime(5*time.Second))
	require.NoError(t, err, "被占应软失败非 error")
	assert.False(t, ok, "不同 owner 应互斥，B 抢不到")
	assert.Nil(t, b)

	require.NoError(t, a.Unlock(ctx))
	b, ok, err = cli.TryLock(ctx, "k1", WithReentrant("owner-B"), WithLeaseTime(5*time.Second))
	require.NoError(t, err)
	assert.True(t, ok, "A 释放后 B 应能拿到")
	require.NotNil(t, b)
	t.Cleanup(func() { _ = b.Unlock(ctx) })
}

// 默认（不传 WithReentrant）每次获取用独立随机 token，天然不可重入——重入是 opt-in。
func TestReentrant_DefaultIsOptIn(t *testing.T) {
	cli, _, _ := newTestClient(t)
	ctx := context.Background()

	first, ok, err := cli.TryLock(ctx, "k1", WithLeaseTime(5*time.Second))
	require.NoError(t, err)
	require.True(t, ok)
	t.Cleanup(func() { _ = first.Unlock(ctx) })

	second, ok, err := cli.TryLock(ctx, "k1", WithLeaseTime(5*time.Second))
	require.NoError(t, err)
	assert.False(t, ok, "未显式共享身份，第二次应被占（不可重入）")
	assert.Nil(t, second)
}

// 跨 goroutine 共享持有者身份：两 goroutine 传同一 ownerId 各获取一次，都成功且计数累加到 2。
// ADR-2 的核心价值——Go 无稳定 goroutine id，靠显式 ownerId 让协作的 goroutine 共享临界区。
func TestReentrant_CrossGoroutineSharesIdentity(t *testing.T) {
	cli, _, rdb := newTestClient(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	var oks [2]bool
	var errs [2]error
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, ok, err := cli.TryLock(ctx, "k1", WithReentrant("shared"), WithLeaseTime(5*time.Second))
			oks[idx], errs[idx] = ok, err
		}(i)
	}
	wg.Wait()

	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	assert.True(t, oks[0] && oks[1], "同 ownerId 的两 goroutine 都应获取成功（重入）")

	cnt, err := rdb.HGet(ctx, lockKey("k1"), "shared").Int()
	require.NoError(t, err)
	assert.Equal(t, 2, cnt, "两 goroutine 共享身份，重入计数应累加到 2")
}
