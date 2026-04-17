package prometheus

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
)

// Builder Redis 指标 Hook 注册器接口
type Builder interface {
	Build() redis.Hook
}

// PrometheusBuilder Redis Prometheus 指标 Hook
//
// 按需启用：
//   - WithCounter()   → {name}_total (Counter, 标签: cmd/biz/hit)
//   - WithHistogram() → {name}_duration_seconds (Histogram, 标签: cmd/biz)
//   - WithSummary()   → {name}_duration_seconds_summary (Summary, 标签: cmd/biz)
//
// cmd：命令名（get/set/del/hset 等），取 Cmder.Name() 小写
// biz：业务名，从第一个 key 的 ":" 前缀解析（user/article/interaction/chat 等）
//      多 key 命令（MGET 等）只按第一个 key 归类
// hit：仅 Counter 有，按命令返回类型判断：
//      - nilCmds（get/hget/zscore 等）：redis.Nil → "false"
//      - collectionCmds（hgetall/smembers/lrange 等）：空集合 → "false"
//      - intCmds（exists/hexists/sismember）：0/false → "false"
//      - 写命令 → ""（不参与命中率）
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

// 默认桶：Redis 操作通常 < 100ms
var defaultBuckets = []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1}

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

// nilCmds：key/field 不存在时返回 redis.Nil 的命令
// 注：hrandfield/srandmember 带 count 参数时返回 slice，命中率语义不明，不纳入
var nilCmds = map[string]bool{
	"get": true, "getset": true, "getdel": true, "getex": true,
	"hget": true,
	"lindex": true,
	"zscore": true, "zrank": true, "zrevrank": true,
}

// collectionCmds：返回集合/map 的命令，key 不存在时返回空集合而非 Nil
var collectionCmds = map[string]bool{
	"hgetall": true, "hmget": true, "hkeys": true, "hvals": true,
	"smembers": true, "sinter": true, "sunion": true, "sdiff": true,
	"lrange": true,
	"zrange": true, "zrangebyscore": true, "zrevrange": true, "zrevrangebyscore": true,
	"mget": true,
	"xrange": true, "xrevrange": true,
}

// intCmds：返回 *IntCmd / *BoolCmd 的查询类命令
// 存在检查（exists/hexists/sismember）：0/false → miss
// 长度类（llen/hlen/scard/zcard）：0 → key 不存在或空集合，语义上也是 miss
// 注：bitcount/strlen/xlen 归 0 有歧义（空值 vs 不存在），不纳入命中率
var intCmds = map[string]bool{
	"exists": true, "hexists": true, "sismember": true,
	"llen": true, "hlen": true, "scard": true, "zcard": true,
}

// durationCmds：返回 *DurationCmd 的命令，-2 表示 key 不存在（miss）
// TTL 返回 -2=不存在, -1=无过期, >0=剩余秒数（后两者均视为 hit）
var durationCmds = map[string]bool{
	"ttl": true, "pttl": true,
}

// statusCmds：返回 *StatusCmd 的命令，"none" 表示 key 不存在（miss）
// TYPE 返回 "none"=不存在, "string"/"hash"/"list"/... = 存在
var statusCmds = map[string]bool{
	"type": true,
}

func (b *PrometheusBuilder) Build() redis.Hook {
	var counter *prometheus.CounterVec
	var histogram *prometheus.HistogramVec
	var summary *prometheus.SummaryVec

	if b.enableCounter {
		// Counter 多一个 hit 标签
		counter = prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: b.namespace,
			Subsystem: b.subsystem,
			Name:      b.name + "_total",
			Help:      b.help,
		}, []string{"cmd", "biz", "hit"})
		b.registry.MustRegister(counter)
	}

	if b.enableHistogram {
		histogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: b.namespace,
			Subsystem: b.subsystem,
			Name:      b.name + "_duration_seconds",
			Help:      b.help + "（耗时分布）",
			Buckets:   b.buckets,
		}, []string{"cmd", "biz"})
		b.registry.MustRegister(histogram)
	}

	if b.enableSummary {
		summary = prometheus.NewSummaryVec(prometheus.SummaryOpts{
			Namespace:  b.namespace,
			Subsystem:  b.subsystem,
			Name:       b.name + "_duration_seconds_summary",
			Help:       b.help + "（分位数）",
			Objectives: b.objectives,
		}, []string{"cmd", "biz"})
		b.registry.MustRegister(summary)
	}

	return &hook{
		counter:   counter,
		histogram: histogram,
		summary:   summary,
	}
}

type hook struct {
	counter   *prometheus.CounterVec
	histogram *prometheus.HistogramVec
	summary   *prometheus.SummaryVec
}

func (h *hook) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return next(ctx, network, addr)
	}
}

func (h *hook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		start := time.Now()
		err := next(ctx, cmd)
		duration := time.Since(start).Seconds()
		cmdName := strings.ToLower(cmd.Name())
		biz := extractBiz(cmd)
		hit := isHit(cmdName, cmd, err)

		if h.counter != nil {
			h.counter.WithLabelValues(cmdName, biz, hit).Inc()
		}
		if h.histogram != nil {
			h.histogram.WithLabelValues(cmdName, biz).Observe(duration)
		}
		if h.summary != nil {
			h.summary.WithLabelValues(cmdName, biz).Observe(duration)
		}
		return err
	}
}

