//go:build wireinject

package main

import (
	"net/http"

	"github.com/google/wire"
	"github.com/robfig/cron/v3"

	"github.com/webook/worker/consumer"
	"github.com/webook/worker/ioc"
	"github.com/webook/worker/job"
)

// App worker 调度器进程入口：cron 定时任务 + Kafka 消费者 + 最小 HTTP(metrics/health)。
type App struct {
	Cron     *cron.Cron
	Consumer *consumer.InteractionConsumer
	Web      *http.ServeMux
}

func InitApp() (App, func(), error) {
	wire.Build(
		// 基础设施
		ioc.InitTimezone,
		ioc.InitOTel,
		ioc.InitLogger,
		// gRPC（下游：core 的 RankingJobService + interaction）
		ioc.InitEtcdClient,
		ioc.InitGRPCMetrics, // core + interaction client 共享同一指标 builder
		ioc.InitCoreConn,
		ioc.InitInteractionConn,
		ioc.InitRankingJobClient,
		ioc.InitInteractionClient,
		// Kafka 消费者
		ioc.InitKafkaConfig,
		ioc.InitSaramaConfig,
		ioc.InitConsumerConfig,
		consumer.NewInteractionConsumer,
		// cron：redis 分布式锁 + 任务指标 + wrapper
		ioc.InitRedis,
		ioc.InitLockClient,
		ioc.InitCronMetrics,
		ioc.InitCronWrapper,
		job.NewRankingJob,
		ioc.InitCron,
		ioc.InitWebServer,

		wire.Struct(new(App), "*"),
	)
	return App{}, nil, nil
}
