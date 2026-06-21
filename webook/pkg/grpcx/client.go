package grpcx

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	etcdv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	_ "github.com/webook/pkg/grpcx/balancer/swrr" // 注册 custom_swrr / breaker_swrr / group_swrr
	etcdresolver "github.com/webook/pkg/grpcx/resolver/etcd"
)

// ClientConfig 描述一个下游 gRPC 连接：解析目标 + 传输安全 + 负载均衡。
type ClientConfig struct {
	Target   string `yaml:"target"`   // 解析目标，如 etcd:///service/webook-core
	Secure   bool   `yaml:"secure"`   // 传输安全：false→insecure（默认），true→TLS
	CAFile   string `yaml:"caFile"`   // secure=true 时验签 CA；空则用系统根证书
	Balancer string `yaml:"balancer"` // 负载均衡器名（custom_swrr/breaker_swrr/group_swrr）；空→pick_first
}

// NewClient 按 etcd 服务发现拨号下游：装带权 etcd resolver + 按 cfg.Balancer 选负载均衡器
// + 按 cfg 构造传输凭证；otel、拦截器等正交 option 由调用方经 opts 传入。
// client 生命周期归调用方，返回的 cleanup 仅关闭 conn。
func NewClient(client *etcdv3.Client, cfg ClientConfig, opts ...grpc.DialOption) (*grpc.ClientConn, func(), error) {
	r, err := etcdresolver.NewBuilder(client)
	if err != nil {
		return nil, nil, err
	}
	creds, err := cfg.credentials()
	if err != nil {
		return nil, nil, err
	}
	dialOpts := append([]grpc.DialOption{grpc.WithResolvers(r), creds}, opts...)
	if cfg.Balancer != "" {
		dialOpts = append(dialOpts,
			grpc.WithDefaultServiceConfig(`{"loadBalancingConfig":[{"`+cfg.Balancer+`":{}}]}`))
	}
	conn, err := grpc.NewClient(cfg.Target, dialOpts...)
	if err != nil {
		return nil, nil, err
	}
	// 退出阶段 logger 可能已关，cleanup 用 stderr 兜底
	cleanup := func() {
		if err := conn.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "grpcx: client conn close:", err)
		}
	}
	return conn, cleanup, nil
}

// credentials 按 Secure/CAFile 构造传输凭证：insecure，或 TLS（CAFile 验签，空用系统根证书）。
func (c ClientConfig) credentials() (grpc.DialOption, error) {
	if !c.Secure {
		return grpc.WithTransportCredentials(insecure.NewCredentials()), nil
	}
	tlsCfg := &tls.Config{}
	if c.CAFile != "" {
		pem, err := os.ReadFile(c.CAFile)
		if err != nil {
			return nil, err
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("grpcx: caFile 解析失败: %s", c.CAFile)
		}
		tlsCfg.RootCAs = pool
	}
	return grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)), nil
}
