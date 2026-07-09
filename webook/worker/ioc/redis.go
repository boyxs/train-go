package ioc

import (
	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"

	"github.com/boyxs/train-go/webook/pkg/redislock"
	lockprom "github.com/boyxs/train-go/webook/pkg/redislock/prometheus"

	"github.com/boyxs/train-go/webook/pkg/redisx"
	redisprom "github.com/boyxs/train-go/webook/pkg/redisx/prometheus"
	"github.com/boyxs/train-go/webook/shared/confkey"
)

// InitRedis 与 chat/core 同源。worker 仅用 redis 做 cron 分布式锁，但仍接上
// Prometheus hook（webook_redis_*）+ OTel，保持全链路可观测一致。
func InitRedis() redis.UniversalClient {
	var cfg redisx.Config
	if err := viper.UnmarshalKey(confkey.DataRedis, &cfg); err != nil {
		panic(err)
	}
	// worker 的 redis 仅用于 cron 分布式锁：锁专用校准（`max_retries: -1` 关重试防 acquire 非幂等
	// 重发重复计数、`context_timeout_enabled: true` 让 ctx 作用到 I/O）经 data.redis 显式配置
	// （见 config/*.yaml），与 chat/core 共享 cache client 刻意分道（它们要透明重试，锁不能要）。
	client := redisx.NewClient(cfg)
	client.AddHook(redisprom.NewPrometheusBuilder("webook", "redis", "cmd", "Redis 命令统计").
		WithCounter().
		WithHistogram().
		WithSummary().
		Build())
	if err := redisotel.InstrumentTracing(client); err != nil {
		panic(err)
	}
	return client
}

func InitLockClient(cmd redis.UniversalClient) redislock.Client {
	return lockprom.NewPrometheusBuilder("webook", "lock", "分布式锁").
		Build(redislock.NewClient(cmd))
}
