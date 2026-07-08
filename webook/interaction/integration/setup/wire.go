//go:build wireinject

package setup

import (
	"github.com/google/wire"

	interactiongrpc "github.com/boyxs/train-go/webook/interaction/grpc"
	"github.com/boyxs/train-go/webook/interaction/repository"
	"github.com/boyxs/train-go/webook/interaction/repository/cache"
	"github.com/boyxs/train-go/webook/interaction/repository/dao"
	"github.com/boyxs/train-go/webook/interaction/service"
)

// InitInteractionServer 组装真 dao/cache/repository/service 的 InteractionServer，
// 由集成测试注册到 bufconn gRPC server，发真实请求打通全链路。
// 基础设施走本包 InitDB/InitRedis/InitLogger（连测试库），与 comment/integration/setup 同构。
func InitInteractionServer() *interactiongrpc.InteractionServer {
	wire.Build(
		InitDB,
		InitRedis,
		InitLogger,
		dao.NewGormInteractionDAO,
		cache.NewRedisInteractionCache,
		repository.NewCacheInteractionRepository,
		service.NewInternalInteractionService,
		interactiongrpc.NewInteractionServer,
	)
	return nil
}
