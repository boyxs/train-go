package swrr

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/resolver/manual"

	userv1 "webook/sandbox/grpc/gen/user/v1"
	"webook/sandbox/grpc/server"
)

// TestSWRR_EndToEnd 真起 3 个 gRPC server(flag A/B/C,权重 10:40:50),经 custom_swrr 均衡器
// 发 1000 次调用,按响应里的 server flag 统计命中,验证真链路上的分流比例 ≈ 权重比。
func TestSWRR_EndToEnd(t *testing.T) {
	flags := []string{"A", "B", "C"}
	weights := []int{10, 40, 50}
	addrs := make([]resolver.Address, len(flags))
	for i := range flags {
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		srv := grpc.NewServer()
		userv1.RegisterUserServiceServer(srv, server.NewMemoryUserServer(flags[i]))
		go func() {
			if e := srv.Serve(lis); e != nil {
				t.Logf("serve exit: %v", e)
			}
		}()
		defer srv.GracefulStop()
		addrs[i] = SetWeight(resolver.Address{Addr: lis.Addr().String()}, weights[i])
	}

	// 手动 resolver 把 3 个带权地址喂给客户端;服务配置选中 custom_swrr。
	r := manual.NewBuilderWithScheme("swrrdemo")
	r.InitialState(resolver.State{Addresses: addrs})
	cc, err := grpc.NewClient("swrrdemo:///",
		grpc.WithResolvers(r),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(`{"loadBalancingConfig":[{"custom_swrr":{}}]}`),
	)
	require.NoError(t, err)
	defer func() { require.NoError(t, cc.Close()) }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client := userv1.NewUserServiceClient(cc)
	// 返回响应里的 server flag(server 把它拼进 Name:"Alice from <flag>")。
	call := func() string {
		u, err := client.GetUser(ctx, &userv1.GetUserRequest{Id: 1}, grpc.WaitForReady(true))
		require.NoError(t, err)
		return u.GetName()
	}

	// 预热:循环到 3 个 server 都被命中过,确保 picker 已纳入全部就绪连接再统计。
	seen := map[string]bool{}
	for len(seen) < len(flags) {
		seen[call()] = true
	}

	// 正式统计 1000 次(= 100 个 SWRR 周期 → 命中数应紧贴权重比)。
	const count = 1000
	counts := map[string]int{}
	for i := 0; i < count; i++ {
		counts[call()]++
	}
	for i, f := range flags {
		got := counts["Alice from "+f]
		want := float64(count) * float64(weights[i]) / 100.0
		t.Logf("server %s 权重 %d → 命中 %d 次", f, weights[i], got)
		require.InDelta(t, want, float64(got), want*0.1)
	}
}
