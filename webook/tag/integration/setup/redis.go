package setup

import (
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"

	"github.com/boyxs/train-go/webook/pkg/redisx"
	"github.com/boyxs/train-go/webook/shared/confkey"
)

// InitRedis 集成测试用裸 redis client（不挂 prometheus/otel hook），连 test.yaml 的 data.redis。
func InitRedis() redis.Cmdable {
	var cfg redisx.Config
	if err := viper.UnmarshalKey(confkey.DataRedis, &cfg); err != nil {
		panic(err)
	}
	return redisx.NewClient(cfg)
}
