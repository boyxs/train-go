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
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/resolver/manual"

	"github.com/boyxs/train-go/webook/pkg/grpcx/balancer/group"
)

// startHealthServer 起一个报 SERVING 的健康 server(免自定义 proto),返回监听地址。
func startHealthServer(t *testing.T) string {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := grpc.NewServer()
	hs := health.NewServer()
	hs.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(srv, hs)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.GracefulStop)
	return lis.Addr().String()
}

// TestGroupSWRR_RequestRouting 验证按请求 tier 分流:带 x-tier:vip 的请求只落 VIP 节点,
// 不带 tier 的请求只落默认(普通)节点。
func TestGroupSWRR_RequestRouting(t *testing.T) {
	vip := map[string]bool{startHealthServer(t): true, startHealthServer(t): true}
	std := map[string]bool{startHealthServer(t): true, startHealthServer(t): true}

	addrs := make([]resolver.Address, 0, 4)
	for a := range vip {
		addrs = append(addrs, group.Set(resolver.Address{Addr: a}, "vip"))
	}
	for a := range std {
		addrs = append(addrs, group.Set(resolver.Address{Addr: a}, "")) // 默认组
	}

	r := manual.NewBuilderWithScheme("grouptest")
	r.InitialState(resolver.State{Addresses: addrs})
	cc, err := grpc.NewClient("grouptest:///",
		grpc.WithResolvers(r),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(`{"loadBalancingConfig":[{"`+NameGroup+`":{}}]}`),
	)
	require.NoError(t, err)
	defer func() { require.NoError(t, cc.Close()) }()

	client := healthpb.NewHealthClient(cc)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	call := func(c context.Context) string {
		var p peer.Peer
		_, cerr := client.Check(c, &healthpb.HealthCheckRequest{}, grpc.WaitForReady(true), grpc.Peer(&p))
		require.NoError(t, cerr)
		return p.Addr.String()
	}

	vipCtx := metadata.AppendToOutgoingContext(ctx, TierMetadataKey, "vip")

	// 预热:轮询到 4 个节点都就绪(都被命中过)再统计,确保 VIP 组也 READY,
	// 否则 VIP 组没节点会触发 fallback 落到普通组,造成误判。
	seen := map[string]bool{}
	for len(seen) < 4 {
		seen[call(vipCtx)] = true
		seen[call(ctx)] = true
	}

	// 正式验证隔离:各发 200 次,并统计命中分布
	vipHits := map[string]int{}
	for i := 0; i < 200; i++ {
		addr := call(vipCtx)
		require.Truef(t, vip[addr], "VIP 请求落到非 VIP 节点 %s", addr)
		vipHits[addr]++
	}
	t.Logf("VIP 请求 200 次命中分布(应只在 VIP 节点): %v", vipHits)

	stdHits := map[string]int{}
	for i := 0; i < 200; i++ {
		addr := call(ctx)
		require.Truef(t, std[addr], "普通请求落到非普通节点 %s", addr)
		stdHits[addr]++
	}
	t.Logf("普通请求 200 次命中分布(应只在普通节点): %v", stdHits)
}
