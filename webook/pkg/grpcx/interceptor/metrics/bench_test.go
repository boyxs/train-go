package metrics

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
)

// BenchmarkServerInterceptor 量 metrics server 拦截器每请求开销
// （splitMethod + peer + inflight/histogram/counter 打点 + start 闭包）。
func BenchmarkServerInterceptor(b *testing.B) {
	interceptor := NewPrometheusBuilder("bench", "grpc", "requests", "x").
		Registry(prometheus.NewRegistry()).
		WithCounter().WithHistogram().WithInFlight().
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
