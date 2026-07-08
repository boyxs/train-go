package swrr

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/resolver/manual"

	"github.com/boyxs/train-go/webook/pkg/grpcx/balancer/weight"
)

// TestSWRR_Distribution 真起 3 个 gRPC server(权重 10:40:50),经 custom_swrr 发 1000 次调用,
// 按 peer 地址统计命中,验证真链路上分流比例 ≈ 权重比。用内置 health 服务免去自定义 proto。
func TestSWRR_Distribution(t *testing.T) {
	weights := []int{10, 40, 50}
	addrs := make([]resolver.Address, len(weights))
	for i, w := range weights {
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		srv := grpc.NewServer()
		hs := health.NewServer()
		hs.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
		healthpb.RegisterHealthServer(srv, hs)
		go func() { _ = srv.Serve(lis) }()
		defer srv.GracefulStop()
		addrs[i] = weight.Set(resolver.Address{Addr: lis.Addr().String()}, w)
	}

	// 手动 resolver 把 3 个带权地址喂给客户端;服务配置选中 custom_swrr。
	r := manual.NewBuilderWithScheme("swrrtest")
	r.InitialState(resolver.State{Addresses: addrs})
	cc, err := grpc.NewClient("swrrtest:///",
		grpc.WithResolvers(r),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(`{"loadBalancingConfig":[{"`+Name+`":{}}]}`),
	)
	require.NoError(t, err)
	defer func() { require.NoError(t, cc.Close()) }()

	client := healthpb.NewHealthClient(cc)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	call := func() string {
		var p peer.Peer
		_, err := client.Check(ctx, &healthpb.HealthCheckRequest{}, grpc.WaitForReady(true), grpc.Peer(&p))
		require.NoError(t, err)
		return p.Addr.String()
	}

	// 预热:循环到 3 个 server 都被命中过,确保 picker 已纳入全部就绪连接再统计。
	seen := map[string]bool{}
	for len(seen) < len(addrs) {
		seen[call()] = true
	}

	const count = 1000 // = 10 个 SWRR 周期(总权重 100)→ 命中数应紧贴权重比
	counts := map[string]int{}
	for i := 0; i < count; i++ {
		counts[call()]++
	}
	for i, a := range addrs {
		got := counts[a.Addr]
		want := float64(count) * float64(weights[i]) / 100.0
		t.Logf("addr %s 权重 %d → 命中 %d 次", a.Addr, weights[i], got)
		require.InDelta(t, want, float64(got), want*0.1)
	}
}
