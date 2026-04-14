package ioc

import (
	"context"
	"strings"
	"time"

	"gitee.com/train-cloud/geektime-basic-go/internal/consts"
	"gitee.com/train-cloud/geektime-basic-go/internal/web"
	"gitee.com/train-cloud/geektime-basic-go/internal/web/jwt"
	"gitee.com/train-cloud/geektime-basic-go/internal/web/middleware"
	"gitee.com/train-cloud/geektime-basic-go/pkg/ginx/middleware/metrics"
	"gitee.com/train-cloud/geektime-basic-go/pkg/ginx/middleware/ratelimit"
	"gitee.com/train-cloud/geektime-basic-go/pkg/logger"
	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/sessions"
	redisSession "github.com/gin-contrib/sessions/redis"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

func InitWebServer(
	middlewares []gin.HandlerFunc,
	userHandler web.UserHandler,
	articleHandler web.ArticleAuthorHandler,
	articleReaderHandler web.ArticleReaderHandler,
	interactionHandler web.InteractionHandler,
	oauth2Handler web.OAuth2Handler,
	chatHandler web.ChatHandler,
	searchHandler web.ArticleSearchHandler,
	clickEventHandler web.ClickEventHandler,
	polishHandler web.ArticlePolishHandler,
) *gin.Engine {
	server := gin.Default()
	// /metrics 放在中间件之前，Prometheus 抓取不经过 CORS/限流/JWT/日志
	server.GET("/metrics", gin.WrapH(promhttp.Handler()))
	server.Use(middlewares...)
	userHandler.RegisterRoutes(server)
	articleHandler.RegisterRoutes(server)
	articleReaderHandler.RegisterRoutes(server)
	interactionHandler.RegisterRoutes(server)
	oauth2Handler.RegisterRoutes(server)
	chatHandler.RegisterRoutes(server)
	searchHandler.RegisterRoutes(server)
	clickEventHandler.RegisterRoutes(server)
	polishHandler.RegisterRoutes(server)
	return server
}

func InitMiddlewares(
	hdl jwt.JwtHandler,
	l logger.LoggerX,
	cmd redis.Cmdable,
) []gin.HandlerFunc {
	return []gin.HandlerFunc{
		// Prometheus 指标采集（最外层，能统计所有请求包括被限流/拒绝的）
		metrics.NewPrometheusBuilder("webook", "http", "requests", "HTTP 请求统计").
			WithCounter().
			WithHistogram().
			WithSummary().
			WithInFlight().
			Build(),
		//cors
		cors.New(cors.Config{
			AllowOrigins: []string{"http://localhost:3000"},
			//AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
			AllowHeaders:     []string{"Content-Type", "Content-Length", "Authorization", consts.AccessHeader, consts.LastEventIDHeader},
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
