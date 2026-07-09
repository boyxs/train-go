package ioc

import (
	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"

	"github.com/boyxs/train-go/webook/pkg/redisx"
	redisprom "github.com/boyxs/train-go/webook/pkg/redisx/prometheus"
	"github.com/boyxs/train-go/webook/shared/confkey"
)

func InitRedis() redis.Cmdable {
	var cfg redisx.Config
	if err := viper.UnmarshalKey(confkey.DataRedis, &cfg); err != nil {
		panic(err)
	}
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
