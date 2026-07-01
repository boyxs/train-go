//go:build wireinject

package main

import (
	"github.com/gin-gonic/gin"
	"github.com/google/wire"

	intrevt "github.com/webook/internal/events/interaction"
	grpcsrv "github.com/webook/internal/grpc"
	"github.com/webook/internal/ioc"
	"github.com/webook/internal/repository"
	"github.com/webook/internal/repository/cache"
	"github.com/webook/internal/repository/dao"
	"github.com/webook/internal/service"
	"github.com/webook/internal/web"
	"github.com/webook/pkg/grpcx"
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

// polishProviderSet 文章润色模块（含 LLM 配置/客户端 — 之前在 chatProviderSet 里）
var polishProviderSet = wire.NewSet(
	ioc.InitLLMConfig,
	ioc.InitLLMClient,
	service.NewAIArticlePolishService,
)

// articleRankingProviderSet 文章榜单模块（数据 + 重算逻辑留 core；定时调度由 webook-worker 经 RankingJobService gRPC 触发）
var articleRankingProviderSet = wire.NewSet(
	dao.NewGormArticleRankingDAO,
	cache.NewRedisArticleRankingCache,
	repository.NewCacheArticleRankingRepository,
	service.NewArticleRankingService,
	web.NewArticleRankingHandler,
	grpcsrv.NewRankingJobServer,
)

// migratorSDKProviderSet 业务侧迁移 SDK（默认 NoOp 零开销，yaml migrator.sdk.enabled=true 切 Redis 实现）
var migratorSDKProviderSet = wire.NewSet(
	ioc.InitMigratorSDKSwitchReader,
	ioc.InitMigratorSDKDualWriter,
	ioc.InitMigratorSDKTaskName,
	dao.NewGormArticleReaderNewDAO,
)

// kafkaProviderSet Kafka 生产侧：core 只产 read 事件，消费由 webook-worker（调度器）负责。
var kafkaProviderSet = wire.NewSet(
	ioc.InitKafkaConfig,
	ioc.InitSaramaConfig,
	ioc.InitEventProducer,
	intrevt.NewSaramaInteractionEventProducer,
)

// App 应用入口：Web 服务 + gRPC 服务（消费者/定时任务已抽到 webook-worker）。
type App struct {
	Server     *gin.Engine
	GRPCServer *grpcx.Server
}

func InitWebServer() (App, func(), error) {
	wire.Build(
		//infra
		ioc.InitDB, ioc.InitRedis, ioc.InitLogger, ioc.InitTimezone,
		ioc.InitOTel,
		//dao
		dao.NewGormUserDAO,
		dao.NewGormArticleAuthorDAO,
		dao.NewGormArticleReaderDAO,
		//cache
		cache.NewRedisUserCache, cache.NewRedisCodeCache, cache.NewRedisArticleCache,
		//repository
		repository.NewRedisUserRepository,
		repository.NewRedisCodeRepository,
		repository.NewCacheArticleAuthorRepository,
		repository.NewCacheArticleReaderRepository,
		//service
		ioc.InitSmsService,
		ioc.InitWechatOAuth2Service,
		service.NewInternalUserService,
		service.NewSmsCodeService,
		service.NewInternalArticleAuthorService,
		service.NewInternalArticleReaderService,
		ioc.InitRankingBoostPool,
		ioc.InitInteractionService,
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
		// 搜索模块
		searchProviderSet,
		migratorSDKProviderSet,
		// 点击埋点
		clickEventProviderSet,
		// 文章润色
		polishProviderSet,
		// 文章榜单
		articleRankingProviderSet,
		// kafka 生产侧（read 事件）
		kafkaProviderSet,

		ioc.InitMiddlewares,
		ioc.InitWebServer,
		// gRPC server
		ioc.InitEtcdClient,
		ioc.InitGRPCMetrics, // server + comment client 共享同一指标 builder
		ioc.InitGRPCServer,
		grpcsrv.NewSearchServer,
		grpcsrv.NewArticleReaderServer,
		// comment gRPC client（core 作 HTTP 网关 → comment 后端）
		ioc.InitCommentConn,
		ioc.InitCommentClient,
		web.NewInternalCommentHandler,
		// interaction gRPC client（core 作 HTTP 网关 → interaction 后端）
		ioc.InitInteractionConn,
		ioc.InitInteractionClient,
		wire.Struct(new(App), "*"),
	)
	return App{}, nil, nil
}
