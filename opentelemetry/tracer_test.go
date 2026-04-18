package otel

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// 初始化 TracerProvider：stdout exporter + service.name 资源标签
// 返回的 shutdown 必须在测试结束时调用，确保 span flush
// 注意：pretty print 会输出大段 JSON span，-short 模式下跳过避免 CI 噪音
func initTracer(t *testing.T) (trace.Tracer, func()) {
	if testing.Short() {
		t.Skip("stdout exporter 输出噪音大，-short 模式跳过")
	}
	exporter, err := stdouttrace.New(
		stdouttrace.WithPrettyPrint(),
	)
	require.NoError(t, err)

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("otel-demo"),
			semconv.ServiceVersion("v0.0.1"),
		),
	)
	require.NoError(t, err)

	tp := sdktrace.NewTracerProvider(
		// 测试场景：同步导出，避免 goroutine 异步导致测试结束 span 还没打出来
		sdktrace.WithSyncer(exporter),
		sdktrace.WithResource(res),
		// 全采样：开发/测试常用
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	// 全局注册：otel.Tracer(name) 才能拿到
	otel.SetTracerProvider(tp)

	tracer := tp.Tracer("otel-demo/tracer_test")
	return tracer, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = tp.Shutdown(ctx)
	}
}

// 父子 span：通过 ctx 串联调用链
func TestTracer(t *testing.T) {
	tracer, shutdown := initTracer(t)
	defer shutdown()

	ctx, parent := tracer.Start(context.Background(), "parent-op")
	parent.SetAttributes(attribute.String("layer", "web"))

	// 模拟一次内部调用：把 ctx 透传下去，子 span 自动挂到 parent
	func(ctx context.Context) {
		_, child := tracer.Start(ctx, "child-op")
		defer child.End()
		child.SetAttributes(attribute.String("layer", "service"))
		time.Sleep(10 * time.Millisecond)
	}(ctx)

	parent.End()

	// 断言 SpanContext 有效（TraceID 非零）
	assert.True(t, parent.SpanContext().IsValid())
	assert.True(t, parent.SpanContext().TraceID().IsValid())
}

// attribute / event / status：常见的 span 装饰
func TestSpanAttributes(t *testing.T) {
	tracer, shutdown := initTracer(t)
	defer shutdown()

	_, span := tracer.Start(context.Background(), "http-request")
	defer span.End()

	// 属性：键值对，用于过滤/聚合
	span.SetAttributes(
		attribute.String("http.method", "POST"),
		attribute.String("http.route", "/users/:id"),
		attribute.Int("http.status_code", 200),
		attribute.Int64("user.id", 1024),
	)

	// 事件：时间点 + 上下文，用于标记关键节点
	span.AddEvent("cache.miss", trace.WithAttributes(
		attribute.String("key", "user:1024"),
	))
	time.Sleep(5 * time.Millisecond)
	span.AddEvent("db.query.done")

	// 状态：错误标记 + RecordError
	// 这里只演示，正常路径用 codes.Ok
	span.SetStatus(codes.Ok, "")

	assert.True(t, span.SpanContext().IsValid())
}
