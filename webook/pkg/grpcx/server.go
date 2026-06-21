package grpcx

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	etcdv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/naming/endpoints"
	"google.golang.org/grpc"

	"github.com/webook/pkg/logger"
	"github.com/webook/pkg/netx"
)

// defaultLeaseTTL 是未配置 TTL 时的租约默认值(秒)。
const defaultLeaseTTL int64 = 30

// Server 包装 *grpc.Server,叠加 etcd 服务注册/续租/注销。
// etcd client 由外部注入,生命周期不归本 Server。
type Server struct {
	*grpc.Server
	Port   int
	Name   string
	Host   string // 注册到 etcd 的广告 host;空则用 netx.ExternalIp()
	TTL    int64  // 租约 TTL(秒),<=0 用 defaultLeaseTTL
	Weight int    // 注册权重(供带权 balancer 读)
	Client *etcdv3.Client
	L      logger.LoggerX

	key      string // 注册一次定下,重注册复用
	addr     string
	em       endpoints.Manager
	kaCancel func()
}

// ServerConfig 是 gRPC server 的配置。
type ServerConfig struct {
	Port   int    `yaml:"port"`
	Name   string `yaml:"name"`
	Host   string `yaml:"host"`   // 广告 host(k8s 填 POD_IP);空则探测出口 IP
	TTL    int64  `yaml:"ttl"`    // 租约 TTL(秒),<=0 用 defaultLeaseTTL
	Weight int    `yaml:"weight"` // 注册到 etcd 的权重(供带权 balancer 读);<=0 不写,resolver 按 1 计
}

// NewServer 建底层 grpc.Server(option 全由调用方经 opts 传入,如 otel StatsHandler、错误拦截器);
// 调用方在返回值的 .Server 上注册 service。
func NewServer(cfg ServerConfig, client *etcdv3.Client, l logger.LoggerX, opts ...grpc.ServerOption) *Server {
	return &Server{
		Server: grpc.NewServer(opts...),
		Port:   cfg.Port,
		Name:   cfg.Name,
		Host:   cfg.Host,
		TTL:    cfg.TTL,
		Weight: cfg.Weight,
		Client: client,
		L:      l,
	}
}

// Serve 在 Port 上监听并启动 gRPC server,阻塞直到 server 停止。
func (s *Server) Serve() error {
	lis, err := net.Listen("tcp", ":"+strconv.Itoa(s.Port))
	if err != nil {
		return err
	}
	return s.Server.Serve(lis)
}

// Register 向 etcd 注册本实例(key = "service/<Name>/<host:port>")并启动后台续租;
// 续租中断(etcd 抖动 / lease 过期)会自动重注册,直到 Close。
// 排障:etcdctl --endpoints=127.0.0.1:2379 get --prefix service/<Name>
func (s *Server) Register() error {
	if s.Name == "" || s.Port <= 0 {
		return fmt.Errorf("grpcx: 非法 server 配置 name=%q port=%d", s.Name, s.Port)
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
	s.addr = host + ":" + strconv.Itoa(s.Port)
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
	gctx, gcancel := context.WithTimeout(ctx, time.Second)
	defer gcancel()
	lease, err := s.Client.Grant(gctx, ttl)
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
			s.L.Error("gRPC etcd 续租中断,尝试重注册", logger.String("key", s.key))
			for {
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second):
				}
				if err := s.register(ctx); err != nil {
					s.L.Error("gRPC etcd 重注册失败,重试", logger.Error(err))
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
