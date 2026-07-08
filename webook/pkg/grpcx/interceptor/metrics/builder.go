// Package metrics 提供 gRPC 指标拦截器：请求量 / 耗时直方图 / 在途请求数。
// 指标统一命名 webook_<subsystem>_*；标签 type(server/client) / service / method / peer / code。
package metrics

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"

	"github.com/boyxs/train-go/webook/pkg/grpcx/interceptor"
)

// Builder 构造 server / client 两侧的指标采集拦截器。
type Builder interface {
	BuildUnaryServer() grpc.UnaryServerInterceptor
	BuildUnaryClient() grpc.UnaryClientInterceptor
}

// PrometheusBuilder 是 Builder 的 Prometheus 实现。
//
// 按需启用：
//   - WithCounter()   → {name}_total (Counter, 标签 type/service/method/code/peer)
//   - WithHistogram() → {name}_duration_seconds (Histogram, 标签 type/service/method)
//   - WithSummary()   → {name}_duration_seconds_summary (Summary, ...)
//   - WithInFlight()  → {name}_in_flight (Gauge, ...)
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
	enableInFlight  bool

	once      sync.Once
	counter   *prometheus.CounterVec
	histogram *prometheus.HistogramVec
	summary   *prometheus.SummaryVec
	inflight  *prometheus.GaugeVec
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

func (b *PrometheusBuilder) WithInFlight() *PrometheusBuilder {
	b.enableInFlight = true
	return b
}

func (b *PrometheusBuilder) BuildUnaryServer() grpc.UnaryServerInterceptor {
	b.ensure()
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		done := b.start(ctx, "server", info.FullMethod)
		resp, err := handler(ctx, req)
		done(err)
		return resp, err
	}
}

func (b *PrometheusBuilder) BuildUnaryClient() grpc.UnaryClientInterceptor {
	b.ensure()
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		done := b.start(ctx, "client", method)
		err := invoker(ctx, method, req, reply, cc, opts...)
		done(err)
		return err
	}
}

// start 在请求开始时记录在途、计时，返回的闭包在请求结束时落各项指标。
func (b *PrometheusBuilder) start(ctx context.Context, typ, fullMethod string) func(error) {
	service, method := splitMethod(fullMethod)
	peer := interceptor.PeerName(ctx)
	begin := time.Now()
	if b.inflight != nil {
		b.inflight.WithLabelValues(typ, service, method).Inc()
	}
	return func(err error) {
		if b.inflight != nil {
			b.inflight.WithLabelValues(typ, service, method).Dec()
		}
		dur := time.Since(begin).Seconds()
		if b.histogram != nil {
			b.histogram.WithLabelValues(typ, service, method).Observe(dur)
		}
		if b.summary != nil {
			b.summary.WithLabelValues(typ, service, method).Observe(dur)
		}
		if b.counter != nil {
			b.counter.WithLabelValues(typ, service, method, peer, status.Code(err).String()).Inc()
		}
	}
}

// ensure 按开关创建并注册启用的 collector（once 保证 server/client 共享一套、只注册一次）。
func (b *PrometheusBuilder) ensure() {
	b.once.Do(func() {
		base := []string{"type", "service", "method"}
		if b.enableCounter {
			b.counter = prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: b.namespace,
				Subsystem: b.subsystem,
				Name:      b.name + "_total",
				Help:      b.help,
			}, []string{"type", "service", "method", "peer", "code"})
			b.registry.MustRegister(b.counter)
		}
		if b.enableHistogram {
			b.histogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
				Namespace: b.namespace,
				Subsystem: b.subsystem,
				Name:      b.name + "_duration_seconds",
				Help:      b.help + "（耗时分布）",
				Buckets:   b.buckets,
			}, base)
			b.registry.MustRegister(b.histogram)
		}
		if b.enableSummary {
			b.summary = prometheus.NewSummaryVec(prometheus.SummaryOpts{
				Namespace:  b.namespace,
				Subsystem:  b.subsystem,
				Name:       b.name + "_duration_seconds_summary",
				Help:       b.help + "（分位数）",
				Objectives: b.objectives,
			}, base)
			b.registry.MustRegister(b.summary)
		}
		if b.enableInFlight {
			b.inflight = prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: b.namespace,
				Subsystem: b.subsystem,
				Name:      b.name + "_in_flight",
				Help:      b.help + "（在途）",
			}, base)
			b.registry.MustRegister(b.inflight)
		}
	})
}

// splitMethod 拆 full method：/pkg.Service/Method → (pkg.Service, Method)
func splitMethod(fullMethod string) (service, method string) {
	s := strings.TrimPrefix(fullMethod, "/")
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}
