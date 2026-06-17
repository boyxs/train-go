package registry

import (
	"context"
	"os"
	"testing"
	"time"

	userv1 "webook/sandbox/grpc/gen/user/v1"
	"webook/sandbox/grpc/server"

	"github.com/go-kratos/kratos/contrib/registry/etcd/v2"
	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	etcdv3 "go.etcd.io/etcd/client/v3"
)

// KratosSuite 演示 kratos 的服务注册/发现(etcd registrar + discovery resolver)。
// 注:kratos etcd 注册默认带 /microservices/ 前缀,与 go-zero discov 的 key 编码不互通。
// TestServer 的 Run 会阻塞,分两个终端、设 ETCD_MANUAL=1:
//
//	$env:ETCD_MANUAL="1"; go test ./registry/ -run 'TestKratos/TestServer' -v   # 终端1
//	$env:ETCD_MANUAL="1"; go test ./registry/ -run 'TestKratos/TestClient' -v   # 终端2
type KratosSuite struct {
	suite.Suite
	cli *etcdv3.Client
}

func TestKratos(t *testing.T) {
	suite.Run(t, new(KratosSuite))
}

func (s *KratosSuite) SetupSuite() {
	cli, err := etcdv3.New(etcdv3.Config{
		Endpoints: []string{"127.0.0.1:2379"},
	})
	require.NoError(s.T(), err)
	s.cli = cli
}

func (s *KratosSuite) TestServer() {
	if os.Getenv("ETCD_MANUAL") == "" {
		s.T().Skip("手动脚本:Run 会阻塞;设 ETCD_MANUAL=1 且本地有 etcd 时运行")
	}
	grpcSrv := grpc.NewServer(
		grpc.Address(":8090"),
		grpc.Middleware(
			recovery.Recovery(),
		),
	)
	userv1.RegisterUserServiceServer(grpcSrv, server.NewMemoryUserServer())

	r := etcd.New(s.cli)
	app := kratos.New(
		kratos.Name("user.rpc"),
		kratos.Server(grpcSrv),
		kratos.Registrar(r),
	)
	s.T().Log("kratos rpc server listening on :8090")
	require.NoError(s.T(), app.Run())
}

func (s *KratosSuite) TestClient() {
	if os.Getenv("ETCD_MANUAL") == "" {
		s.T().Skip("手动脚本:需 TestServer 已注册端点;设 ETCD_MANUAL=1 运行")
	}

	r := etcd.New(s.cli)
	conn, err := grpc.DialInsecure(
		context.Background(),
		grpc.WithEndpoint("discovery:///user.rpc"),
		grpc.WithDiscovery(r),
	)
	require.NoError(s.T(), err)
	defer conn.Close()

	client := userv1.NewUserServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	user, err := client.GetUser(ctx, &userv1.GetUserRequest{Id: 1})
	require.NoError(s.T(), err)
	s.T().Logf("got user: %v", user)
}
