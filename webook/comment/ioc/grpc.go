package ioc

import (
	"github.com/spf13/viper"
	etcdv3 "go.etcd.io/etcd/client/v3"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	commentv1 "github.com/boyxs/train-go/webook/api/gen/comment/v1"
	commentgrpc "github.com/boyxs/train-go/webook/comment/grpc"
	"github.com/boyxs/train-go/webook/pkg/grpcx"
	"github.com/boyxs/train-go/webook/pkg/grpcx/interceptor/errconv"
	"github.com/boyxs/train-go/webook/pkg/grpcx/interceptor/metrics"
	"github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/shared/confkey"
)

// InitGRPCServer 组装 gRPC server 并注册 CommentService，由 main 起 goroutine 监听。
// 镜像 internal/ioc/grpc.go：otel StatsHandler + metrics/errconv 拦截器显式传入。
// 消费注入的 TracerProvider（comment 无 web 中间件，由此处接住 InitOTel 的输出）。
func InitGRPCServer(commentSrv *commentgrpc.CommentServer, client *etcdv3.Client, l logger.LoggerX, tp trace.TracerProvider) *grpcx.Server {
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
		grpc.ChainUnaryInterceptor(grpcMetrics.BuildUnaryServer(), errconv.UnaryServerInterceptor(l)),
	)
	commentv1.RegisterCommentServiceServer(srv.Server, commentSrv)
	healthpb.RegisterHealthServer(srv.Server, health.NewServer()) // k8s / LB 健康探测
	return srv
}
