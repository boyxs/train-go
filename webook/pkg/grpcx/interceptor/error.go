// Package interceptor 提供 gRPC 通用拦截器：错误处理（*errs.Error ↔ status 双向转换）。
package interceptor

import (
	"context"
	"errors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/webook/pkg/errs"
	"github.com/webook/pkg/logger"
)

// L 全局 Logger，由 ioc 在启动时注入；默认 NopLogger 防 nil panic。
var L logger.LoggerX = logger.NewNopLogger()

// UnaryServerError 把 handler 的 *errs.Error 转成 status（client 拿到对应 code）；
// 非 *errs.Error 的系统错误只记日志 + 回 generic message，避免泄漏内部细节。
func UnaryServerError() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		resp, err := handler(ctx, req)
		if err == nil {
			return resp, nil
		}
		var be *errs.Error
		if errors.As(err, &be) {
			return nil, be.GRPCStatus().Err()
		}
		L.Error("gRPC server unhandled error",
			logger.String("method", info.FullMethod),
			logger.Error(err))
		return nil, status.Error(codes.Internal, "internal error")
	}
}

// UnaryClientError 把 status 转回 *errs.Error，调用方 errors.Is / As 跨服务透明。
func UnaryClientError() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		err := invoker(ctx, method, req, reply, cc, opts...)
		if err == nil {
			return nil
		}
		return errs.FromError(err)
	}
}
