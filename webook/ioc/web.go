package ioc

import (
	"context"
	"strings"
	"time"

	"gitee.com/train-cloud/geektime-basic-go/internal/consts"
	"gitee.com/train-cloud/geektime-basic-go/internal/web"
	"gitee.com/train-cloud/geektime-basic-go/internal/web/jwt"
	"gitee.com/train-cloud/geektime-basic-go/internal/web/middleware"
	"gitee.com/train-cloud/geektime-basic-go/pkg/ginx/middleware/ratelimit"
	"gitee.com/train-cloud/geektime-basic-go/pkg/logger"
	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/sessions"
	redisSession "github.com/gin-contrib/sessions/redis"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func InitWebServer(
	middlewares []gin.HandlerFunc,
	userHandler web.UserHandler,
	articleHandler web.ArticleAuthorHandler,
	articleReaderHandler web.ArticleReaderHandler,
	interactionHandler web.InteractionHandler,
	oauth2Handler web.OAuth2Handler,
) *gin.Engine {
	server := gin.Default()
	server.Use(middlewares...)
	userHandler.RegisterRoutes(server)
	articleHandler.RegisterRoutes(server)
	articleReaderHandler.RegisterRoutes(server)
	interactionHandler.RegisterRoutes(server)
	oauth2Handler.RegisterRoutes(server)
	return server
}

func InitMiddlewares(
	hdl jwt.JwtHandler,
	l logger.LoggerX,
	cmd redis.Cmdable,
) []gin.HandlerFunc {
	return []gin.HandlerFunc{
		//自定义中间件
		//func(ctx *gin.Context) {},
		//cors
		cors.New(cors.Config{
			AllowOrigins: []string{"http://localhost:3000"},
			//AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
			AllowHeaders:     []string{"Content-Type", "Content-Length", "Authorization"},
			ExposeHeaders:    []string{consts.AccessHeader, consts.RefreshHeader},
			AllowCredentials: true,
			AllowOriginFunc: func(origin string) bool {
				if strings.HasPrefix(origin, "http://localhost") {
					return true
				}
				return strings.Contains(origin, "https://github.com")
			},
			MaxAge: 12 * time.Hour,
		}),
		//限流
		ratelimit.NewBuilder(cmd, time.Second, 20, l).Prefix("ip_limiter").Build(),
		//jwt
		loginJwtMiddleware(hdl),
		//session
		//redisSessionMiddleware(),
		//loginMiddleware(),
		// logger
		loggerMiddleware(l),
	}
}

// ConfigChangeCallbacks 远程配置变更时调用
var ConfigChangeCallbacks []func()

func loggerMiddleware(l logger.LoggerX) gin.HandlerFunc {
	builder := middleware.NewLoggerMiddlewareBuilder(func(ctx context.Context, val middleware.RequestLog) {
		l.Debug("HTTP request", logger.Field{
			Key: "request",
			Val: val,
		})
	}).
		AllowReqBody(true).
		AllowResBody(true)
	builder.LoadConfig()
	ConfigChangeCallbacks = append(ConfigChangeCallbacks, builder.LoadConfig)
	return builder.Build()
}

func loginJwtMiddleware(hdl jwt.JwtHandler) gin.HandlerFunc {
	return middleware.NewLoginJwtMiddlewareBuilder(hdl).
		IgnoredPaths("/user/register",
			"/user/login",
			"/user/refresh_token",
			"/user/login_sms/code/send",
			"/user/login_sms",
			"/oauth2/wechat/authurl",
			"/oauth2/wechat/callback",
			"/article/reader/detail",
			"/article/reader/page",
			"/interaction/view",
		).
		OptionalPaths(
			"/interaction/detail",
		).
		Build()
}

func loginMiddleware(l logger.LoggerX) gin.HandlerFunc {
	return middleware.NewLoginMiddlewareBuilder(l).
		IgnorePaths("/user/register", "/user/login").
		Build()
}

func redisSessionMiddleware() gin.HandlerFunc {
	store, err := redisSession.NewStore(16,
		"tcp",
		"localhost:6379",
		"",
		"",
		[]byte("k6CswdUm75WKcbM68UQUuxVsHSpTCwgK"),
	)
	if err != nil {
		panic(err)
	}
	return sessions.Sessions("ssid", store)
}
