package ioc

import (
	"github.com/spf13/viper"
	etcdv3 "go.etcd.io/etcd/client/v3"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	articlev1 "github.com/boyxs/train-go/webook/api/gen/article/v1"
	feedv1 "github.com/boyxs/train-go/webook/api/gen/feed/v1"
	relationv1 "github.com/boyxs/train-go/webook/api/gen/relation/v1"
	feedgrpc "github.com/boyxs/train-go/webook/feed/grpc"

	"github.com/boyxs/train-go/webook/pkg/grpcx"
	"github.com/boyxs/train-go/webook/pkg/grpcx/interceptor/errconv"
	"github.com/boyxs/train-go/webook/pkg/grpcx/interceptor/logging"
	"github.com/boyxs/train-go/webook/pkg/grpcx/interceptor/metrics"
	"github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/shared/confkey"
)

// InitGRPCMetrics 构造进程内唯一的 gRPC 指标 builder，server 拦截器与下游 client 拦截器共享同一实例
// （builder.once 保证 collector 只注册一次，type 标签区分 server/client）；分两个实例会重复注册而 panic。
func InitGRPCMetrics() *metrics.PrometheusBuilder {
	return metrics.NewPrometheusBuilder("webook", "grpc", "requests", "gRPC 请求").
		WithCounter().WithHistogram().WithInFlight()
}

// InitGRPCServer 组装 gRPC server 并注册 FeedService，由 main 起 goroutine 监听。
func InitGRPCServer(feedSrv *feedgrpc.FeedServer, client *etcdv3.Client, grpcMetrics *metrics.PrometheusBuilder, l logger.LoggerX, tp trace.TracerProvider) *grpcx.Server {
	cfg := grpcx.ServerConfig{
		Addr:    viper.GetString(confkey.ServerGRPCAddr),
		Name:    viper.GetString(confkey.ServerGRPCName),
		Host:    viper.GetString(confkey.ServerGRPCHost),
		TTL:     viper.GetDuration(confkey.ServerGRPCTTL),
		Weight:  viper.GetInt(confkey.ServerGRPCWeight),
		Timeout: viper.GetDuration(confkey.ServerGRPCTimeout),
	}
	srv := grpcx.NewServer(cfg, client, l,
		grpc.StatsHandler(otelgrpc.NewServerHandler(otelgrpc.WithTracerProvider(tp))),
		// 入站 a→b→c→handler：metrics → logging → errconv → 校验（最内层）
		grpc.ChainUnaryInterceptor(
			grpcMetrics.BuildUnaryServer(),
			logging.NewInterceptorBuilder(l).BuildUnaryServer(),
			errconv.UnaryServerInterceptor(l),
			feedgrpc.ValidateUnaryInterceptor,
		),
	)
	feedv1.RegisterFeedServiceServer(srv.Server, feedSrv)
	healthpb.RegisterHealthServer(srv.Server, health.NewServer()) // k8s / LB 健康探测
	return srv
}

// ── 下游 gRPC client（feed 回源拉 relation 关系 + core article 轻量投影）─────────

// RelationConn 是到 webook-relation 的 gRPC 连接。独立类型让 wire 区分多下游 conn。
type RelationConn struct{ *grpc.ClientConn }

func InitRelationConn(client *etcdv3.Client, grpcMetrics *metrics.PrometheusBuilder, l logger.LoggerX) (RelationConn, func(), error) {
	conn, cleanup, err := dialDownstream(client, grpcMetrics, l, "webook-relation")
	if err != nil {
		return RelationConn{}, nil, err
	}
	return RelationConn{conn}, cleanup, nil
}

func InitRelationClient(c RelationConn) relationv1.RelationServiceClient {
	return relationv1.NewRelationServiceClient(c)
}

// CoreConn 是到 webook-core 的 gRPC 连接（回源 ArticleReaderService.ListAuthorArticles）。
type CoreConn struct{ *grpc.ClientConn }

func InitCoreConn(client *etcdv3.Client, grpcMetrics *metrics.PrometheusBuilder, l logger.LoggerX) (CoreConn, func(), error) {
	conn, cleanup, err := dialDownstream(client, grpcMetrics, l, "webook-core")
	if err != nil {
		return CoreConn{}, nil, err
	}
	return CoreConn{conn}, cleanup, nil
}

func InitArticleClient(c CoreConn) articlev1.ArticleReaderServiceClient {
	return articlev1.NewArticleReaderServiceClient(c)
}

// dialDownstream 拨号一个下游服务：读 client.grpc.<name>（target 必填缺失即 dial 失败）+ 对称拦截链
// （otel + metrics(client) + logging(client) + errconv，复用进程内唯一 grpcMetrics）。各下游唯一差异是服务名。
// 出站访问日志：调用失败时记 grpc.method（含被调服务，如 webook.relation.v1.RelationService/...）+
// grpc.code（Unavailable=不可达/DeadlineExceeded=超时）+ grpc.cost，用于定位「哪个下游挂了」（正常 RPC 记 Debug 不刷屏）。
func dialDownstream(client *etcdv3.Client, grpcMetrics *metrics.PrometheusBuilder, l logger.LoggerX, name string) (*grpc.ClientConn, func(), error) {
	var cfg grpcx.ClientConfig
	if err := viper.UnmarshalKey("client.grpc."+name, &cfg); err != nil {
		return nil, nil, err
	}
	return grpcx.NewClient(client, cfg,
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithChainUnaryInterceptor(
			grpcMetrics.BuildUnaryClient(),
			logging.NewInterceptorBuilder(l).BuildUnaryClient(),
			errconv.UnaryClientInterceptor(),
		),
	)
}
