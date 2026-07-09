package setup

import (
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"

	"github.com/boyxs/train-go/webook/pkg/redisx"
	"github.com/boyxs/train-go/webook/shared/confkey"
)

// InitRedis 返回 redis.UniversalClient（redisx.NewClient 按 mode 建单机/集群）；wire 侧 Bind
// Cmdable（cache/中间件）+ 直供 UniversalClient（分布式锁），集成测试连真实测试库 Redis。
func InitRedis() redis.UniversalClient {
	var cfg redisx.Config
	if err := viper.UnmarshalKey(confkey.DataRedis, &cfg); err != nil {
		panic(err)
	}
	return redisx.NewClient(cfg)
}
