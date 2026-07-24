//go:build wireinject

package setup

import (
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/google/wire"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	commentv1 "github.com/boyxs/train-go/webook/api/gen/comment/v1"
	"github.com/boyxs/train-go/webook/internal/consts"
	"github.com/boyxs/train-go/webook/internal/ioc"
	"github.com/boyxs/train-go/webook/internal/repository"
	"github.com/boyxs/train-go/webook/internal/repository/cache"
	"github.com/boyxs/train-go/webook/internal/repository/dao"
	"github.com/boyxs/train-go/webook/internal/service"
	"github.com/boyxs/train-go/webook/internal/web"
	"github.com/boyxs/train-go/webook/pkg/ginx/middleware/metrics"
	"github.com/boyxs/train-go/webook/pkg/jwtx"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

// 集成测试不连真实 OTel Collector，注入 Noop TracerProvider 满足依赖
func provideNoopTracerProvider() trace.TracerProvider {
	return noop.NewTracerProvider()
}

// provideNilCommentClient 集成测试不拉起 comment gRPC server / etcd，注入 nil client 满足
// InitWebServer 依赖。现有集成用例不触达 /api/comment/*（RegisterRoutes 不调 client）；
// 如后续要集成测评论网关，改为拨号真实 comment server。
func provideNilCommentClient() commentv1.CommentServiceClient {
	return nil
}

// provideTestRelationHandler：集成测试不拉起 relation gRPC server / kafka，用 nil client + nil producer
// 构造 GRPCRelationService 再包成 handler（现有用例不触达 /relation/*）。nil 在此内联（不出现在
// wire_gen 签名里）→ wire_gen 无需 import relation proto 包，规避 goimports 对 gen/relation/v1
// （目录名 v1≠包名 relationv1）bare import 的重复别名误判。
func provideTestRelationHandler(userSvc service.UserService, l logger.LoggerX) web.RelationHandler {
	return web.NewInternalRelationHandler(service.NewGRPCRelationService(nil, userSvc, nil, l))
}

// provideTestFeedHandler：集成测试不拉起 feed / comment gRPC server，用 nil 下游依赖构造（现有用例不触达 /feed/*）。
// nil 内联（不出现在 wire_gen 签名）→ wire_gen 无需 import feed/comment proto 包。
func provideTestFeedHandler(l logger.LoggerX) web.FeedHandler {
	return web.NewInternalFeedHandler(service.NewGRPCFeedService(nil, nil, nil, nil, nil, nil, l))
}

// provideTestMiddlewares 与 ioc.InitMiddlewares 同结构，但 metrics 走独立 Registry，
// 避免每个集成测试 SetupSuite 调 InitWebServer 时 DefaultRegisterer.MustRegister 重复 panic。
// 省略 cors / 限流 / logger（httptest 无 cross-origin、不需限流、避免输出污染）。
func provideTestMiddlewares(l logger.LoggerX, cmd redis.Cmdable, tp trace.TracerProvider) []gin.HandlerFunc {
	return []gin.HandlerFunc{
		metrics.NewPrometheusBuilder("test", "http", "requests", "test").
			WithCounter().WithHistogram().WithSummary().WithInFlight().
			Registry(prometheus.NewRegistry()).
			Build(),
		otelgin.Middleware("webook-test", otelgin.WithTracerProvider(tp)),
		// 集成测试不需要真 cors，但 ioc 行为对齐：放空头允许；不抢先返
		cors.New(cors.Config{AllowOriginFunc: func(string) bool { return true }}),
		jwtx.NewMiddlewareBuilder(jwtx.MiddlewareConfig{
			AccessKey:      consts.AccessKey,
			UserKey:        consts.UserKey,
			Cmd:            cmd,
			SsidKeyPattern: consts.UserSsidPattern,
		}).
			IgnoredPaths("/user/register", "/user/login", "/user/refresh_token",
				"/user/login_sms/code/send", "/user/login_sms",
				"/oauth2/wechat/authurl", "/oauth2/wechat/callback",
				"/article/reader/detail", "/article/reader/page",
				"/interaction/view", "/interaction/detail",
				"/article/ranking/page", "/article/ranking/archive/dates",
				"/metrics").
			Build(),
	}
}

// 这个需要登录权限
func InitWebServer() *gin.Engine {
	wire.Build(
		//infra
		infraSvcProvider,
		//provider
		userSvcProvider,
		articleSvcProvider,
		llmSvcProvider,
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
		web.NewInternalArticleSearchHandler,
		web.NewInternalTagHandler,
		web.NewAIClickEventHandler,
		web.NewAIArticlePolishHandler,
		service.NewGRPCCommentService,
		web.NewInternalCommentHandler,
		provideNilCommentClient,
		provideTestRelationHandler,
		provideTestFeedHandler,
		ioc.InitJwtHandler,
		// 点击埋点
		clickEventSvcProvider,
		// 文章润色
		polishSvcProvider,
		// 文章榜单
		articleRankingSvcProvider,

		provideTestMiddlewares,
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
		provideNilCommentClient, // 集成测试不拉 comment gRPC，注 nil（现有用例不触达评论数聚合）
		newFakeTagService,       // 阅读详情回显补标签：集成测试注 no-op 桩
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
		llmSvcProvider,
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

var infraSvcProvider = wire.NewSet(
	InitRedis,
	// InitRedis 出 UniversalClient（redisx.NewClient）；cache/中间件按需收窄成 Cmdable，锁用 UniversalClient
	wire.Bind(new(redis.Cmdable), new(redis.UniversalClient)),
	InitLockClient, // 集成测试版：锁指标走独立 Registry（见 lock.go），避免 InitWebServer 重复注册 panic
	InitDB,
	InitLogger,
)

var userSvcProvider = wire.NewSet(
	dao.NewGormUserDAO,
	cache.NewRedisUserCache,
	repository.NewRedisUserRepository,
	service.NewInternalUserService,
)

// searchSvcProvider search/tag 已拆独立 gRPC 服务，集成测试注入 no-op 桩（供 article 作者服务的后台索引/同步空转）。
var searchSvcProvider = wire.NewSet(
	newFakeSearchService,
	newFakeTagService,
)

var articleSvcProvider = wire.NewSet(
	dao.NewGormArticleAuthorDAO,
	dao.NewGormArticleReaderDAO,
	dao.NewGormArticleReaderNewDAO,
	cache.NewRedisArticleCache,
	repository.NewCacheArticleAuthorRepository,
	repository.NewCacheArticleReaderRepository,
	service.NewInternalArticleAuthorService,
	service.NewInternalArticleReaderService,
	NewFakeArticleEventProducer,
	ioc.InitMigratorSDKSwitchReader,
	ioc.InitMigratorSDKDualWriter,
	ioc.InitMigratorSDKTaskName,
	interactionSvcProvider,
	searchSvcProvider,
)

var articleReaderSvcProvider = wire.NewSet(
	dao.NewGormArticleReaderDAO,
	dao.NewGormArticleReaderNewDAO,
	cache.NewRedisArticleCache,
	repository.NewCacheArticleReaderRepository,
	service.NewInternalArticleReaderService,
	ioc.InitMigratorSDKSwitchReader,
	ioc.InitMigratorSDKDualWriter,
	ioc.InitMigratorSDKTaskName,
	interactionSvcProvider,
)

// 互动已拆 webook-interaction 独立服务；集成测试注入桩 InteractionService（见 fake_interaction.go）。
var interactionSvcProvider = wire.NewSet(
	newFakeInteractionService,
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

// llmSvcProvider 仅 LLM 相关 provider，供 article_polish 用。
// 原 chatSvcProvider 已随 chat 服务搬到 chat/wire.go。
var llmSvcProvider = wire.NewSet(
	ioc.InitLLMConfig,
	ioc.InitLLMClient,
)
