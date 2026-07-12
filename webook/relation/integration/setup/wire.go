//go:build wireinject

package setup

import (
	"github.com/google/wire"

	relationgrpc "github.com/boyxs/train-go/webook/relation/grpc"
	"github.com/boyxs/train-go/webook/relation/repository"
	"github.com/boyxs/train-go/webook/relation/repository/cache"
	"github.com/boyxs/train-go/webook/relation/repository/dao"
	"github.com/boyxs/train-go/webook/relation/service"
)

// InitRelationServer 组装真 dao/cache/repository/service 的 RelationServer，供集成测试注册到 bufconn gRPC server，
// 经 RelationServiceClient 发真实请求打通 gRPC → service → repository → dao/cache → MySQL/Redis 全链路。
// 与 interaction/integration/setup 同构（同为 gRPC 服务 + Cache-Aside）。
func InitRelationServer() *relationgrpc.RelationServer {
	wire.Build(
		InitDB,
		InitRedis,
		InitLogger,
		dao.NewGormRelationDAO,
		cache.NewRedisRelationCache,
		repository.NewCacheRelationRepository,
		service.NewInternalRelationService,
		relationgrpc.NewRelationServer,
	)
	return nil
}
