//go:build wireinject

package setup

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

// 这个需要登录权限
func InitWebServer() *gin.Engine {
	wire.Build(
		//infra
		infraSvcProvider,
		//provider
		userSvcProvider,
		articleSvcProvider,
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
		jwt.NewRedisJwtHandler,

		ioc.InitMiddlewares,
		ioc.InitWebServer,
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

var articleSvcProvider = wire.NewSet(
	dao.NewGormArticleAuthorDAO,
	dao.NewGormArticleReaderDAO,
	cache.NewRedisArticleCache,
	repository.NewCacheArticleAuthorRepository,
	repository.NewCacheArticleReaderRepository,
	service.NewInternalArticleAuthorService,
	service.NewInternalArticleReaderService,
	interactionSvcProvider,
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
