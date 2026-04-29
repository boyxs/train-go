//go:build wireinject

package setup

import (
	"fmt"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/google/wire"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/webook/internal/consts"
	"github.com/webook/internal/ioc"
	"github.com/webook/internal/job"
	"github.com/webook/internal/repository"
	"github.com/webook/internal/repository/cache"
	"github.com/webook/internal/repository/dao"
	"github.com/webook/internal/service"
	"github.com/webook/internal/web"
	cronprom "github.com/webook/pkg/cronx/prometheus"
	"github.com/webook/pkg/ginx/middleware/metrics"
	"github.com/webook/pkg/jwtx"
	"github.com/webook/pkg/logger"
	"github.com/webook/pkg/redislockx"
	lockprom "github.com/webook/pkg/redislockx/prometheus"
)

// 集成测试不连真实 OTel Collector，注入 Noop TracerProvider 满足依赖
func provideNoopTracerProvider() trace.TracerProvider {
	return noop.NewTracerProvider()
}

// 集成测试每次调用都给独立 prometheus Registry，避免 MustRegister 重复 panic。
// 生产 ioc.InitLockClient / InitCronMetrics 走 DefaultRegisterer，跨测试调用会撞名。
func provideTestLockClient(cmd redis.Cmdable) redislockx.Client {
	return lockprom.NewPrometheusBuilder("test", "lock", "test").
		Registry(prometheus.NewRegistry()).
		Build(redislockx.NewClient(cmd))
}

func provideTestCronMetrics() *cronprom.Metrics {
	return cronprom.NewPrometheusBuilder("test", "cron", "test").
		Registry(prometheus.NewRegistry()).Build()
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
			AccessKey: consts.AccessKey,
			UserKey:   consts.UserKey,
			Session: func(ctx *gin.Context, ssid string) bool {
				cnt, err := cmd.Exists(ctx, fmt.Sprintf(consts.UserSsidPattern, ssid)).Result()
				return err == nil && cnt > 0
			},
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
		web.NewAIClickEventHandler,
		web.NewAIArticlePolishHandler,
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

// InitRankingCron 集成测试用：拉起完整 cron + lock + wrapper + RankingJob 链路，
// 返回 cleanup 验证 graceful shutdown。每次独立 prometheus Registry。
func InitRankingCron() (*cron.Cron, func()) {
	wire.Build(
		infraSvcProvider,
		articleSvcProvider, // 依赖：interaction + article reader
		articleRankingSvcProvider,
		provideTestLockClient,
		provideTestCronMetrics,
		ioc.InitCronWrapper,
		clickEventSvcProvider, // ranking service 依赖 ClickEventService
		job.NewRankingJob,
		ioc.InitCron,
	)
	return nil, nil
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

// llmSvcProvider 仅 LLM 相关 provider，供 article_polish 用。
// 原 chatSvcProvider 已随 chat 服务搬到 chat/wire.go。
var llmSvcProvider = wire.NewSet(
	ioc.InitLLMConfig,
	ioc.InitLLMClient,
)