func (h *hook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		start := time.Now()
		err := next(ctx, cmds)
		duration := time.Since(start).Seconds()

		// 区分 pipeline 和 transaction：事务首条命令是 MULTI
		cmdLabel := "pipeline"
		if len(cmds) > 0 && strings.ToLower(cmds[0].Name()) == "multi" {
			cmdLabel = "transaction"
		}

		if h.counter != nil {
			h.counter.WithLabelValues(cmdLabel, "mixed", "").Inc()
		}
		if h.histogram != nil {
			h.histogram.WithLabelValues(cmdLabel, "mixed").Observe(duration)
		}
		if h.summary != nil {
			h.summary.WithLabelValues(cmdLabel, "mixed").Observe(duration)
		}
		return err
	}
}

// extractBiz 从命令的第一个 key 参数解析业务前缀
// "user:123" → "user"，"article:pub:1" → "article"
// 无 key 或解析不出 → "unknown"
// 多 key 命令（MGET、DEL k1 k2）：只按第一个 key 归类，MSET 这种跨业务的场景会被错分
func extractBiz(cmd redis.Cmder) string {
	args := cmd.Args()
	if len(args) < 2 {
		return "unknown"
	}
	key, ok := args[1].(string)
	if !ok || key == "" {
		return "unknown"
	}
	idx := strings.Index(key, ":")
	if idx <= 0 {
		return key
	}
	return key[:idx]
}

// isHit 判断读命令是否命中缓存
//   - nilCmds（get/hget 等）：redis.Nil → miss
//   - collectionCmds（hgetall/smembers 等）：空集合 → miss
//   - intCmds（exists/hexists/sismember/llen 等）：0/false → miss
//   - durationCmds（ttl/pttl）：值 == -2 → miss
//   - statusCmds（type）："none" → miss
//   - 非读命令 → ""（不参与命中率统计）
func isHit(cmdName string, cmd redis.Cmder, err error) string {
	if nilCmds[cmdName] {
		if errors.Is(err, redis.Nil) {
			return "false"
		}
		return "true"
	}
	if collectionCmds[cmdName] {
		return isCollectionHit(cmd)
	}
	if intCmds[cmdName] {
		return isIntHit(cmd)
	}
	if durationCmds[cmdName] {
		return isDurationHit(cmd)
	}
	if statusCmds[cmdName] {
		return isStatusHit(cmd)
	}
	return ""
}

// isIntHit 检查命令返回值：IntCmd 的 0 或 BoolCmd 的 false → miss
func isIntHit(cmd redis.Cmder) string {
	switch c := cmd.(type) {
	case *redis.IntCmd: // exists/llen/hlen/scard/zcard/strlen/xlen/bitcount
		val, err := c.Result()
		if err != nil || val == 0 {
			return "false"
		}
		return "true"
	case *redis.BoolCmd: // hexists, sismember
		val, err := c.Result()
		if err != nil || !val {
			return "false"
		}
		return "true"
	}
	return ""
}

// isDurationHit TTL/PTTL 返回 -2 表示 key 不存在
// 真 Redis 返回 -2 * time.Second（TTL）或 -2 * time.Millisecond（PTTL）
// miniredis 直接返回 -2 ns
// 统一判断：< 0 的值中只有 -2 表示 key 不存在
func isDurationHit(cmd redis.Cmder) string {
	c, ok := cmd.(*redis.DurationCmd)
	if !ok {
		return ""
	}
	val, err := c.Result()
	if err != nil {
		return "false"
	}
	// -2 * 单位 → key 不存在；-1 * 单位 → 有 key 无过期
	if val == -2*time.Second || val == -2*time.Millisecond || val == -2 {
		return "false"
	}
	return "true"
}

// isStatusHit TYPE 返回 "none" 表示 key 不存在
func isStatusHit(cmd redis.Cmder) string {
	c, ok := cmd.(*redis.StatusCmd)
	if !ok {
		return ""
	}
	val, err := c.Result()
	if err != nil || val == "none" {
		return "false"
	}
	return "true"
}

// isCollectionHit 检查集合类命令返回是否为空
func isCollectionHit(cmd redis.Cmder) string {
	switch c := cmd.(type) {
	case *redis.MapStringStringCmd: // hgetall
		if val, err := c.Result(); err != nil || len(val) == 0 {
			return "false"
		}
	case *redis.StringSliceCmd: // smembers, lrange, zrange, hkeys, hvals
		if val, err := c.Result(); err != nil || len(val) == 0 {
			return "false"
		}
	case *redis.SliceCmd: // mget, hmget
		if val, err := c.Result(); err != nil || len(val) == 0 {
			return "false"
		}
	case *redis.XMessageSliceCmd: // xrange, xrevrange
		if val, err := c.Result(); err != nil || len(val) == 0 {
			return "false"
		}
	default:
		return ""
	}
	return "true"
}
