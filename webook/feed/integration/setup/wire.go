//go:build wireinject

package setup

import (
	"github.com/google/wire"

	feedgrpc "github.com/boyxs/train-go/webook/feed/grpc"
	"github.com/boyxs/train-go/webook/feed/repository"
	"github.com/boyxs/train-go/webook/feed/repository/cache"
	"github.com/boyxs/train-go/webook/feed/service"
)

// InitFeedServer 组装真 cache/repository/service 的 FeedServer（真实 Redis + fake relation/article gRPC client），
// 供集成测试注册到 bufconn，经 FeedServiceClient 发真实请求打通 gRPC → service → repository → cache → Redis。
func InitFeedServer() *feedgrpc.FeedServer {
	wire.Build(
		InitRedis,
		InitLogger,
		InitCacheConfig,
		InitServiceConfig,
		NewFakeRelationClient,
		NewFakeArticleClient,
		cache.NewRedisFeedCache,
		repository.NewCacheFeedRepository,
		service.NewInternalFeedService,
		feedgrpc.NewFeedServer,
	)
	return nil
}
