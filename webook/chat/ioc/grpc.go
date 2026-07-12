package ioc

import (
	"github.com/spf13/viper"
	etcdv3 "go.etcd.io/etcd/client/v3"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"

	"github.com/boyxs/train-go/webook/pkg/grpcx"
	"github.com/boyxs/train-go/webook/pkg/grpcx/interceptor/errconv"
	"github.com/boyxs/train-go/webook/pkg/grpcx/interceptor/metrics"

	articlev1 "github.com/boyxs/train-go/webook/api/gen/article/v1"
	interactionv1 "github.com/boyxs/train-go/webook/api/gen/interaction/v1"
	searchv1 "github.com/boyxs/train-go/webook/api/gen/search/v1"
)

// InitGRPCMetrics 进程内唯一的 gRPC 指标 builder，多个下游 conn 共享。
// builder.once 保证 collector 只注册一次；分两个实例会因 webook_grpc_requests_* 重复注册而 panic。
func InitGRPCMetrics() *metrics.PrometheusBuilder {
	return metrics.NewPrometheusBuilder("webook", "grpc", "requests", "gRPC 请求").
		WithCounter().WithHistogram().WithInFlight()
}

// CoreConn 是到 webook-core 的 gRPC 连接。独立类型(而非裸 *grpc.ClientConn)让 wire 能区分多个下游 conn。
type CoreConn struct{ *grpc.ClientConn }

// InitCoreConn 拨号 webook-core(grpc.client.webook-core,默认 etcd:///service/webook-core)。
// article-reader 工具走这里；search 已拆独立服务见 InitSearchConn，interaction 见 InitInteractionConn。
func InitCoreConn(client *etcdv3.Client, grpcMetrics *metrics.PrometheusBuilder) (CoreConn, func(), error) {
	cfg, err := clientConfig("webook-core")
	if err != nil {
		return CoreConn{}, nil, err
	}
	conn, cleanup, err := grpcx.NewClient(client, cfg,
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithChainUnaryInterceptor(grpcMetrics.BuildUnaryClient(), errconv.UnaryClientInterceptor()),
	)
	if err != nil {
		return CoreConn{}, nil, err
	}
	return CoreConn{conn}, cleanup, nil
}

// InteractionConn 是到 webook-interaction 的 gRPC 连接（互动已拆独立服务，不再经 core 中转）。
type InteractionConn struct{ *grpc.ClientConn }

// InitInteractionConn 拨号 webook-interaction(grpc.client.webook-interaction,默认 etcd:///service/webook-interaction)。
func InitInteractionConn(client *etcdv3.Client, grpcMetrics *metrics.PrometheusBuilder) (InteractionConn, func(), error) {
	cfg, err := clientConfig("webook-interaction")
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

// clientConfig 读 client.grpc.<name>(target/balancer/…);target 必填,缺失 → dial 失败,代码不派生。
func clientConfig(name string) (grpcx.ClientConfig, error) {
	var cfg grpcx.ClientConfig
	if err := viper.UnmarshalKey("client.grpc."+name, &cfg); err != nil {
		return grpcx.ClientConfig{}, err
	}
	return cfg, nil
}

// SearchConn 是到 webook-search 的 gRPC 连接（search 已从 core 抽出为独立服务，chat 直连不再经 core）。
type SearchConn struct{ *grpc.ClientConn }

// InitSearchConn 拨号 webook-search(grpc.client.webook-search,默认 etcd:///service/webook-search)。拦截链与 CoreConn 对称。
func InitSearchConn(client *etcdv3.Client, grpcMetrics *metrics.PrometheusBuilder) (SearchConn, func(), error) {
	cfg, err := clientConfig("webook-search")
	if err != nil {
		return SearchConn{}, nil, err
	}
	conn, cleanup, err := grpcx.NewClient(client, cfg,
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithChainUnaryInterceptor(grpcMetrics.BuildUnaryClient(), errconv.UnaryClientInterceptor()),
	)
	if err != nil {
		return SearchConn{}, nil, err
	}
	return SearchConn{conn}, cleanup, nil
}

func InitSearchClient(c SearchConn) searchv1.SearchServiceClient {
	return searchv1.NewSearchServiceClient(c)
}

func InitArticleReaderClient(c CoreConn) articlev1.ArticleReaderServiceClient {
	return articlev1.NewArticleReaderServiceClient(c)
}

func InitInteractionClient(c InteractionConn) interactionv1.InteractionServiceClient {
	return interactionv1.NewInteractionServiceClient(c)
}
