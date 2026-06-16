package registry

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	etcdv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/naming/endpoints"
	"go.etcd.io/etcd/client/v3/naming/resolver"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	userv1 "webook/sandbox/grpc/gen/user/v1"
	server2 "webook/sandbox/grpc/server"
)

// EtcdSuite 是一组手动探索脚本,演示 gRPC + etcd 的服务注册 / 续租 / 发现链路。
// 需本地 etcd(127.0.0.1:2379),默认 Skip(设 ETCD_MANUAL=1 才跑)。自动化版本见 TestRegistryRoundTrip。
//
// 注意:TestServer 的 Serve 会一直阻塞需手动 Ctrl+C 停,且不能和 TestClient 同一次
// go test 跑(suite 按字母序先跑 TestClient,Serve 阻塞后根本到不了它)。分两个终端跑:
//
//	# 终端1 起 server(阻塞,Ctrl+C 停;-timeout 0 关掉默认超时,否则 Serve 永不返回会被 panic)
//	$env:ETCD_MANUAL="1"; go test ./registry/ -run 'TestEtcd/TestServer' -v -timeout 0
//	ETCD_MANUAL="1" go test ./registry/ -run 'TestEtcd/TestServer' -v -timeout 0
//	# 终端2 趁 server 在,跑 client 调一次
//	$env:ETCD_MANUAL="1"; go test ./registry/ -run 'TestEtcd/TestClient' -v
//	ETCD_MANUAL="1" go test ./registry/ -run 'TestEtcd/TestClient' -v
type EtcdSuite struct {
	suite.Suite
	cli *etcdv3.Client
}

func (e *EtcdSuite) SetupSuite() {
	cli, err := etcdv3.New(etcdv3.Config{
		Endpoints: []string{"127.0.0.1:2379"},
	})
	//cli, err := etcdv3.NewFromURL("127.0.0.1:2379")
	require.NoError(e.T(), err)
	e.cli = cli
}

// TearDownSuite 关闭整个 suite 共享的 etcd client。
func (e *EtcdSuite) TearDownSuite() {
	if e.cli != nil {
		require.NoError(e.T(), e.cli.Close())
	}
}

func (e *EtcdSuite) TestServer() {
	if os.Getenv("ETCD_MANUAL") == "" {
		e.T().Skip("手动脚本:Serve 会阻塞需手动停;设 ETCD_MANUAL=1 且本地有 etcd 时运行")
	}
	port := ":8091"
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
	err = em.AddEndpoint(ctx, key, endpoints.Endpoint{Addr: addr}, etcdv3.WithLease(leaseGrantResp.ID))
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

	// 模拟注册数据变动:每秒刷新 Metadata,演示变更如何传播给 watcher。
	// 监听 kaCtx,kaCancel 后退出并 Stop ticker,不泄漏 goroutine。
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-kaCtx.Done():
				return
			case now := <-ticker.C:
				ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
				err2 := em.Update(ctx2, []*endpoints.UpdateWithOpts{
					{
						Update: endpoints.Update{
							Op:  endpoints.Add,
							Key: key,
							Endpoint: endpoints.Endpoint{
								Addr:     addr,
								Metadata: now.String(),
							},
						},
					},
				})
				cancel2()
				if err2 != nil {
					t.Log(err2)
				}
			}
		}
	}()

	server := grpc.NewServer()
	userv1.RegisterUserServiceServer(server, server2.NewMemoryUserServer())
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

func (e *EtcdSuite) TestClient() {
	if os.Getenv("ETCD_MANUAL") == "" {
		e.T().Skip("手动脚本:需 TestServer 已注册端点;设 ETCD_MANUAL=1 运行")
	}
	t := e.T()
	// "etcd:///service/user" 里的 service 段即注册命名空间,
	// resolver watch "service/user/" 前缀拿到全部实例地址。
	etcdResolver, err := resolver.NewBuilder(e.cli)
	require.NoError(t, err)
	cc, err := grpc.NewClient("etcd:///service/user",
		grpc.WithResolvers(etcdResolver),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer func() { require.NoError(t, cc.Close()) }()
	client := userv1.NewUserServiceClient(cc)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	user, err := client.GetUser(ctx, &userv1.GetUserRequest{Id: 1})
	require.NoError(t, err)
	require.NotNil(t, user)
	t.Log(user)
}

func TestEtcd(t *testing.T) {
	suite.Run(t, new(EtcdSuite))
}
