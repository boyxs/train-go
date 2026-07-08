package logging

import (
	"context"
	"testing"

	"google.golang.org/grpc"

	"github.com/boyxs/train-go/webook/pkg/logger"
)

// BenchmarkServerInterceptor 量 logging server 拦截器每请求开销
// （fields 切片构建 + appendStatus + defer）。NopLogger 排除日志 I/O。
func BenchmarkServerInterceptor(b *testing.B) {
	interceptor := NewInterceptorBuilder(logger.NewNopLogger()).BuildUnaryServer()
	handler := func(context.Context, any) (any, error) { return "ok", nil }
	info := &grpc.UnaryServerInfo{FullMethod: "/grpc.health.v1.Health/Check"}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = interceptor(ctx, nil, info, handler)
	}
}
