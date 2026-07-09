package prometheus

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/boyxs/train-go/webook/pkg/redislock"
)

func newTestClient(t *testing.T, b *PrometheusBuilder) (redislock.Client, *miniredis.Miniredis) {
	s := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	base := redislock.NewClient(rdb)
	return b.Build(base), s
}

func TestBuild_AcquireSuccessAndBusy(t *testing.T) {
	reg := prometheus.NewRegistry()
	cli, _ := newTestClient(t,
		NewPrometheusBuilder("webook", "lock", "test").Registry(reg))
	ctx := context.Background()

	lock, ok, err := cli.TryLock(ctx, "k1", redislock.WithLeaseTime(time.Second))
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, lock)

	_, ok2, err := cli.TryLock(ctx, "k1", redislock.WithLeaseTime(time.Second))
	require.NoError(t, err)
	require.False(t, ok2)

	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_lock_acquire_total", "success"))
	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_lock_acquire_total", "busy"))
}

// 锁中途丢失（关 Redis → watchdog 续约持续网络错误、超租约视同丢锁）→ watchdog_lost_total +1
func TestBuild_WatchdogLost_Counter(t *testing.T) {
	reg := prometheus.NewRegistry()
	cli, s := newTestClient(t,
		NewPrometheusBuilder("webook", "lock", "test").Registry(reg))
	ctx := context.Background()

	lock, ok, err := cli.TryLock(ctx, "k1",
		redislock.WithWatchdogTimeout(120*time.Millisecond)) // 租约 120ms、续约 40ms
	require.NoError(t, err)
	require.True(t, ok)
	t.Cleanup(func() { _ = lock.Unlock(context.Background()) })

	s.Close() // 后续续约都网络错误，超租约后视同丢锁 → OnLost

	require.Eventually(t, func() bool {
		return getCounterValueNoLabel(t, reg, "webook_lock_watchdog_lost_total") >= 1
	}, 2*time.Second, 20*time.Millisecond, "watchdog_lost_total 应被 +1")
}

// TryLock 也须观测 wait_seconds（获取耗时）——否则 TryLock-only 的消费者（cron/loadserver
// 全是 TryLock）该指标永远空、公平锁/WaitTime 的排队阻塞时间也不可见。
func TestBuild_TryLockWait_Histogram(t *testing.T) {
	reg := prometheus.NewRegistry()
	cli, _ := newTestClient(t,
		NewPrometheusBuilder("webook", "lock", "test").Registry(reg))
	ctx := context.Background()

	lock, ok, err := cli.TryLock(ctx, "k1", redislock.WithLeaseTime(time.Second))
	require.NoError(t, err)
	require.True(t, ok)
	t.Cleanup(func() { _ = lock.Unlock(ctx) })

	assert.Equal(t, uint64(1), getHistogramCount(t, reg, "webook_lock_wait_seconds"),
		"TryLock 后应观测一次 wait_seconds（获取耗时）")
}

// Lock 阻塞模式：实际等待时长应被 wait_seconds 观测（用增量断言，不受 setup TryLock 是否观测影响）。
func TestBuild_LockWait_Histogram(t *testing.T) {
	reg := prometheus.NewRegistry()
	cli, _ := newTestClient(t,
		NewPrometheusBuilder("webook", "lock", "test").Registry(reg))
	bg := context.Background()

	first, ok, err := cli.TryLock(bg, "k1", redislock.WithLeaseTime(5*time.Second))
	require.NoError(t, err)
	require.True(t, ok)
	t.Cleanup(func() { _ = first.Unlock(bg) })

	before := getHistogramCount(t, reg, "webook_lock_wait_seconds")

	ctx, cancel := context.WithTimeout(bg, 100*time.Millisecond)
	defer cancel()
	_, _ = cli.Lock(ctx, "k1",
		redislock.WithLeaseTime(5*time.Second), redislock.WithRetryInterval(20*time.Millisecond))

	assert.Equal(t, before+1, getHistogramCount(t, reg, "webook_lock_wait_seconds"),
		"Lock 阻塞结束（含失败）后必须观测一次 wait_seconds")
}

func TestBuild_HeldHistogramOnUnlock(t *testing.T) {
	reg := prometheus.NewRegistry()
	cli, _ := newTestClient(t,
		NewPrometheusBuilder("webook", "lock", "test").Registry(reg))
	ctx := context.Background()

	lock, ok, err := cli.TryLock(ctx, "k1", redislock.WithLeaseTime(5*time.Second))
	require.NoError(t, err)
	require.True(t, ok)

	time.Sleep(20 * time.Millisecond)
	require.NoError(t, lock.Unlock(ctx))

	assert.Equal(t, uint64(1), getHistogramCount(t, reg, "webook_lock_held_seconds"))
}

// 全新 fencing 获取 → fence_issued_total +1；非 fencing 获取不计。
func TestBuild_FenceIssued_Counter(t *testing.T) {
	reg := prometheus.NewRegistry()
	cli, _ := newTestClient(t,
		NewPrometheusBuilder("webook", "lock", "test").Registry(reg))
	ctx := context.Background()

	plain, ok, err := cli.TryLock(ctx, "k0", redislock.WithLeaseTime(5*time.Second))
	require.NoError(t, err)
	require.True(t, ok)
	t.Cleanup(func() { _ = plain.Unlock(ctx) })

	fenced, ok, err := cli.TryLock(ctx, "k1",
		redislock.WithLeaseTime(5*time.Second), redislock.WithFencing())
	require.NoError(t, err)
	require.True(t, ok)
	t.Cleanup(func() { _ = fenced.Unlock(ctx) })

	assert.Equal(t, 1.0, getCounterValueNoLabel(t, reg, "webook_lock_fence_issued_total"),
		"只有全新 fencing 获取计入 fence_issued_total")
}

func getCounterValue(t *testing.T, reg *prometheus.Registry, name, result string) float64 {
	mfs, err := reg.Gather()
	require.NoError(t, err)
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == "result" && lp.GetValue() == result {
					return m.GetCounter().GetValue()
				}
			}
		}
	}
	return 0
}

func getCounterValueNoLabel(t *testing.T, reg *prometheus.Registry, name string) float64 {
	mfs, err := reg.Gather()
	require.NoError(t, err)
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		var sum float64
		for _, m := range mf.GetMetric() {
			sum += m.GetCounter().GetValue()
		}
		return sum
	}
	return 0
}

func getHistogramCount(t *testing.T, reg *prometheus.Registry, name string) uint64 {
	mfs, err := reg.Gather()
	require.NoError(t, err)
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		var sum uint64
		for _, m := range mf.GetMetric() {
			sum += m.GetHistogram().GetSampleCount()
		}
		return sum
	}
	return 0
}
