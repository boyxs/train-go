package prometheus

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMetrics(reg *prometheus.Registry) *Metrics {
	return NewPrometheusBuilder("webook", "cron", "test").Registry(reg).Build()
}

func TestBuild_RegisterAndObserve(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newMetrics(reg)

	// 模拟一次 success 全流程
	m.InFlight("hot").Inc()
	m.Duration("hot").Observe(0.5)
	m.Runs("hot", "success").Inc()
	m.MarkSuccess("hot")
	m.InFlight("hot").Dec()

	// 多实例下锁被占
	m.Runs("best", "skipped").Inc()
	// 业务报错
	m.Runs("new", "failed").Inc()

	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_cron_runs_total", "hot", "success"))
	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_cron_runs_total", "best", "skipped"))
	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_cron_runs_total", "new", "failed"))
	assert.Equal(t, uint64(1), getHistogramCount(t, reg, "webook_cron_duration_seconds", "hot"))
	assert.Equal(t, 0.0, getGaugeValue(t, reg, "webook_cron_in_flight", "hot"))
	assert.Greater(t, getGaugeValue(t, reg, "webook_cron_last_success_timestamp", "hot"), float64(0))
}

func TestBuild_MarkSuccessRecordsNow(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newMetrics(reg)

	before := float64(time.Now().Unix())
	m.MarkSuccess("hot")
	after := float64(time.Now().Unix())

	got := getGaugeValue(t, reg, "webook_cron_last_success_timestamp", "hot")
	assert.GreaterOrEqual(t, got, before)
	assert.LessOrEqual(t, got, after)
}

// ─────────────── 工具 ───────────────

func getCounterValue(t *testing.T, reg *prometheus.Registry, name, task, result string) float64 {
	mfs, err := reg.Gather()
	require.NoError(t, err)
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			var taskV, resultV string
			for _, lp := range m.GetLabel() {
				switch lp.GetName() {
				case "task":
					taskV = lp.GetValue()
				case "result":
					resultV = lp.GetValue()
				}
			}
			if taskV == task && resultV == result {
				return m.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func getHistogramCount(t *testing.T, reg *prometheus.Registry, name, task string) uint64 {
	mfs, err := reg.Gather()
	require.NoError(t, err)
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == "task" && lp.GetValue() == task {
					return m.GetHistogram().GetSampleCount()
				}
			}
		}
	}
	return 0
}

func getGaugeValue(t *testing.T, reg *prometheus.Registry, name, task string) float64 {
	mfs, err := reg.Gather()
	require.NoError(t, err)
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == "task" && lp.GetValue() == task {
					return m.GetGauge().GetValue()
				}
			}
		}
	}
	return 0
}
