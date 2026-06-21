package etcd

// 本文件改编自官方 go.etcd.io/etcd/client/v3/naming/resolver(Apache-2.0)。
// 与官方逐字一致,唯一逻辑增量(均以 [增量] 标注):watch 下发时额外填充带权 Addresses。
// 原因:官方只填 State.Endpoints 且 convertToGRPCEndpoint 丢弃 Metadata,base 系均衡器
// (只读 State.Addresses,如 custom_swrr)因此拿不到地址、也读不到权重。Endpoints 原样保留,

import (
	"context"
	"strings"
	"sync"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/naming/endpoints"
	"google.golang.org/grpc/codes"
	gresolver "google.golang.org/grpc/resolver"
	"google.golang.org/grpc/status"

	"github.com/webook/pkg/grpcx/balancer/group"
	"github.com/webook/pkg/grpcx/balancer/weight"
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
	r := &resolver{
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

// NewBuilder creates a resolver builder.
func NewBuilder(client *clientv3.Client) (gresolver.Builder, error) {
	return builder{c: client}, nil
}

type resolver struct {
	c      *clientv3.Client
	target string
	cc     gresolver.ClientConn
	wch    endpoints.WatchChannel
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func (r *resolver) watch() {
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
			// [增量] 额外下发带权 + 带组 Addresses;官方此处仅 State{Endpoints: eps}。
			r.cc.UpdateState(gresolver.State{Addresses: convertToAddresses(allUps), Endpoints: eps})
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

// [增量] convertToAddresses 把端点转成 LB 地址:从 etcd metadata 取权重和分组,经 weight.Set /
// group.Set 打进 Address.Attributes,供 base 系均衡器(custom_swrr / group_swrr)读取。
func convertToAddresses(ups map[string]*endpoints.Update) []gresolver.Address {
	var addrs []gresolver.Address
	for _, up := range ups {
		addr := gresolver.Address{Addr: up.Endpoint.Addr}
		addr = weight.Set(addr, weightOf(up.Endpoint.Metadata))
		addr = group.Set(addr, groupOf(up.Endpoint.Metadata))
		addrs = append(addrs, addr)
	}
	return addrs
}

// [增量] groupOf 从端点 metadata 取分组标签(如 "vip");缺失按空串(默认组)。
func groupOf(meta any) string {
	if m, ok := meta.(map[string]any); ok {
		if g, ok := m["group"].(string); ok {
			return g
		}
	}
	return ""
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
func (r *resolver) ResolveNow(gresolver.ResolveNowOptions) {}

func (r *resolver) Close() {
	r.cancel()
	r.wg.Wait()
}
