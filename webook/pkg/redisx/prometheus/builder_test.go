package prometheus

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRedis(t *testing.T, reg *prometheus.Registry) redis.Cmdable {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: s.Addr()})
	client.AddHook(NewPrometheusBuilder("webook", "redis", "cmd", "test").
		Registry(reg).
		WithCounter().
		WithHistogram().
		WithSummary().
		Build())
	t.Cleanup(func() { client.Close() })
	return client
}

func TestCounter_CmdAndBizLabels(t *testing.T) {
	reg := prometheus.NewRegistry()
	rdb := newTestRedis(t, reg)
	ctx := context.Background()

	rdb.Set(ctx, "user:1", "v", 0)
	rdb.Set(ctx, "article:pub:1", "v", 0)
	rdb.Get(ctx, "chat:conv:list:1")

	// labels 顺序: biz, cmd, hit
	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_redis_cmd_total", "user", "set", ""))
	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_redis_cmd_total", "article", "set", ""))
	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_redis_cmd_total", "chat", "get", "false"))
}

func TestCounter_HitMiss(t *testing.T) {
	reg := prometheus.NewRegistry()
	rdb := newTestRedis(t, reg)
	ctx := context.Background()

	rdb.Set(ctx, "user:1", "hello", 0)
	rdb.Get(ctx, "user:1")
	rdb.Get(ctx, "user:1")
	rdb.Get(ctx, "user:999")

	// user biz 的 get hit=true 计数应为 2，hit=false 计数应为 1
	assert.Equal(t, 2.0, getCounterValue(t, reg, "webook_redis_cmd_total", "user", "get", "true"))
	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_redis_cmd_total", "user", "get", "false"))
	// set 是写命令，hit 为空
	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_redis_cmd_total", "user", "set", ""))
}

func TestCounter_HitMiss_HGetAll(t *testing.T) {
	reg := prometheus.NewRegistry()
	rdb := newTestRedis(t, reg)
	ctx := context.Background()

	rdb.HSet(ctx, "interaction:article:1", "like_count", "5")
	rdb.HGetAll(ctx, "interaction:article:1")   // hit（非空 map）
	rdb.HGetAll(ctx, "interaction:article:999") // miss（空 map）

	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_redis_cmd_total", "interaction", "hgetall", "true"))
	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_redis_cmd_total", "interaction", "hgetall", "false"))
}

func TestCounter_HitMiss_SMembers(t *testing.T) {
	reg := prometheus.NewRegistry()
	rdb := newTestRedis(t, reg)
	ctx := context.Background()

	rdb.SAdd(ctx, "chat:set:1", "a", "b")
	rdb.SMembers(ctx, "chat:set:1")   // hit
	rdb.SMembers(ctx, "chat:set:999") // miss

	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_redis_cmd_total", "chat", "smembers", "true"))
	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_redis_cmd_total", "chat", "smembers", "false"))
}

func TestCounter_HitMiss_LRange(t *testing.T) {
	reg := prometheus.NewRegistry()
	rdb := newTestRedis(t, reg)
	ctx := context.Background()

	rdb.RPush(ctx, "article:list:1", "a")
	rdb.LRange(ctx, "article:list:1", 0, -1)   // hit
	rdb.LRange(ctx, "article:list:999", 0, -1) // miss

	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_redis_cmd_total", "article", "lrange", "true"))
	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_redis_cmd_total", "article", "lrange", "false"))
}

func TestCounter_HitMiss_Exists(t *testing.T) {
	reg := prometheus.NewRegistry()
	rdb := newTestRedis(t, reg)
	ctx := context.Background()

	rdb.Set(ctx, "user:1", "hello", 0)
	rdb.Exists(ctx, "user:1")   // hit (val=1)
	rdb.Exists(ctx, "user:999") // miss (val=0)

	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_redis_cmd_total", "user", "exists", "true"))
	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_redis_cmd_total", "user", "exists", "false"))
}

func TestCounter_HitMiss_HExists(t *testing.T) {
	reg := prometheus.NewRegistry()
	rdb := newTestRedis(t, reg)
	ctx := context.Background()

	rdb.HSet(ctx, "interaction:1", "like", "5")
	rdb.HExists(ctx, "interaction:1", "like")    // hit
	rdb.HExists(ctx, "interaction:1", "missing") // miss

	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_redis_cmd_total", "interaction", "hexists", "true"))
	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_redis_cmd_total", "interaction", "hexists", "false"))
}

