package ioc

import (
	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"

	redisprom "github.com/webook/pkg/redisx/prometheus"

	"github.com/webook/shared/confkey"
)

func InitRedis() redis.Cmdable {
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
