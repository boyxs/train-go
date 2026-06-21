package registry

// 本文件改编自官方 go.etcd.io/etcd/client/v3/naming/resolver(Apache-2.0)。
// 与官方逐字一致(仅类型名 resolver → etcdResolver,因本包已 import 官方 resolver 包,
// Go 不允许包级标识符与文件级 import 同名),唯一逻辑增量(均以 [增量] 标注):
// watch 下发时额外填充带权 Addresses。
// 原因:官方只填 State.Endpoints 且 convertToGRPCEndpoint 丢弃 Metadata,base 系均衡器
// (只读 State.Addresses,如 custom_swrr)因此拿不到地址、也读不到权重。Endpoints 原样保留,

import (
	"context"
	"strings"
	"sync"

	"google.golang.org/grpc/codes"
	gresolver "google.golang.org/grpc/resolver"
	"google.golang.org/grpc/status"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/naming/endpoints"

	"webook/sandbox/grpc/balancer/balancer/swrr"
)

type builder struct {
	c *clientv3.Client
}

func (b builder) Build(target gresolver.Target, cc gresolver.ClientConn, opts gresolver.BuildOptions) (gresolver.Resolver, error) {
	endpoint := target.URL.Path
	if endpoint == "" {
		endpoint = target.URL.Opaque
	}
	endpoint = strings.TrimPrefix(endpoint, "/")
	r := &etcdResolver{
		c:      b.c,
		target: endpoint,
		cc:     cc,
	}
	r.ctx, r.cancel = context.WithCancel(context.Background())

	em, err := endpoints.NewManager(r.c, r.target)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "resolver: failed to new endpoint manager: %s", err)
	}
	r.wch, err = em.NewWatchChannel(r.ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "resolver: failed to new watch channer: %s", err)
	}

	r.wg.Add(1)
	go r.watch()
	return r, nil
}

func (b builder) Scheme() string {
	return "etcd"
}

// NewEtcdResolverBuilder creates a resolver builder.
func NewEtcdResolverBuilder(client *clientv3.Client) gresolver.Builder {
	return builder{c: client}
}

type etcdResolver struct {
	c      *clientv3.Client
	target string
	cc     gresolver.ClientConn
	wch    endpoints.WatchChannel
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func (r *etcdResolver) watch() {
	defer r.wg.Done()

	allUps := make(map[string]*endpoints.Update)
	for {
		select {
		case <-r.ctx.Done():
			return
		case ups, ok := <-r.wch:
			if !ok {
				return
			}

			for _, up := range ups {
				switch up.Op {
				case endpoints.Add:
					allUps[up.Key] = up
				case endpoints.Delete:
					delete(allUps, up.Key)
				}
			}

			eps := convertToGRPCEndpoint(allUps)
			// [增量] 额外下发带权 Addresses;官方此处仅 State{Endpoints: eps}。
			r.cc.UpdateState(gresolver.State{Addresses: convertToWeightedAddresses(allUps), Endpoints: eps})
		}
	}
}

func convertToGRPCEndpoint(ups map[string]*endpoints.Update) []gresolver.Endpoint {
	var eps []gresolver.Endpoint
	for _, up := range ups {
		ep := gresolver.Endpoint{
			Addresses: []gresolver.Address{
				{
					Addr: up.Endpoint.Addr,
				},
			},
		}
		eps = append(eps, ep)
	}
	return eps
}

// [增量] convertToWeightedAddresses 把端点转成带权地址:权重从 etcd metadata 取,
// 经 swrr.SetWeight 打进 Address.Attributes,供 custom_swrr 读取。
func convertToWeightedAddresses(ups map[string]*endpoints.Update) []gresolver.Address {
	var addrs []gresolver.Address
	for _, up := range ups {
		addrs = append(addrs, swrr.SetWeight(gresolver.Address{Addr: up.Endpoint.Addr}, weightOf(up.Endpoint.Metadata)))
	}
	return addrs
}

// [增量] weightOf 从端点 metadata 取权重;etcd 存 JSON,数字回读是 float64;缺失 / 非正按 1 计。
func weightOf(meta any) int {
	m, ok := meta.(map[string]any)
	if !ok {
		return 1
	}
	switch w := m["weight"].(type) {
	case float64:
		if w > 0 {
			return int(w)
		}
	case int:
		if w > 0 {
			return w
		}
	}
	return 1
}

// ResolveNow is a no-op here.
// It's just a hint, resolver can ignore this if it's not necessary.
func (r *etcdResolver) ResolveNow(gresolver.ResolveNowOptions) {}

func (r *etcdResolver) Close() {
	r.cancel()
	r.wg.Wait()
}
