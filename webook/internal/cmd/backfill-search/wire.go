//go:build wireinject

package main

import (
	"github.com/google/wire"

	"github.com/boyxs/train-go/webook/internal/ioc"
	"github.com/boyxs/train-go/webook/internal/repository/dao"
)

// InitSearchBackfiller 复用 core 的 infra + 下游 gRPC client provider 装配回填工具。
// 仅取回填所需子集：DB（源库）+ etcd 发现 + search/tag client，不拉起 web/gRPC server。
func InitSearchBackfiller() (*SearchBackfiller, func(), error) {
	wire.Build(
		// infra
		ioc.InitTimezone,
		ioc.InitLogger,
		ioc.InitDB,
		dao.NewGormArticleReaderDAO,
		// 下游 gRPC client（etcd 发现，与 core BFF 同款拨号）
		ioc.InitEtcdClient,
		ioc.InitGRPCMetrics,
		ioc.InitSearchConn,
		ioc.InitSearchClient,
		ioc.InitTagConn,
		ioc.InitTagClient,
		NewSearchBackfiller,
	)
	return nil, nil, nil
}
