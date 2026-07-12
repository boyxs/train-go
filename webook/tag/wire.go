//go:build wireinject

package main

import (
	"github.com/google/wire"

	taggrpc "github.com/boyxs/train-go/webook/tag/grpc"
	"github.com/boyxs/train-go/webook/tag/ioc"
	"github.com/boyxs/train-go/webook/tag/repository"
	"github.com/boyxs/train-go/webook/tag/repository/cache"
	"github.com/boyxs/train-go/webook/tag/repository/dao"
	"github.com/boyxs/train-go/webook/tag/service"

	"github.com/boyxs/train-go/webook/pkg/grpcx"
)

// App tag 服务进程入口（纯 gRPC server）。
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
		dao.NewGormTagDAO,
		dao.NewGormTaggingDAO,
		dao.NewGormTagFollowDAO,
		cache.NewRedisTagCache,
		repository.NewInternalTagRepository,
		// service
		service.NewInternalTagService,
		// gRPC server
		taggrpc.NewTagServer,
		ioc.InitGRPCServer,

		wire.Struct(new(App), "*"),
	)
	return App{}, nil, nil
}
