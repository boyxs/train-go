package prometheus

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Builder 任务级指标构造器接口
type Builder interface {
	Build() *Metrics
}

// PrometheusBuilder cron 任务 Prometheus 指标构造器
//
// 与 pkg/redisx/prometheus、pkg/gormx/prometheus 同款 builder 风格：
// namespace + subsystem + help + 链式 Registry/Buckets + 终端 Build。
//
// 默认输出四组指标（cron 场景四个全要，没有"按需启用"的真实需求）：
//   - {namespace}_{subsystem}_runs_total (Counter, 标签: task/result)
//   - {namespace}_{subsystem}_duration_seconds (Histogram, 标签: task)
//   - {namespace}_{subsystem}_in_flight (Gauge, 标签: task)
//   - {namespace}_{subsystem}_last_success_timestamp (Gauge, 标签: task)
//
// result 取值：success / failed / skipped / panic
//   - skipped 单独标签：多实例下"锁被别人占"是常态而非错误，不要混进 failed
type PrometheusBuilder struct {
	namespace string
	subsystem string
	help      string
	buckets   []float64
	registry  prometheus.Registerer
}

// 默认桶：cron 任务从秒级（轻量重算）到分钟级（归档）都要覆盖
var defaultDurationBuckets = []float64{0.1, 0.5, 1, 5, 10, 30, 60, 120}

func NewPrometheusBuilder(namespace, subsystem, help string) *PrometheusBuilder {
	return &PrometheusBuilder{
		namespace: namespace,
		subsystem: subsystem,
		help:      help,
		buckets:   defaultDurationBuckets,
		registry:  prometheus.DefaultRegisterer,
	}
}

func (b *PrometheusBuilder) Registry(r prometheus.Registerer) *PrometheusBuilder {
	b.registry = r
	return b
}

func (b *PrometheusBuilder) Buckets(buckets []float64) *PrometheusBuilder {
	b.buckets = buckets
	return b
}

// Build 注册四组指标，返回供 Job 埋点的 *Metrics 句柄。
func (b *PrometheusBuilder) Build() *Metrics {
	m := &Metrics{
		runs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: b.namespace,
			Subsystem: b.subsystem,
			Name:      "runs_total",
			Help:      b.help + "（执行次数：result=success/failed/skipped/panic）",
		}, []string{"task", "result"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: b.namespace,
			Subsystem: b.subsystem,
			Name:      "duration_seconds",
			Help:      b.help + "（耗时分布）",
			Buckets:   b.buckets,
		}, []string{"task"}),
		inFlight: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: b.namespace,
			Subsystem: b.subsystem,
			Name:      "in_flight",
			Help:      b.help + "（当前正在执行的任务数）",
		}, []string{"task"}),
		lastSuccess: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: b.namespace,
			Subsystem: b.subsystem,
			Name:      "last_success_timestamp",
			Help:      b.help + "（上次成功完成的 Unix 秒戳）",
		}, []string{"task"}),
	}
	b.registry.MustRegister(m.runs, m.duration, m.inFlight, m.lastSuccess)
	return m
}

// Metrics 任务级指标句柄，一个 *Metrics 在所有 Job 间共享，task 标签区分。
type Metrics struct {
	runs        *prometheus.CounterVec
	duration    *prometheus.HistogramVec
	inFlight    *prometheus.GaugeVec
	lastSuccess *prometheus.GaugeVec
}

func (m *Metrics) Runs(task, result string) prometheus.Counter {
	return m.runs.WithLabelValues(task, result)
}

func (m *Metrics) Duration(task string) prometheus.Observer {
	return m.duration.WithLabelValues(task)
}

func (m *Metrics) InFlight(task string) prometheus.Gauge {
	return m.inFlight.WithLabelValues(task)
}

// MarkSuccess 设置 last_success_timestamp 为当前时间，任务成功收尾时调用一次。
func (m *Metrics) MarkSuccess(task string) {
	m.lastSuccess.WithLabelValues(task).Set(float64(time.Now().Unix()))
}
