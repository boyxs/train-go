// Package accesslog 提供 HTTP access log 中间件（Apache/nginx 同义术语）。
//
// 与已有 pkg/ginx/middleware/{metrics,ratelimit} 同层；接入方法见两个 ioc/web.go。
//
// 配置 yaml 段（可选，缺省仅记 path/method/status/duration 基础四元组）：
//
//	web:
//	  logger:
//	    allowReqBody: true
//	    allowResBody: false
//	    maxReqLen: 2048
//	    maxResLen: 2048
//	    maxPathLen: 256
//
// 通过 viper.OnConfigChange + ioc.ConfigChangeCallbacks 注册 LoadConfig，etcd 推送时热更。
package accesslog

import (
	"bytes"
	"context"
	"io"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

type LoggerMiddlewareBuilder struct {
	loggerFunc   func(ctx context.Context, l RequestLog)
	allowReqBody *atomic.Bool
	allowResBody *atomic.Bool
	maxReqLen    *atomic.Int64
	maxResLen    *atomic.Int64
	maxPathLen   *atomic.Int64
}

func NewLoggerMiddlewareBuilder(loggerFunc func(ctx context.Context, l RequestLog)) *LoggerMiddlewareBuilder {
	b := &LoggerMiddlewareBuilder{
		loggerFunc:   loggerFunc,
		allowReqBody: &atomic.Bool{},
		allowResBody: &atomic.Bool{},
		maxReqLen:    &atomic.Int64{},
		maxResLen:    &atomic.Int64{},
		maxPathLen:   &atomic.Int64{},
	}
	return b
}

func (b *LoggerMiddlewareBuilder) AllowReqBody(flag bool) *LoggerMiddlewareBuilder {
	b.allowReqBody.Store(flag)
	return b
}

func (b *LoggerMiddlewareBuilder) AllowResBody(flag bool) *LoggerMiddlewareBuilder {
	b.allowResBody.Store(flag)
	return b
}

// LoadConfig 从 viper.UnmarshalKey("web.logger", ...) 读配置；缺省值全 false/0
// （只记基础四元组，不抓 req/res body），生产风险低。注册到 ConfigChangeCallbacks 即可热更。
func (b *LoggerMiddlewareBuilder) LoadConfig() {
	var cfg LoggerConfig
	_ = viper.UnmarshalKey("web.logger", &cfg)
	b.allowReqBody.Store(cfg.AllowReqBody)
	b.allowResBody.Store(cfg.AllowResBody)
	b.maxReqLen.Store(int64(cfg.MaxReqLen))
	b.maxResLen.Store(int64(cfg.MaxResLen))
	b.maxPathLen.Store(int64(cfg.MaxPathLen))
}

func (b *LoggerMiddlewareBuilder) Build() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		path := ctx.Request.URL.Path
		path = truncate(path, int(b.maxPathLen.Load()))

		rl := RequestLog{
			Path:      path,
			Query:     ctx.Request.URL.RawQuery,
			Method:    ctx.Request.Method,
			ClientIP:  ctx.ClientIP(),
			UserAgent: ctx.Request.UserAgent(),
		}

		if b.allowReqBody.Load() {
			body, _ := ctx.GetRawData()
			rl.ReqBody = truncate(string(body), int(b.maxReqLen.Load()))
			ctx.Request.Body = io.NopCloser(bytes.NewReader(body))
		}

		start := time.Now()

		if b.allowResBody.Load() {
			ctx.Writer = &responseWriter{
				l:              &rl,
				maxLen:         int(b.maxResLen.Load()),
				ResponseWriter: ctx.Writer,
			}
		}

		defer func() {
			rl.Duration = time.Since(start)
			if rl.Status == 0 {
				rl.Status = ctx.Writer.Status()
			}
			b.loggerFunc(ctx, rl)
		}()

		ctx.Next()
	}
}

func truncate(s string, maxLen int) string {
	if maxLen > 0 && len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}

type responseWriter struct {
	gin.ResponseWriter
	l      *RequestLog
	maxLen int
}

func (r *responseWriter) Write(data []byte) (int, error) {
	r.l.ResBody = truncate(string(data), r.maxLen)
	return r.ResponseWriter.Write(data)
}

func (r *responseWriter) WriteString(s string) (int, error) {
	r.l.ResBody = truncate(s, r.maxLen)
	return r.ResponseWriter.WriteString(s)
}

func (r *responseWriter) WriteHeader(statusCode int) {
	r.l.Status = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

type RequestLog struct {
	Path      string        `json:"path"`
	Query     string        `json:"query"`
	Method    string        `json:"method"`
	ClientIP  string        `json:"client_ip"`
	UserAgent string        `json:"user_agent"`
	ReqBody   string        `json:"req_body"`
	Status    int           `json:"status"`
	ResBody   string        `json:"res_body"`
	Duration  time.Duration `json:"duration"`
}

type LoggerConfig struct {
	AllowReqBody bool `mapstructure:"allowReqBody"`
	AllowResBody bool `mapstructure:"allowResBody"`
	MaxReqLen    int  `mapstructure:"maxReqLen"`
	MaxResLen    int  `mapstructure:"maxResLen"`
	MaxPathLen   int  `mapstructure:"maxPathLen"`
}
