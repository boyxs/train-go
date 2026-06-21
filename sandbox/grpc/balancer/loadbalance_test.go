package balancer

import (
	"fmt"
	"math/rand"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 客户端负载均衡 4 种策略的最小实现 + 共享测试台。
// 统一在 Strategy 接口下,runStrategy 跑 N 次,统计各节点「命中数 / 占比 / 最大连续命中」。
// 断言:命中占比 ≈ 期望分布;Smooth WRR 额外断言最大连续命中 <= 2(平滑性)。

// ── 模型与接口 ────────────────────────────────────────────

// Node 是候选后端。Weight 是配置权重;CurrWeight 仅 SmoothWRR 用作动态当前权重。
type Node struct {
	Index      int
	Name       string
	Weight     int
	CurrWeight int
}

// String 让日志里 %v/%s 直接打出可读状态(如 A(w=10,cur=-50)),而非指针地址。
func (n *Node) String() string {
	return fmt.Sprintf("%s(w=%d,cur=%d)", n.Name, n.Weight, n.CurrWeight)
}

// Strategy 选点策略:pick 返回本次选中的节点。
type Strategy interface {
	pick() *Node
}

// newNodes 每次返回一组全新节点(CurrWeight 从 0 起,符合 nginx SWRR 标准)。
// 每个用例独立一份,避免 SmoothWRR 改动 CurrWeight 污染别的用例。
func newNodes() []*Node {
	return []*Node{
		{Index: 0, Name: "A", Weight: 10},
		{Index: 1, Name: "B", Weight: 40},
		{Index: 2, Name: "C", Weight: 50},
	}
}

// ── 策略实现 ──────────────────────────────────────────────

// RoundRobin 轮询。
// 原理:维护自增游标,第 k 次请求落到 nodes[k mod n],循环 0,1,2,0,1,2…。
// 特点:无视权重,绝对均匀;原子自增、无锁并发安全;结果确定无随机。
// 适用:节点同构、处理能力相近;不适用异构节点或请求耗时差异大的场景。
// 复杂度:每次 pick O(1)。
type RoundRobin struct {
	index *atomic.Uint32
	nodes []*Node
}

func (r *RoundRobin) pick() *Node {
	// Add 先自增再返回新值,减 1 让首次命中 nodes[0];用 uint32 取模,
	// 即便计数器回绕(2^32 次后)也保持非负,不会出现负下标 panic。
	idx := (r.index.Add(1) - 1) % uint32(len(r.nodes))
	return r.nodes[idx]
}

// SmoothWRR 平滑加权轮询(nginx 经典算法)。
// 原理:每轮 ① 每节点 cur+=weight ② 选 cur 最大者命中 ③ 命中者 cur-=总权重。
// 特点:命中比例严格=权重比例,且高权重节点被散开(无 C,C,C 突发),无随机。
// 适用:权重稳定的平滑分摊;不适用动态负载/长尾(那用最少连接)。
// 复杂度:每次 pick O(n),需加锁(cur 是共享可变状态)。
type SmoothWRR struct {
	mu    sync.Mutex
	nodes []*Node
	logf  func(string, ...any) // 可选:非 nil 时打印每步权重演化(测试传入 t.Logf)
}

func (s *SmoothWRR) pick() *Node {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 计算总权重,并给每个节点的当前权重加上自身配置权重
	total := 0
	for _, n := range s.nodes {
		total += n.Weight
		n.CurrWeight += n.Weight
	}
	// 选当前权重最大的节点;严格大于 → 平局取靠前
	target := s.nodes[0]
	for _, n := range s.nodes {
		if n.CurrWeight > target.CurrWeight {
			target = n
		}
	}
	s.trace("加权后 %v → 选中 %s", s.nodes, target.Name)
	// 命中者减去总权重,"退回"低位,避免高权重节点连续霸占(平滑的关键)
	target.CurrWeight -= total
	s.trace("重置后 %v", s.nodes)
	return target
}

func (s *SmoothWRR) trace(format string, args ...any) {
	if s.logf != nil {
		s.logf(format, args...)
	}
}

// Random 等权随机。
// 原理:每次在 [0,n) 上均匀随机取一个下标。
// 特点:无跨请求状态;量大趋于均匀,但短期会扎堆。持有独立 *rand.Rand,非并发安全(并发需加锁)。
// 适用:节点同构、实现求极简、不在意短期抖动。
// 复杂度:每次 pick O(1)。
type Random struct {
	rng   *rand.Rand
	nodes []*Node
}

func (r *Random) pick() *Node {
	return r.nodes[r.rng.Intn(len(r.nodes))]
}

// WeightedRandom 加权随机。
// 原理:在 [0,总权重) 上取随机落点,按累计权重区间定位命中(权重大的区间宽、概率高)。
// 特点:期望命中比例=权重比例,但单次随机、会连续命中(不平滑);无跨请求状态。
// 适用:要按权重分摊但不要求平滑;比 SmoothWRR 实现简单。
// 复杂度:每次 pick O(n) 线性扫区间(预累加 + 二分可降到 O(log n))。
type WeightedRandom struct {
	rng   *rand.Rand
	nodes []*Node
}

func (w *WeightedRandom) pick() *Node {
	total := 0
	for _, n := range w.nodes {
		total += n.Weight
	}
	hit := w.rng.Intn(total)
	for _, n := range w.nodes {
		if hit -= n.Weight; hit < 0 {
			return n
		}
	}
	return w.nodes[len(w.nodes)-1] // 边界兜底,理论不可达
}

// ── 测试台 ────────────────────────────────────────────────

// runStrategy 调用 strat.pick() count 次,按节点 Index 统计命中数与最大连续命中长度。
func runStrategy(strat Strategy, nodeNum, count int) (counts, maxRun []int) {
	counts = make([]int, nodeNum)
	maxRun = make([]int, nodeNum)
	var prev *Node
	run := 0
	for i := 0; i < count; i++ {
		target := strat.pick()
		counts[target.Index]++
		if target == prev {
			run++
		} else {
			run = 1
		}
		if run > maxRun[target.Index] {
			maxRun[target.Index] = run
		}
		prev = target
	}
	return counts, maxRun
}

// ── 表驱动测试 ────────────────────────────────────────────

// TestStrategies 表驱动跑 4 种策略,断言命中分布。
// 验证:等权策略(RR/Random)期望均分,加权策略(SWRR/WeightedRandom)期望=权重比;SWRR 额外验证平滑。
// 期望值由来:count=9000 同时是 RR 周期 3 与 SWRR 周期 10 的倍数 → 确定性策略命中数可精确断言;
// 随机策略固定 seed 可复现,用 15% 容差表达"近似"。
func TestStrategies(t *testing.T) {
	const count = 9000

	testCases := []struct {
		name    string
		build   func(nodes []*Node, rng *rand.Rand) Strategy
		weights []int // 期望命中权重(等权策略填等值);命中占比应 ≈ 各值占比
		exact   bool  // true=确定性策略,命中数精确;false=随机策略,15% 容差
		smooth  bool  // true=额外断言最大连续命中 <= 2
	}{
		{
			name: "RoundRobin",
			build: func(nodes []*Node, _ *rand.Rand) Strategy {
				return &RoundRobin{nodes: nodes, index: &atomic.Uint32{}}
			},
			weights: []int{1, 1, 1}, // 无视权重,等概率
			exact:   true,
		},
		{
			name: "SmoothWRR",
			build: func(nodes []*Node, _ *rand.Rand) Strategy {
				return &SmoothWRR{nodes: nodes}
			},
			weights: []int{10, 40, 50},
			exact:   true,
			smooth:  true,
		},
		{
			name: "Random",
			build: func(nodes []*Node, rng *rand.Rand) Strategy {
				return &Random{rng: rng, nodes: nodes}
			},
			weights: []int{1, 1, 1},
		},
		{
			name: "WeightedRandom",
			build: func(nodes []*Node, rng *rand.Rand) Strategy {
				return &WeightedRandom{rng: rng, nodes: nodes}
			},
			weights: []int{10, 40, 50},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			nodes := newNodes()
			// 固定 seed:随机策略结果可复现,测试不抖动。
			strat := tc.build(nodes, rand.New(rand.NewSource(42)))
			counts, maxRun := runStrategy(strat, len(nodes), count)

			wTotal := 0
			for _, w := range tc.weights {
				wTotal += w
			}
			for _, n := range nodes {
				got := counts[n.Index]
				w := tc.weights[n.Index]
				wantF := float64(count) * float64(w) / float64(wTotal)
				t.Logf("%s 命中 %d 次,占比 %.2f%%,最大连续命中 %d",
					n.Name, got, float64(got)/float64(count)*100, maxRun[n.Index])
				if tc.exact {
					assert.Equal(t, count*w/wTotal, got, "%s 确定性策略命中数应精确", n.Name)
				} else {
					assert.InDelta(t, wantF, float64(got), wantF*0.15, "%s 命中占比应 ≈ 权重占比", n.Name)
				}
			}
			if tc.smooth {
				// 平滑性:任意节点都不会连续命中超过 2 次(对比加权随机可连击 5+)。
				assert.LessOrEqual(t, slices.Max(maxRun), 2, "SmoothWRR 应平滑,无长连击")
			}
		})
	}
}

