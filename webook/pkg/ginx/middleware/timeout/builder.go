// Package timeout 提供 HTTP 请求超时中间件:给 c.Request.Context() 挂 deadline,到点 ctx 取消,
// 下游 ctx-aware 操作(DB/RPC)自然中止;流式 / SSE 路径按前缀豁免(不能被超时切断)。
// 软超时:不强制中断忽略 ctx 的 handler、不代写 504,与 grpcx unary 超时拦截器同语义。
// 与 pkg/ginx/middleware/{accesslog,metrics,ratelimit} 同层;接入见各 ioc/web.go。
package timeout

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// defaultTimeout 未配置 server.http.timeout 时的兜底。
const defaultTimeout = 15 * time.Second

type MiddlewareBuilder struct {
	timeout        time.Duration
	exemptPrefixes []string
}

// NewMiddlewareBuilder d<=0 用兜底默认 15s。
func NewMiddlewareBuilder(d time.Duration) *MiddlewareBuilder {
	if d <= 0 {
		d = defaultTimeout
	}
	return &MiddlewareBuilder{timeout: d}
}

// ExemptPrefix 追加豁免路径前缀(流式 / SSE,如 chat 的 /chat);命中前缀的请求不设 deadline。
func (b *MiddlewareBuilder) ExemptPrefix(prefixes ...string) *MiddlewareBuilder {
	b.exemptPrefixes = append(b.exemptPrefixes, prefixes...)
	return b
}

func (b *MiddlewareBuilder) Build() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		for _, p := range b.exemptPrefixes {
			if strings.HasPrefix(path, p) {
				c.Next()
				return
			}
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), b.timeout)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