func TestCounter_HitMiss_SIsMember(t *testing.T) {
	reg := prometheus.NewRegistry()
	rdb := newTestRedis(t, reg)
	ctx := context.Background()

	rdb.SAdd(ctx, "article:tags:1", "go")
	rdb.SIsMember(ctx, "article:tags:1", "go")     // hit
	rdb.SIsMember(ctx, "article:tags:1", "python") // miss

	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_redis_cmd_total", "article", "sismember", "true"))
	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_redis_cmd_total", "article", "sismember", "false"))
}

func TestCounter_HitMiss_LengthCmds(t *testing.T) {
	reg := prometheus.NewRegistry()
	rdb := newTestRedis(t, reg)
	ctx := context.Background()

	// LLEN
	rdb.RPush(ctx, "chat:list:1", "msg")
	rdb.LLen(ctx, "chat:list:1")   // hit
	rdb.LLen(ctx, "chat:list:999") // miss (val=0)
	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_redis_cmd_total", "chat", "llen", "true"))
	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_redis_cmd_total", "chat", "llen", "false"))

	// HLEN
	rdb.HSet(ctx, "user:info:1", "name", "a")
	rdb.HLen(ctx, "user:info:1")   // hit
	rdb.HLen(ctx, "user:info:999") // miss
	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_redis_cmd_total", "user", "hlen", "true"))
	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_redis_cmd_total", "user", "hlen", "false"))

	// SCARD / ZCARD / STRLEN 同理，写命令 hit=""
}

func TestCounter_HitMiss_TTL(t *testing.T) {
	reg := prometheus.NewRegistry()
	rdb := newTestRedis(t, reg)
	ctx := context.Background()

	rdb.Set(ctx, "user:1", "v", time.Hour)
	rdb.TTL(ctx, "user:1")   // hit（有过期时间）
	rdb.TTL(ctx, "user:999") // miss（key 不存在，返回 -2）

	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_redis_cmd_total", "user", "ttl", "true"))
	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_redis_cmd_total", "user", "ttl", "false"))
}

func TestCounter_HitMiss_Type(t *testing.T) {
	reg := prometheus.NewRegistry()
	rdb := newTestRedis(t, reg)
	ctx := context.Background()

	rdb.Set(ctx, "user:1", "v", 0)
	rdb.Type(ctx, "user:1")   // hit（返回 "string"）
	rdb.Type(ctx, "user:999") // miss（返回 "none"）

	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_redis_cmd_total", "user", "type", "true"))
	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_redis_cmd_total", "user", "type", "false"))
}

func TestCounter_HitMiss_HKeys(t *testing.T) {
	reg := prometheus.NewRegistry()
	rdb := newTestRedis(t, reg)
	ctx := context.Background()

	rdb.HSet(ctx, "interaction:1", "like", "5")
	rdb.HKeys(ctx, "interaction:1")   // hit
	rdb.HKeys(ctx, "interaction:999") // miss

	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_redis_cmd_total", "interaction", "hkeys", "true"))
	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_redis_cmd_total", "interaction", "hkeys", "false"))
}

func TestCounter_Pipeline(t *testing.T) {
	reg := prometheus.NewRegistry()
	rdb := newTestRedis(t, reg)
	ctx := context.Background()

	// 先预热一次，消耗掉客户端初始化可能触发的 pipeline（HELLO/AUTH 等）
	rdb.Ping(ctx)
	before := getCounterValue(t, reg, "webook_redis_cmd_total", "mixed", "pipeline", "")

	pipe := rdb.Pipeline()
	pipe.Set(ctx, "user:1", "1", 0)
	pipe.Set(ctx, "user:2", "2", 0)
	_, err := pipe.Exec(ctx)
	require.NoError(t, err)

	// 只多了 1 次 pipeline
	after := getCounterValue(t, reg, "webook_redis_cmd_total", "mixed", "pipeline", "")
	assert.Equal(t, 1.0, after-before)
}

