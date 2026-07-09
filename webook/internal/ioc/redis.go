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

// InitRedis 按 data.redis 的 mode 建单机 / 集群 client（redisx.NewClient，返回 redis.UniversalClient）；
// wire 侧 Bind redis.Cmdable（cache / jwt 中间件消费）+ 直供 redis.UniversalClient（分布式锁
// InitLockClient 消费）——一个连接、两种接口视图，避免第二个连接池 / 重复注册 Prometheus hook。
// core 的 redis 是共享 cache（非锁专用），故用 go-redis 默认重试，不设 max_retries=-1。
func InitRedis() redis.UniversalClient {
	var cfg redisx.Config
	if err := viper.UnmarshalKey(confkey.DataRedis, &cfg); err != nil {
		panic(err)
	}
	client := redisx.NewClient(cfg)
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
