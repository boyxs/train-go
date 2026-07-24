package ioc

import (
	"github.com/spf13/viper"
	etcdv3 "go.etcd.io/etcd/client/v3"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/boyxs/train-go/webook/pkg/logger"

	articlev1 "github.com/boyxs/train-go/webook/api/gen/article/v1"
	commentv1 "github.com/boyxs/train-go/webook/api/gen/comment/v1"
	feedv1 "github.com/boyxs/train-go/webook/api/gen/feed/v1"
	interactionv1 "github.com/boyxs/train-go/webook/api/gen/interaction/v1"
	rankingv1 "github.com/boyxs/train-go/webook/api/gen/ranking/v1"
	relationv1 "github.com/boyxs/train-go/webook/api/gen/relation/v1"
	searchv1 "github.com/boyxs/train-go/webook/api/gen/search/v1"
	tagv1 "github.com/boyxs/train-go/webook/api/gen/tag/v1"
	grpcsrv "github.com/boyxs/train-go/webook/internal/grpc"
	"github.com/boyxs/train-go/webook/pkg/grpcx"
	"github.com/boyxs/train-go/webook/pkg/grpcx/interceptor/errconv"
	"github.com/boyxs/train-go/webook/pkg/grpcx/interceptor/logging"
	"github.com/boyxs/train-go/webook/pkg/grpcx/interceptor/metrics"
	"github.com/boyxs/train-go/webook/shared/confkey"
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
		grpc.ChainUnaryInterceptor(grpcMetrics.BuildUnaryServer(), logging.NewInterceptorBuilder(l).BuildUnaryServer(), errconv.UnaryServerInterceptor(l)),
	)
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
func InitCommentConn(client *etcdv3.Client, grpcMetrics *metrics.PrometheusBuilder, l logger.LoggerX) (CommentConn, func(), error) {
	conn, cleanup, err := dialDownstream(client, grpcMetrics, l, "webook-comment")
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
func InitInteractionConn(client *etcdv3.Client, grpcMetrics *metrics.PrometheusBuilder, l logger.LoggerX) (InteractionConn, func(), error) {
	conn, cleanup, err := dialDownstream(client, grpcMetrics, l, "webook-interaction")
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

// SearchConn 是到 webook-search 的 gRPC 连接（search 已从 core 抽出为独立服务）。独立类型让 wire 区分多下游 conn。
type SearchConn struct{ *grpc.ClientConn }

// InitSearchConn 拨号 webook-search(grpc.client.webook-search,默认 etcd:///service/webook-search)。拦截链与其他 client 对称。
func InitSearchConn(client *etcdv3.Client, grpcMetrics *metrics.PrometheusBuilder, l logger.LoggerX) (SearchConn, func(), error) {
	conn, cleanup, err := dialDownstream(client, grpcMetrics, l, "webook-search")
	if err != nil {
		return SearchConn{}, nil, err
	}
	return SearchConn{conn}, cleanup, nil
}

func InitSearchClient(c SearchConn) searchv1.SearchServiceClient {
	return searchv1.NewSearchServiceClient(c)
}

// TagConn 是到 webook-tag 的 gRPC 连接。独立类型让 wire 区分多下游 conn。
type TagConn struct{ *grpc.ClientConn }

// InitTagConn 拨号 webook-tag(grpc.client.webook-tag,默认 etcd:///service/webook-tag)。拦截链与其他 client 对称。
func InitTagConn(client *etcdv3.Client, grpcMetrics *metrics.PrometheusBuilder, l logger.LoggerX) (TagConn, func(), error) {
	conn, cleanup, err := dialDownstream(client, grpcMetrics, l, "webook-tag")
	if err != nil {
		return TagConn{}, nil, err
	}
	return TagConn{conn}, cleanup, nil
}

func InitTagClient(c TagConn) tagv1.TagServiceClient {
	return tagv1.NewTagServiceClient(c)
}

// FeedConn 是到 webook-feed 的 gRPC 连接（core 作 HTTP 网关调 feed 读关注流）。独立类型让 wire 区分多下游 conn。
type FeedConn struct{ *grpc.ClientConn }

// InitFeedConn 拨号 webook-feed(client.grpc.webook-feed,默认 etcd:///service/webook-feed)。拦截链与其他 client 对称。
func InitFeedConn(client *etcdv3.Client, grpcMetrics *metrics.PrometheusBuilder, l logger.LoggerX) (FeedConn, func(), error) {
	conn, cleanup, err := dialDownstream(client, grpcMetrics, l, "webook-feed")
	if err != nil {
		return FeedConn{}, nil, err
	}
	return FeedConn{conn}, cleanup, nil
}

func InitFeedClient(c FeedConn) feedv1.FeedServiceClient {
	return feedv1.NewFeedServiceClient(c)
}

// dialDownstream 拨号一个下游服务：读 client.grpc.<name>（target/balancer，target 必填缺失即 dial 失败）
// + 装对称拦截链（otel + metrics(client) + logging(client) + errconv，复用进程内唯一 grpcMetrics）。
// 出站访问日志：调用失败记 grpc.method（含被调服务）+ grpc.code + grpc.cost，定位「哪个下游挂了」（正常 RPC 记 Debug 不刷屏）。
// 各下游唯一差异是服务名，各 InitXxxConn 只做薄类型包装让 wire 区分多 conn。
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
