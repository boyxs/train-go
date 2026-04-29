package ioc

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	articlev1 "github.com/webook/api/gen/article/v1"
	interactionv1 "github.com/webook/api/gen/interaction/v1"
	searchv1 "github.com/webook/api/gen/search/v1"
	"github.com/webook/pkg/grpcx"
)

// InitCoreConn 拨号到主仓 webook-core 的 gRPC server（grpc.coreAddr 配置）。
// 复用同一个 ClientConn 给三个 service client，gRPC 内部自带连接池。
func InitCoreConn() (*grpc.ClientConn, func(), error) {
	addr := viper.GetString("grpc.coreAddr")
	if addr == "" {
		addr = "127.0.0.1:8090"
	}
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithUnaryInterceptor(grpcx.UnaryClientErrorInterceptor()),
	)
	if err != nil {
		return nil, nil, err
	}
	// cleanup 跑在进程退出阶段，logger 可能已 sync 关闭，用 stderr 兜底
	cleanup := func() {
		if err := conn.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "[chat] gRPC conn close:", err)
		}
	}
	return conn, cleanup, nil
}

func InitSearchClient(conn *grpc.ClientConn) searchv1.SearchServiceClient {
	return searchv1.NewSearchServiceClient(conn)
}

func InitArticleReaderClient(conn *grpc.ClientConn) articlev1.ArticleReaderServiceClient {
	return articlev1.NewArticleReaderServiceClient(conn)
}

func InitInteractionClient(conn *grpc.ClientConn) interactionv1.InteractionServiceClient {
	return interactionv1.NewInteractionServiceClient(conn)
}
