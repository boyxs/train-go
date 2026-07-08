//go:build wireinject

package setup

import (
	"github.com/google/wire"

	commentgrpc "github.com/boyxs/train-go/webook/comment/grpc"
	"github.com/boyxs/train-go/webook/comment/ioc"
	"github.com/boyxs/train-go/webook/comment/repository"
	"github.com/boyxs/train-go/webook/comment/repository/cache"
	"github.com/boyxs/train-go/webook/comment/repository/dao"
	"github.com/boyxs/train-go/webook/comment/service"
)

// InitCommentServer 组装真 dao/cache/repository/service 的 CommentServer，
// 由集成测试注册到 bufconn gRPC server，发真实请求打通全链路。
// 基础设施走本包 InitDB/InitRedis/InitLogger（连测试库），与 internal/integration/setup 同构。
func InitCommentServer() *commentgrpc.CommentServer {
	wire.Build(
		InitDB,
		InitRedis,
		InitLogger,
		ioc.InitSensitiveFilter,
		ioc.InitLimiter,
		dao.NewGormCommentDAO,
		cache.NewRedisCommentCache,
		repository.NewCacheCommentRepository,
		service.NewCommentService,
		commentgrpc.NewCommentServer,
	)
	return nil
}
