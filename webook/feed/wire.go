//go:build wireinject

package main

import (
	"github.com/google/wire"

	feedgrpc "github.com/boyxs/train-go/webook/feed/grpc"
	"github.com/boyxs/train-go/webook/feed/ioc"
	"github.com/boyxs/train-go/webook/feed/repository"
	"github.com/boyxs/train-go/webook/feed/repository/cache"
	"github.com/boyxs/train-go/webook/feed/service"

	"github.com/boyxs/train-go/webook/pkg/grpcx"
)

// App feed 服务进程入口（纯同步 gRPC server；无 MySQL，数据全 Redis 投影）。
type App struct {
	GRPCServer *grpcx.Server
}

func InitApp() (App, func(), error) {
	wire.Build(
		// 基础设施（无 DB）
		ioc.InitOTel,
		ioc.InitLogger,
		ioc.InitRedis,
		ioc.InitEtcdClient,
		ioc.InitGRPCMetrics,
		// 下游 gRPC client（relation 关系 + core article 回源）
		ioc.InitRelationConn,
		ioc.InitRelationClient,
		ioc.InitCoreConn,
		ioc.InitArticleClient,
		// cache + repository + service
		ioc.InitCacheConfig,
		cache.NewRedisFeedCache,
		repository.NewCacheFeedRepository,
		ioc.InitServiceConfig,
		service.NewInternalFeedService,
		// gRPC server
		feedgrpc.NewFeedServer,
		ioc.InitGRPCServer,

		wire.Struct(new(App), "*"),
	)
	return App{}, nil, nil
}