// TestSmoothWRR_Sequence 验证 SmoothWRR 的平滑性,并开 logf 打印每步权重演化。
// 验证:跑满一个完整周期 + 边界,高权重节点 C(50%)最大连续命中不超过 2。
// 为何 ≤2:权重 10/40/50 总和 100、gcd 10 → 周期 10 次,单周期序列 C,B,C,B,A,C,B,C,B,C 无相邻重复;
// 仅周期交界处(…C | C…)出现一次 C,C,故跨周期最大连击=2。
func TestSmoothWRR_Sequence(t *testing.T) {
	const count = 12 // 周期 10 + 跨边界多跑 2 次
	s := &SmoothWRR{nodes: newNodes(), logf: t.Logf}
	seq := make([]string, 0, count)
	for i := 0; i < count; i++ {
		t.Logf("— 第 %d 次选点 —", i+1)
		seq = append(seq, s.pick().Name)
	}
	t.Logf("SmoothWRR 前 %d 次选点:%v", count, seq)

	maxC, run := 0, 0
	for _, name := range seq {
		if name == "C" {
			run++
		} else {
			run = 0
		}
		maxC = max(maxC, run)
	}
	require.LessOrEqual(t, maxC, 2, "高权重节点 C 不应连续命中超过 2 次")
}
