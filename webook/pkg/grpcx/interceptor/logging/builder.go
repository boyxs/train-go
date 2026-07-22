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
					logger.String("grpc.panic", fmt.Sprintf("%v", rec)),
					logger.String("grpc.stack", string(captureStack())))
				err = status.Error(codes.Internal, "internal error")
			}
			fields = append(fields,
				logger.Int64("grpc.cost", time.Since(start).Milliseconds()),
				logger.String("grpc.type", "unary"),
				logger.String("grpc.method", info.FullMethod),
				logger.String("grpc.event", event),
				logger.String("grpc.peer", interceptor.PeerName(ctx)),
				logger.String("grpc.peer_ip", interceptor.PeerIp(ctx)),
			)
			fields = appendStatus(fields, err)
			// 正常调用记 Debug（ELK 默认丢 debug，不刷每 RPC 一条的流水）；出错/panic 记 Error，info+ 环境也可见
			lg := b.l
			if event == "recover" || err != nil {
				lg.Error(ctx, "Server RPC请求", fields...)
			} else {
				lg.Debug(ctx, "Server RPC请求", fields...)
			}
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
					logger.String("grpc.panic", fmt.Sprintf("%v", rec)),
					logger.String("grpc.stack", string(captureStack())))
				err = status.Error(codes.Internal, "internal error")
			}
			fields = append(fields,
				logger.Int64("grpc.cost", time.Since(start).Milliseconds()),
				logger.String("grpc.type", "unary"),
				logger.String("grpc.method", method),
				logger.String("grpc.event", event),
			)
			fields = appendStatus(fields, err)
			lg := b.l
			if event == "recover" || err != nil {
				lg.Error(ctx, "Client RPC请求", fields...)
			} else {
				lg.Debug(ctx, "Client RPC请求", fields...)
			}
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
	// grpc.* 命名空间：message 若用裸键会与 zap 的日志 message 键撞成重复 JSON 键
	return append(fields,
		logger.String("grpc.code", s.Code().String()),
		logger.String("grpc.message", s.Message()),
	)
}
