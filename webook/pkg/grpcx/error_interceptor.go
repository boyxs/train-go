// Package grpcx 提供 gRPC 通用拦截器，目前包含错误处理（kratos 风格 *errs.Error ↔ status.Status 双向转换）。
package grpcx

import (
	"context"
	"errors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/webook/pkg/errs"
	"github.com/webook/pkg/logger"
)

// L 全局 Logger，由 ioc 在启动时注入；默认 NopLogger 防 nil panic
var L logger.LoggerX = logger.NewNopLogger()

// UnaryServerErrorInterceptor 服务端拦截器：handler 返 *errs.Error → 自动转 status.Status，
// 客户端 status.Code(err) 直接拿到业务对应的 gRPC code，不丢语义。
//
// 注：当前 status 不带 details，跨进程后 Metadata 丢失（仅保留 Code+Message）。
// 后续如需保留 Metadata，可用 status.WithDetails 把 *errs.Error 序列化为 details proto。
func UnaryServerErrorInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		resp, err := handler(ctx, req)
		if err == nil {
			return resp, nil
		}
		var be *errs.Error
		if errors.As(err, &be) {
			return nil, be.GRPCStatus().Err()
		}
		// 非 *errs.Error 的系统错误：原 err 留 server 日志，client 只收 generic message
		// 避免泄漏 SQL stmt / DSN / stack 等敏感信息到调用方
		L.Error("gRPC server unhandled error",
			logger.String("method", info.FullMethod),
			logger.Error(err))
		return nil, status.Error(codes.Internal, "internal error")
	}
}

// UnaryClientErrorInterceptor 客户端拦截器：把 status.Status 转回 *errs.Error，
// 调用方 errors.Is(err, errs.ErrXxx) / errors.As(&be) 透明，跨服务等价单进程内调用体验。
func UnaryClientErrorInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		err := invoker(ctx, method, req, reply, cc, opts...)
		if err == nil {
			return nil
		}
		return errs.FromError(err)
	}
}
