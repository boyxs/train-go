//go:build wireinject

package main

import (
	"github.com/google/wire"

	searchgrpc "github.com/boyxs/train-go/webook/search/grpc"
	"github.com/boyxs/train-go/webook/search/ioc"
	"github.com/boyxs/train-go/webook/search/repository"
	"github.com/boyxs/train-go/webook/search/service"

	"github.com/boyxs/train-go/webook/pkg/embedding"
	"github.com/boyxs/train-go/webook/pkg/grpcx"
)

// App search 服务进程入口（纯 gRPC server）。
type App struct {
	GRPCServer *grpcx.Server
}

func InitApp() (App, func(), error) {
	wire.Build(
		// 基础设施（search 全链路 int64 毫秒、无 carbon，故无需 InitTimezone）
		ioc.InitOTel,
		ioc.InitLogger,
		ioc.InitESClient,
		ioc.InitRedis,
		ioc.InitEtcdClient,
		// embedding：Ollama 优先 + 收费兜底 + Redis 缓存（pkg/embedding）
		ioc.InitEmbeddingConfig,
		ioc.InitOllamaEmbeddingConfig,
		ioc.InitEmbeddingClient,
		// dao + repository
		ioc.InitArticleDAO,
		repository.NewESArticleRepository,
		// service（embedding.EmbeddingClient 满足 service.Embedder）
		service.NewInternalArticleService,
		wire.Bind(new(service.Embedder), new(embedding.EmbeddingClient)),
		// gRPC server
		searchgrpc.NewSearchServer,
		ioc.InitGRPCServer,

		wire.Struct(new(App), "*"),
	)
	return App{}, nil, nil
}
