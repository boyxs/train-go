package cronx

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dto "github.com/prometheus/client_model/go"

	cronprom "github.com/boyxs/train-go/webook/pkg/cronx/prometheus"
	"github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/pkg/redislock"
)

// recordingLogger 收集日志，用于断言 panic 路径里 stack 字段是否落地。
type recordingLogger struct {
	mu      sync.Mutex
	entries []logEntry
}

type logEntry struct {
	level  string
	msg    string
	fields map[string]any
}

func (r *recordingLogger) record(level, msg string, args []logger.Field) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := make(map[string]any, len(args))
	for _, f := range args {
		m[f.Key] = f.Val
	}
	r.entries = append(r.entries, logEntry{level: level, msg: msg, fields: m})
}

func (r *recordingLogger) Debug(_ context.Context, msg string, args ...logger.Field) {
	r.record("debug", msg, args)
}
func (r *recordingLogger) Info(_ context.Context, msg string, args ...logger.Field) {
	r.record("info", msg, args)
}
func (r *recordingLogger) Warn(_ context.Context, msg string, args ...logger.Field) {
	r.record("warn", msg, args)
}
func (r *recordingLogger) Error(_ context.Context, msg string, args ...logger.Field) {
	r.record("error", msg, args)
}

func (r *recordingLogger) byLevel(level string) []logEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]logEntry, 0)
	for _, e := range r.entries {
		if e.level == level {
			out = append(out, e)
		}
	}
	return out
}

// fixture：真锁（miniredis）+ 独立 prom reg + recordingLogger
type fixture struct {
	w       *Wrapper
	mr      *miniredis.Miniredis
	rdb     *redis.Client
	lock    redislock.Client
	metrics *cronprom.Metrics
	reg     *prometheus.Registry
	log     *recordingLogger
}

func newFixture(t *testing.T, opts ...WrapperOption) *fixture {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	reg := prometheus.NewRegistry()
	metrics := cronprom.NewPrometheusBuilder("webook", "cron", "test").Registry(reg).Build()
	lock := redislock.NewClient(rdb)
	rl := &recordingLogger{}
	w := NewWrapper(lock, metrics, rl, opts...)

	return &fixture{w: w, mr: mr, rdb: rdb, lock: lock, metrics: metrics, reg: reg, log: rl}
}

// ── 工具：从 reg 读 counter / gauge / histogram count ──────────────────

func counterVal(t *testing.T, reg *prometheus.Registry, name string, labels map[string]string) float64 {
	mfs, err := reg.Gather()
	require.NoError(t, err)
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			if matchLabels(m.GetLabel(), labels) {
				return m.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func gaugeVal(t *testing.T, reg *prometheus.Registry, name string, labels map[string]string) float64 {
	mfs, err := reg.Gather()
	require.NoError(t, err)
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			if matchLabels(m.GetLabel(), labels) {
				return m.GetGauge().GetValue()
			}
		}
	}
	return 0
}

func histCount(t *testing.T, reg *prometheus.Registry, name string, labels map[string]string) uint64 {
	mfs, err := reg.Gather()
	require.NoError(t, err)
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			if matchLabels(m.GetLabel(), labels) {
				return m.GetHistogram().GetSampleCount()
			}
		}
	}
	return 0
}

func matchLabels(lps []*dto.LabelPair, want map[string]string) bool {
	got := make(map[string]string, len(lps))
	for _, lp := range lps {
		got[lp.GetName()] = lp.GetValue()
	}
	for k, v := range want {
		if got[k] != v {
			return false
		}
	}
	return true
}

// ───────── 用例 ─────────

