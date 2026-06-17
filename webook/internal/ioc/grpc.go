package ioc

import (
	"github.com/spf13/viper"
	etcdv3 "go.etcd.io/etcd/client/v3"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/webook/pkg/logger"

	articlev1 "github.com/webook/api/gen/article/v1"
	interactionv1 "github.com/webook/api/gen/interaction/v1"
	searchv1 "github.com/webook/api/gen/search/v1"
	grpcsrv "github.com/webook/internal/grpc"
	"github.com/webook/pkg/grpcx"
	"github.com/webook/pkg/grpcx/interceptor"
)

// InitGRPCServer 组装 server 并注册所有 gRPC service，由 main 起 goroutine 监听。
// otel StatsHandler 与错误拦截器在此显式传入（grpcx 不内置默认 option）。
func InitGRPCServer(
	searchSrv *grpcsrv.SearchServer,
	articleSrv *grpcsrv.ArticleReaderServer,
	intrSrv *grpcsrv.InteractionServer,
	client *etcdv3.Client,
	l logger.LoggerX,
) *grpcx.Server {
	var cfg grpcx.ServerConfig
	if err := viper.UnmarshalKey("grpc.server", &cfg); err != nil {
		panic(err)
	}
	srv := grpcx.NewServer(cfg, client, l,
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.UnaryInterceptor(interceptor.UnaryServerError()),
	)
	searchv1.RegisterSearchServiceServer(srv.Server, searchSrv)
	articlev1.RegisterArticleReaderServiceServer(srv.Server, articleSrv)
	interactionv1.RegisterInteractionServiceServer(srv.Server, intrSrv)
	healthpb.RegisterHealthServer(srv.Server, health.NewServer()) // k8s / LB 健康探测
	return srv
}
