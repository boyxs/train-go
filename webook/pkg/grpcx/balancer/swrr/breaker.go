package swrr

import (
	"sync"
	"time"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/webook/pkg/grpcx/balancer/weight"
)

// NameBreaker 是带熔断的 SWRR 均衡器名(circuit breaker):在 custom_swrr 基础上,RPC 失败经 Done
// 累计,连续失败达阈值即把节点摘出可用组,摘除后过冷却期半开放行一次探活,探活成功则恢复。
// 与 base 的 HealthCheck(连接级 gRPC health 协议)互补——那个管连接是否健康,这个管调用结果。
const NameBreaker = "breaker_swrr"

const (
	failThreshold = 3               // 连续失败多少次摘除节点
	coolDown      = 5 * time.Second // 摘除后冷却多久放行一次半开探活
)

func init() {
	balancer.Register(base.NewBalancerBuilder(NameBreaker, breakerPickerBuilder{}, base.Config{HealthCheck: true}))
}

type breakerPickerBuilder struct{}

func (breakerPickerBuilder) Build(info base.PickerBuildInfo) balancer.Picker {
	if len(info.ReadySCs) == 0 {
		return base.NewErrPicker(balancer.ErrNoSubConnAvailable)
	}
	conns := make([]*conn, 0, len(info.ReadySCs))
	for sc, sci := range info.ReadySCs {
		conns = append(conns, &conn{sc: sc, weight: weight.Of(sci.Address), available: true})
	}
	return &breakerPicker{conns: conns}
}

// breakerPicker 的熔断状态(available/fails/downAt)挂在 picker 上,而 gRPC 在任一连接状态变化时
// 会重建 picker → 状态重置回全可用。轻量场景够用;要跨重建持久,需把这组状态提到 builder 层按
// address 存。
type breakerPicker struct {
	mu    sync.Mutex
	conns []*conn
}

func (p *breakerPicker) Pick(balancer.PickInfo) (balancer.PickResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	live, total := p.liveConns()
	best := swrrPick(live, total)
	if !best.available {
		best.downAt = nowMs() // 半开:本次已放行探活,刷新冷却,探活未归前不再反复放行
	}

	return balancer.PickResult{
		SubConn: best.sc,
		Done: func(di balancer.DoneInfo) {
			p.mu.Lock()
			defer p.mu.Unlock()
			if isNodeFailure(di.Err) {
				best.fails++
				if best.fails >= failThreshold {
					best.available = false
					best.downAt = nowMs()
				}
				return
			}
			if !best.available {
				best.curWeight = 0 // 半开探活成功,清当前权重干净回归
			}
			best.available = true
			best.fails = 0
		},
	}, nil
}

// liveConns 选出本轮参选节点:available 的,加上熔断后冷却到点可半开探活的;若全部熔断且都还在
// 冷却期(live 为空),fail-open 放行全部,避免整体不可用。
func (p *breakerPicker) liveConns() (live []*conn, total int) {
	now := nowMs()
	live = make([]*conn, 0, len(p.conns))
	for _, c := range p.conns {
		if c.available || now-c.downAt >= coolDown.Milliseconds() {
			live = append(live, c)
			total += c.weight
		}
	}
	if len(live) == 0 {
		for _, c := range p.conns {
			total += c.weight
		}
		return p.conns, total
	}
	return live, total
}

// isNodeFailure 只把节点级错误算作熔断信号:不可用 / 超时 / 过载;业务错误(NotFound、
// InvalidArgument 等)不代表节点不健康,不计入。
func isNodeFailure(err error) bool {
	switch status.Code(err) {
	case codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted:
		return true
	default:
		return false
	}
}

func nowMs() int64 {
	return time.Now().UnixMilli()
}
