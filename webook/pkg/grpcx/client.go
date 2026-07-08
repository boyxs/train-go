package grpcx

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	etcdv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	_ "google.golang.org/grpc/health" // init 注册客户端健康检查函数,HealthCheck=true 才真正生效
	"google.golang.org/grpc/keepalive"

	_ "github.com/boyxs/train-go/webook/pkg/grpcx/balancer/swrr" // 注册 custom_swrr / breaker_swrr / group_swrr
	etcdresolver "github.com/boyxs/train-go/webook/pkg/grpcx/resolver/etcd"
)

// ClientConfig 描述一个下游 gRPC 连接。target/balancer 必填;其余为调参 / 功能开关式缺省(不写=关)。
type ClientConfig struct {
	Target         string        `mapstructure:"target"`            // [必填] 解析目标 etcd:///service/webook-xxx
	Balancer       string        `mapstructure:"balancer"`          // [必填] 负载均衡器名;空 → pick_first
	Secure         bool          `mapstructure:"secure"`            // [默认 false=insecure]
	CAFile         string        `mapstructure:"ca_file"`           // secure=true 时验签 CA;空用系统根证书
	Timeout        time.Duration `mapstructure:"timeout"`           // 单次调用超时;>0 才启用(省略=靠服务端超时;非幂等写慎设 < 服务端)
	KeepAlive      KeepAlive     `mapstructure:"keep_alive"`        // 功能开关式:time>0 才启用
	MaxRecvMsgSize int           `mapstructure:"max_recv_msg_size"` // 字节;0=库默认 4MB
	MaxSendMsgSize int           `mapstructure:"max_send_msg_size"` // 字节;0=库默认
	HealthCheck    bool          `mapstructure:"health_check"`      // 启用 grpc 健康检查(服务端已注册 health service)
	Retry          *GRPCRetry    `mapstructure:"retry"`             // nil=不重试;启用须过幂等/熔断/观测评审
}

// KeepAlive 客户端长连接保活;Time<=0 关闭整段。
type KeepAlive struct {
	Time                time.Duration `mapstructure:"time"`
	Timeout             time.Duration `mapstructure:"timeout"`
	PermitWithoutStream bool          `mapstructure:"permit_without_stream"`
}

// GRPCRetry 组进 service config 的 methodConfig.retryPolicy。
type GRPCRetry struct {
	MaxAttempts       int           `mapstructure:"max_attempts"` // 总尝试次数 ∈ [2,5](grpc-go 硬上限 5)
	InitialBackoff    time.Duration `mapstructure:"initial_backoff"`
	MaxBackoff        time.Duration `mapstructure:"max_backoff"`
	BackoffMultiplier float64       `mapstructure:"backoff_multiplier"`
	RetryableCodes    []string      `mapstructure:"retryable_codes"` // gRPC 码名,如 UNAVAILABLE
	Methods           []string      `mapstructure:"methods"`         // pkg.Service 或 pkg.Service/Method;空=全部方法
}

// NewClient 按 etcd 服务发现拨号下游:装带权 etcd resolver + 从类型化配置组 service config
// (loadBalancingConfig / methodConfig.timeout / retryPolicy / healthCheckConfig)+ keepalive / 消息尺寸;
// otel、拦截器等正交 option 由调用方经 opts 传入。client 生命周期归调用方,返回的 cleanup 仅关闭 conn。
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

	sc, err := cfg.serviceConfigJSON()
	if err != nil {
		return nil, nil, err
	}
	if sc != "" {
		dialOpts = append(dialOpts, grpc.WithDefaultServiceConfig(sc))
	}
	if cfg.KeepAlive.Time > 0 {
		dialOpts = append(dialOpts, grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                cfg.KeepAlive.Time,
			Timeout:             cfg.KeepAlive.Timeout,
			PermitWithoutStream: cfg.KeepAlive.PermitWithoutStream,
		}))
	}
	var callOpts []grpc.CallOption
	if cfg.MaxRecvMsgSize > 0 {
		callOpts = append(callOpts, grpc.MaxCallRecvMsgSize(cfg.MaxRecvMsgSize))
	}
	if cfg.MaxSendMsgSize > 0 {
		callOpts = append(callOpts, grpc.MaxCallSendMsgSize(cfg.MaxSendMsgSize))
	}
	if len(callOpts) > 0 {
		dialOpts = append(dialOpts, grpc.WithDefaultCallOptions(callOpts...))
	}

	conn, err := grpc.NewClient(cfg.Target, dialOpts...)
	if err != nil {
		return nil, nil, err
	}
	// 退出阶段 logger 可能已关,cleanup 用 stderr 兜底
	cleanup := func() {
		if err := conn.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "grpcx: client conn close:", err)
		}
	}
	return conn, cleanup, nil
}

// credentials 按 Secure/CAFile 构造传输凭证:insecure,或 TLS(CAFile 验签,空用系统根证书)。
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

