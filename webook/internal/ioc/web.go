package ioc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel/trace"

	"github.com/webook/internal/consts"
	"github.com/webook/internal/web"
	"github.com/webook/pkg/ginx/middleware/accesslog"
	"github.com/webook/pkg/ginx/middleware/metrics"
	"github.com/webook/pkg/ginx/middleware/ratelimit"
	"github.com/webook/pkg/jwtx"
	"github.com/webook/pkg/logger"
)

func InitWebServer(
	middlewares []gin.HandlerFunc,
	userHandler web.UserHandler,
	articleHandler web.ArticleAuthorHandler,
	articleReaderHandler web.ArticleReaderHandler,
	interactionHandler web.InteractionHandler,
	oauth2Handler web.OAuth2Handler,
	searchHandler web.ArticleSearchHandler,
	clickEventHandler web.ClickEventHandler,
	polishHandler web.ArticlePolishHandler,
	rankingHandler web.RankingHandler,
) *gin.Engine {
	server := gin.Default()
	// 开启后 *gin.Context.Value() 会 fallback 到 c.Request.Context().Value()；
	// 否则 trace.SpanFromContext(*gin.Context) 拿不到 otelgin 写进 Request.Context 的 span，
	// 导致 DB/Redis 子 span 断链成独立 trace（详见 ioc/web_ctx_propagation_test.go）
	server.ContextWithFallback = true
	// /metrics 放在中间件之前，Prometheus 抓取不经过 CORS/限流/JWT/日志
	server.GET("/metrics", gin.WrapH(promhttp.Handler()))
	server.Use(middlewares...)
	userHandler.RegisterRoutes(server)
	articleHandler.RegisterRoutes(server)
	articleReaderHandler.RegisterRoutes(server)
	interactionHandler.RegisterRoutes(server)
	oauth2Handler.RegisterRoutes(server)
	searchHandler.RegisterRoutes(server)
	clickEventHandler.RegisterRoutes(server)
	polishHandler.RegisterRoutes(server)
	rankingHandler.RegisterRoutes(server)
	return server
}

func InitMiddlewares(
	l logger.LoggerX,
	cmd redis.Cmdable,
	tp trace.TracerProvider,
) []gin.HandlerFunc {
	return []gin.HandlerFunc{
		// Prometheus 指标采集（最外层，能统计所有请求包括被限流/拒绝的）
		metrics.NewPrometheusBuilder("webook", "http", "requests", "HTTP 请求统计").
			WithCounter().
			WithHistogram().
			WithSummary().
			WithInFlight().
			Build(),
		// OTel：紧随 Prometheus，创建 root span，让后续所有中间件 / handler 都在 span 上下文里
		otelgin.Middleware("webook-core",
			otelgin.WithTracerProvider(tp),
		),
		//cors
		cors.New(cors.Config{
			AllowOrigins: []string{"http://localhost:3000"},
			//AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
			AllowHeaders:     []string{"Content-Type", "Content-Length", jwtx.HeaderAuthorization, consts.AccessHeader, consts.LastEventIDHeader},
			ExposeHeaders:    []string{consts.AccessHeader, consts.RefreshHeader},
			AllowCredentials: true,
			AllowOriginFunc: func(origin string) bool {
				if strings.HasPrefix(origin, "http://localhost") {
					return true
				}
				// 局域网部署放行（同源 nginx 反代场景，浏览器仍会带 Origin header）
				if strings.HasPrefix(origin, "http://127.0.0.1") ||
					strings.HasPrefix(origin, "http://192.168.") ||
					strings.HasPrefix(origin, "http://10.") ||
					strings.HasPrefix(origin, "http://172.") {
					return true
				}
				return strings.Contains(origin, "https://github.com")
			},
			MaxAge: 12 * time.Hour,
		}),
		//限流
		ratelimit.NewBuilder(cmd, time.Second, 20, l).Prefix("ip_limiter").Build(),
		//jwt
		loginJwtMiddleware(cmd),
		// logger
		loggerMiddleware(l),
	}
}

func loggerMiddleware(l logger.LoggerX) gin.HandlerFunc {
	builder := accesslog.NewLoggerMiddlewareBuilder(func(ctx context.Context, val accesslog.RequestLog) {
		// 正式环境 默认 INFO，不输出 DEBUG
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

func loginJwtMiddleware(cmd redis.Cmdable) gin.HandlerFunc {
	// 验签走 pkg/jwtx 公共中间件，跨服务一致；签发由 jwtx.Handler 在登录 handler 内完成
	return jwtx.NewMiddlewareBuilder(jwtx.MiddlewareConfig{
		AccessKey: consts.AccessKey,
		UserKey:   consts.UserKey,
		Session: func(ctx *gin.Context, ssid string) bool {
			cnt, err := cmd.Exists(ctx, fmt.Sprintf(consts.UserSsidPattern, ssid)).Result()
			return err == nil && cnt > 0
		},
	}).
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
			"/article/ranking/page",
			"/article/ranking/archive/dates",
			"/metrics", // Prometheus 抓取端点，由 nginx /metrics 的 IP 白名单层把关
		).
		Build()
}
