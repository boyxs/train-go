package ioc

import (
	"github.com/spf13/viper"
	etcdv3 "go.etcd.io/etcd/client/v3"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	interactionv1 "github.com/boyxs/train-go/webook/api/gen/interaction/v1"
	interactiongrpc "github.com/boyxs/train-go/webook/interaction/grpc"
	"github.com/boyxs/train-go/webook/pkg/grpcx"
	"github.com/boyxs/train-go/webook/pkg/grpcx/interceptor/errconv"
	"github.com/boyxs/train-go/webook/pkg/grpcx/interceptor/logging"
	"github.com/boyxs/train-go/webook/pkg/grpcx/interceptor/metrics"
	"github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/shared/confkey"
)

// InitGRPCServer 组装 gRPC server 并注册 InteractionService，由 main 起 goroutine 监听。
// otel StatsHandler + metrics/errconv 拦截器显式传入；消费注入的 TracerProvider。
func InitGRPCServer(interactionSrv *interactiongrpc.InteractionServer, client *etcdv3.Client, l logger.LoggerX, tp trace.TracerProvider) *grpcx.Server {
	cfg := grpcx.ServerConfig{
		Addr:    viper.GetString(confkey.ServerGRPCAddr),
		Name:    viper.GetString(confkey.ServerGRPCName),
		Host:    viper.GetString(confkey.ServerGRPCHost),
		TTL:     viper.GetDuration(confkey.ServerGRPCTTL),
		Weight:  viper.GetInt(confkey.ServerGRPCWeight),
		Timeout: viper.GetDuration(confkey.ServerGRPCTimeout),
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
			logging.NewInterceptorBuilder(l).BuildUnaryServer(),
			errconv.UnaryServerInterceptor(l),
			interactiongrpc.ValidateUnaryInterceptor,
		),
	)
	interactionv1.RegisterInteractionServiceServer(srv.Server, interactionSrv)
	healthpb.RegisterHealthServer(srv.Server, health.NewServer()) // k8s / LB 健康探测
	return srv
}
