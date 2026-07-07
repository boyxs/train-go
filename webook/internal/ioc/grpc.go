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
	commentv1 "github.com/webook/api/gen/comment/v1"
	interactionv1 "github.com/webook/api/gen/interaction/v1"
	rankingv1 "github.com/webook/api/gen/ranking/v1"
	relationv1 "github.com/webook/api/gen/relation/v1"
	searchv1 "github.com/webook/api/gen/search/v1"
	grpcsrv "github.com/webook/internal/grpc"
	"github.com/webook/pkg/grpcx"
	"github.com/webook/pkg/grpcx/interceptor/errconv"
	"github.com/webook/pkg/grpcx/interceptor/metrics"
	"github.com/webook/shared/confkey"
)

// InitGRPCMetrics 构造进程内唯一的 gRPC 指标 builder。
// server 拦截器与下游 client 拦截器共享同一实例（builder.once 保证 collector 只注册一次，
// type 标签区分 server/client）；分两个实例会因 webook_grpc_requests_* 重复注册而 panic。
func InitGRPCMetrics() *metrics.PrometheusBuilder {
	return metrics.NewPrometheusBuilder("webook", "grpc", "requests", "gRPC 请求").
		WithCounter().WithHistogram().WithInFlight()
}

// InitGRPCServer 组装 server 并注册所有 gRPC service，由 main 起 goroutine 监听。
// otel StatsHandler 与错误拦截器在此显式传入（grpcx 不内置默认 option）。
func InitGRPCServer(
	searchSrv *grpcsrv.SearchServer,
	articleSrv *grpcsrv.ArticleReaderServer,
	rankingJobSrv *grpcsrv.RankingJobServer,
	client *etcdv3.Client,
	grpcMetrics *metrics.PrometheusBuilder,
	l logger.LoggerX,
) *grpcx.Server {
	cfg := grpcx.ServerConfig{
		Addr:    viper.GetString(confkey.ServerGRPCAddr),
		Name:    viper.GetString(confkey.ServerGRPCName),
		Host:    viper.GetString(confkey.ServerGRPCHost),
		TTL:     viper.GetDuration(confkey.ServerGRPCTTL),
		Weight:  viper.GetInt(confkey.ServerGRPCWeight),
		Timeout: viper.GetDuration(confkey.ServerGRPCTimeout),
	}
	srv := grpcx.NewServer(cfg, client, l,
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		// metrics 在外层：观测 errconv 转换后的最终 status code
		grpc.ChainUnaryInterceptor(grpcMetrics.BuildUnaryServer(), errconv.UnaryServerInterceptor(l)),
	)
	searchv1.RegisterSearchServiceServer(srv.Server, searchSrv)
	articlev1.RegisterArticleReaderServiceServer(srv.Server, articleSrv)
	rankingv1.RegisterRankingJobServiceServer(srv.Server, rankingJobSrv) // webook-worker 调度器触发重算
	healthpb.RegisterHealthServer(srv.Server, health.NewServer())        // k8s / LB 健康探测
	return srv
}

// ── 下游 gRPC client（core 作 HTTP 网关调 comment 后端）─────────────────────

// CommentConn 是到 webook-comment 的 gRPC 连接。独立类型(而非裸 *grpc.ClientConn)让 wire 能区分多个下游 conn。
type CommentConn struct{ *grpc.ClientConn }

// InitCommentConn 拨号 webook-comment(grpc.client.webook-comment,默认 etcd:///service/webook-comment)。
// 复用进程内唯一的 grpcMetrics(与 server 共享)，拦截链与 chat→core 对称：otel + metrics(client) + errconv。
func InitCommentConn(client *etcdv3.Client, grpcMetrics *metrics.PrometheusBuilder) (CommentConn, func(), error) {
	cfg, err := grpcClientConfig("webook-comment")
	if err != nil {
		return CommentConn{}, nil, err
	}
	conn, cleanup, err := grpcx.NewClient(client, cfg,
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithChainUnaryInterceptor(grpcMetrics.BuildUnaryClient(), errconv.UnaryClientInterceptor()),
	)
	if err != nil {
		return CommentConn{}, nil, err
	}
	return CommentConn{conn}, cleanup, nil
}

func InitCommentClient(c CommentConn) commentv1.CommentServiceClient {
	return commentv1.NewCommentServiceClient(c)
}

// InteractionConn 是到 webook-interaction 的 gRPC 连接。独立类型让 wire 能区分多个下游 conn。
type InteractionConn struct{ *grpc.ClientConn }

// InitInteractionConn 拨号 webook-interaction(grpc.client.webook-interaction,默认 etcd:///service/webook-interaction)。
// 复用进程内唯一的 grpcMetrics(与 server / comment client 共享)，拦截链与 comment client 对称。
func InitInteractionConn(client *etcdv3.Client, grpcMetrics *metrics.PrometheusBuilder) (InteractionConn, func(), error) {
	cfg, err := grpcClientConfig("webook-interaction")
	if err != nil {
		return InteractionConn{}, nil, err
	}
	conn, cleanup, err := grpcx.NewClient(client, cfg,
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithChainUnaryInterceptor(grpcMetrics.BuildUnaryClient(), errconv.UnaryClientInterceptor()),
	)
	if err != nil {
		return InteractionConn{}, nil, err
	}
	return InteractionConn{conn}, cleanup, nil
}

func InitInteractionClient(c InteractionConn) interactionv1.InteractionServiceClient {
	return interactionv1.NewInteractionServiceClient(c)
}

// RelationConn 是到 webook-relation 的 gRPC 连接。独立类型让 wire 能区分多个下游 conn。
type RelationConn struct{ *grpc.ClientConn }

// InitRelationConn 拨号 webook-relation。复用进程内唯一 grpcMetrics（与 server/其他 client 共享），拦截链对称。
func InitRelationConn(client *etcdv3.Client, grpcMetrics *metrics.PrometheusBuilder) (RelationConn, func(), error) {
	cfg, err := grpcClientConfig("webook-relation")
	if err != nil {
		return RelationConn{}, nil, err
	}
	conn, cleanup, err := grpcx.NewClient(client, cfg,
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithChainUnaryInterceptor(grpcMetrics.BuildUnaryClient(), errconv.UnaryClientInterceptor()),
	)
	if err != nil {
		return RelationConn{}, nil, err
	}
	return RelationConn{conn}, cleanup, nil
}

func InitRelationClient(c RelationConn) relationv1.RelationServiceClient {
	return relationv1.NewRelationServiceClient(c)
}

// grpcClientConfig 读 client.grpc.<name>(target/balancer/…);target 必填,缺失 → dial 失败,代码不派生。
func grpcClientConfig(name string) (grpcx.ClientConfig, error) {
	var cfg grpcx.ClientConfig
	if err := viper.UnmarshalKey("client.grpc."+name, &cfg); err != nil {
		return grpcx.ClientConfig{}, err
	}
	return cfg, nil
}
