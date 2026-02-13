package setup

import (
	"gitee.com/train-cloud/geektime-basic-go/config"
	"github.com/redis/go-redis/v9"
)

func InitRedis() redis.Cmdable {
	client := redis.NewClient(&redis.Options{
		Addr:     config.Config.Redis.Addr,
		Password: config.Config.Redis.Password,
	})
	return client
}
