package registry

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	etcdv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	_ "webook/sandbox/grpc/balancer/balancer/swrr"
	userv1 "webook/sandbox/grpc/gen/user/v1"
	server2 "webook/sandbox/grpc/server"
)

func TestWeightOf(t *testing.T) {
	tests := []struct {
		name string
		meta any
		want int
	}{
		{"nil", nil, 1},
		{"非 map", "oops", 1},
		{"无 weight 键", map[string]any{"meta": "x"}, 1},
		{"float64(etcd JSON 回读)", map[string]any{"weight": float64(40)}, 40},
		{"int(防御)", map[string]any{"weight": 50}, 50},
		{"零值按 1", map[string]any{"weight": float64(0)}, 1},
		{"负值按 1", map[string]any{"weight": float64(-5)}, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, weightOf(tc.meta))
		})
	}
}

// TestSWRRWeightedDiscovery 自包含验证完整链路:EtcdRegistry 注册 3 个带权实例(10:40:50)
// → 自定义 EtcdResolverBuilder 发现并把权重下发 → custom_swrr 均衡器按权重分流。
// 临时命名空间避免污染 service/user;本机 / CI 无 etcd 时显式 Skip。
func TestSWRRWeightedDiscovery(t *testing.T) {
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

	// 后台起 3 个真实 server(flag A/B/C),并以对应权重注册进临时命名空间。
	flags := []string{"A", "B", "C"}
	weights := []uint32{10, 40, 50}
	reg := NewEtcdRegistry(cli)
	for i, flag := range flags {
		lis, lerr := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, lerr)
		srv := grpc.NewServer()
		userv1.RegisterUserServiceServer(srv, server2.NewMemoryUserServer(flag))
		go func() {
			if serveErr := srv.Serve(lis); serveErr != nil {
				t.Logf("serve exit: %v", serveErr)
			}
		}()
		defer srv.GracefulStop()
		regCtx, regCancel := context.WithTimeout(context.Background(), 3*time.Second)
		require.NoError(t, reg.Register(regCtx, ServiceInstance{
			Name: "user-swrr-test", Addr: lis.Addr().String(), Weight: weights[i],
		}))
		regCancel()
	}
	defer func() { require.NoError(t, reg.Close()) }() // 停续租 + 注销

	cc, err := grpc.NewClient("etcd:///service/user-swrr-test",
		grpc.WithResolvers(NewEtcdResolverBuilder(cli)),
		grpc.WithDefaultServiceConfig(`{"loadBalancingConfig":[{"custom_swrr":{}}]}`),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer func() { require.NoError(t, cc.Close()) }()

	client := userv1.NewUserServiceClient(cc)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	call := func() string {
		u, cerr := client.GetUser(ctx, &userv1.GetUserRequest{Id: 1}, grpc.WaitForReady(true))
		require.NoError(t, cerr)
		return u.GetName()
	}

	// 预热:等 3 个实例都被发现并就绪,再正式统计。
	seen := map[string]bool{}
	for len(seen) < len(flags) {
		seen[call()] = true
	}
	const count = 1000
	counts := map[string]int{}
	for i := 0; i < count; i++ {
		counts[call()]++
	}
	for i, flag := range flags {
		got := counts["Alice from "+flag]
		want := float64(count) * float64(weights[i]) / 100.0
		t.Logf("server %s 权重 %d → 命中 %d 次", flag, weights[i], got)
		require.InDelta(t, want, float64(got), want*0.1)
	}
}
