// Package tracing 提供 gRPC 链路追踪拦截器：注入 / 透传 span 与 traceId。
// 注：otelgrpc 的 StatsHandler（传输层事件更细）与本拦截器二选一，
// 勿同时启用——否则每个 RPC 会产生两条 span。
package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/boyxs/train-go/webook/pkg/grpcx/interceptor"
)

const instrumentationName = "github.com/boyxs/train-go/webook/pkg/grpcx/interceptor/tracing"

// Builder 构造 server / client 两侧的链路追踪拦截器。
type Builder interface {
	BuildUnaryServer() grpc.UnaryServerInterceptor
	BuildUnaryClient() grpc.UnaryClientInterceptor
}

// InterceptorBuilder 注入 tracer / propagator；任一为 nil 时回退全局（由 InitOTel 注册），
// 注入则便于测试（传 recorder-backed tracer）。
type InterceptorBuilder struct {
	tracer     trace.Tracer
	propagator propagation.TextMapPropagator
}

func NewInterceptorBuilder(tracer trace.Tracer, propagator propagation.TextMapPropagator) Builder {
	return &InterceptorBuilder{tracer: tracer, propagator: propagator}
}

// resolve 取出 tracer/propagator，nil 时兜底全局。
func (b *InterceptorBuilder) resolve() (trace.Tracer, propagation.TextMapPropagator) {
	tracer := b.tracer
	if tracer == nil {
		tracer = otel.Tracer(instrumentationName)
	}
	propagator := b.propagator
	if propagator == nil {
		propagator = otel.GetTextMapPropagator()
	}
	return tracer, propagator
}

func (b *InterceptorBuilder) BuildUnaryClient() grpc.UnaryClientInterceptor {
	tracer, propagator := b.resolve()
	attrs := []attribute.KeyValue{
		attribute.String("rpc.system", "grpc"),
		attribute.String("rpc.grpc.kind", "unary"),
		attribute.String("rpc.component", "client"),
	}
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx, span := tracer.Start(ctx, method,
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(attrs...))
		defer span.End()

		// 注入 trace context 到 outgoing metadata，透传给下游
		ctx = inject(ctx, propagator)
		err := invoker(ctx, method, req, reply, cc, opts...)
		setStatus(span, err)
		return err
	}
}

func (b *InterceptorBuilder) BuildUnaryServer() grpc.UnaryServerInterceptor {
	tracer, propagator := b.resolve()
	attrs := []attribute.KeyValue{
		attribute.String("rpc.system", "grpc"),
		attribute.String("rpc.grpc.kind", "unary"),
		attribute.String("rpc.component", "server"),
	}
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		// 从 incoming metadata 提取上游 trace context
		ctx = extract(ctx, propagator)
		ctx, span := tracer.Start(ctx, info.FullMethod,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(attrs...))
		defer span.End()
		span.SetAttributes(
			attribute.String("rpc.method", info.FullMethod),
			attribute.String("net.peer.name", interceptor.PeerName(ctx)),
			attribute.String("net.peer.ip", interceptor.PeerIp(ctx)),
		)

		resp, err := handler(ctx, req)
		setStatus(span, err)
		return resp, err
	}
}

// setStatus 落 grpc status code 属性 + span 状态；出错标红 + RecordError。client / server 共用。
func setStatus(span trace.Span, err error) {
	st, _ := status.FromError(err)
	span.SetAttributes(attribute.Int64("rpc.grpc.status_code", int64(st.Code())))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, st.Message())
		return
	}
	span.SetStatus(codes.Ok, "OK")
}

// inject 把 trace context 写进 outgoing metadata。
func inject(ctx context.Context, propagator propagation.TextMapPropagator) context.Context {
	md, ok := metadata.FromOutgoingContext(ctx)
	if ok {
		md = md.Copy() // 不改上游可能共享的 outgoing md
	} else {
		md = metadata.MD{}
	}
	propagator.Inject(ctx, metadataCarrier(md))
	return metadata.NewOutgoingContext(ctx, md)
}

// extract 从 incoming metadata 还原上游 trace context。
func extract(ctx context.Context, propagator propagation.TextMapPropagator) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		md = metadata.MD{}
	}
	return propagator.Extract(ctx, metadataCarrier(md))
}

// metadataCarrier 让 grpc metadata.MD 适配 OTel TextMapCarrier（extract / inject traceparent）。
type metadataCarrier metadata.MD

func (m metadataCarrier) Get(key string) string {
	if vals := metadata.MD(m).Get(key); len(vals) > 0 {
		return vals[0]
	}
	return ""
}

func (m metadataCarrier) Set(key, value string) { metadata.MD(m).Set(key, value) }

func (m metadataCarrier) Keys() []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
