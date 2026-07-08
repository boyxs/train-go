package setup

import (
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"

	"github.com/boyxs/train-go/webook/shared/confkey"
)

// InitRedis 返回具体 *redis.Client，wire 侧用 wire.Bind 绑到 Cmdable + UniversalClient，
// 与 ioc.InitRedis 同构（集成测试连真实测试库 Redis）。
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
	return client
}
