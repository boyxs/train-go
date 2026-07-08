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

// InitRedis 返回具体 *redis.Client；wire 侧用 wire.Bind 同时绑到 redis.Cmdable
// （cache / jwt 中间件消费）和 redis.UniversalClient（分布式锁 InitLockClient 消费）——
// 一个连接、两种接口视图，避免第二个连接池 / 重复注册 Prometheus hook。
func InitRedis() *redis.Client {
	type Config struct {
		Addr     string `yaml:"addr" mapstructure:"addr"`
		Password string `yaml:"password" mapstructure:"password"`
	}
	var cfg = Config{}
	err := viper.UnmarshalKey(confkey.DataRedis, &cfg)
	if err != nil {
		panic(err)
	}
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
	})
	// 注册 Prometheus 指标 Hook（Counter + Histogram + Summary）
	client.AddHook(redisprom.NewPrometheusBuilder("webook", "redis", "cmd", "Redis 命令统计").
		WithCounter().
		WithHistogram().
		WithSummary().
		Build())
	// OTel：每条 Redis 命令自动产生 span（kind=Client）+ db.system="redis" 属性
	if err := redisotel.InstrumentTracing(client); err != nil {
		panic(err)
	}
	return client
}

func InitLockClient(cmd redis.UniversalClient) redislock.Client {
	return lockprom.NewPrometheusBuilder("webook", "lock", "分布式锁").
		Build(redislock.NewClient(cmd))
}
