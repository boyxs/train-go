package setup

import (
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"

	"github.com/webook/shared/confkey"
)

func InitRedis() redis.Cmdable {
	type Config struct {
		Addr     string `yaml:"addr" mapstructure:"addr"`
		Password string `yaml:"password" mapstructure:"password"`
	}
	var cfg = Config{}
	if err := viper.UnmarshalKey(confkey.DataRedis, &cfg); err != nil {
		panic(err)
	}
	return redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
	})
}
