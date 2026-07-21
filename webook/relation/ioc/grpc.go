package ioc

import (
	"github.com/spf13/viper"
	etcdv3 "go.etcd.io/etcd/client/v3"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	relationv1 "github.com/boyxs/train-go/webook/api/gen/relation/v1"
	relationgrpc "github.com/boyxs/train-go/webook/relation/grpc"

	"github.com/boyxs/train-go/webook/pkg/grpcx"
	"github.com/boyxs/train-go/webook/pkg/grpcx/interceptor/errconv"
	"github.com/boyxs/train-go/webook/pkg/grpcx/interceptor/logging"
	"github.com/boyxs/train-go/webook/pkg/grpcx/interceptor/metrics"
	"github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/shared/confkey"
)

// InitGRPCServer 组装 gRPC server 并注册 RelationService，由 main 起 goroutine 监听。
func InitGRPCServer(relationSrv *relationgrpc.RelationServer, client *etcdv3.Client, l logger.LoggerX, tp trace.TracerProvider) *grpcx.Server {
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
		// ChainUnaryInterceptor(a,b,c) 入站 a→b→c→handler：metrics → errconv → 校验（最内层）
		grpc.ChainUnaryInterceptor(
			grpcMetrics.BuildUnaryServer(),
			logging.NewInterceptorBuilder(l).BuildUnaryServer(),
			errconv.UnaryServerInterceptor(l),
			relationgrpc.ValidateUnaryInterceptor,
		),
	)
	relationv1.RegisterRelationServiceServer(srv.Server, relationSrv)
	healthpb.RegisterHealthServer(srv.Server, health.NewServer()) // k8s / LB 健康探测
	return srv
}
