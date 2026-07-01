package ioc

import (
	"github.com/spf13/viper"
	etcdv3 "go.etcd.io/etcd/client/v3"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	interactionv1 "github.com/webook/api/gen/interaction/v1"
	interactiongrpc "github.com/webook/interaction/grpc"
	"github.com/webook/pkg/grpcx"
	"github.com/webook/pkg/grpcx/interceptor/errconv"
	"github.com/webook/pkg/grpcx/interceptor/metrics"
	"github.com/webook/pkg/logger"
)

// InitGRPCServer 组装 gRPC server 并注册 InteractionService，由 main 起 goroutine 监听。
// otel StatsHandler + metrics/errconv 拦截器显式传入；消费注入的 TracerProvider。
func InitGRPCServer(interactionSrv *interactiongrpc.InteractionServer, client *etcdv3.Client, l logger.LoggerX, tp trace.TracerProvider) *grpcx.Server {
	cfg := grpcx.ServerConfig{
		Port:   viper.GetInt("grpc.server.port"),
		Name:   viper.GetString("grpc.server.name"),
		Host:   viper.GetString("grpc.server.host"),
		TTL:    viper.GetInt64("grpc.server.ttl"),
		Weight: viper.GetInt("grpc.server.weight"),
	}
	grpcMetrics := metrics.NewPrometheusBuilder("webook", "grpc", "requests", "gRPC 请求").
		WithCounter().WithHistogram().WithInFlight()
	srv := grpcx.NewServer(cfg, client, l,
		grpc.StatsHandler(otelgrpc.NewServerHandler(otelgrpc.WithTracerProvider(tp))),
		// 校验拦截器挂在最内层
		// ChainUnaryInterceptor(a, b, c) 的调用顺序是 a → b → c → handler:
		//  - 入站: a → b → c → handler
		//  - 出站: handler → c → b → a
		grpc.ChainUnaryInterceptor(
			grpcMetrics.BuildUnaryServer(),
			errconv.UnaryServerInterceptor(l),
			interactiongrpc.ValidateUnaryInterceptor,
		),
	)
	interactionv1.RegisterInteractionServiceServer(srv.Server, interactionSrv)
	healthpb.RegisterHealthServer(srv.Server, health.NewServer()) // k8s / LB 健康探测
	return srv
}
