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

func InitWebServer() *gin.Engine {
	wire.Build(
		//infra
		ioc.InitDB, ioc.InitRedis, ioc.InitLogger, ioc.InitTimezone,
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
		//handler
		web.NewInternalUserHandler,
		web.NewInternalArticleAuthorHandler,
		web.NewInternalArticleReaderHandler,
		web.NewOAuth2WechatHandler,
		jwt.NewRedisJwtHandler,

		ioc.InitMiddlewares,
		ioc.InitWebServer,
	)
	return gin.Default()
}
