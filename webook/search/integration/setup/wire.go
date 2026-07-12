//go:build wireinject

package setup

import (
	"github.com/google/wire"

	searchgrpc "github.com/boyxs/train-go/webook/search/grpc"
	"github.com/boyxs/train-go/webook/search/repository"
	"github.com/boyxs/train-go/webook/search/service"
)

// InitSearchServer 组装真 ES dao/repository/service（+ stub embedder）的 SearchServer，
// 供集成测试注册到 bufconn gRPC server，经 SearchServiceClient 发真实请求打通全链路。
// embedding 走 stub（InitStubEmbedder，文本相关确定向量），不调外部 API。
func InitSearchServer() *searchgrpc.SearchServer {
	wire.Build(
		InitLogger,
		InitESClient,
		InitArticleDAO,
		repository.NewESArticleRepository,
		InitStubEmbedder,
		service.NewInternalArticleService,
		searchgrpc.NewSearchServer,
	)
	return nil
}
