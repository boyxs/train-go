package metrics

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
)

// Builder 指标中间件构造器接口
type Builder interface {
	Build() gin.HandlerFunc
}

// PrometheusBuilder Prometheus 实现的 HTTP 指标中间件构造器
//
// 按需启用：
//   - WithCounter()   → {name}_total (Counter, 标签: method/path/status)
//   - WithHistogram() → {name}_duration_seconds (Histogram, 标签: method/path)
//   - WithSummary()   → {name}_duration_seconds_summary (Summary, 标签: method/path)
//   - WithInFlight()  → {name}_in_flight (Gauge, 标签: method/path)
type PrometheusBuilder struct {
	namespace string
	subsystem string
	name      string
	help      string
	buckets   []float64
	objectives map[float64]float64
	registry  prometheus.Registerer

	enableCounter   bool
	enableHistogram bool
	enableSummary   bool
	enableInFlight  bool
}

var defaultBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5}

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

// Registry 自定义注册表（测试用）
func (b *PrometheusBuilder) Registry(r prometheus.Registerer) *PrometheusBuilder {
	b.registry = r
	return b
}

// Buckets 自定义 histogram 桶
func (b *PrometheusBuilder) Buckets(buckets []float64) *PrometheusBuilder {
	b.buckets = buckets
	return b
}

// Objectives 自定义 summary 分位数
func (b *PrometheusBuilder) Objectives(obj map[float64]float64) *PrometheusBuilder {
	b.objectives = obj
	return b
}

func (b *PrometheusBuilder) fullPath(ctx *gin.Context) string {
	path := ctx.FullPath()
	if path == "" {
		path = "unknown"
	}
	return path
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

func (b *PrometheusBuilder) WithInFlight() *PrometheusBuilder {
	b.enableInFlight = true
	return b
}

func (b *PrometheusBuilder) Build() gin.HandlerFunc {
	var counter *prometheus.CounterVec
	var histogram *prometheus.HistogramVec
	var summary *prometheus.SummaryVec
	var inflight *prometheus.GaugeVec

	if b.enableCounter {
		counter = prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: b.namespace,
			Subsystem: b.subsystem,
			Name:      b.name + "_total",
			Help:      b.help,
		}, []string{"method", "path", "status"})
		b.registry.MustRegister(counter)
	}

	if b.enableHistogram {
		histogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: b.namespace,
			Subsystem: b.subsystem,
			Name:      b.name + "_duration_seconds",
			Help:      b.help + "（耗时分布）",
			Buckets:   b.buckets,
		}, []string{"method", "path"})
		b.registry.MustRegister(histogram)
	}

	if b.enableSummary {
		summary = prometheus.NewSummaryVec(prometheus.SummaryOpts{
			Namespace:  b.namespace,
			Subsystem:  b.subsystem,
			Name:       b.name + "_duration_seconds_summary",
			Help:       b.help + "（分位数）",
			Objectives: b.objectives,
		}, []string{"method", "path"})
		b.registry.MustRegister(summary)
	}

	if b.enableInFlight {
		inflight = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: b.namespace,
			Subsystem: b.subsystem,
			Name:      b.name + "_in_flight",
			Help:      b.help + "（正在处理中）",
		}, []string{"method", "path"})
		b.registry.MustRegister(inflight)
	}

	return func(ctx *gin.Context) {
		start := time.Now()
		if inflight != nil {
			inflight.WithLabelValues(ctx.Request.Method, b.fullPath(ctx)).Inc()
		}

		defer func() {
			path := b.fullPath(ctx)
			method := ctx.Request.Method
			duration := time.Since(start).Seconds()

			if counter != nil {
				counter.WithLabelValues(method, path, strconv.Itoa(ctx.Writer.Status())).Inc()
			}
			if histogram != nil {
				histogram.WithLabelValues(method, path).Observe(duration)
			}
			if summary != nil {
				summary.WithLabelValues(method, path).Observe(duration)
			}
			if inflight != nil {
				inflight.WithLabelValues(method, path).Dec()
			}
		}()

		ctx.Next()
	}
}
