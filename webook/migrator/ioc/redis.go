package ioc

import (
	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"

	redisprom "github.com/webook/pkg/redisx/prometheus"
	"github.com/webook/shared/confkey"
)

// InitRedis 初始化 migrator 使用的 Redis 客户端。
// 与 chat/ioc/redis.go 同构；metric / OTel 自动注入。
func InitRedis() redis.Cmdable {
	type Config struct {
		Addr     string `yaml:"addr" mapstructure:"addr"`
		Password string `yaml:"password" mapstructure:"password"`
	}
	var cfg Config
	if err := viper.UnmarshalKey(confkey.DataRedis, &cfg); err != nil {
		panic(err)
	}
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
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
