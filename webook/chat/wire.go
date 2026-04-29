//go:build wireinject

package main

import (
	"github.com/gin-gonic/gin"
	"github.com/google/wire"
	"google.golang.org/grpc"

	"github.com/webook/chat/ioc"
	"github.com/webook/chat/repository"
	"github.com/webook/chat/repository/cache"
	"github.com/webook/chat/repository/dao"
	"github.com/webook/chat/service"
	"github.com/webook/chat/web"
	"github.com/webook/pkg/streamer"
)

// App chat 服务进程入口。
type App struct {
	Server *gin.Engine
	Conn   *grpc.ClientConn
}

func InitApp() (App, func(), error) {
	wire.Build(
		// 基础设施
		ioc.InitTimezone,
		ioc.InitOTel,
		ioc.InitLogger,
		ioc.InitDB,
		ioc.InitRedis,
		// gRPC 客户端（拨号到主仓 webook-core :8090）
		ioc.InitCoreConn,
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
