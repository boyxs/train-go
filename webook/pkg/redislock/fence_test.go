package redislock

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 全新获取的 fencing 令牌必须跨获取单调递增（唯一真安全的基石）。
func TestFencing_MonotonicAcrossAcquisitions(t *testing.T) {
	cli, _, _ := newTestClient(t)
	ctx := context.Background()

	l1, ok, err := cli.TryLock(ctx, "k1", WithLeaseTime(5*time.Second), WithFencing())
	require.NoError(t, err)
	require.True(t, ok)
	f1 := l1.Fence()
	assert.Greater(t, f1, int64(0), "fencing 令牌应 >0")
	require.NoError(t, l1.Unlock(ctx))

	l2, ok, err := cli.TryLock(ctx, "k1", WithLeaseTime(5*time.Second), WithFencing())
	require.NoError(t, err)
	require.True(t, ok)
	f2 := l2.Fence()
	require.NoError(t, l2.Unlock(ctx))

	assert.Greater(t, f2, f1, "每次全新获取 fencing 令牌必须单调递增")
	assert.Equal(t, f1+1, f2, "计数器持久（不过期），连续全新获取应 +1")
}

// 未启用 fencing 时 Fence()=0。
func TestFencing_Disabled_FenceZero(t *testing.T) {
	cli, _, _ := newTestClient(t)
	ctx := context.Background()

	lock, ok, err := cli.TryLock(ctx, "k1", WithLeaseTime(5*time.Second))
	require.NoError(t, err)
	require.True(t, ok)
	t.Cleanup(func() { _ = lock.Unlock(ctx) })

	assert.Equal(t, int64(0), lock.Fence(), "未启用 fencing 时 Fence()=0")
}

// fencing 模式被占仍是软失败（false, nil），且不 bump 计数器。
func TestFencing_BusyReturnsFalse(t *testing.T) {
	cli, _, rdb := newTestClient(t)
	ctx := context.Background()

	first, ok, err := cli.TryLock(ctx, "k1", WithLeaseTime(5*time.Second), WithFencing())
	require.NoError(t, err)
	require.True(t, ok)
	t.Cleanup(func() { _ = first.Unlock(ctx) })
	require.Greater(t, first.Fence(), int64(0))

	before, err := rdb.Get(ctx, fenceKey("k1")).Int64()
	require.NoError(t, err)

	second, ok, err := cli.TryLock(ctx, "k1", WithLeaseTime(5*time.Second), WithFencing())
	require.NoError(t, err, "被占是软失败")
	assert.False(t, ok)
	assert.Nil(t, second)

	after, err := rdb.Get(ctx, fenceKey("k1")).Int64()
	require.NoError(t, err)
	assert.Equal(t, before, after, "被占不该 bump fencing 计数器")
}

// 同 owner 重入不 bump fencing 计数器：重入沿用首次令牌，重入句柄 Fence()=0（fence.lua）。
func TestFencing_ReentrantDoesNotBump(t *testing.T) {
	cli, _, rdb := newTestClient(t)
	ctx := context.Background()

	first, ok, err := cli.TryLock(ctx, "k1", WithReentrant("owner-A"), WithLeaseTime(5*time.Second), WithFencing())
	require.NoError(t, err)
	require.True(t, ok)
	require.Greater(t, first.Fence(), int64(0), "首次全新获取应发一个令牌")

	before, err := rdb.Get(ctx, fenceKey("k1")).Int64()
	require.NoError(t, err)

	second, ok, err := cli.TryLock(ctx, "k1", WithReentrant("owner-A"), WithLeaseTime(5*time.Second), WithFencing())
	require.NoError(t, err)
	require.True(t, ok, "同 owner 应重入成功")

	after, err := rdb.Get(ctx, fenceKey("k1")).Int64()
	require.NoError(t, err)
	assert.Equal(t, before, after, "重入不该 bump fencing 计数器")
	assert.Equal(t, int64(0), second.Fence(), "重入句柄 Fence()=0（沿用首次令牌）")

	// 重入 2 次须释放 2 次
	require.NoError(t, second.Unlock(ctx))
	require.NoError(t, first.Unlock(ctx))
}
