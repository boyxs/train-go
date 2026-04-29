package ioc

import (
	"github.com/spf13/viper"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"

	articlev1 "github.com/webook/api/gen/article/v1"
	interactionv1 "github.com/webook/api/gen/interaction/v1"
	searchv1 "github.com/webook/api/gen/search/v1"
	grpcsrv "github.com/webook/internal/grpc"
	"github.com/webook/pkg/grpcx"
)

// GRPCServerConfig 主仓 gRPC 监听配置。
type GRPCServerConfig struct {
	Addr string `yaml:"addr" mapstructure:"addr"`
}

func InitGRPCConfig() GRPCServerConfig {
	var cfg GRPCServerConfig
	if err := viper.UnmarshalKey("grpc", &cfg); err != nil {
		panic("grpc 配置加载失败: " + err.Error())
	}
	if cfg.Addr == "" {
		cfg.Addr = ":8090"
	}
	return cfg
}

// InitGRPCServer 组装好所有 gRPC service 注册的 *grpc.Server，由 main 起 goroutine 监听。
// 内置 otelgrpc StatsHandler，trace 自动跨服务传播。
func InitGRPCServer(
	searchSrv *grpcsrv.SearchServer,
	articleSrv *grpcsrv.ArticleReaderServer,
	intrSrv *grpcsrv.InteractionServer,
) *grpc.Server {
	s := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.UnaryInterceptor(grpcx.UnaryServerErrorInterceptor()),
	)
	searchv1.RegisterSearchServiceServer(s, searchSrv)
	articlev1.RegisterArticleReaderServiceServer(s, articleSrv)
	interactionv1.RegisterInteractionServiceServer(s, intrSrv)
	return s
}
