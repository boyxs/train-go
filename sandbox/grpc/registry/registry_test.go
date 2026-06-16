package registry

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	etcdv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/naming/resolver"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	userv1 "webook/sandbox/grpc/gen/user/v1"
	server2 "webook/sandbox/grpc/server"
)

// TestRegistryRoundTrip 是自包含的自动化测试:用 ServiceRegistry(registry.go)做一次
// 完整的「注册 → etcd resolver 发现 → 调用 → 优雅注销」往返。与手动脚本(etcd_demo_test.go)不同,
// 它后台起 server(不阻塞)、所有错误都 require、用临时命名空间避免污染真实数据;
// 本机 / CI 没有 etcd 时显式 t.Skip,而非假绿或挂起。
func TestRegistryRoundTrip(t *testing.T) {
	// 探测 etcd:连不上就跳过(不引入内嵌 etcd 依赖)。
	cli, err := etcdv3.New(etcdv3.Config{
		Endpoints:   []string{"127.0.0.1:2379"},
		DialTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Skipf("etcd 客户端创建失败,跳过:%v", err)
	}
	defer func() { require.NoError(t, cli.Close()) }()

	probeCtx, probeCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer probeCancel()
	if _, err = cli.Status(probeCtx, "127.0.0.1:2379"); err != nil {
		t.Skipf("etcd 不可达,跳过:%v", err)
	}

	// 真实 TCP server,端口 0 由系统分配;后台 Serve,不阻塞测试主流程。
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := grpc.NewServer()
	userv1.RegisterUserServiceServer(srv, server2.NewMemoryUserServer())
	go func() {
		if serveErr := srv.Serve(lis); serveErr != nil {
			t.Logf("grpc serve exit: %v", serveErr)
		}
	}()
	defer srv.GracefulStop()
	addr := lis.Addr().String()

	// 用 registry.go 的抽象注册(独立命名空间,避免和真实 service/user 冲突)。
	reg, err := NewServiceRegistry(cli, "service/user-test", 5)
	require.NoError(t, err)
	regCtx, regCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer regCancel()
	require.NoError(t, reg.Register(regCtx, addr))
	defer func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer closeCancel()
		require.NoError(t, reg.Close(closeCtx)) // 停续租 + 注销端点,不泄漏 goroutine
	}()

	// etcd resolver 客户端:从 "etcd:///service/user-test" 发现刚注册的实例。
	etcdResolver, err := resolver.NewBuilder(cli)
	require.NoError(t, err)
	cc, err := grpc.NewClient("etcd:///service/user-test",
		grpc.WithResolvers(etcdResolver),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer func() { require.NoError(t, cc.Close()) }()

	client := userv1.NewUserServiceClient(cc)
	callCtx, callCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer callCancel()
	// WaitForReady:容忍注册数据传播到 resolver 的短暂延迟,在 ctx 截止前等连接就绪。
	u, err := client.GetUser(callCtx, &userv1.GetUserRequest{Id: 1}, grpc.WaitForReady(true))
	require.NoError(t, err)
	require.NotNil(t, u)
	require.Equal(t, int64(1), u.GetId())
	require.Equal(t, "Alice", u.GetName())
}
