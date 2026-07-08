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

	"github.com/boyxs/train-go/webook/pkg/redislockx"
)

func newTestClient(t *testing.T, b *PrometheusBuilder) (redislockx.Client, *miniredis.Miniredis) {
	s := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	t.Cleanup(func() { rdb.Close() })
	base := redislockx.NewClient(rdb)
	return b.Build(base), s
}

func TestBuild_AcquireSuccessAndBusy(t *testing.T) {
	reg := prometheus.NewRegistry()
	cli, _ := newTestClient(t,
		NewPrometheusBuilder("webook", "lock", "test").Registry(reg))
	ctx := context.Background()

	lock, ok, err := cli.TryLock(ctx, "k1", time.Second)
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, lock)

	_, ok2, err := cli.TryLock(ctx, "k1", time.Second)
	require.NoError(t, err)
	require.False(t, ok2)

	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_lock_acquire_total", "success"))
	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_lock_acquire_total", "busy"))
}

// 锁中途丢失（被人改 token → watchdog Refresh 失败）→ watchdog_lost_total +1
func TestBuild_WatchdogLost_Counter(t *testing.T) {
	reg := prometheus.NewRegistry()
	cli, s := newTestClient(t,
		NewPrometheusBuilder("webook", "lock", "test").Registry(reg))
	ctx := context.Background()

	lock, ok, err := cli.TryLock(ctx, "k1", 200*time.Millisecond,
		redislockx.WithWatchdog(40*time.Millisecond))
	require.NoError(t, err)
	require.True(t, ok)
	t.Cleanup(func() { _ = lock.Unlock(context.Background()) })

	// 模拟锁被别人接管，下一次 Refresh 必败
	require.NoError(t, s.Set("k1", "stolen-token"))

	require.Eventually(t, func() bool {
		return getCounterValueNoLabel(t, reg, "webook_lock_watchdog_lost_total") >= 1
	}, time.Second, 20*time.Millisecond, "watchdog_lost_total 应被 +1")
}

// Lock 阻塞模式：实际等待时长应被 wait_seconds 观测
func TestBuild_LockWait_Histogram(t *testing.T) {
	reg := prometheus.NewRegistry()
	cli, _ := newTestClient(t,
		NewPrometheusBuilder("webook", "lock", "test").Registry(reg))
	bg := context.Background()

	first, ok, err := cli.TryLock(bg, "k1", 5*time.Second)
	require.NoError(t, err)
	require.True(t, ok)
	t.Cleanup(func() { _ = first.Unlock(bg) })

	ctx, cancel := context.WithTimeout(bg, 100*time.Millisecond)
	defer cancel()

	_, _ = cli.Lock(ctx, "k1", 5*time.Second,
		redislockx.WithRetryInterval(20*time.Millisecond))

	assert.Equal(t, uint64(1), getHistogramCount(t, reg, "webook_lock_wait_seconds"),
		"Lock 阻塞结束（含失败）后必须观测一次 wait_seconds")
}

func TestBuild_HeldHistogramOnUnlock(t *testing.T) {
	reg := prometheus.NewRegistry()
	cli, _ := newTestClient(t,
		NewPrometheusBuilder("webook", "lock", "test").Registry(reg))
	ctx := context.Background()

	lock, ok, err := cli.TryLock(ctx, "k1", 5*time.Second)
	require.NoError(t, err)
	require.True(t, ok)

	time.Sleep(20 * time.Millisecond)
	require.NoError(t, lock.Unlock(ctx))

	assert.Equal(t, uint64(1), getHistogramCount(t, reg, "webook_lock_held_seconds"))
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
