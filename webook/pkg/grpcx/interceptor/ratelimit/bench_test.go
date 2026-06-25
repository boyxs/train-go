package ratelimit

import (
	"context"
	"testing"

	"google.golang.org/grpc"
)

// BenchmarkAllow_MethodLevel 量方法级路径的拦截器自身开销
// （resolve 命中 methods + key 拼接）。fakeLimiter 放行,排除 Redis。
func BenchmarkAllow_MethodLevel(b *testing.B) {
	lim := &fakeLimiter{limited: false}
	interceptor := NewInterceptorBuilder(nil, "", nil).
		WithMethod("/grpc.health.v1.Health/Check", lim).
		BuildUnaryServer()
	handler := func(context.Context, any) (any, error) { return "ok", nil }
	info := &grpc.UnaryServerInfo{FullMethod: "/grpc.health.v1.Health/Check"}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = interceptor(ctx, nil, info, handler)
	}
}
