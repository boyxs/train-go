//go:build wireinject

package main

import (
	"github.com/google/wire"
	"github.com/redis/go-redis/v9"

	interactiongrpc "github.com/boyxs/train-go/webook/interaction/grpc"
	"github.com/boyxs/train-go/webook/interaction/ioc"
	"github.com/boyxs/train-go/webook/interaction/repository"
	"github.com/boyxs/train-go/webook/interaction/repository/cache"
	"github.com/boyxs/train-go/webook/interaction/repository/dao"
	"github.com/boyxs/train-go/webook/interaction/service"
	"github.com/boyxs/train-go/webook/pkg/grpcx"
)

// App interaction 服务进程入口（纯 gRPC server）。
type App struct {
	GRPCServer *grpcx.Server
}

func InitApp() (App, func(), error) {
	wire.Build(
		// 基础设施
		ioc.InitTimezone,
		ioc.InitOTel,
		ioc.InitLogger,
		ioc.InitDB,
		ioc.InitRedis,
		ioc.InitEtcdClient,
		// Bind
		wire.Bind(new(redis.Cmdable), new(*redis.Client)),
		// dao + cache + repository
		dao.NewGormInteractionDAO,
		cache.NewRedisInteractionCache,
		repository.NewCacheInteractionRepository,
		// service
		service.NewInternalInteractionService,
		// gRPC server
		interactiongrpc.NewInteractionServer,
		ioc.InitGRPCServer,

		wire.Struct(new(App), "*"),
	)
	return App{}, nil, nil
}
