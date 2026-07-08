package ioc

import (
	"github.com/redis/go-redis/v9"

	"github.com/boyxs/train-go/webook/pkg/redislockx"
	lockprom "github.com/boyxs/train-go/webook/pkg/redislockx/prometheus"
)

// InitLockClient 分布式锁客户端（类 Redisson：bsm/redislock + 自研 Watchdog），镜像 core。
// 指标走 DefaultRegisterer，/metrics 一起暴露 webook_lock_*（含 watchdog_lost）。
func InitLockClient(cmd redis.Cmdable) redislockx.Client {
	return lockprom.NewPrometheusBuilder("webook", "lock", "分布式锁").
		Build(redislockx.NewClient(cmd))
}