// serviceConfigJSON 从类型化配置组 grpc service config JSON(不设裸 JSON 键)。
// 无任何段时返回空串,保持 grpc 默认(pick_first、无 methodConfig)。
func (c ClientConfig) serviceConfigJSON() (string, error) {
	var sc grpcServiceConfig
	if c.Balancer != "" {
		sc.LoadBalancingConfig = []map[string]struct{}{{c.Balancer: {}}}
	}
	if c.HealthCheck {
		sc.HealthCheckConfig = &healthCheckConfig{ServiceName: ""}
	}
	// 默认条目(所有方法 name:[{}]):timeout 显式(>0)才写;retry 无方法作用域时并入。
	def := grpcMethodConfig{Name: []map[string]string{{}}}
	hasDef := false
	if c.Timeout > 0 {
		def.Timeout = durString(c.Timeout)
		hasDef = true
	}
	if c.Retry != nil && len(c.Retry.Methods) == 0 {
		def.RetryPolicy = c.Retry.policy()
		hasDef = true
	}
	if hasDef {
		sc.MethodConfig = append(sc.MethodConfig, def)
	}
	// retry 限定方法:独立条目(带上 timeout,避免比默认条目更具体而丢失超时)。
	if c.Retry != nil && len(c.Retry.Methods) > 0 {
		names, err := parseMethodNames(c.Retry.Methods)
		if err != nil {
			return "", err
		}
		mc := grpcMethodConfig{Name: names, RetryPolicy: c.Retry.policy()}
		if c.Timeout > 0 {
			mc.Timeout = durString(c.Timeout)
		}
		sc.MethodConfig = append(sc.MethodConfig, mc)
	}
	if sc.LoadBalancingConfig == nil && sc.HealthCheckConfig == nil && len(sc.MethodConfig) == 0 {
		return "", nil
	}
	b, err := json.Marshal(sc)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// grpc service config 的类型化子集(仅本项目消费的字段)。
type grpcServiceConfig struct {
	LoadBalancingConfig []map[string]struct{} `json:"loadBalancingConfig,omitempty"`
	HealthCheckConfig   *healthCheckConfig    `json:"healthCheckConfig,omitempty"`
	MethodConfig        []grpcMethodConfig    `json:"methodConfig,omitempty"`
}

type healthCheckConfig struct {
	ServiceName string `json:"serviceName"`
}

type grpcMethodConfig struct {
	Name        []map[string]string `json:"name"`
	Timeout     string              `json:"timeout,omitempty"`
	RetryPolicy *retryPolicy        `json:"retryPolicy,omitempty"`
}

type retryPolicy struct {
	MaxAttempts          int      `json:"maxAttempts"`
	InitialBackoff       string   `json:"initialBackoff"`
	MaxBackoff           string   `json:"maxBackoff"`
	BackoffMultiplier    float64  `json:"backoffMultiplier"`
	RetryableStatusCodes []string `json:"retryableStatusCodes"`
}

// policy 生成 retryPolicy。grpc-go 要求 maxAttempts∈[2,5]、backoff>0、codes 非空,缺一整段被拒→拨号失败;
// 故缺省字段就地兜底,部分配置也能起来。
func (r *GRPCRetry) policy() *retryPolicy {
	attempts := r.MaxAttempts
	switch {
	case attempts < 2:
		attempts = 3
	case attempts > 5:
		attempts = 5 // grpc-go 硬上限
	}
	initBackoff := r.InitialBackoff
	if initBackoff <= 0 {
		initBackoff = 100 * time.Millisecond
	}
	maxBackoff := r.MaxBackoff
	if maxBackoff <= 0 {
		maxBackoff = time.Second
	}
	mult := r.BackoffMultiplier
	if mult <= 0 {
		mult = 2
	}
	codes := r.RetryableCodes
	if len(codes) == 0 {
		codes = []string{"UNAVAILABLE"}
	}
	return &retryPolicy{
		MaxAttempts:          attempts,
		InitialBackoff:       durString(initBackoff),
		MaxBackoff:           durString(maxBackoff),
		BackoffMultiplier:    mult,
		RetryableStatusCodes: codes,
	}
}

// durString 转成 grpc service config 接受的秒格式("3s" / "0.25s")。
func durString(d time.Duration) string {
	return fmt.Sprintf("%gs", d.Seconds())
}

// parseMethodNames 把 "pkg.Service" / "pkg.Service/Method" 解析成 service config 的 name 条目。
func parseMethodNames(methods []string) ([]map[string]string, error) {
	names := make([]map[string]string, 0, len(methods))
	for _, m := range methods {
		svc, method, hasMethod := strings.Cut(m, "/")
		if svc == "" {
			return nil, fmt.Errorf("grpcx: 非法 retry method %q", m)
		}
		entry := map[string]string{"service": svc}
		if hasMethod && method != "" {
			entry["method"] = method
		}
		names = append(names, entry)
	}
	return names, nil
}
