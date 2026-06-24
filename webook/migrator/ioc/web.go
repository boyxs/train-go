package ioc

import (
	"context"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel/trace"

	"github.com/webook/migrator/consts"
	"github.com/webook/migrator/web"
	"github.com/webook/migrator/web/middleware"
	"github.com/webook/pkg/ginx/middleware/accesslog"
	"github.com/webook/pkg/ginx/middleware/metrics"
	"github.com/webook/pkg/ginx/middleware/ratelimit"
	"github.com/webook/pkg/jwtx"
	"github.com/webook/pkg/logger"
)

// InitWebServer 装配 gin engine + 注入 middleware 链 + 注册路由。
//
// middleware 链由 InitMiddlewares 统一构造好传进来，
// InitWebServer 只负责 server.Use(...) 装配 + 路由注册。
func InitWebServer(
	middlewares []gin.HandlerFunc,
	taskHandler web.TaskHandler,
	_ *MigrationMetricsCollector, // 仅 wire 依赖牵引：保证业务指标 Collector 在 server 起前已 MustRegister
) *gin.Engine {
	server := gin.Default()
	// otelgin 把 span 写在 Request.Context()，开启 ContextWithFallback 让 *gin.Context.Value() 也能取到，
	// 否则 DB/Redis 子 span 会断链。
	server.ContextWithFallback = true
	// /metrics + /health 在中间件之前注册：prom 抓取不走 cors/jwt/audit。
	server.GET("/metrics", gin.WrapH(promhttp.Handler()))
	server.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "service": "migrator"})
	})
	server.Use(middlewares...)
	taskHandler.RegisterRoutes(server)
	return server
}

// InitMiddlewares 统一构造全部业务路由共享的中间件链。
//
// 顺序（执行先后）：
//
//	metrics  → otel  → cors  → jwt  → ratelimit  → accesslog  → audit
//	最外层    span    跨域    认证   限流          访问日志       审计落表
//
// yaml 开关：
//
//	web.jwt.disabled: true   跳过 JWT middleware（本地无 webook-core 签发 token 时用）
func InitMiddlewares(
	l logger.LoggerX,
	cmd redis.Cmdable,
	tp trace.TracerProvider,
	audit *middleware.AuditMiddleware,
) []gin.HandlerFunc {
	mws := []gin.HandlerFunc{
		metrics.NewPrometheusBuilder("webook", "http", "requests", "HTTP 请求统计").
			WithCounter().
			WithHistogram().
			WithSummary().
			WithInFlight().
			Build(),
		otelgin.Middleware("webook-migrator", otelgin.WithTracerProvider(tp)),
		cors.New(cors.Config{
			AllowMethods:     []string{"PUT", "PATCH", "POST", "GET", "DELETE", "OPTIONS"},
			AllowHeaders:     []string{"Content-Type", jwtx.HeaderAuthorization, consts.AccessHeader, consts.RefreshHeader},
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
	}
	if !viper.GetBool("web.jwt.disabled") {
		mws = append(mws, jwtx.NewMiddlewareBuilder(jwtx.MiddlewareConfig{
			AccessKey:      consts.AccessKey,
			UserKey:        consts.UserKey,
			Cmd:            cmd,
			SsidKeyPattern: consts.UserSsidPattern,
		}).Build())
	}
	// Rate limit：默认 IP 级，1 秒 100 请求；yaml `web.ratelimit.{interval,rate}` 可覆盖。
	rlInterval := viper.GetDuration("web.ratelimit.interval")
	if rlInterval <= 0 {
		rlInterval = time.Second
	}
	rlRate := viper.GetInt("web.ratelimit.rate")
	if rlRate <= 0 {
		rlRate = 100
	}
	mws = append(mws,
		ratelimit.NewBuilder(cmd, rlInterval, rlRate, l).Prefix("migrator-ip-limiter").Build(),
		loggerMiddleware(l),
		audit.Build(),
	)
	return mws
}

func loggerMiddleware(l logger.LoggerX) gin.HandlerFunc {
	builder := accesslog.NewLoggerMiddlewareBuilder(func(ctx context.Context, val accesslog.RequestLog) {
		l.Debug("HTTP request", logger.Field{Key: "request", Val: val})
	})
	builder.LoadConfig()
	ConfigChangeCallbacks = append(ConfigChangeCallbacks, builder.LoadConfig)
	return builder.Build()
}
