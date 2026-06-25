package interceptor

import (
	"context"
	"net"
	"strings"

	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

// PeerName 获取对端应用名称（取 incoming metadata 的 app 头）。
func PeerName(ctx context.Context) string {
	return grpcHeaderValue(ctx, "app")
}

// PeerIp 获取对端 IP：优先 client-ip 头，回落传输层 peer 地址。
func PeerIp(ctx context.Context) string {
	if clientIp := grpcHeaderValue(ctx, "client-ip"); clientIp != "" {
		return clientIp
	}
	pr, ok := peer.FromContext(ctx)
	if !ok || pr.Addr == nil {
		return ""
	}
	// net.SplitHostPort 同时正确处理 IPv4("1.2.3.4:80") 与 IPv6("[::1]:80")
	host, _, err := net.SplitHostPort(pr.Addr.String())
	if err != nil {
		return ""
	}
	return host
}

func grpcHeaderValue(ctx context.Context, key string) string {
	if key == "" {
		return ""
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	return strings.Join(md.Get(key), ";")
}
