package ioc

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// TestGinContextSpanPropagation 复现并验证：
// otelgin 把 span 放进 c.Request.Context()；handler 把 *gin.Context 当 context.Context
// 传下去时，trace.SpanFromContext(*gin.Context) 能否拿到同一个 span。
//
// Gin 1.11 的 Context.Value 需要 engine.ContextWithFallback = true 才 fallback
// 到 Request.Context().Value()，否则返回 nil → span 链断掉。
func TestGinContextSpanPropagation(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = tp.Shutdown(nil) })
	otel.SetTracerProvider(tp)
	tracer := tp.Tracer("test")

	// 辅助函数：模拟 otelgin 把 span 注入 Request.Context，然后 handler 传 *gin.Context 到 service
	makeRequest := func(enableFallback bool) trace.Span {
		eng := gin.New()
		eng.ContextWithFallback = enableFallback

		var capturedSpan trace.Span
		eng.GET("/t", func(c *gin.Context) {
			// 模拟 otelgin：把 span 写进 Request ctx
			ctx, span := tracer.Start(c.Request.Context(), "http-root")
			defer span.End()
			c.Request = c.Request.WithContext(ctx)

			// 模拟 handler：把 *gin.Context 当 context.Context 传下去
			capturedSpan = trace.SpanFromContext(c) // ← 等价于 ctx.Value(currentSpanKey)
		})

		req := httptest.NewRequest(http.MethodGet, "/t", nil)
		w := httptest.NewRecorder()
		eng.ServeHTTP(w, req)
		return capturedSpan
	}

	// 未开 ContextWithFallback：span 拿不到（noop），bug 复现
	spanWithoutFallback := makeRequest(false)
	assert.False(t, spanWithoutFallback.SpanContext().IsValid(),
		"bug 复现：未开 ContextWithFallback 时 trace.SpanFromContext(*gin.Context) 应返回 noop span")

	// 开启 ContextWithFallback：span 正确传递
	spanWithFallback := makeRequest(true)
	assert.True(t, spanWithFallback.SpanContext().IsValid(),
		"修复后：开启 ContextWithFallback，*gin.Context.Value 能 fallback 到 Request.Context()，span 可取到")
}
