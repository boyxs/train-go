package ioc

import (
	"github.com/redis/go-redis/v9"

	"github.com/webook/pkg/redislockx"
	lockprom "github.com/webook/pkg/redislockx/prometheus"
)

// InitLockClient 初始化分布式锁客户端（类 Redisson：bsm/redislock + 自研 Watchdog）。
// 默认 reg=DefaultRegisterer，跟项目其他指标共用注册表，/metrics 一起暴露。
func InitLockClient(cmd redis.Cmdable) redislockx.Client {
	return lockprom.NewPrometheusBuilder("webook", "lock", "分布式锁").
		Build(redislockx.NewClient(cmd))
}
