//go:build wireinject

package main

import (
	"gitee.com/train-cloud/geektime-basic-go/internal/repository"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository/cache"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository/dao"
	"gitee.com/train-cloud/geektime-basic-go/internal/service"
	"gitee.com/train-cloud/geektime-basic-go/internal/web"
	"gitee.com/train-cloud/geektime-basic-go/internal/web/jwt"
	"gitee.com/train-cloud/geektime-basic-go/ioc"
	"github.com/gin-gonic/gin"
	"github.com/google/wire"
)

// searchProviderSet 搜索模块的 Wire Provider 集合（不含 Handler）
var searchProviderSet = wire.NewSet(
	ioc.InitESClient,
	ioc.InitOllamaEmbeddingConfig,
	ioc.InitEmbeddingConfig,
	ioc.InitEmbeddingClient,
	dao.NewElasticArticleDAO,
	repository.NewESArticleSearchRepository,
	service.NewArticleSearchService,
)

// chatProviderSet Chat 模块的 Wire Provider 集合（不含 Handler）
var chatProviderSet = wire.NewSet(
	ioc.InitLLMConfig,
	ioc.InitLLMClient,
	ioc.InitChatLimiter,
	dao.NewGormConversationDAO,
	dao.NewGormMessageDAO,
	cache.NewRedisConversationCache,
	cache.NewRedisMessageCache,
	repository.NewCacheConversationRepository,
	repository.NewCacheMessageRepository,
	service.NewChatService,
)

func InitWebServer() *gin.Engine {
	wire.Build(
		//infra
		ioc.InitDB, ioc.InitRedis, ioc.InitLogger, ioc.InitTimezone,
		//dao
		dao.NewGormUserDAO,
		dao.NewGormArticleAuthorDAO,
		dao.NewGormArticleReaderDAO,
		dao.NewGormInteractionDAO,
		//cache
		cache.NewRedisUserCache, cache.NewRedisCodeCache, cache.NewRedisArticleCache,
		cache.NewRedisInteractionCache,
		//repository
		repository.NewRedisUserRepository,
		repository.NewRedisCodeRepository,
		repository.NewCacheArticleAuthorRepository,
		repository.NewCacheArticleReaderRepository,
		repository.NewCacheInteractionRepository,
		//service
		ioc.InitSmsService,
		ioc.InitWechatOAuth2Service,
		service.NewInternalUserService,
		service.NewSmsCodeService,
		service.NewInternalArticleAuthorService,
		service.NewInternalArticleReaderService,
		service.NewInternalInteractionService,
		//handler
		web.NewInternalUserHandler,
		web.NewInternalArticleAuthorHandler,
		web.NewInternalArticleReaderHandler,
		web.NewInternalInteractionHandler,
		web.NewOAuth2WechatHandler,
		web.NewInternalChatHandler,
		web.NewInternalArticleSearchHandler,
		jwt.NewRedisJwtHandler,
		// chat 模块
		chatProviderSet,
		// 搜索模块
		searchProviderSet,

		ioc.InitMiddlewares,
		ioc.InitWebServer,
	)
	return gin.Default()
}
