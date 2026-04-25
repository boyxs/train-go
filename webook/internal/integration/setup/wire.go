//go:build wireinject

package setup

import (
	"github.com/gin-gonic/gin"
	"github.com/google/wire"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/webook/internal/repository"
	"github.com/webook/internal/repository/cache"
	"github.com/webook/internal/repository/dao"
	"github.com/webook/internal/service"
	"github.com/webook/internal/web"
	"github.com/webook/internal/web/jwt"
	"github.com/webook/ioc"
	"github.com/webook/pkg/streamer"
)

// 集成测试不连真实 OTel Collector，注入 Noop TracerProvider 满足依赖
func provideNoopTracerProvider() trace.TracerProvider {
	return noop.NewTracerProvider()
}

// 这个需要登录权限
func InitWebServer() *gin.Engine {
	wire.Build(
		//infra
		infraSvcProvider,
		//provider
		userSvcProvider,
		articleSvcProvider,
		chatSvcProvider,
		//cache
		cache.NewRedisCodeCache,
		//repository
		repository.NewRedisCodeRepository,
		//service
		ioc.InitSmsService,
		ioc.InitWechatOAuth2Service,
		service.NewSmsCodeService,
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
		// 点击埋点
		clickEventSvcProvider,
		// 文章润色
		polishSvcProvider,
		// 文章榜单
		articleRankingSvcProvider,

		ioc.InitMiddlewares,
		ioc.InitWebServer,
		provideNoopTracerProvider,
	)
	return gin.Default()
}

func InitArticleAuthorHandler() web.ArticleAuthorHandler {
	wire.Build(
		infraSvcProvider,
		articleSvcProvider,
		web.NewInternalArticleAuthorHandler,
	)
	return &web.InternalArticleAuthorHandler{}
}

func InitArticleReaderHandler() web.ArticleReaderHandler {
	wire.Build(
		infraSvcProvider,
		articleReaderSvcProvider,
		web.NewInternalArticleReaderHandler,
	)
	return &web.InternalArticleReaderHandler{}
}

func InitInteractionHandler() web.InteractionHandler {
	wire.Build(
		infraSvcProvider,
		interactionSvcProvider,
		web.NewInternalInteractionHandler,
	)
	return &web.InternalInteractionHandler{}
}

func InitArticlePolishHandler() web.ArticlePolishHandler {
	wire.Build(
		infraSvcProvider,
		chatSvcProvider,
		polishSvcProvider,
		web.NewAIArticlePolishHandler,
	)
	return &web.AIArticlePolishHandler{}
}

func InitClickEventHandler() web.ClickEventHandler {
	wire.Build(
		infraSvcProvider,
		clickEventSvcProvider,
		web.NewAIClickEventHandler,
	)
	return &web.AIClickEventHandler{}
}

func InitChatHandler() web.ChatHandler {
	wire.Build(
		infraSvcProvider,
		chatSvcProvider,
		searchSvcProvider,
		articleReaderSvcProvider,
		web.NewInternalChatHandler,
	)
	return &web.InternalChatHandler{}
}

var infraSvcProvider = wire.NewSet(
	InitRedis,
	InitDB,
	InitLogger,
)

var userSvcProvider = wire.NewSet(
	dao.NewGormUserDAO,
	cache.NewRedisUserCache,
	repository.NewRedisUserRepository,
	service.NewInternalUserService,
)

var searchSvcProvider = wire.NewSet(
	ioc.InitESClient,
	ioc.InitOllamaEmbeddingConfig,
	ioc.InitEmbeddingConfig,
	ioc.InitEmbeddingClient,
	dao.NewElasticArticleDAO,
	repository.NewESArticleSearchRepository,
	service.NewArticleSearchService,
)

var articleSvcProvider = wire.NewSet(
	dao.NewGormArticleAuthorDAO,
	dao.NewGormArticleReaderDAO,
	cache.NewRedisArticleCache,
	repository.NewCacheArticleAuthorRepository,
	repository.NewCacheArticleReaderRepository,
	service.NewInternalArticleAuthorService,
	service.NewInternalArticleReaderService,
	interactionSvcProvider,
	searchSvcProvider,
)

var articleReaderSvcProvider = wire.NewSet(
	dao.NewGormArticleReaderDAO,
	cache.NewRedisArticleCache,
	repository.NewCacheArticleReaderRepository,
	service.NewInternalArticleReaderService,
	interactionSvcProvider,
)

var interactionSvcProvider = wire.NewSet(
	dao.NewGormInteractionDAO,
	cache.NewRedisInteractionCache,
	repository.NewCacheInteractionRepository,
	service.NewInternalInteractionService,
)

var clickEventSvcProvider = wire.NewSet(
	dao.NewGormAIClickEventDAO,
	cache.NewRedisAIClickEventCache,
	repository.NewCacheAIClickEventRepository,
	service.NewAIClickEventService,
)

var polishSvcProvider = wire.NewSet(
	service.NewAIArticlePolishService,
)

// 文章榜单：集成测试不拉起 cron
var articleRankingSvcProvider = wire.NewSet(
	dao.NewGormArticleRankingDAO,
	cache.NewRedisArticleRankingCache,
	repository.NewCacheArticleRankingRepository,
	service.NewArticleRankingService,
	web.NewArticleRankingHandler,
)

var chatSvcProvider = wire.NewSet(
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
