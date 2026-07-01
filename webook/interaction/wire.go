//go:build wireinject

package main

import (
	"github.com/google/wire"

	interactiongrpc "github.com/webook/interaction/grpc"
	"github.com/webook/interaction/ioc"
	"github.com/webook/interaction/repository"
	"github.com/webook/interaction/repository/cache"
	"github.com/webook/interaction/repository/dao"
	"github.com/webook/interaction/service"
	"github.com/webook/pkg/grpcx"
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