func TestCounter_Transaction(t *testing.T) {
	reg := prometheus.NewRegistry()
	rdb := newTestRedis(t, reg)
	ctx := context.Background()

	rdb.Ping(ctx) // 预热

	tx := rdb.TxPipeline()
	tx.Set(ctx, "user:1", "a", 0)
	tx.Set(ctx, "user:2", "b", 0)
	_, err := tx.Exec(ctx)
	require.NoError(t, err)

	// Transaction 独立统计
	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_redis_cmd_total", "mixed", "transaction", ""))
}

func TestExtractBiz(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{"user:123", "user"},
		{"article:pub:1", "article"},
		{"interaction:state:article:1:1", "interaction"},
		{"chat:conv:list:1", "chat"},
		{"embedding:cache:abc", "embedding"},
		{"noprefix", "noprefix"},
		{"", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			cmd := redis.NewCmd(context.Background(), "get", tt.key)
			assert.Equal(t, tt.want, extractBiz(cmd))
		})
	}
}

func TestHistogram_Observed(t *testing.T) {
	reg := prometheus.NewRegistry()
	rdb := newTestRedis(t, reg)
	ctx := context.Background()

	rdb.Set(ctx, "user:1", "v", 0)

	// Histogram 有 1 个样本
	assert.Equal(t, uint64(1), getHistogramCount(t, reg, "webook_redis_cmd_duration_seconds", "user", "set"))
}

func TestSummary_Observed(t *testing.T) {
	reg := prometheus.NewRegistry()
	rdb := newTestRedis(t, reg)
	ctx := context.Background()

	rdb.Set(ctx, "user:1", "v", 0)

	// Summary 有 1 个样本
	assert.Equal(t, uint64(1), getSummaryCount(t, reg, "webook_redis_cmd_duration_seconds_summary", "user", "set"))
}

func TestBuild_OnlyCounter(t *testing.T) {
	reg := prometheus.NewRegistry()
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: s.Addr()})
	client.AddHook(NewPrometheusBuilder("webook", "redis", "cmd", "test").
		Registry(reg).WithCounter().Build())
	defer client.Close()

	client.Set(context.Background(), "user:1", "v", 0)

	// Counter 有数据，histogram 不应注册
	assert.Equal(t, 1.0, getCounterValue(t, reg, "webook_redis_cmd_total", "user", "set", ""))
	assert.Equal(t, 0, testutil.CollectAndCount(reg, "webook_redis_cmd_duration_seconds"))
}

func TestBuild_NoneEnabled(t *testing.T) {
	reg := prometheus.NewRegistry()
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: s.Addr()})
	client.AddHook(NewPrometheusBuilder("webook", "redis", "cmd", "test").
		Registry(reg).Build())
	defer client.Close()

	client.Set(context.Background(), "user:1", "v", 0)

	// 什么都没启用，Registry 内无任何时序
	mfs, err := reg.Gather()
	require.NoError(t, err)
	assert.Empty(t, mfs)
}

// findMetric 按标签找到匹配的 metric（labels 顺序按字母序：biz, cmd, hit）
// 返回 nil 表示未找到
func findMetric(t *testing.T, reg *prometheus.Registry, name string, labels ...string) *metricRow {
	mfs, err := reg.Gather()
	require.NoError(t, err)
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			lps := m.GetLabel()
			if len(lps) != len(labels) {
				continue
			}
			ok := true
			for i := range labels {
				if lps[i].GetValue() != labels[i] {
					ok = false
					break
				}
			}
			if ok {
				return &metricRow{
					counterValue:   m.GetCounter().GetValue(),
					histogramCount: m.GetHistogram().GetSampleCount(),
					summaryCount:   m.GetSummary().GetSampleCount(),
				}
			}
		}
	}
	return nil
}

type metricRow struct {
	counterValue   float64
	histogramCount uint64
	summaryCount   uint64
}

func getCounterValue(t *testing.T, reg *prometheus.Registry, name string, labels ...string) float64 {
	m := findMetric(t, reg, name, labels...)
	if m == nil {
		return 0
	}
	return m.counterValue
}

func getHistogramCount(t *testing.T, reg *prometheus.Registry, name string, labels ...string) uint64 {
	m := findMetric(t, reg, name, labels...)
	if m == nil {
		return 0
	}
	return m.histogramCount
}

func getSummaryCount(t *testing.T, reg *prometheus.Registry, name string, labels ...string) uint64 {
	m := findMetric(t, reg, name, labels...)
	if m == nil {
		return 0
	}
	return m.summaryCount
}
