//go:build wireinject

package main

import (
	"github.com/gin-gonic/gin"
	"github.com/google/wire"

	"github.com/boyxs/train-go/webook/chat/ioc"
	"github.com/boyxs/train-go/webook/chat/repository"
	"github.com/boyxs/train-go/webook/chat/repository/cache"
	"github.com/boyxs/train-go/webook/chat/repository/dao"
	"github.com/boyxs/train-go/webook/chat/service"
	"github.com/boyxs/train-go/webook/chat/web"
	"github.com/boyxs/train-go/webook/pkg/streamer"
)

// App chat 服务进程入口。
type App struct {
	Server *gin.Engine
	Conn   ioc.CoreConn
}

func InitApp() (App, func(), error) {
	wire.Build(
		// 基础设施
		ioc.InitTimezone,
		ioc.InitOTel,
		ioc.InitLogger,
		ioc.InitDB,
		ioc.InitRedis,
		// gRPC
		ioc.InitEtcdClient,
		ioc.InitGRPCMetrics,
		ioc.InitCoreConn,
		ioc.InitInteractionConn,
		ioc.InitSearchConn,
		ioc.InitSearchClient,
		ioc.InitArticleReaderClient,
		ioc.InitInteractionClient,
		// LLM
		ioc.InitLLMConfig,
		ioc.InitLLMClient,
		ioc.InitChatLimiter,
		// stream（SSE 重连）
		streamer.NewRedisStreamer,
		// dao + cache + repository
		dao.NewGormConversationDAO,
		dao.NewGormMessageDAO,
		cache.NewRedisConversationCache,
		cache.NewRedisMessageCache,
		repository.NewCacheConversationRepository,
		repository.NewCacheMessageRepository,
		// service
		service.NewAIChatService,
		service.NewAIChatToolExecutor,
		// handler + Gin
		web.NewInternalChatHandler,
		ioc.InitMiddlewares,
		ioc.InitWebServer,

		wire.Struct(new(App), "*"),
	)
	return App{}, nil, nil
}
