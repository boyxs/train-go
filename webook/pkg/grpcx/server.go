package grpcx

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	etcdv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/naming/endpoints"
	"google.golang.org/grpc"

	"github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/pkg/netx"
)

// defaultLeaseTTL 是未配置 TTL 时的租约默认值。
const defaultLeaseTTL = 30 * time.Second

// defaultServerTimeout 是未配置 server.grpc.timeout 时的 unary 处理超时兜底。
const defaultServerTimeout = 5 * time.Second

// Server 包装 *grpc.Server,叠加 etcd 服务注册/续租/注销。
// etcd client 由外部注入,生命周期不归本 Server。
type Server struct {
	*grpc.Server
	Addr   string // 监听地址,如 ":8011"
	Name   string
	Host   string        // 注册到 etcd 的广告 host;空则用 netx.ExternalIp()
	TTL    time.Duration // 租约 TTL,<=0 用 defaultLeaseTTL
	Weight int           // 注册权重(供带权 balancer 读)
	Client *etcdv3.Client
	L      logger.LoggerX

	key      string // 注册一次定下,重注册复用
	addr     string // 广告地址 host:port,由 Addr 的端口 + Host/出口 IP 组装
	em       endpoints.Manager
	kaCancel func()
}

// ServerConfig 是 gRPC server 的配置。addr/name 必填;ttl/weight/host/timeout 是调参键,缺省就地兜底。
type ServerConfig struct {
	Addr    string        `mapstructure:"addr"`    // [必填] 监听地址 ":8011";空 → Serve/Register 报错
	Name    string        `mapstructure:"name"`    // [必填] etcd 注册名;空 → Register 报错
	Host    string        `mapstructure:"host"`    // [默认 探测出口 IP] 广告 host(k8s 填 POD_IP)
	TTL     time.Duration `mapstructure:"ttl"`     // [默认 30s] 租约 TTL,<=0 用 defaultLeaseTTL
	Weight  int           `mapstructure:"weight"`  // [默认 1] 注册权重;<=0 不写,resolver 按 1 计
	Timeout time.Duration `mapstructure:"timeout"` // [默认 5s] unary 处理超时,<=0 兜底;streaming 天然豁免
}

// NewServer 建底层 grpc.Server:超时拦截器置于最外层(先设 deadline 再进调用方 metrics/errconv/业务),
// 其余 option 全由调用方经 opts 传入(如 otel StatsHandler、错误拦截器);调用方在返回值的 .Server 上注册 service。
func NewServer(cfg ServerConfig, client *etcdv3.Client, l logger.LoggerX, opts ...grpc.ServerOption) *Server {
	to := cfg.Timeout
	if to <= 0 {
		to = defaultServerTimeout
	}
	// ChainUnaryInterceptor 跨多次调用累加;本拦截器在最前 = 最外层,streaming 不走 unary 拦截器天然豁免。
	opts = append([]grpc.ServerOption{grpc.ChainUnaryInterceptor(unaryTimeoutInterceptor(to))}, opts...)
	return &Server{
		Server: grpc.NewServer(opts...),
		Addr:   cfg.Addr,
		Name:   cfg.Name,
		Host:   cfg.Host,
		TTL:    cfg.TTL,
		Weight: cfg.Weight,
		Client: client,
		L:      l,
	}
}

// unaryTimeoutInterceptor 给每个 unary 调用设 d 的处理超时;streaming 不经此拦截器,天然豁免。
func unaryTimeoutInterceptor(d time.Duration) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx, cancel := context.WithTimeout(ctx, d)
		defer cancel()
		return handler(ctx, req)
	}
}

// Serve 在 Addr 上监听并启动 gRPC server,阻塞直到 server 停止。
func (s *Server) Serve() error {
	lis, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return err
	}
	return s.Server.Serve(lis)
}

