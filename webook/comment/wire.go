//go:build wireinject

package main

import (
	"github.com/google/wire"

	commentgrpc "github.com/boyxs/train-go/webook/comment/grpc"
	"github.com/boyxs/train-go/webook/comment/ioc"
	"github.com/boyxs/train-go/webook/comment/repository"
	"github.com/boyxs/train-go/webook/comment/repository/cache"
	"github.com/boyxs/train-go/webook/comment/repository/dao"
	"github.com/boyxs/train-go/webook/comment/service"
	"github.com/boyxs/train-go/webook/pkg/grpcx"
)

// App comment 服务进程入口（纯 gRPC server）。
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
		// 业务依赖
		ioc.InitSensitiveFilter,
		ioc.InitLimiter,
		// dao + cache + repository
		dao.NewGormCommentDAO,
		cache.NewRedisCommentCache,
		repository.NewCacheCommentRepository,
		// service
		service.NewCommentService,
		// gRPC server
		commentgrpc.NewCommentServer,
		ioc.InitGRPCServer,

		wire.Struct(new(App), "*"),
	)
	return App{}, nil, nil
}
