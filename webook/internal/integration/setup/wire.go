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
		web.NewInternalArticleHandler,
		web.NewOAuth2WechatHandler,
		jwt.NewRedisJwtHandler,

		ioc.InitMiddlewares,
		ioc.InitWebServer,
	)
	return gin.Default()
}

func InitArticleHandler() web.ArticleHandler {
	wire.Build(
		infraSvcProvider,
		//userSvcProvider,
		articleSvcProvider,
		web.NewInternalArticleHandler,
	)
	return &web.InternalArticleHandler{}
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
	repository.NewCacheArticleAuthorRepository,
	service.NewInternalArticleService,
)
