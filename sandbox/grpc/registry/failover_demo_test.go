package registry

import (
	"context"
	_ "embed"
	"net"
	"os"
	"testing"
	"time"
	_ "webook/sandbox/grpc/balancer/balancer/swrr"
	userv1 "webook/sandbox/grpc/gen/user/v1"
	"webook/sandbox/grpc/server"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	etcdv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/naming/endpoints"
	"google.golang.org/grpc"
	_ "google.golang.org/grpc/balancer/weightedroundrobin"
	"google.golang.org/grpc/credentials/insecure"
)

type FailoverSuite struct {
	suite.Suite
	cli *etcdv3.Client
}

func (e *FailoverSuite) SetupSuite() {
	cli, err := etcdv3.New(etcdv3.Config{
		Endpoints: []string{"127.0.0.1:2379"},
	})
	//cli, err := etcdv3.NewFromURL("127.0.0.1:2379")
	require.NoError(e.T(), err)
	e.cli = cli
}

// TearDownSuite 关闭整个 suite 共享的 etcd client。
func (e *FailoverSuite) TearDownSuite() {
	if e.cli != nil {
		require.NoError(e.T(), e.cli.Close())
	}
}

//go:embed failover.json
var svcCfg string

func (e *FailoverSuite) TestServer() {
	if os.Getenv("ETCD_MANUAL") == "" {
		e.T().Skip("手动脚本:Serve 会阻塞需手动停;设 ETCD_MANUAL=1 且本地有 etcd 时运行")
	}
	go func() {
		e.startServer(":8090", server.NewAlwaysFailedServer("failed"))
	}()
	go func() {
		e.startServer(":8091", server.NewMemoryUserServer(":8091"))
	}()
	e.startServer(":8092", server.NewMemoryUserServer(":8092"))
}

func (e *FailoverSuite) TestRoundRobinClient() {
	if os.Getenv("ETCD_MANUAL") == "" {
		e.T().Skip("手动脚本:需 TestServer 在另一终端注册端点;设 ETCD_MANUAL=1 运行")
	}
	e.startClient(svcCfg)
}

func (e *FailoverSuite) startServer(port string, srv userv1.UserServiceServer) {
	t := e.T()
	em, err := endpoints.NewManager(e.cli, "service/user")
	require.NoError(t, err)
	addr := "127.0.0.1" + port
	key := "service/user/" + addr
	lis, err := net.Listen("tcp", port)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	// 租期
	var ttl int64 = 5
	leaseGrantResp, err := e.cli.Grant(ctx, ttl)
	require.NoError(t, err)

	// 添加端点
	err = em.AddEndpoint(ctx, key, endpoints.Endpoint{
		Addr: addr,
	}, etcdv3.WithLease(leaseGrantResp.ID))
	require.NoError(t, err)

	// KeepAlive 在主 goroutine 调用并校验错误(require 只能在测试 goroutine 用),
	// 子 goroutine 只消费续租响应;kaCancel 后 channel 关闭,goroutine 自然退出。
	kaCtx, kaCancel := context.WithCancel(context.Background())
	lch, err := e.cli.KeepAlive(kaCtx, leaseGrantResp.ID)
	require.NoError(t, err)
	go func() {
		for l := range lch {
			t.Log(l.String())
		}
	}()

	server := grpc.NewServer()
	userv1.RegisterUserServiceServer(server, srv)
	// Serve 阻塞直到外部停掉 server 才返回,因此下面的 cCancel / 注销 / GracefulStop
	// 只有手动中断后才执行——这也是本用例只能手动观察、不进自动化套件的根因。
	err = server.Serve(lis)
	if err != nil {
		t.Log(err)
	}
	kaCancel()

	// 删除端点:用新 ctx,开头那个 1s 超时的早过期了。
	delCtx, delCancel := context.WithTimeout(context.Background(), time.Second)
	err = em.DeleteEndpoint(delCtx, key)
	delCancel()
	if err != nil {
		t.Log(err)
	}
	server.GracefulStop()
	// e.cli 由 SetupSuite 创建、整个 suite 共享,这里不关,统一在 TearDownSuite 关。
}

func (e *FailoverSuite) startClient(svcCfg string) {
	t := e.T()
	// "etcd:///service/user" 里整段 path 即注册命名空间,
	// 自定义 resolver watch "service/user/" 前缀,把地址 + 权重下发给均衡器。
	etcdResolver := NewEtcdResolverBuilder(e.cli)
	cc, err := grpc.NewClient("etcd:///service/user",
		grpc.WithResolvers(etcdResolver),
		grpc.WithDefaultServiceConfig(svcCfg),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer func() { require.NoError(t, cc.Close()) }()
	client := userv1.NewUserServiceClient(cc)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 单客户端连发多次并统计命中分布:SWRR 的加权靠 picker 的 curWeight 跨调用累积才体现,
	// 多客户端各调一次只会得到连接竞速噪声。
	counts := map[string]int{}
	var prev *userv1.User
	maxRun := map[string]int{}
	run := 0
	for i := 0; i < 10; i++ {
		user, err := client.GetUser(ctx, &userv1.GetUserRequest{Id: 1}, grpc.WaitForReady(true))
		require.NoError(t, err)
		counts[user.GetName()]++
		if user == prev {
			run++
		} else {
			run = 1
		}
		prev = user
		if run > maxRun[user.Name] {
			maxRun[user.Name] = run
		}
	}
	for name, n := range counts {
		t.Logf("%s → %d 次,最大连续命中 %d", name, n, maxRun[name])
	}
}

func TestFailover(t *testing.T) {
	suite.Run(t, new(FailoverSuite))
}
