package registry

import (
	"context"
	"os"
	"testing"
	"time"

	userv1 "webook/sandbox/grpc/gen/user/v1"
	"webook/sandbox/grpc/server"

	"github.com/stretchr/testify/suite"
	"github.com/zeromicro/go-zero/core/discov"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
)

// GoZeroSuite 演示 go-zero zrpc 的服务注册/发现。
// TestServer 的 Start 会阻塞,不能和 TestClient 同一次 go test 跑;分两个终端、设 ETCD_MANUAL=1:
//
//	$env:ETCD_MANUAL="1"; go test ./registry/ -run 'TestGoZero/TestServer' -v   # 终端1
//	$env:ETCD_MANUAL="1"; go test ./registry/ -run 'TestGoZero/TestClient' -v   # 终端2
type GoZeroSuite struct {
	suite.Suite
}

func TestGoZero(t *testing.T) {
	suite.Run(t, new(GoZeroSuite))
}

func (s *GoZeroSuite) TestServer() {
	if os.Getenv("ETCD_MANUAL") == "" {
		s.T().Skip("手动脚本:Start 会阻塞需手动停;设 ETCD_MANUAL=1 且本地有 etcd 时运行")
	}
	scfg := zrpc.RpcServerConf{
		ListenOn: ":8090",
		Etcd: discov.EtcdConf{
			Hosts: []string{"127.0.0.1:2379"},
			Key:   "user.rpc", // 服务发现 key
		},
	}
	srv := zrpc.MustNewServer(scfg, func(grpcServer *grpc.Server) {
		userv1.RegisterUserServiceServer(grpcServer, server.NewMemoryUserServer())
	})

	// 自定义中间件
	srv.AddUnaryInterceptors(exampleUnaryInterceptor)
	srv.AddStreamInterceptors(exampleStreamInterceptor)

	s.T().Logf("go-zero rpc server listening on %s", scfg.ListenOn)
	srv.Start()
}

// exampleUnaryInterceptor / exampleStreamInterceptor:最简单的放行拦截器示例。
func exampleUnaryInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	return handler(ctx, req)
}

func exampleStreamInterceptor(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	return handler(srv, ss)
}

func (s *GoZeroSuite) TestClient() {
	if os.Getenv("ETCD_MANUAL") == "" {
		s.T().Skip("手动脚本:需 TestServer 已注册端点;设 ETCD_MANUAL=1 运行")
	}
	ccfg := zrpc.RpcClientConf{
		Etcd: discov.EtcdConf{
			Hosts: []string{"127.0.0.1:2379"},
			Key:   "user.rpc", // 服务发现 key
		},
		Timeout: 1000,
		// 必须显式开拦截器开关,Timeout 才生效:手写 config 不经 conf.Load,`default=true` 不会被填(零值=全关)
		Middlewares: zrpc.ClientMiddlewaresConf{Timeout: true},
	}
	conn := zrpc.MustNewClient(ccfg)
	client := userv1.NewUserServiceClient(conn.Conn())

	// 兜底超时策略
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	user, err := client.GetUser(ctx, &userv1.GetUserRequest{Id: 1})
	s.Require().NoError(err)
	s.T().Logf("got user: %v", user)
}
