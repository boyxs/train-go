package swrr

import (
	"context"
	"sync"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"google.golang.org/grpc/metadata"

	"github.com/boyxs/train-go/webook/pkg/grpcx/balancer/group"
	"github.com/boyxs/train-go/webook/pkg/grpcx/balancer/weight"
)

// NameGroup 是按请求 tier 分流的 SWRR 均衡器名:节点按 group 标签分桶,请求经 metadata
// TierMetadataKey 指定目标组,命中组内做 SWRR。典型用途:VIP 请求走 VIP 节点池。
const NameGroup = "group_swrr"

// TierMetadataKey 是请求侧指定目标组的 metadata key。调用方:
// metadata.AppendToOutgoingContext(ctx, TierMetadataKey, "vip")。
const TierMetadataKey = "x-tier"

// defaultTier 是请求未带 tier、或指定组无节点时回落的组;空串对应没打 group 标签的节点。
const defaultTier = ""

func init() {
	balancer.Register(base.NewBalancerBuilder(NameGroup, tierPickerBuilder{}, base.Config{HealthCheck: true}))
}

type tierPickerBuilder struct{}

func (tierPickerBuilder) Build(info base.PickerBuildInfo) balancer.Picker {
	if len(info.ReadySCs) == 0 {
		return base.NewErrPicker(balancer.ErrNoSubConnAvailable)
	}
	// 按 group 标签把就绪节点分桶,各桶预存权重和
	buckets := make(map[string]*tierBucket)
	for sc, sci := range info.ReadySCs {
		g := group.Of(sci.Address)
		bkt := buckets[g]
		if bkt == nil {
			bkt = &tierBucket{}
			buckets[g] = bkt
		}
		w := weight.Of(sci.Address)
		bkt.conns = append(bkt.conns, &conn{sc: sc, weight: w})
		bkt.total += w
	}
	return &tierPicker{buckets: buckets}
}

// tierBucket 是一个组的节点集合 + 权重和。
type tierBucket struct {
	conns []*conn
	total int
}

// tierPicker 按请求 tier 选组,组内 SWRR 选点。
type tierPicker struct {
	mu      sync.Mutex
	buckets map[string]*tierBucket
}

func (p *tierPicker) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	tier := tierFromCtx(info.Ctx)

	p.mu.Lock()
	defer p.mu.Unlock()

	bkt := p.buckets[tier]
	if bkt == nil || len(bkt.conns) == 0 {
		// 指定组无节点:回落默认组,再无则任取一组,保证可用
		// (要严格隔离——VIP 请求宁失败不打普通池——把这里改成返回 ErrNoSubConnAvailable)
		bkt = p.fallback(tier)
	}
	if bkt == nil {
		return balancer.PickResult{}, balancer.ErrNoSubConnAvailable
	}
	best := swrrPick(bkt.conns, bkt.total)
	return balancer.PickResult{SubConn: best.sc}, nil
}

// fallback 在请求指定组无节点时回落:优先默认组,再无则任取一个非空组。
func (p *tierPicker) fallback(tier string) *tierBucket {
	if tier != defaultTier {
		if b := p.buckets[defaultTier]; b != nil && len(b.conns) > 0 {
			return b
		}
	}
	for _, b := range p.buckets {
		if len(b.conns) > 0 {
			return b
		}
	}
	return nil
}

// tierFromCtx 从出站 metadata 读请求指定的目标组,缺省回落 defaultTier。
func tierFromCtx(ctx context.Context) string {
	if ctx == nil {
		return defaultTier
	}
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		return defaultTier
	}
	if vals := md.Get(TierMetadataKey); len(vals) > 0 {
		return vals[0]
	}
	return defaultTier
}
