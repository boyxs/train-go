//go:build wireinject

package setup

import (
	"github.com/google/wire"

	taggrpc "github.com/boyxs/train-go/webook/tag/grpc"
	"github.com/boyxs/train-go/webook/tag/repository"
	"github.com/boyxs/train-go/webook/tag/repository/cache"
	"github.com/boyxs/train-go/webook/tag/repository/dao"
	"github.com/boyxs/train-go/webook/tag/service"
)

// InitTagServer 组装真 dao/repository/service 的 TagServer，供集成测试注册到 bufconn gRPC server，
// 经 TagServiceClient 发真实请求打通 gRPC → service → repository → dao → MySQL 全链路。
// 基础设施走本包 InitDB（连测试库 webook_test），与 interaction/comment integration/setup 同构。
func InitTagServer() *taggrpc.TagServer {
	wire.Build(
		InitDB,
		InitRedis,
		InitLogger,
		dao.NewGormTagDAO,
		dao.NewGormTaggingDAO,
		dao.NewGormTagFollowDAO,
		cache.NewRedisTagCache,
		repository.NewInternalTagRepository,
		service.NewInternalTagService,
		taggrpc.NewTagServer,
	)
	return nil
}
