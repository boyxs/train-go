package otel

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/zipkin"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// 默认 Zipkin collector 地址，本地起：
//
//	docker run -d -p 9411:9411 openzipkin/zipkin
//
// 浏览器访问 http://localhost:9411 查看 trace
const zipkinEndpoint = "http://localhost:9411/api/v2/spans"

// 初始化 Zipkin TracerProvider：BatchSpanProcessor + service.name
// 跟 stdout 版的区别：
//  1. exporter 改成 zipkin.New(endpoint)
//  2. 用 BatchSpanProcessor（生产标配，攒批发送，降低 IO）
//  3. 测试结束 Shutdown 会 flush 剩余 span
func initZipkinTracer(t *testing.T) (trace.Tracer, func()) {
	exporter, err := zipkin.New(zipkinEndpoint)
	require.NoError(t, err)

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("otel-demo-zipkin"),
			semconv.ServiceVersion("v0.0.1"),
		),
	)
	require.NoError(t, err)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter,
			sdktrace.WithBatchTimeout(2*time.Second),
		),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)

	tracer := tp.Tracer("otel-demo/zipkin_test")
	return tracer, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tp.Shutdown(ctx)
	}
}

// Zipkin 没起就跳过，避免本地/CI 没装 zipkin 的人测试红灯
func requireZipkin(t *testing.T) {
	client := http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get("http://localhost:9411/health")
	if err != nil {
		t.Skipf("zipkin 未运行，跳过：%v（启动：docker run -d -p 9411:9411 openzipkin/zipkin）", err)
		return
	}
	_ = resp.Body.Close()
}

// 父子 span：发到 Zipkin 后可在 UI 看到调用链火焰图
func TestZipkinTracer(t *testing.T) {
	requireZipkin(t)
	tracer, shutdown := initZipkinTracer(t)
	defer shutdown()

	ctx, parent := tracer.Start(context.Background(), "zipkin-parent-op")
	parent.SetAttributes(attribute.String("layer", "web"))

	func(ctx context.Context) {
		_, child := tracer.Start(ctx, "zipkin-child-op")
		defer child.End()
		child.SetAttributes(attribute.String("layer", "service"))
		time.Sleep(20 * time.Millisecond)
	}(ctx)

	parent.End()
	assert.True(t, parent.SpanContext().IsValid())
	t.Logf("trace_id=%s 在 http://localhost:9411 用 traceID 搜索查看",
		parent.SpanContext().TraceID().String())
}

// 错误标记：Zipkin UI 会把 status=Error 的 span 标红
func TestZipkinErrorSpan(t *testing.T) {
	requireZipkin(t)
	tracer, shutdown := initZipkinTracer(t)
	defer shutdown()

	_, span := tracer.Start(context.Background(), "zipkin-failing-op")
	defer span.End()

	span.SetAttributes(attribute.String("http.method", "GET"))
	span.AddEvent("about to fail")
	span.RecordError(assertErr{msg: "模拟错误：DB 超时"})
	span.SetStatus(codes.Error, "DB 超时")

	assert.True(t, span.SpanContext().IsValid())
}

type assertErr struct{ msg string }

func (e assertErr) Error() string { return e.msg }
