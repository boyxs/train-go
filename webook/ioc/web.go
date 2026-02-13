package ioc

import (
	"strings"
	"time"

	"gitee.com/train-cloud/geektime-basic-go/internal/consts"
	"gitee.com/train-cloud/geektime-basic-go/internal/web"
	"gitee.com/train-cloud/geektime-basic-go/internal/web/middleware"
	"gitee.com/train-cloud/geektime-basic-go/pkg/ginx/middleware/ratelimit"
	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/sessions"
	redisSession "github.com/gin-contrib/sessions/redis"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func InitWebServer(
	middlewares []gin.HandlerFunc,
	userHandler web.UserHandler,
) *gin.Engine {
	server := gin.Default()
	server.Use(middlewares...)
	userHandler.RegisterRoutes(server)
	return server
}

func InitMiddlewares(cmd redis.Cmdable) []gin.HandlerFunc {
	return []gin.HandlerFunc{
		//自定义中间件
		//func(ctx *gin.Context) {},
		//cors
		cors.New(cors.Config{
			AllowOrigins: []string{"http://localhost:3000"},
			//AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
			AllowHeaders:     []string{"Content-Type", "Content-Length", "Authorization"},
			ExposeHeaders:    []string{consts.JwtHeader},
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
		ratelimit.NewBuilder(cmd, time.Second, 20).Prefix("ip_limiter").Build(),
		//jwt
		loginJwtMiddleware(),
		//session
		//redisSessionMiddleware(),
		//loginMiddleware(),
	}
}

func loginJwtMiddleware() gin.HandlerFunc {
	return middleware.NewLoginJwtMiddlewareBuilder().
		IgnorePaths("/user/register", "/user/login", "/user/login_sms/code/send", "/user/login_sms").
		Build()
}

func loginMiddleware() gin.HandlerFunc {
	return middleware.NewLoginMiddlewareBuilder().
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
