// Package swrr 是一个 gRPC 自定义负载均衡器:平滑加权轮询(Smooth Weighted Round Robin)。
package swrr

import (
	"sync"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"google.golang.org/grpc/resolver"
)

const name = "custom_swrr"

func init() {
	balancer.Register(newBalancerBuilder())
}

func newBalancerBuilder() balancer.Builder {
	return base.NewBalancerBuilder(
		name,
		&PickerBuilder{},
		base.Config{HealthCheck: true},
	)
}

// ── 权重:通过 resolver.Address 属性携带 ───────────────────

type weightKey struct{}

func SetWeight(addr resolver.Address, weight int) resolver.Address {
	addr.Attributes = addr.Attributes.WithValue(weightKey{}, weight)
	return addr
}

func getWeight(addr resolver.Address) int {
	if w, ok := addr.Attributes.Value(weightKey{}).(int); ok && w > 0 {
		return w
	}
	return 1
}

// ── PickerBuilder ───────────────────

type PickerBuilder struct{}

func (p PickerBuilder) Build(info base.PickerBuildInfo) balancer.Picker {
	if len(info.ReadySCs) == 0 {
		return base.NewErrPicker(balancer.ErrNoSubConnAvailable)
	}
	subConns := make([]*SubConn, 0, len(info.ReadySCs))
	total := 0
	for sc, scInfo := range info.ReadySCs {
		w := getWeight(scInfo.Address)
		total += w
		subConns = append(subConns, &SubConn{sc: sc, weight: w})
	}
	return &Picker{subConns: subConns, total: total}
}

// ── Picker ───────────────────

type SubConn struct {
	sc        balancer.SubConn
	weight    int
	curWeight int
	available bool
}

type Picker struct {
	subConns []*SubConn
	total    int // Build 时算好,生命周期内不变
	mu       sync.Mutex
}

// Pick 口诀:加权重 → 挑最大 → 减总和。
func (p *Picker) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	var maxSc *SubConn
	for _, sc := range p.subConns {
		sc.curWeight += sc.weight
		if maxSc == nil || sc.curWeight > maxSc.curWeight {
			maxSc = sc
		}
	}
	maxSc.curWeight -= p.total

	return balancer.PickResult{SubConn: maxSc.sc, Done: func(info balancer.DoneInfo) {
		//TODO
	}}, nil
}
