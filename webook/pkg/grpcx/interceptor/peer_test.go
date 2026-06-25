package interceptor

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

// fakeAddr 实现 net.Addr，用于伪造传输层 peer 地址。
type fakeAddr string

func (a fakeAddr) Network() string { return "tcp" }
func (a fakeAddr) String() string  { return string(a) }

func incomingMD(kv ...string) context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(kv...))
}

func withPeer(addr net.Addr) context.Context {
	return peer.NewContext(context.Background(), &peer.Peer{Addr: addr})
}

func TestPeerName(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{"有 app 头", incomingMD("app", "chat"), "chat"},
		{"无 app 头", incomingMD("other", "x"), ""},
		{"无 incoming metadata", context.Background(), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, PeerName(tt.ctx))
		})
	}
}

func TestGrpcHeaderValue_EmptyKey(t *testing.T) {
	assert.Equal(t, "", grpcHeaderValue(incomingMD("app", "x"), ""))
}

func TestPeerIp(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{"client-ip 头优先", incomingMD("client-ip", "9.9.9.9"), "9.9.9.9"},
		{"回落 peer IPv4", withPeer(fakeAddr("1.2.3.4:5678")), "1.2.3.4"},
		{"回落 peer IPv6", withPeer(fakeAddr("[::1]:5678")), "::1"},
		{"无头无 peer", context.Background(), ""},
		{"peer 地址无端口", withPeer(fakeAddr("unixsocket")), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, PeerIp(tt.ctx))
		})
	}
}
