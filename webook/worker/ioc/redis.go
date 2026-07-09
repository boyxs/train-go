package ioc

import (
	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"

	"github.com/boyxs/train-go/webook/pkg/redislock"
	lockprom "github.com/boyxs/train-go/webook/pkg/redislock/prometheus"

	redisprom "github.com/boyxs/train-go/webook/pkg/redisx/prometheus"
	"github.com/boyxs/train-go/webook/shared/confkey"
)

// InitRedis 与 chat/core 同源。worker 仅用 redis 做 cron 分布式锁，但仍接上
// Prometheus hook（webook_redis_*）+ OTel，保持全链路可观测一致。
func InitRedis() redis.UniversalClient {
	type Config struct {
		Addr     string `yaml:"addr" mapstructure:"addr"`
		Password string `yaml:"password" mapstructure:"password"`
	}
	var cfg Config
	if err := viper.UnmarshalKey(confkey.DataRedis, &cfg); err != nil {
		panic(err)
	}
	// 锁专用 client 的关键校准（worker 的 redis 仅用于 cron 分布式锁，故与 chat/core 的
	// 共享 redis client 在此刻意分道——它们要 cache 的透明重试，锁不能要）：
	//   MaxRetries=-1 关闭自动重试：acquire 脚本是 hincrby 计数、非幂等，go-redis 在
	//     "命令已执行但响应丢失" 时会重发 → 重复 +1 → 计数虚高、Unlock 减不到 0、
	//     锁滞留到 lease 过期（幻觉持有，别的副本这段时间抢不到）。锁的瞬时错误交给
	//     调用方降级 + watchdog 自身重试循环，不靠 go-redis 静默重试（refresh/release
	//     幂等，唯 acquire 有此患，故整体关重试最稳）。
	//   ContextTimeoutEnabled 让 ctx deadline 真正作用到 I/O：Lock/TryLock 的 ctx 上限能被及时打断。
	client := redis.NewClient(&redis.Options{
		Addr:                  cfg.Addr,
		Password:              cfg.Password,
		MaxRetries:            -1,
		ContextTimeoutEnabled: true,
	})
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
