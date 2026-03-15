package setup

import (
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
)

func InitRedis() redis.Cmdable {
	type Config struct {
		Addr     string `yaml:"addr" mapstructure:"addr"`
		Password string `yaml:"password" mapstructure:"password"`
	}
	var cfg = Config{}
	err := viper.UnmarshalKey("redis", &cfg)
	if err != nil {
		panic(err)
	}
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
	})
	return client
}