// Register 向 etcd 注册本实例(key = "service/<Name>/<host:port>")并启动后台续租;
// 续租中断(etcd 抖动 / lease 过期)会自动重注册,直到 Close。
// 排障:etcdctl --endpoints=127.0.0.1:2379 get --prefix service/<Name>
func (s *Server) Register() error {
	if s.Name == "" || s.Addr == "" {
		return fmt.Errorf("grpcx: 非法 server 配置 name=%q addr=%q", s.Name, s.Addr)
	}
	_, port, err := net.SplitHostPort(s.Addr)
	if err != nil {
		return fmt.Errorf("grpcx: 非法 server addr %q: %w", s.Addr, err)
	}
	em, err := endpoints.NewManager(s.Client, "service/"+s.Name)
	if err != nil {
		return err
	}
	s.em = em

	host := s.Host
	if host == "" {
		host = netx.ExternalIp()
	}
	s.addr = net.JoinHostPort(host, port)
	s.key = "service/" + s.Name + "/" + s.addr

	kaCtx, cancel := context.WithCancel(context.Background())
	s.kaCancel = cancel
	// 首次注册同步执行,把启动期错误返回给调用方。
	return s.register(kaCtx)
}

// register 申请租约、写端点、起续租;ctx 绑 Server,Close 时取消。
func (s *Server) register(ctx context.Context) error {
	ttl := s.TTL
	if ttl <= 0 {
		ttl = defaultLeaseTTL
	}
	ttlSec := int64(ttl.Seconds())
	if ttlSec < 1 {
		ttlSec = 1 // etcd 租约按整秒;亚秒 TTL 向上取到 1s,避免 int64(0.x)=0 变成永不过期租约
	}
	gctx, gcancel := context.WithTimeout(ctx, time.Second)
	defer gcancel()
	lease, err := s.Client.Grant(gctx, ttlSec)
	if err != nil {
		return err
	}
	ep := endpoints.Endpoint{Addr: s.addr}
	if s.Weight > 0 {
		ep.Metadata = map[string]any{"weight": s.Weight}
	}
	if err = s.em.AddEndpoint(gctx, s.key, ep, etcdv3.WithLease(lease.ID)); err != nil {
		return errors.Join(err, s.revoke(lease.ID)) // 注册失败回收租约,不悬挂
	}
	kaCh, err := s.Client.KeepAlive(ctx, lease.ID) // etcd 按 ttl/3 自动续租
	if err != nil {
		return errors.Join(err, s.revoke(lease.ID))
	}
	go s.keepAlive(ctx, kaCh)
	return nil
}

// keepAlive 消费续租响应;channel 关闭 = 续租中断(etcd 故障),非 Close 主动取消时退避重注册。
func (s *Server) keepAlive(ctx context.Context, kaCh <-chan *etcdv3.LeaseKeepAliveResponse) {
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-kaCh:
			if ok {
				continue
			}
			s.L.Error(ctx, "gRPC etcd 续租中断,尝试重注册", logger.String("key", s.key))
			for {
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second):
				}
				if err := s.register(ctx); err != nil {
					s.L.Error(ctx, "gRPC etcd 重注册失败,重试", logger.Error(err))
					continue
				}
				return // register 已起新 keepAlive goroutine,本 goroutine 退出
			}
		}
	}
}

// Close 停续租 → 注销端点 → 优雅停 gRPC server,返回首个错误。
// 不关闭 etcd client(由注入方 / ioc cleanup 负责)。
func (s *Server) Close() error {
	if s.kaCancel != nil {
		s.kaCancel()
	}
	var firstErr error
	if s.em != nil && s.key != "" {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		if err := s.em.DeleteEndpoint(ctx, s.key); err != nil {
			firstErr = err
		}
		cancel()
	}
	s.GracefulStop()
	return firstErr
}

// revoke 尽力回收租约,返回回收错误(供 errors.Join 合并)。
func (s *Server) revoke(id etcdv3.LeaseID) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := s.Client.Revoke(ctx, id); err != nil {
		return err
	}
	return nil
}
