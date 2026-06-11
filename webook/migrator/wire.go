//go:build wireinject

package main

import (
	"github.com/gin-gonic/gin"
	"github.com/google/wire"

	"github.com/webook/migrator/ioc"
	"github.com/webook/migrator/repository"
	"github.com/webook/migrator/repository/cache"
	"github.com/webook/migrator/repository/dao"
	"github.com/webook/migrator/service"
	"github.com/webook/migrator/service/replay"
	"github.com/webook/migrator/service/switching"
	"github.com/webook/migrator/web"
	"github.com/webook/migrator/web/middleware"
)

// App migrator 服务进程入口。
type App struct {
	Server *gin.Engine
}

func InitApp() (App, func(), error) {
	wire.Build(
		// 基础设施
		ioc.InitTimezone,
		ioc.InitOTel,
		ioc.InitLogger,
		ioc.InitDB,
		ioc.InitRedis,
		ioc.InitMongoClient,

		// dao
		dao.NewGormTaskDAO,
		dao.NewGormAuditLogDAO,
		dao.NewGormCheckpointDAO,
		dao.NewGormValidateLogDAO,
		dao.NewGormDeadLetterDAO,

		// cache
		cache.NewRedisThrottleCache,
		cache.NewRedisSwitchStateCache,

		// repository
		repository.NewTaskRepository,
		repository.NewCheckpointRepository,
		repository.NewValidateLogRepository,
		repository.NewSwitchStateRepository,
		repository.NewThrottleRepository,
		repository.NewDeadLetterRepository,
		repository.NewAuditLogRepository,

		// service
		service.NewTaskService,
		switching.NewSwitchService,
		replay.NewReplayService,

		// pipeline factory + engines：handler / 引擎按需 BuildFullSrc/BuildIncrSrc/BuildDst，按 task 动态构造 Source/Sink
		// （IncrEngine 当前接 MySQLSource，IncrSubscribe 返 ErrIncrNotSupported；接 CanalSource 走真实 binlog）
		ioc.InitDBResolver,
		ioc.InitSourceFactory,
		ioc.InitSinkFactory,
		ioc.InitFullEngine,
		ioc.InitIncrEngine,
		ioc.InitVerifyEngine,
		ioc.InitMigrationMetrics,

		// web — handler + middleware
		web.NewTaskHandler,
		middleware.NewAuditMiddleware,

		// 装配
		ioc.InitMiddlewares,
		ioc.InitWebServer,
		wire.Struct(new(App), "*"),
	)
	return App{}, nil, nil
}
