package registry

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	_ "webook/sandbox/grpc/balancer/balancer/swrr" // 注册 custom_swrr 均衡器
	userv1 "webook/sandbox/grpc/gen/user/v1"
	"webook/sandbox/grpc/server"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/zeromicro/go-zero/core/discov"
	"github.com/zeromicro/go-zero/zrpc"
	etcdv3 "go.etcd.io/etcd/client/v3"
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
		userv1.RegisterUserServiceServer(grpcServer, server.NewMemoryUserServer("x"))
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
	// 选中自定义 custom_swrr 均衡器(覆盖 go-zero 默认 p2c)。
	// 注意:go-zero 的 etcd resolver 不写我们的 weight 属性,故 custom_swrr 此处退化为轮询。
	conn := zrpc.MustNewClient(ccfg,
		zrpc.WithDialOption(grpc.WithDefaultServiceConfig(`{"loadBalancingConfig":[{"custom_swrr":{}}]}`)),
	)
	client := userv1.NewUserServiceClient(conn.Conn())

	// 兜底超时策略
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	user, err := client.GetUser(ctx, &userv1.GetUserRequest{Id: 1})
	s.Require().NoError(err)
	s.T().Logf("got user: %v", user)
}

// TestGoZeroSWRRDistribution 自包含验证:go-zero discov 发布 3 个实例 + zrpc discovery +
// custom_swrr,连发 1000 次统计命中。go-zero resolver 不写我们的 weight 属性,故应 ≈ 均分(非加权)。
// etcd 不可达时自动 Skip。
func TestGoZeroSWRRDistribution(t *testing.T) {
	hosts := []string{"127.0.0.1:2379"}
	cli, err := etcdv3.New(etcdv3.Config{Endpoints: hosts, DialTimeout: 2 * time.Second})
	if err != nil {
		t.Skipf("etcd 客户端创建失败,跳过:%v", err)
	}
	probeCtx, probeCancel := context.WithTimeout(context.Background(), 2*time.Second)
	_, err = cli.Status(probeCtx, hosts[0])
	probeCancel()
	_ = cli.Close()
	if err != nil {
		t.Skipf("etcd 不可达,跳过:%v", err)
	}

	const key = "user-gozero-swrr-test"
	flags := []string{"A", "B", "C"}
	for _, flag := range flags {
		lis, lerr := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, lerr)
		srv := grpc.NewServer()
		userv1.RegisterUserServiceServer(srv, server.NewMemoryUserServer(flag))
		go func() { _ = srv.Serve(lis) }()
		defer srv.GracefulStop()
		pub := discov.NewPublisher(hosts, key, lis.Addr().String())
		require.NoError(t, pub.KeepAlive())
		defer pub.Stop()
	}

	conn := zrpc.MustNewClient(
		zrpc.RpcClientConf{
			Etcd:        discov.EtcdConf{Hosts: hosts, Key: key},
			Timeout:     5000,
			Middlewares: zrpc.ClientMiddlewaresConf{Timeout: true},
		},
		//退化为普通轮询
		//zrpc.WithDialOption(grpc.WithDefaultServiceConfig(`{"loadBalancingConfig":[{"custom_swrr":{}}]}`)),
	)
	client := userv1.NewUserServiceClient(conn.Conn())
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	call := func() string {
		u, cerr := client.GetUser(ctx, &userv1.GetUserRequest{Id: 1}, grpc.WaitForReady(true))
		require.NoError(t, cerr)
		return u.GetName()
	}
	seen := map[string]bool{}
	for len(seen) < len(flags) {
		seen[call()] = true
	}
	counts := map[string]int{}
	for i := 0; i < 1000; i++ {
		counts[call()]++
	}
	for _, flag := range flags {
		t.Logf("go-zero + custom_swrr: %q → %d 次", "Alice from "+flag, counts["Alice from "+flag])
	}
}
