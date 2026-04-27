package redislockx

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Watchdog Refresh 失败时，OnLost 必须被调用一次（key + err 透传）。
// 企业级排查：锁中途丢失等同于幻觉持锁，必须能告警。
func TestWatchdog_OnLost_Invoked(t *testing.T) {
	s := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	t.Cleanup(func() { rdb.Close() })

	mutex := NewClient(rdb)
	ctx := context.Background()

	var (
		lostCount int32
		mu        sync.Mutex
		lostKey   string // 受 mu 保护：watchdog goroutine 写、测试 goroutine 读
	)
	lock, ok, err := mutex.TryLock(ctx, "k1", 200*time.Millisecond,
		WithWatchdog(50*time.Millisecond),
		WithOnLost(func(key string, _ error) {
			atomic.AddInt32(&lostCount, 1)
			mu.Lock()
			lostKey = key
			mu.Unlock()
		}))
	require.NoError(t, err)
	require.True(t, ok)
	t.Cleanup(func() { _ = lock.Unlock(context.Background()) })

	// 模拟锁被别人接管，下一次 Refresh 必败
	rdb.Set(ctx, "k1", "stolen-token", 5*time.Second)

	// 等 watchdog 至少触发一次（>50ms）
	assert.Eventually(t, func() bool {
		return atomic.LoadInt32(&lostCount) >= 1
	}, time.Second, 20*time.Millisecond, "OnLost 应被调用至少一次")

	mu.Lock()
	got := lostKey
	mu.Unlock()
	assert.Equal(t, "k1", got)
}
