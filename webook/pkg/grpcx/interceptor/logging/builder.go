// Package logging 提供 gRPC 访问日志拦截器：记录 method / 耗时 / peer / status code。
package logging

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/boyxs/train-go/webook/pkg/grpcx/interceptor"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

type Builder interface {
	BuildUnaryServer() grpc.UnaryServerInterceptor
	BuildUnaryClient() grpc.UnaryClientInterceptor
}

type InterceptorBuilder struct {
	l logger.LoggerX
}

func NewInterceptorBuilder(l logger.LoggerX) Builder {
	return &InterceptorBuilder{l: l}
}

func (b *InterceptorBuilder) BuildUnaryServer() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		start := time.Now()
		defer func() {
			event := "normal"
			var fields []logger.Field
			if rec := recover(); rec != nil {
				event = "recover"
				// panic 详情（原值 + 栈）只进日志，不回客户端，避免泄漏内部细节
				fields = append(fields,
					logger.String("panic", fmt.Sprintf("%v", rec)),
					logger.String("stack", string(captureStack())))
				err = status.Error(codes.Internal, "internal error")
			}
			fields = append(fields,
				logger.Int64("cost", time.Since(start).Milliseconds()),
				logger.String("type", "unary"),
				logger.String("method", info.FullMethod),
				logger.String("event", event),
				logger.String("peer", interceptor.PeerName(ctx)),
				logger.String("peer_ip", interceptor.PeerIp(ctx)),
			)
			fields = appendStatus(fields, err)
			b.l.Info("Server RPC请求", fields...)
		}()
		resp, err = handler(ctx, req)
		return
	}
}

func (b *InterceptorBuilder) BuildUnaryClient() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) (err error) {
		start := time.Now()
		defer func() {
			event := "normal"
			var fields []logger.Field
			if rec := recover(); rec != nil {
				event = "recover"
				fields = append(fields,
					logger.String("panic", fmt.Sprintf("%v", rec)),
					logger.String("stack", string(captureStack())))
				err = status.Error(codes.Internal, "internal error")
			}
			fields = append(fields,
				logger.Int64("cost", time.Since(start).Milliseconds()),
				logger.String("type", "unary"),
				logger.String("method", method),
				logger.String("event", event),
			)
			fields = appendStatus(fields, err)
			b.l.Info("Client RPC请求", fields...)
		}()
		err = invoker(ctx, method, req, reply, cc, opts...)
		return
	}
}

// captureStack 抓当前 goroutine 的调用栈（false，不 dump 全部 goroutine）。
func captureStack() []byte {
	buf := make([]byte, 4096)
	return buf[:runtime.Stack(buf, false)]
}

// appendStatus 追加 gRPC code / message 字段（err 非 nil 时）。
func appendStatus(fields []logger.Field, err error) []logger.Field {
	if err == nil {
		return fields
	}
	s, _ := status.FromError(err)
	return append(fields,
		logger.String("code", s.Code().String()),
		logger.String("message", s.Message()),
	)
}
