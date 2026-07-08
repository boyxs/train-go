package prometheus

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/boyxs/train-go/webook/pkg/redislock"
)

// Builder 锁指标装饰器构造器接口。
type Builder interface {
	Build(inner redislock.Client) redislock.Client
}

// PrometheusBuilder 分布式锁 Prometheus 指标装饰器。
//
// namespace + subsystem + help + 链式 Registry/Buckets + 终端 Build。
// 指标命名 webook_lock_*（subsystem=lock），service 区分靠 prometheus 注入的 job label。
//
// 默认输出（cron / 业务都全要）：
//   - {ns}_{sub}_acquire_total       (Counter, result=success/busy/error)
//   - {ns}_{sub}_held_seconds        (Histogram, 无标签；key 基数高不打)
//   - {ns}_{sub}_wait_seconds        (Histogram, 阻塞 Lock 等待时长)
//   - {ns}_{sub}_watchdog_lost_total (Counter, watchdog 续约失败 = 幻觉持锁告警)
type PrometheusBuilder struct {
	namespace string
	subsystem string
	help      string
	buckets   []float64
	registry  prometheus.Registerer
}

// 默认桶：cron 任务级锁通常持有 1s~2min；首段密一些观察短锁，尾段稀疏盖住超时锁。
var defaultHeldBuckets = []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60, 120}

// wait_seconds 桶：阻塞 Lock 等待时长，多数应亚秒，超过 1s 就该告警。
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

// Build 包装 inner 为带指标的 Client。除了 acquire/held/wait，还会自动给每次 TryLock/Lock
// 注入一个 OnLost 回调，watchdog 续约失败时累加 watchdog_lost_total。
func (b *PrometheusBuilder) Build(inner redislock.Client) redislock.Client {
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
	fenceIssued := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: b.namespace,
		Subsystem: b.subsystem,
		Name:      "fence_issued_total",
		Help:      b.help + "（fencing 令牌发放次数：每次全新获取 +1，重入不计）",
	})
	b.registry.MustRegister(acquire, held, wait, watchdogLost, fenceIssued)

	return &MetricsClient{
		inner:        inner,
		acquire:      acquire,
		held:         held,
		wait:         wait,
		watchdogLost: watchdogLost,
		fenceIssued:  fenceIssued,
	}
}

// MetricsClient 实现 redislock.Client；切点：acquire / unlock / wait / watchdog-lost。
type MetricsClient struct {
	inner        redislock.Client
	acquire      *prometheus.CounterVec
	held         prometheus.Histogram
	wait         prometheus.Histogram
	watchdogLost prometheus.Counter
	fenceIssued  prometheus.Counter
}

// withOnLost 把"watchdog 续约失败 → 计数器 +1"作为默认 Option 注入到调用方 opts 前面；
// 调用方自己若也传 WithOnLost，按 Options for-range 顺序后写入会覆盖默认。
func (c *MetricsClient) withOnLost(opts []redislock.Options) []redislock.Options {
	defaultOnLost := redislock.WithOnLost(func(string, error) { c.watchdogLost.Inc() })
	return append([]redislock.Options{defaultOnLost}, opts...)
}

func (c *MetricsClient) TryLock(ctx context.Context, key string, opts ...redislock.Options) (redislock.RedisLock, bool, error) {
	lock, ok, err := c.inner.TryLock(ctx, key, c.withOnLost(opts)...)
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

func (c *MetricsClient) Lock(ctx context.Context, key string, opts ...redislock.Options) (redislock.RedisLock, error) {
	start := time.Now()
	lock, err := c.inner.Lock(ctx, key, c.withOnLost(opts)...)
	c.wait.Observe(time.Since(start).Seconds())
	if err != nil {
		c.acquire.WithLabelValues("error").Inc()
		return nil, err
	}
	c.acquire.WithLabelValues("success").Inc()
	return c.wrap(lock), nil
}

func (c *MetricsClient) wrap(lock redislock.RedisLock) redislock.RedisLock {
	if lock.Fence() > 0 { // 全新 fencing 获取才发放令牌（重入 Fence()=0）
		c.fenceIssued.Inc()
	}
	return &ObservedLock{RedisLock: lock, held: c.held, acquiredAt: time.Now()}
}

// ObservedLock 代理真锁，Unlock 时观测持有时长；其余方法透传内嵌 RedisLock。
type ObservedLock struct {
	redislock.RedisLock
	held       prometheus.Histogram
	acquiredAt time.Time
}

func (o *ObservedLock) Unlock(ctx context.Context) error {
	err := o.RedisLock.Unlock(ctx)
	o.held.Observe(time.Since(o.acquiredAt).Seconds())
	return err
}
