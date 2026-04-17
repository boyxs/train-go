package prometheus

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"gorm.io/gorm"
)

// Builder GORM 指标 callback 注册器接口
type Builder interface {
	Register(db *gorm.DB) error
}

// PrometheusBuilder GORM Prometheus 指标 callback 注册器
//
// 按需启用：
//   - WithCounter()   → {name}_total (Counter, 标签: type/table)
//   - WithHistogram() → {name}_duration_seconds (Histogram, 标签: type/table)
//   - WithSummary()   → {name}_duration_seconds_summary (Summary, 标签: type/table)
//
// type 取值：query/create/update/delete/raw/row
// table 取值：Statement.Table，无表名时 fallback "unknown"
type PrometheusBuilder struct {
	namespace  string
	subsystem  string
	name       string
	help       string
	buckets    []float64
	objectives map[float64]float64
	registry   prometheus.Registerer

	enableCounter   bool
	enableHistogram bool
	enableSummary   bool
}

// 默认桶：DB 操作通常 < 1s，桶比 HTTP 紧密
var defaultBuckets = []float64{0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5}

// 默认 Summary 分位数
var defaultObjectives = map[float64]float64{
	0.5:  0.05,
	0.9:  0.01,
	0.95: 0.005,
	0.99: 0.001,
}

func NewPrometheusBuilder(namespace, subsystem, name, help string) *PrometheusBuilder {
	return &PrometheusBuilder{
		namespace:  namespace,
		subsystem:  subsystem,
		name:       name,
		help:       help,
		buckets:    defaultBuckets,
		objectives: defaultObjectives,
		registry:   prometheus.DefaultRegisterer,
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

func (b *PrometheusBuilder) Objectives(obj map[float64]float64) *PrometheusBuilder {
	b.objectives = obj
	return b
}

func (b *PrometheusBuilder) WithCounter() *PrometheusBuilder {
	b.enableCounter = true
	return b
}

func (b *PrometheusBuilder) WithHistogram() *PrometheusBuilder {
	b.enableHistogram = true
	return b
}

func (b *PrometheusBuilder) WithSummary() *PrometheusBuilder {
	b.enableSummary = true
	return b
}

// Register 把 callback 注册到 GORM
// 启动时一次性调用，失败返回 error 由调用方决定 panic 还是降级
func (b *PrometheusBuilder) Register(db *gorm.DB) error {
	var counter *prometheus.CounterVec
	var histogram *prometheus.HistogramVec
	var summary *prometheus.SummaryVec

	if b.enableCounter {
		counter = prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: b.namespace,
			Subsystem: b.subsystem,
			Name:      b.name + "_total",
			Help:      b.help,
		}, []string{"type", "table"})
		b.registry.MustRegister(counter)
	}

	if b.enableHistogram {
		histogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: b.namespace,
			Subsystem: b.subsystem,
			Name:      b.name + "_duration_seconds",
			Help:      b.help + "（耗时分布）",
			Buckets:   b.buckets,
		}, []string{"type", "table"})
		b.registry.MustRegister(histogram)
	}

	if b.enableSummary {
		summary = prometheus.NewSummaryVec(prometheus.SummaryOpts{
			Namespace:  b.namespace,
			Subsystem:  b.subsystem,
			Name:       b.name + "_duration_seconds_summary",
			Help:       b.help + "（分位数）",
			Objectives: b.objectives,
		}, []string{"type", "table"})
		b.registry.MustRegister(summary)
	}

	// 没启用任何指标，回调也没必要注册
	if counter == nil && histogram == nil && summary == nil {
		return nil
	}

	before := func(db *gorm.DB) {
		db.InstanceSet("prom_start", time.Now())
	}

	after := func(typ string) func(*gorm.DB) {
		return func(db *gorm.DB) {
			v, ok := db.InstanceGet("prom_start")
			if !ok {
				return
			}
			start, ok := v.(time.Time)
			if !ok {
				return
			}
			table := db.Statement.Table
			if table == "" {
				table = "unknown"
			}
			duration := time.Since(start).Seconds()
			if counter != nil {
				counter.WithLabelValues(typ, table).Inc()
			}
			if histogram != nil {
				histogram.WithLabelValues(typ, table).Observe(duration)
			}
			if summary != nil {
				summary.WithLabelValues(typ, table).Observe(duration)
			}
		}
	}

	cb := db.Callback()
	type pair struct {
		typ      string
		register func(name string, fn func(*gorm.DB)) error
	}
	pairs := []pair{
		{"create", cb.Create().Before("*").Register},
		{"query", cb.Query().Before("*").Register},
		{"update", cb.Update().Before("*").Register},
		{"delete", cb.Delete().Before("*").Register},
		{"raw", cb.Raw().Before("*").Register},
		{"row", cb.Row().Before("*").Register},
	}
	for _, p := range pairs {
		if err := p.register("prometheus:"+p.typ+":before", before); err != nil {
			return err
		}
	}

	afters := []pair{
		{"create", cb.Create().After("*").Register},
		{"query", cb.Query().After("*").Register},
		{"update", cb.Update().After("*").Register},
		{"delete", cb.Delete().After("*").Register},
		{"raw", cb.Raw().After("*").Register},
		{"row", cb.Row().After("*").Register},
	}
	for _, p := range afters {
		if err := p.register("prometheus:"+p.typ+":after", after(p.typ)); err != nil {
			return err
		}
	}

	return nil
}
