package setup

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"

	"github.com/boyxs/train-go/webook/pkg/redislock"
	lockprom "github.com/boyxs/train-go/webook/pkg/redislock/prometheus"
)

// InitLockClient 集成测试版分布式锁 client：锁指标走独立 Registry。
//
// 生产 ioc.InitLockClient 在 DefaultRegisterer 上 MustRegister（进程内只建一次，正确）；
// 集成测试每个用例都调 InitWebServer 重建全套 → 第二次注册同名 collector 就 panic。
// 故这里像 provideTestMiddlewares 一样把 lockprom 绑到 prometheus.NewRegistry()，逐次隔离。
func InitLockClient(cmd redis.UniversalClient) redislock.Client {
	return lockprom.NewPrometheusBuilder("webook", "lock", "分布式锁").
		Registry(prometheus.NewRegistry()).
		Build(redislock.NewClient(cmd))
}
