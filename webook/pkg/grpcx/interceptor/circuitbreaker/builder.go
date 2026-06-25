// Package circuitbreaker 提供 gRPC 熔断拦截器：熔断打开时 fail-fast 拒发请求，
// 按调用结果反馈给 aegis 自适应熔断器。
package circuitbreaker

import (
	"context"

	cb "github.com/go-kratos/aegis/circuitbreaker"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Builder interface {
	BuildUnaryServer() grpc.UnaryServerInterceptor
	BuildUnaryClient() grpc.UnaryClientInterceptor
}

type InterceptorBuilder struct {
	breaker cb.CircuitBreaker
}

func NewInterceptorBuilder(breaker cb.CircuitBreaker) Builder {
	return &InterceptorBuilder{breaker: breaker}
}

func (b *InterceptorBuilder) BuildUnaryServer() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if b.breaker.Allow() != nil {
			// 熔断打开：fail-fast，不调用 handler，也不计失败（拒绝由 aegis 自管）
			return nil, status.Error(codes.Unavailable, "circuit breaker open")
		}
		resp, err := handler(ctx, req)
		b.mark(err)
		return resp, err
	}
}

func (b *InterceptorBuilder) BuildUnaryClient() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if b.breaker.Allow() != nil {
			return status.Error(codes.Unavailable, "circuit breaker open")
		}
		err := invoker(ctx, method, req, reply, cc, opts...)
		b.mark(err)
		return err
	}
}

// mark 按错误类型反馈熔断器：仅服务端/依赖故障计失败；
// 客户端错误（InvalidArgument / NotFound 等）说明依赖本身健康，计成功，避免误伤熔断。
func (b *InterceptorBuilder) mark(err error) {
	if isDependencyFailure(err) {
		b.breaker.MarkFailed()
	} else {
		b.breaker.MarkSuccess()
	}
}

func isDependencyFailure(err error) bool {
	if err == nil {
		return false
	}
	switch status.Code(err) {
	case codes.Unavailable, codes.DeadlineExceeded, codes.Internal, codes.ResourceExhausted, codes.Unknown:
		return true
	default:
		return false
	}
}
