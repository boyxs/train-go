package redislock

import (
	"context"
	"errors"
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
