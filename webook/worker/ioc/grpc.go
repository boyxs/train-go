package ioc

import (
	"github.com/spf13/viper"
	etcdv3 "go.etcd.io/etcd/client/v3"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"

	"github.com/boyxs/train-go/webook/pkg/grpcx"
	"github.com/boyxs/train-go/webook/pkg/grpcx/interceptor/errconv"
	"github.com/boyxs/train-go/webook/pkg/grpcx/interceptor/logging"
	"github.com/boyxs/train-go/webook/pkg/grpcx/interceptor/metrics"
	"github.com/boyxs/train-go/webook/pkg/logger"

	feedv1 "github.com/boyxs/train-go/webook/api/gen/feed/v1"
	interactionv1 "github.com/boyxs/train-go/webook/api/gen/interaction/v1"
	rankingv1 "github.com/boyxs/train-go/webook/api/gen/ranking/v1"
)

// InitGRPCMetrics 进程内唯一的 gRPC 指标 builder，多个下游 conn 共享（分两个会重复注册 panic）。
func InitGRPCMetrics() *metrics.PrometheusBuilder {
	return metrics.NewPrometheusBuilder("webook", "grpc", "requests", "gRPC 请求").
		WithCounter().WithHistogram().WithInFlight()
}

// CoreConn 是到 webook-core 的 gRPC 连接（ranking 重算/归档触发走这里）。
type CoreConn struct{ *grpc.ClientConn }

// InitCoreConn 拨号 webook-core（grpc.client.webook-core，默认 etcd:///service/webook-core）。
func InitCoreConn(client *etcdv3.Client, grpcMetrics *metrics.PrometheusBuilder, l logger.LoggerX, tp trace.TracerProvider) (CoreConn, func(), error) {
	cfg, err := clientConfig("webook-core")
	if err != nil {
		return CoreConn{}, nil, err
	}
	conn, cleanup, err := grpcx.NewClient(client, cfg,
		grpc.WithStatsHandler(otelgrpc.NewClientHandler(otelgrpc.WithTracerProvider(tp))),
		grpc.WithChainUnaryInterceptor(
			grpcMetrics.BuildUnaryClient(),
			logging.NewInterceptorBuilder(l).BuildUnaryClient(),
			errconv.UnaryClientInterceptor(),
		),
	)
	if err != nil {
		return CoreConn{}, nil, err
	}
	return CoreConn{conn}, cleanup, nil
}

// InteractionConn 是到 webook-interaction 的 gRPC 连接（read 计数累加走这里）。
type InteractionConn struct{ *grpc.ClientConn }

// InitInteractionConn 拨号 webook-interaction（grpc.client.webook-interaction，默认 etcd:///service/webook-interaction）。
func InitInteractionConn(client *etcdv3.Client, grpcMetrics *metrics.PrometheusBuilder, l logger.LoggerX, tp trace.TracerProvider) (InteractionConn, func(), error) {
	cfg, err := clientConfig("webook-interaction")
	if err != nil {
		return InteractionConn{}, nil, err
	}
	conn, cleanup, err := grpcx.NewClient(client, cfg,
		grpc.WithStatsHandler(otelgrpc.NewClientHandler(otelgrpc.WithTracerProvider(tp))),
		grpc.WithChainUnaryInterceptor(
			grpcMetrics.BuildUnaryClient(),
			logging.NewInterceptorBuilder(l).BuildUnaryClient(),
			errconv.UnaryClientInterceptor(),
		),
	)
	if err != nil {
		return InteractionConn{}, nil, err
	}
	return InteractionConn{conn}, cleanup, nil
}

// FeedConn 是到 webook-feed 的 gRPC 连接（feed 写扩散/失效重建走这里）。
type FeedConn struct{ *grpc.ClientConn }

// InitFeedConn 拨号 webook-feed（client.grpc.webook-feed，默认 etcd:///service/webook-feed）。
func InitFeedConn(client *etcdv3.Client, grpcMetrics *metrics.PrometheusBuilder, l logger.LoggerX, tp trace.TracerProvider) (FeedConn, func(), error) {
	cfg, err := clientConfig("webook-feed")
	if err != nil {
		return FeedConn{}, nil, err
	}
	conn, cleanup, err := grpcx.NewClient(client, cfg,
		grpc.WithStatsHandler(otelgrpc.NewClientHandler(otelgrpc.WithTracerProvider(tp))),
		grpc.WithChainUnaryInterceptor(
			grpcMetrics.BuildUnaryClient(),
			logging.NewInterceptorBuilder(l).BuildUnaryClient(),
			errconv.UnaryClientInterceptor(),
		),
	)
	if err != nil {
		return FeedConn{}, nil, err
	}
	return FeedConn{conn}, cleanup, nil
}

// InitFeedClient feed 写扩散/移除/失效 → webook-feed。
func InitFeedClient(c FeedConn) feedv1.FeedServiceClient {
	return feedv1.NewFeedServiceClient(c)
}

// clientConfig 读 client.grpc.<name>（target/balancer/…）;target 必填,缺失 → dial 失败,代码不派生。
func clientConfig(name string) (grpcx.ClientConfig, error) {
	var cfg grpcx.ClientConfig
	if err := viper.UnmarshalKey("client.grpc."+name, &cfg); err != nil {
		return grpcx.ClientConfig{}, err
	}
	return cfg, nil
}

// InitRankingJobClient ranking 重算/归档触发 → webook-core 的 RankingJobService。
func InitRankingJobClient(c CoreConn) rankingv1.RankingJobServiceClient {
	return rankingv1.NewRankingJobServiceClient(c)
}

// InitInteractionClient read 计数累加 → webook-interaction。
func InitInteractionClient(c InteractionConn) interactionv1.InteractionServiceClient {
	return interactionv1.NewInteractionServiceClient(c)
}
