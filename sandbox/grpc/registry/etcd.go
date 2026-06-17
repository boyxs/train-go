package registry

import (
	"context"
	"errors"
	"sync"
	"time"

	etcdv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/naming/endpoints"
)

// EtcdRegistry 是基于 etcd 的 Registry 实现:租约 + 自动续租 + 注销,
// 支持多服务(按 ServiceInstance.Name 区分命名空间)。
type EtcdRegistry struct {
	client *etcdv3.Client
	ttl    int64 // 租约 TTL(秒)

	mu       sync.Mutex
	managers map[string]endpoints.Manager // 服务名 -> endpoints.Manager(复用)
	entries  map[string]*regEntry         // endpoint key -> 注册项
}

// regEntry 记录单个已注册端点:所属 manager + 续租取消函数。
type regEntry struct {
	em     endpoints.Manager
	cancel context.CancelFunc
}

// Option 配置 EtcdRegistry。
type Option func(*EtcdRegistry)

// WithTTL 设置租约 TTL(秒),默认 5。
func WithTTL(seconds int64) Option {
	return func(r *EtcdRegistry) { r.ttl = seconds }
}

// NewEtcdRegistry 构造 etcd 注册器。client 由调用方拥有,Close 不关它。
func NewEtcdRegistry(client *etcdv3.Client, opts ...Option) Registry {
	r := &EtcdRegistry{
		client:   client,
		ttl:      5,
		managers: make(map[string]endpoints.Manager),
		entries:  make(map[string]*regEntry),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

func (r *EtcdRegistry) Register(ctx context.Context, ins ServiceInstance) error {
	em, err := r.manager(ins.Name)
	if err != nil {
		return err
	}
	lease, err := r.client.Grant(ctx, r.ttl)
	if err != nil {
		return err
	}
	key := r.key(ins)
	ep := endpoints.Endpoint{Addr: ins.Addr}
	if ins.Weight > 0 || len(ins.Metadata) > 0 {
		// weight / metadata 随端点写入,供发现侧(加权 balancer 等)读取。
		ep.Metadata = map[string]any{"weight": ins.Weight, "meta": ins.Metadata}
	}
	if err = em.AddEndpoint(ctx, key, ep, etcdv3.WithLease(lease.ID)); err != nil {
		return errors.Join(err, r.revoke(lease.ID)) // 注册失败回收租约,不悬挂
	}

	// 续租用独立 ctx,生命周期与本注册项绑定,Deregister / Close 时取消。
	kaCtx, cancel := context.WithCancel(context.Background())
	ka, err := r.client.KeepAlive(kaCtx, lease.ID)
	if err != nil {
		cancel()
		return errors.Join(err, r.revoke(lease.ID))
	}
	go func() {
		for range ka { // 持续消费续租响应,直到 cancel 关闭 channel
		}
	}()

	r.mu.Lock()
	if old, ok := r.entries[key]; ok {
		old.cancel() // 覆盖注册:停掉旧续租
	}
	r.entries[key] = &regEntry{em: em, cancel: cancel}
	r.mu.Unlock()
	return nil
}

func (r *EtcdRegistry) Deregister(ctx context.Context, ins ServiceInstance) error {
	em, err := r.manager(ins.Name)
	if err != nil {
		return err
	}
	key := r.key(ins)
	r.mu.Lock()
	if entry, ok := r.entries[key]; ok {
		entry.cancel() // 停该实例续租
		delete(r.entries, key)
	}
	r.mu.Unlock()
	return em.DeleteEndpoint(ctx, key)
}

// Close 停止所有续租并注销所有端点,返回注销过程中的首个错误。
func (r *EtcdRegistry) Close() error {
	r.mu.Lock()
	entries := r.entries
	r.entries = make(map[string]*regEntry)
	r.managers = make(map[string]endpoints.Manager)
	r.mu.Unlock()

	for _, e := range entries {
		e.cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var firstErr error
	for key, e := range entries {
		if err := e.em.DeleteEndpoint(ctx, key); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// manager 返回服务名对应的 endpoints.Manager,按需创建并缓存。
func (r *EtcdRegistry) manager(name string) (endpoints.Manager, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if em, ok := r.managers[name]; ok {
		return em, nil
	}
	em, err := endpoints.NewManager(r.client, "service/"+name)
	if err != nil {
		return nil, err
	}
	r.managers[name] = em
	return em, nil
}

// revoke 尽力回收租约,返回回收过程中的错误(供 errors.Join 合并)。
func (r *EtcdRegistry) revoke(id etcdv3.LeaseID) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := r.client.Revoke(ctx, id); err != nil {
		return err
	}
	return nil
}

func (r *EtcdRegistry) key(ins ServiceInstance) string {
	return "service/" + ins.Name + "/" + ins.Addr
}
