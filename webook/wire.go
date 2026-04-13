//go:build wireinject

package main

import (
	"gitee.com/train-cloud/geektime-basic-go/internal/events"
	intrevt "gitee.com/train-cloud/geektime-basic-go/internal/events/interaction"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository/cache"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository/dao"
	"gitee.com/train-cloud/geektime-basic-go/internal/service"
	"gitee.com/train-cloud/geektime-basic-go/internal/web"
	"gitee.com/train-cloud/geektime-basic-go/internal/web/jwt"
	"gitee.com/train-cloud/geektime-basic-go/ioc"
	"gitee.com/train-cloud/geektime-basic-go/pkg/streamer"
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

// clickEventProviderSet 点击埋点模块
var clickEventProviderSet = wire.NewSet(
	dao.NewGormAIClickEventDAO,
	cache.NewRedisAIClickEventCache,
	repository.NewCacheAIClickEventRepository,
	service.NewAIClickEventService,
)

// polishProviderSet 文章润色模块
var polishProviderSet = wire.NewSet(
	service.NewAIArticlePolishService,
)

// kafkaProviderSet Kafka 基础设施 + 互动事件
var kafkaProviderSet = wire.NewSet(
	ioc.InitKafkaConfig,
	ioc.InitSaramaConfig,
	ioc.InitSaramaSyncProducer,
	ioc.InitSaramaClient,
	ioc.InitEventProducer,
	ioc.InitInteractionConsumerConfig,
	intrevt.NewSaramaInteractionEventProducer,
	intrevt.NewSaramaInteractionEventConsumer,
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
	service.NewAIChatService,
	service.NewAIChatToolExecutor,
	streamer.NewRedisStreamer,
)

// App 应用入口，包含 Web 服务和后台消费者
type App struct {
	Server   *gin.Engine
	Consumer events.Consumer
}

func InitWebServer() App {
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
		ioc.InitInteractionService,
		//handler
		web.NewInternalUserHandler,
		web.NewInternalArticleAuthorHandler,
		web.NewInternalArticleReaderHandler,
		web.NewInternalInteractionHandler,
		web.NewOAuth2WechatHandler,
		web.NewInternalChatHandler,
		web.NewInternalArticleSearchHandler,
		web.NewAIClickEventHandler,
		web.NewAIArticlePolishHandler,
		jwt.NewRedisJwtHandler,
		// chat 模块
		chatProviderSet,
		// 搜索模块
		searchProviderSet,
		// 点击埋点
		clickEventProviderSet,
		// 文章润色
		polishProviderSet,
		// kafka + 互动事件
		kafkaProviderSet,

		ioc.InitMiddlewares,
		ioc.InitWebServer,
		wire.Struct(new(App), "*"),
	)
	return App{}
}