func TestWrapper_Wrap_Success(t *testing.T) {
	fixedNow := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	f := newFixture(t, WithNow(func() time.Time { return fixedNow }))

	var gotDate string
	cb := f.w.Wrap("hot", time.Second, func(ctx context.Context, date string) error {
		gotDate = date
		return nil
	})
	cb()

	assert.Equal(t, "2026-04-26", gotDate)
	assert.Equal(t, 1.0, counterVal(t, f.reg, "webook_cron_runs_total", map[string]string{"task": "hot", "result": "success"}))
	assert.Equal(t, uint64(1), histCount(t, f.reg, "webook_cron_duration_seconds", map[string]string{"task": "hot"}))
	assert.Equal(t, 0.0, gaugeVal(t, f.reg, "webook_cron_in_flight", map[string]string{"task": "hot"}))
	assert.Greater(t, gaugeVal(t, f.reg, "webook_cron_last_success_timestamp", map[string]string{"task": "hot"}), float64(0))

	// 默认 lockKey 前缀（成功路径锁会被释放，所以只能间接验证：业务跑过）
}

func TestWrapper_Wrap_LockBusy(t *testing.T) {
	f := newFixture(t, WithLockKeyPrefix("test:lock:"))

	first, ok, err := f.lock.TryLock(context.Background(), "test:lock:hot", redislock.WithLeaseTime(5*time.Second))
	require.NoError(t, err)
	require.True(t, ok)
	t.Cleanup(func() { _ = first.Unlock(context.Background()) })

	called := false
	cb := f.w.Wrap("hot", time.Second, func(ctx context.Context, date string) error {
		called = true
		return nil
	})
	cb()

	assert.False(t, called)
	assert.Equal(t, 1.0, counterVal(t, f.reg, "webook_cron_runs_total", map[string]string{"task": "hot", "result": "skipped"}))
}

func TestWrapper_Wrap_LockError(t *testing.T) {
	f := newFixture(t)
	f.mr.Close() // 关掉 miniredis 模拟 Redis 不可达

	called := false
	cb := f.w.Wrap("hot", time.Second, func(ctx context.Context, date string) error {
		called = true
		return nil
	})
	cb()

	assert.False(t, called)
	assert.Equal(t, 1.0, counterVal(t, f.reg, "webook_cron_runs_total", map[string]string{"task": "hot", "result": "error"}))
}

func TestWrapper_Wrap_BusinessError(t *testing.T) {
	f := newFixture(t)

	cb := f.w.Wrap("hot", time.Second, func(ctx context.Context, date string) error {
		return errors.New("svc 报错")
	})
	cb()

	assert.Equal(t, 1.0, counterVal(t, f.reg, "webook_cron_runs_total", map[string]string{"task": "hot", "result": "failed"}))
	assert.Equal(t, uint64(1), histCount(t, f.reg, "webook_cron_duration_seconds", map[string]string{"task": "hot"}),
		"业务失败也要记 duration")
}

func TestWrapper_Wrap_Panic(t *testing.T) {
	f := newFixture(t)

	cb := f.w.Wrap("hot", time.Second, func(ctx context.Context, date string) error {
		panic("boom")
	})
	assert.NotPanics(t, cb)

	assert.Equal(t, 1.0, counterVal(t, f.reg, "webook_cron_runs_total", map[string]string{"task": "hot", "result": "panic"}))

	errs := f.log.byLevel("error")
	require.NotEmpty(t, errs, "panic 应记 error 日志")
	stack, ok := errs[0].fields["stack"].(string)
	require.True(t, ok, "panic 日志应有 stack 字段")
	assert.Contains(t, strings.ToLower(stack), "goroutine", "stack 字段应包含 runtime/debug.Stack 输出")
}

func TestWrapper_Wrap_UnlockUsesIndependentCtx(t *testing.T) {
	f := newFixture(t, WithLockKeyPrefix("test:lock:"))

	cb := f.w.Wrap("hot", 50*time.Millisecond, func(ctx context.Context, date string) error {
		<-ctx.Done() // 等业务 ctx timeout，模拟超时业务
		return ctx.Err()
	})
	cb()

	// 业务 ctx 已 canceled，但 Unlock 用独立 ctx 仍走完 → 锁应已释放
	again, ok, err := f.lock.TryLock(context.Background(), "test:lock:hot", redislock.WithLeaseTime(time.Second))
	require.NoError(t, err)
	assert.True(t, ok, "Wrap 退出后锁应已释放，能再次抢到")
	if again != nil {
		_ = again.Unlock(context.Background())
	}
}
