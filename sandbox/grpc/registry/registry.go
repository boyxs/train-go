package registry

import (
	"context"
	"sync"

	etcdv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/naming/endpoints"
)

// ServiceRegistry 基于 etcd 的服务注册器：封装租约、自动续租与注销，
// 调用方只管 Register / Deregister / Close，不直接接触 etcd 原语。
//
// service 即命名空间（如 "service/user"），注册的端点写在
// "<service>/<addr>" 下，与 etcd resolver 的 "etcd:///<service>" 对应。
type ServiceRegistry struct {
	cli     *etcdv3.Client
	em      endpoints.Manager
	service string
	ttl     int64 // 租约 TTL（秒）

	mu      sync.Mutex
	cancels []context.CancelFunc // 各端点的续租取消函数
	keys    []string             // 已注册端点的 key，Close 时统一注销
}

func NewServiceRegistry(cli *etcdv3.Client, service string, ttlSeconds int64) (*ServiceRegistry, error) {
	em, err := endpoints.NewManager(cli, service)
	if err != nil {
		return nil, err
	}
	return &ServiceRegistry{cli: cli, em: em, service: service, ttl: ttlSeconds}, nil
}

// Register 注册一个实例地址：申请租约、写入端点，并启动后台自动续租。
// 续租在 Close 时停止，不会泄漏 goroutine。
func (r *ServiceRegistry) Register(ctx context.Context, addr string) error {
	lease, err := r.cli.Grant(ctx, r.ttl)
	if err != nil {
		return err
	}
	key := r.key(addr)
	if err = r.em.AddEndpoint(ctx, key, endpoints.Endpoint{Addr: addr}, etcdv3.WithLease(lease.ID)); err != nil {
		return err
	}

	// 续租用独立 ctx，生命周期与本注册器绑定，Close 时取消。
	kaCtx, cancel := context.WithCancel(context.Background())
	ka, err := r.cli.KeepAlive(kaCtx, lease.ID)
	if err != nil {
		cancel()
		return err
	}
	go func() {
		for range ka { // 持续消费续租响应，直到 kaCtx 取消使 channel 关闭
		}
	}()

	r.mu.Lock()
	r.cancels = append(r.cancels, cancel)
	r.keys = append(r.keys, key)
	r.mu.Unlock()
	return nil
}

// Deregister 主动注销指定地址，从 etcd 删除其端点。
func (r *ServiceRegistry) Deregister(ctx context.Context, addr string) error {
	return r.em.DeleteEndpoint(ctx, r.key(addr))
}

// Close 停止所有续租并注销所有已注册端点。返回注销过程中的首个错误。
func (r *ServiceRegistry) Close(ctx context.Context) error {
	r.mu.Lock()
	cancels, keys := r.cancels, r.keys
	r.cancels, r.keys = nil, nil
	r.mu.Unlock()

	for _, cancel := range cancels {
		cancel()
	}
	var firstErr error
	for _, key := range keys {
		if err := r.em.DeleteEndpoint(ctx, key); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (r *ServiceRegistry) key(addr string) string {
	return r.service + "/" + addr
}
