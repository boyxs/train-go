// Package swrr 是一组基于平滑加权轮询(SWRR)的 gRPC 自定义负载均衡器:
//   - custom_swrr : 基础 SWRR,按权重平滑分流(Name,本文件)
//   - breaker_swrr : 在 SWRR 上加错误熔断(NameBreaker,见 breaker.go)
//   - group_swrr  : 按请求 tier 分流到节点组,组内 SWRR(NameGroup,见 tier.go)
//
// 三者共用 conn 与 swrrPick;权重 / 分组标签由 resolver 经 weight / group 包打在 Address 上。
//
// 用法(以 custom_swrr 为例):匿名 import 触发注册 + service config 选中:
//
//	import _ "github.com/webook/pkg/grpcx/balancer/swrr"
//	grpc.WithDefaultServiceConfig(`{"loadBalancingConfig":[{"custom_swrr":{}}]}`)
package swrr

import (
	"sync"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"

	"github.com/webook/pkg/grpcx/balancer/weight"
)

// Name 是基础 SWRR 均衡器注册到 gRPC 的名字,service config 里用它选中。
const Name = "custom_swrr"

func init() {
	balancer.Register(base.NewBalancerBuilder(Name, pickerBuilder{}, base.Config{HealthCheck: true}))
}

// conn 是参与 SWRR 的就绪后端。weight/curWeight 三个变体都用;available/fails/downAt 仅
// breaker_swrr(breaker.go)用。
type conn struct {
	sc        balancer.SubConn
	weight    int
	curWeight int
	available bool
	fails     int
	downAt    int64
}

// swrrPick 在给定子集上做一次平滑加权轮询选点:加权重 → 挑最大 → 减总和。total 为子集权重和。
func swrrPick(conns []*conn, total int) *conn {
	var best *conn
	for _, c := range conns {
		c.curWeight += c.weight
		if best == nil || c.curWeight > best.curWeight {
			best = c
		}
	}
	best.curWeight -= total
	return best
}

// pickerBuilder 构造基础 SWRR picker:连接状态变化时 gRPC 调一次 Build 重建。
type pickerBuilder struct{}

func (pickerBuilder) Build(info base.PickerBuildInfo) balancer.Picker {
	if len(info.ReadySCs) == 0 {
		return base.NewErrPicker(balancer.ErrNoSubConnAvailable)
	}
	conns := make([]*conn, 0, len(info.ReadySCs))
	total := 0
	for sc, sci := range info.ReadySCs {
		w := weight.Of(sci.Address)
		total += w
		conns = append(conns, &conn{sc: sc, weight: w})
	}
	return &picker{conns: conns, total: total}
}

// picker 持有一轮就绪节点,按 SWRR 选点。
type picker struct {
	mu    sync.Mutex
	conns []*conn
	total int
}

func (p *picker) Pick(balancer.PickInfo) (balancer.PickResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	best := swrrPick(p.conns, p.total)
	return balancer.PickResult{SubConn: best.sc}, nil
}
