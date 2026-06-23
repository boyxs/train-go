package registry

import (
	"context"
	"net"
	"os"
	"sync/atomic"
	"testing"
	"time"
	_ "webook/sandbox/grpc/balancer/balancer/swrr"
	userv1 "webook/sandbox/grpc/gen/user/v1"
	"webook/sandbox/grpc/interceptor"
	server2 "webook/sandbox/grpc/server"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	etcdv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/naming/endpoints"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	_ "google.golang.org/grpc/balancer/weightedroundrobin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type InterceptorSuite struct {
	suite.Suite
	cli     *etcdv3.Client
	limiter interceptor.Limiter
}

func (e *InterceptorSuite) SetupSuite() {
	cli, err := etcdv3.New(etcdv3.Config{
		Endpoints: []string{"127.0.0.1:2379"},
	})
	//cli, err := etcdv3.NewFromURL("127.0.0.1:2379")
	require.NoError(e.T(), err)
	e.cli = cli

	//e.limiter = interceptor.NewCounterLimiter()
	e.limiter = interceptor.NewFixedWindowLimiter()
	//e.limiter = interceptor.NewSlidingWindowLimiter()
	//e.limiter = interceptor.NewTokenBucketLimiter()
	//e.limiter = interceptor.NewRateTokenBucketLimiter()
	//e.limiter = interceptor.NewLeakyBucketLimiter()
}

// TearDownSuite 关闭整个 suite 共享的 etcd client。
func (e *InterceptorSuite) TearDownSuite() {
	if e.cli != nil {
		require.NoError(e.T(), e.cli.Close())
	}
}

func (e *InterceptorSuite) TestServer() {
	if os.Getenv("ETCD_MANUAL") == "" {
		e.T().Skip("手动脚本:Serve 会阻塞需手动停;设 ETCD_MANUAL=1 且本地有 etcd 时运行")
	}
	port := ":8090"
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

	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			e.limiter.Build(),
		),
	)
	// c -> unreadable: unknown or unsupported kind: "invalid"
	c, ok := e.limiter.(interceptor.Closer)
	if ok {
		defer c.Close()
	}
	userv1.RegisterUserServiceServer(server, server2.NewLimiterServer())
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
}

func (e *InterceptorSuite) TestClient() {
	if os.Getenv("ETCD_MANUAL") == "" {
		e.T().Skip("手动脚本:需 TestServer 已注册端点;设 ETCD_MANUAL=1 运行")
	}
	t := e.T()
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

	// 并发打 1000 个请求,制造真并发让在途数冲过阈值,触发限流。
	// (串行循环时在途数恒为 1,永远到不了阈值,限流不会生效。)
	var eg errgroup.Group
	var passed, limited atomic.Int32
	for i := 0; i < 1000; i++ {
		eg.Go(func() error {
			_, err := client.GetUser(ctx, &userv1.GetUserRequest{Id: 1}, grpc.WaitForReady(true))
			switch status.Code(err) {
			case codes.OK:
				passed.Add(1)
			case codes.ResourceExhausted:
				limited.Add(1)
			default:
				return err
			}
			return nil
		})
	}
	if err = eg.Wait(); err != nil {
		t.Log(err)
		return
	}
	t.Logf("并发 1000:放行 %d,限流 %d", passed.Load(), limited.Load())
}

func TestInterceptor(t *testing.T) {
	suite.Run(t, new(InterceptorSuite))
}
