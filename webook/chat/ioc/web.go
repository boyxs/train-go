package ioc

import (
	"context"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel/trace"

	"github.com/webook/chat/consts"
	"github.com/webook/chat/web"
	"github.com/webook/pkg/ginx"
	"github.com/webook/pkg/ginx/middleware/accesslog"
	"github.com/webook/pkg/ginx/middleware/metrics"
	"github.com/webook/pkg/jwtx"
	"github.com/webook/pkg/logger"
)

// InitWebServer 与主仓 internal/ioc/web.go 同结构：middlewares 由 InitMiddlewares 装配后注入。
func InitWebServer(middlewares []gin.HandlerFunc, handler web.ChatHandler) *gin.Engine {
	server := gin.Default()
	ginx.UserKey = consts.UserKey // 登录态 ctx key，供 ginx.MustClaims/Claims 读取
	// otelgin 把 span 写在 Request.Context()，开启 ContextWithFallback 让 *gin.Context.Value() 也能取到，
	// 否则 DB/Redis 子 span 会断链。
	server.ContextWithFallback = true
	// /metrics 在中间件之前注册，prom 抓取不走 cors/jwt/限流。
	server.GET("/metrics", gin.WrapH(promhttp.Handler()))
	server.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "service": "chat"})
	})
	server.Use(middlewares...)
	handler.RegisterRoutes(server)
	return server
}

func InitMiddlewares(l logger.LoggerX, cmd redis.Cmdable, tp trace.TracerProvider) []gin.HandlerFunc {
	return []gin.HandlerFunc{
		// 1. prom HTTP 指标（最外层）
		metrics.NewPrometheusBuilder("webook", "http", "requests", "HTTP 请求统计").
			WithCounter().
			WithHistogram().
			WithSummary().
			WithInFlight().
			Build(),
		// 2. OTel root span
		otelgin.Middleware("webook-chat", otelgin.WithTracerProvider(tp)),
		// 3. CORS
		cors.New(cors.Config{
			AllowMethods:     []string{"PUT", "PATCH", "POST", "GET", "DELETE", "OPTIONS"},
			AllowHeaders:     []string{"Content-Type", jwtx.HeaderAuthorization, consts.AccessHeader, consts.RefreshHeader, consts.LastEventIDHeader},
			ExposeHeaders:    []string{consts.AccessHeader, consts.RefreshHeader},
			AllowCredentials: true,
			AllowOriginFunc: func(origin string) bool {
				if strings.HasPrefix(origin, "http://localhost") ||
					strings.HasPrefix(origin, "http://127.0.0.1") ||
					strings.HasPrefix(origin, "http://192.168.") ||
					strings.HasPrefix(origin, "http://10.") ||
					strings.HasPrefix(origin, "http://172.") {
					return true
				}
				return strings.Contains(origin, ".webook.com")
			},
			MaxAge: 12 * time.Hour,
		}),
		// 4. JWT 验签（pkg/jwtx 公共实现，所有服务共用）
		jwtx.NewMiddlewareBuilder(jwtx.MiddlewareConfig{
			AccessKey:      consts.AccessKey,
			UserKey:        consts.UserKey,
			Cmd:            cmd,
			SsidKeyPattern: consts.UserSsidPattern,
		}).Build(),
		// 5. Access log（chat yaml 没配 web.logger 段时仅记基础四元组，不抓 req/res body）
		loggerMiddleware(l),
	}
}

func loggerMiddleware(l logger.LoggerX) gin.HandlerFunc {
	builder := accesslog.NewLoggerMiddlewareBuilder(func(ctx context.Context, val accesslog.RequestLog) {
		l.Debug("HTTP request", logger.Field{Key: "request", Val: val})
	})
	builder.LoadConfig()
	ConfigChangeCallbacks = append(ConfigChangeCallbacks, builder.LoadConfig)
	return builder.Build()
}
