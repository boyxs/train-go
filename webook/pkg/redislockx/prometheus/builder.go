package prometheus

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/webook/pkg/redislockx"
)

// Builder 锁指标装饰器构造器接口
type Builder interface {
	Build(inner redislockx.Client) redislockx.Client
}

// PrometheusBuilder 分布式锁 Prometheus 指标装饰器
//
// namespace + subsystem + help + 链式 Registry/Buckets + 终端 Build。
//
// 默认输出两组指标（cron / 业务都全要，没有"按需启用"的真实场景）：
//   - {namespace}_{subsystem}_acquire_total (Counter, 标签: result=success/busy/error)
//   - {namespace}_{subsystem}_held_seconds  (Histogram, 无标签；key 基数高不打)
type PrometheusBuilder struct {
	namespace string
	subsystem string
	help      string
	buckets   []float64
	registry  prometheus.Registerer
}

// 默认桶：cron 任务级锁通常持有 1s~2min；首段密一些观察短锁，尾段稀疏盖住超时锁
var defaultHeldBuckets = []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60, 120}

// wait_seconds 桶：阻塞 Lock 等待时长，多数应该亚秒，超过 1s 就该告警
var defaultWaitBuckets = []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5}

func NewPrometheusBuilder(namespace, subsystem, help string) *PrometheusBuilder {
	return &PrometheusBuilder{
		namespace: namespace,
		subsystem: subsystem,
		help:      help,
		buckets:   defaultHeldBuckets,
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

// Build 包装 inner 为带指标的 Client。除了 acquire/held，还会自动给每次 TryLock/Lock
// 注入一个 OnLost 回调，watchdog 续约失败时累加 watchdog_lost_total。
func (b *PrometheusBuilder) Build(inner redislockx.Client) redislockx.Client {
	acquire := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: b.namespace,
		Subsystem: b.subsystem,
		Name:      "acquire_total",
		Help:      b.help + "（抢锁次数：result=success/busy/error）",
	}, []string{"result"})
	held := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: b.namespace,
		Subsystem: b.subsystem,
		Name:      "held_seconds",
		Help:      b.help + "（锁持有时长，Unlock 时观测）",
		Buckets:   b.buckets,
	})
	wait := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: b.namespace,
		Subsystem: b.subsystem,
		Name:      "wait_seconds",
		Help:      b.help + "（阻塞 Lock 实际等待时长，含失败）",
		Buckets:   defaultWaitBuckets,
	})
	watchdogLost := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: b.namespace,
		Subsystem: b.subsystem,
		Name:      "watchdog_lost_total",
		Help:      b.help + "（watchdog 续约失败次数：锁中途丢失，等价于幻觉持锁告警）",
	})
	b.registry.MustRegister(acquire, held, wait, watchdogLost)

	return &metricsClient{
		inner:        inner,
		acquire:      acquire,
		held:         held,
		wait:         wait,
		watchdogLost: watchdogLost,
	}
}

// metricsClient 实现 redislockx.Client；切点：acquire / unlock / wait / watchdog-lost。
type metricsClient struct {
	inner        redislockx.Client
	acquire      *prometheus.CounterVec
	held         prometheus.Histogram
	wait         prometheus.Histogram
	watchdogLost prometheus.Counter
}

// withOnLost 把"watchdog 续约失败 → 计数器 +1"作为默认 Option 注入到调用方的 opts 前面，
// 调用方自己若也传了 WithOnLost，按 Options for-range 顺序后写入会覆盖默认。
func (c *metricsClient) withOnLost(opts []redislockx.Options) []redislockx.Options {
	defaultOnLost := redislockx.WithOnLost(func(string, error) { c.watchdogLost.Inc() })
	return append([]redislockx.Options{defaultOnLost}, opts...)
}

func (c *metricsClient) TryLock(ctx context.Context, key string, ttl time.Duration, opts ...redislockx.Options) (redislockx.Lock, bool, error) {
	lock, ok, err := c.inner.TryLock(ctx, key, ttl, c.withOnLost(opts)...)
	switch {
	case err != nil:
		c.acquire.WithLabelValues("error").Inc()
		return nil, false, err
	case !ok:
		c.acquire.WithLabelValues("busy").Inc()
		return nil, false, nil
	default:
		c.acquire.WithLabelValues("success").Inc()
		return c.wrap(lock), true, nil
	}
}

func (c *metricsClient) Lock(ctx context.Context, key string, ttl time.Duration, opts ...redislockx.Options) (redislockx.Lock, error) {
	start := time.Now()
	lock, err := c.inner.Lock(ctx, key, ttl, c.withOnLost(opts)...)
	c.wait.Observe(time.Since(start).Seconds())
	if err != nil {
		c.acquire.WithLabelValues("error").Inc()
		return nil, err
	}
	c.acquire.WithLabelValues("success").Inc()
	return c.wrap(lock), nil
}

func (c *metricsClient) wrap(lock redislockx.Lock) redislockx.Lock {
	return &observedLock{Lock: lock, held: c.held, acquiredAt: time.Now()}
}

// observedLock 代理真锁，Unlock 时观测持有时长
type observedLock struct {
	redislockx.Lock
	held       prometheus.Histogram
	acquiredAt time.Time
}

func (o *observedLock) Unlock(ctx context.Context) error {
	err := o.Lock.Unlock(ctx)
	o.held.Observe(time.Since(o.acquiredAt).Seconds())
	return err
}
