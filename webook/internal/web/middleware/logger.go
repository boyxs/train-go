package middleware

import (
	"bytes"
	"context"
	"io"
	"time"

	"github.com/gin-gonic/gin"
)

type LoggerMiddlewareBuilder struct {
	loggerFunc   func(ctx context.Context, l RequestLog)
	allowReqBody bool
	allowResBody bool
}

func (l *LoggerMiddlewareBuilder) AllowReqBody() *LoggerMiddlewareBuilder {
	l.allowReqBody = true
	return l
}

func (l *LoggerMiddlewareBuilder) AllowResBody() *LoggerMiddlewareBuilder {
	l.allowResBody = true
	return l
}

func (l *LoggerMiddlewareBuilder) Build() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		path := ctx.Request.URL.Path
		if len(path) > 1024 {
			path = path[:1024]
		}
		method := ctx.Request.Method
		rl := RequestLog{
			Path:      path,
			Query:     ctx.Request.URL.RawQuery,
			Method:    method,
			ClientIP:  ctx.ClientIP(),
			UserAgent: ctx.Request.UserAgent(),
		}
		if l.allowReqBody {
			body, _ := ctx.GetRawData()
			if len(body) > 2048 {
				rl.ReqBody = string(body[:2048])
			} else {
				rl.ReqBody = string(body)
			}
			ctx.Request.Body = io.NopCloser(bytes.NewReader(body))
			//ctx.Request.Body = io.NopCloser(bytes.NewBuffer(body))
		}

		start := time.Now()

		if l.allowResBody {
			ctx.Writer = &ResponseWriter{
				l:              &rl,
				ResponseWriter: ctx.Writer,
			}
		}

		defer func() {
			rl.Duration = time.Since(start)
			if rl.Status == 0 {
				rl.Status = ctx.Writer.Status()
			}
			l.loggerFunc(ctx, rl)
		}()

		ctx.Next()
	}
}

func NewLoggerMiddlewareBuilder(loggerFunc func(ctx context.Context, l RequestLog)) *LoggerMiddlewareBuilder {
	return &LoggerMiddlewareBuilder{
		loggerFunc: loggerFunc,
	}
}

type ResponseWriter struct {
	gin.ResponseWriter
	l *RequestLog
}

func (r *ResponseWriter) Write(data []byte) (int, error) {
	r.l.ResBody = string(data)
	if len(r.l.ResBody) > 2048 {
		r.l.ResBody = r.l.ResBody[:2048]
	}
	return r.ResponseWriter.Write(data)
}

func (r *ResponseWriter) WriteString(s string) (int, error) {
	r.l.ResBody = s
	if len(r.l.ResBody) > 2048 {
		r.l.ResBody = r.l.ResBody[:2048]
	}
	return r.ResponseWriter.WriteString(s)
}

func (r *ResponseWriter) WriteHeader(statusCode int) {
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
