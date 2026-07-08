//go:build wireinject

package main

import (
	"github.com/google/wire"

	relationgrpc "github.com/boyxs/train-go/webook/relation/grpc"
	"github.com/boyxs/train-go/webook/relation/ioc"
	"github.com/boyxs/train-go/webook/relation/repository"
	"github.com/boyxs/train-go/webook/relation/repository/cache"
	"github.com/boyxs/train-go/webook/relation/repository/dao"
	"github.com/boyxs/train-go/webook/relation/service"

	"github.com/boyxs/train-go/webook/pkg/grpcx"
)

// App relation 服务进程入口（纯 gRPC server）。
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
		dao.NewGormRelationDAO,
		cache.NewRedisRelationCache,
		repository.NewCacheRelationRepository,
		// service
		service.NewInternalRelationService,
		// gRPC server
		relationgrpc.NewRelationServer,
		ioc.InitGRPCServer,

		wire.Struct(new(App), "*"),
	)
	return App{}, nil, nil
}
