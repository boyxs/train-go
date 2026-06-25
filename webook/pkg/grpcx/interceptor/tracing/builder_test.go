package tracing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// newRecorder 给一个内存 SpanRecorder + 基于它的 tracer（注入 builder，不碰全局）。
func newRecorder() (*tracetest.SpanRecorder, trace.Tracer) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	return sr, tp.Tracer("test")
}

func hasInt64Attr(span sdktrace.ReadOnlySpan, key string, val int64) bool {
	for _, a := range span.Attributes() {
		if string(a.Key) == key && a.Value.AsInt64() == val {
			return true
		}
	}
	return false
}

func TestServer_CreatesSpan(t *testing.T) {
	sr, tracer := newRecorder()
	b := NewInterceptorBuilder(tracer, propagation.TraceContext{})
	_, err := b.BuildUnaryServer()(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/x.Y/Z"},
		func(context.Context, any) (any, error) { return "ok", nil })
	require.NoError(t, err)

	spans := sr.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, "/x.Y/Z", spans[0].Name())
	assert.Equal(t, trace.SpanKindServer, spans[0].SpanKind())
	assert.Equal(t, codes.Ok, spans[0].Status().Code)
}

func TestServer_Error_SpanStatusError(t *testing.T) {
	sr, tracer := newRecorder()
	b := NewInterceptorBuilder(tracer, propagation.TraceContext{})
	_, _ = b.BuildUnaryServer()(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/x.Y/Z"},
		func(context.Context, any) (any, error) { return nil, status.Error(grpccodes.NotFound, "nope") })

	spans := sr.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Error, spans[0].Status().Code)
	assert.True(t, hasInt64Attr(spans[0], "rpc.grpc.status_code", int64(grpccodes.NotFound)),
		"应记录 rpc.grpc.status_code 属性")
}

func TestClient_InjectsTraceparent(t *testing.T) {
	sr, tracer := newRecorder()
	b := NewInterceptorBuilder(tracer, propagation.TraceContext{})
	var gotMD metadata.MD
	err := b.BuildUnaryClient()(context.Background(), "/x.Y/Z", nil, nil, nil,
		func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
			gotMD, _ = metadata.FromOutgoingContext(ctx)
			return nil
		})
	require.NoError(t, err)

	require.Len(t, sr.Ended(), 1)
	assert.Equal(t, trace.SpanKindClient, sr.Ended()[0].SpanKind())
	assert.NotEmpty(t, gotMD.Get("traceparent"), "client 应把 traceparent 注入 outgoing metadata")
}

func TestClient_PreservesExistingOutgoingMD(t *testing.T) {
	_, tracer := newRecorder()
	b := NewInterceptorBuilder(tracer, propagation.TraceContext{})
	// 已有 outgoing md：inject 应 copy 并保留原有 header + 追加 traceparent
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("existing", "v"))
	var gotMD metadata.MD
	err := b.BuildUnaryClient()(ctx, "/x.Y/Z", nil, nil, nil,
		func(c context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
			gotMD, _ = metadata.FromOutgoingContext(c)
			return nil
		})
	require.NoError(t, err)
	assert.Equal(t, []string{"v"}, gotMD.Get("existing"), "原有 metadata 保留")
	assert.NotEmpty(t, gotMD.Get("traceparent"), "traceparent 已注入")
}

func TestPropagation_ClientToServer_SameTrace(t *testing.T) {
	sr, tracer := newRecorder()
	b := NewInterceptorBuilder(tracer, propagation.TraceContext{})

	// client：拿到注入后的 outgoing metadata
	var outMD metadata.MD
	_ = b.BuildUnaryClient()(context.Background(), "/x.Y/Z", nil, nil, nil,
		func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
			outMD, _ = metadata.FromOutgoingContext(ctx)
			return nil
		})

	// 把 client 的 outgoing 当作 server 的 incoming（模拟跨进程）
	serverCtx := metadata.NewIncomingContext(context.Background(), outMD)
	_, _ = b.BuildUnaryServer()(serverCtx, nil, &grpc.UnaryServerInfo{FullMethod: "/x.Y/Z"},
		func(context.Context, any) (any, error) { return "ok", nil })

	spans := sr.Ended()
	require.Len(t, spans, 2)
	// client(先 End) 与 server(后 End) 同一条 trace
	assert.Equal(t, spans[0].SpanContext().TraceID(), spans[1].SpanContext().TraceID(),
		"client→server 应串成同一条 trace")
}

func TestNilDeps_FallsBackToGlobal(t *testing.T) {
	// tracer / propagator 传 nil 时回退全局，不应 panic，正常透传
	b := NewInterceptorBuilder(nil, nil)
	resp, err := b.BuildUnaryServer()(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/x.Y/Z"},
		func(context.Context, any) (any, error) { return "ok", nil })
	require.NoError(t, err)
	assert.Equal(t, "ok", resp)
}

func TestMetadataCarrier(t *testing.T) {
	c := metadataCarrier(metadata.MD{})
	c.Set("traceparent", "abc")
	assert.Equal(t, "abc", c.Get("traceparent"))
	assert.Equal(t, "", c.Get("missing"))
	assert.Contains(t, c.Keys(), "traceparent")
}
