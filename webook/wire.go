//go:build wireinject

package main

import (
	"gitee.com/train-cloud/geektime-basic-go/internal/repository"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository/cache"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository/dao"
	"gitee.com/train-cloud/geektime-basic-go/internal/service"
	"gitee.com/train-cloud/geektime-basic-go/internal/web"
	"gitee.com/train-cloud/geektime-basic-go/ioc"
	"github.com/gin-gonic/gin"
	"github.com/google/wire"
)

func InitWebServer() *gin.Engine {
	wire.Build(
		//infra
		ioc.InitDB, ioc.InitRedis,
		//dao
		dao.NewGormUserDAO,
		//cache
		cache.NewRedisUserCache, cache.NewRedisCodeCache,
		//repository
		repository.NewRedisUserRepository, repository.NewRedisCodeRepository,
		//service
		ioc.InitSmsService,
		service.NewInternalUserService, service.NewSmsCodeService,
		//handler
		web.NewUserHandler,

		ioc.InitMiddlewares,
		ioc.InitWebServer,
	)
	return gin.Default()
}
