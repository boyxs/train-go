package balancer

import (
	"math/rand"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 负载感知型策略:LeastConnection 与 P2C。
// 二者都按「在途请求数」选点,故须在请求结束时 done() 归还计数(普通轮询/随机不需要)。

// ── 最少连接 ──────────────────────────────────────────────

// LeastConnection 最少连接。
// 原理:每节点维护在途数 inflight;pick 选 inflight 最小者并 +1,请求结束 done() 时 -1。
// 特点:负载感知——慢节点 inflight 高会被自动跳过,比轮询更抗长尾;须配对 done(),否则计数泄漏。
// 适用:请求耗时差异大、后端能力不一;不适用极短请求(维护计数的开销占比偏高)。
// 复杂度:pick O(n)、done O(1),均需加锁(inflight 是共享可变状态)。
type LeastConnection struct {
	mu       sync.Mutex
	nodes    []*Node
	inflight []int
	logf     func(string, ...any) // 可选:非 nil 时打印每次选点/释放的在途快照
}

func NewLeastConnection(nodes []*Node) *LeastConnection {
	return &LeastConnection{nodes: nodes, inflight: make([]int, len(nodes))}
}

func (l *LeastConnection) pick() *Node {
	l.mu.Lock()
	defer l.mu.Unlock()

	// 扫描在途计数,选最少的那个(严格小于 → 平局取靠前)
	target := l.nodes[0]
	for _, n := range l.nodes {
		if l.inflight[n.Index] < l.inflight[target.Index] {
			target = n
		}
	}
	l.trace("在途 %v → 选最少连接 %s", l.inflight, target.Name)
	l.inflight[target.Index]++ // 占用一个连接,done() 时归还
	return target
}

func (l *LeastConnection) done(n *Node) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.inflight[n.Index]--
	l.trace("释放 %s,在途 %v", n.Name, l.inflight)
}

func (l *LeastConnection) trace(format string, args ...any) {
	if l.logf != nil {
		l.logf(format, args...)
	}
}

// ── P2C 二选一 ────────────────────────────────────────────

// P2C(Power of Two Choices)二选一。
// 原理:随机取 2 个不同节点,选其中在途更少者;请求结束 done() 归还。
// 特点:只比 2 个,省掉"全局最少连接"的全量扫描/全局热点,负载偏差却接近最优(理论 O(loglog n));
// 现代服务网格(Envoy/gRPC/Finagle)默认。
// 适用:大规模节点池 + 高并发,既要均衡又怕全局锁;节点极少时优势不明显。
// 复杂度:每次 pick O(1)(固定比 2 个),需加锁。
type P2C struct {
	rng      *rand.Rand
	mu       sync.Mutex
	nodes    []*Node
	inflight []int
	logf     func(string, ...any) // 可选:非 nil 时打印每次"二选一"的候选与结果
}

func NewP2C(nodes []*Node, rng *rand.Rand) *P2C {
	return &P2C{rng: rng, nodes: nodes, inflight: make([]int, len(nodes))}
}

func (p *P2C) pick() *Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 只有 1 个节点时无从"二选一",直接返回(否则下面 Intn(0) 会 panic)。
	if len(p.nodes) == 1 {
		p.inflight[p.nodes[0].Index]++
		return p.nodes[0]
	}
	// 取两个不同下标:j 从 n-1 个候选里抽,遇到 >= i 就 +1 跳过 i 自身,保证 i != j。
	i := p.rng.Intn(len(p.nodes))
	j := p.rng.Intn(len(p.nodes) - 1)
	if j >= i {
		j++
	}
	// 两个候选里取在途更少的;平局取靠前(a)。
	a, b := p.nodes[i], p.nodes[j]
	target := a
	if p.inflight[b.Index] < p.inflight[a.Index] {
		target = b
	}
	p.trace("二选一 %s(%d)/%s(%d) → %s",
		a.Name, p.inflight[a.Index], b.Name, p.inflight[b.Index], target.Name)
	p.inflight[target.Index]++
	return target
}

func (p *P2C) done(n *Node) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.inflight[n.Index]--
}

func (p *P2C) trace(format string, args ...any) {
	if p.logf != nil {
		p.logf(format, args...)
	}
}

// ── 测试 ──────────────────────────────────────────────────

// TestLeastConnection_AllOpenIsEven 验证"不释放连接"时最少连接退化为均匀轮询。
// 验证:9000 次 pick 全程不 done,各节点命中数严格=count/3,且无连续命中。
// 为何均匀:从不归还 → inflight 单调递增,每次最小者恰好轮转(A→B→C→A…),等价于轮询。
func TestLeastConnection_AllOpenIsEven(t *testing.T) {
	const count = 9000
	lb := NewLeastConnection(newNodes())
	counts, maxRun := runStrategy(lb, 3, count)
	for _, n := range newNodes() {
		assert.Equal(t, count/3, counts[n.Index], "%s 应均摊", n.Name)
	}
	assert.Equal(t, 1, slices.Max(maxRun), "应严格轮转,无连续命中")
}

// TestLeastConnection_LoadAware 验证负载感知:忙节点会被新请求避开。
// 构造:6 次 pick 均摊到各 2 在途,再 done(B)、done(C) → inflight A=2,B=1,C=1。
// 验证:接下来两次 pick 落到更闲的 B、C,忙节点 A 始终停在 2、不被选中。
func TestLeastConnection_LoadAware(t *testing.T) {
	nodes := newNodes()
	lb := NewLeastConnection(nodes)
	lb.logf = t.Logf // 开 trace,-v 下可见每步在途快照与选点

	// 6 个请求均摊:A,B,C,A,B,C → 各 2 在途。
	for i := 0; i < 6; i++ {
		lb.pick()
	}
	require.Equal(t, []int{2, 2, 2}, lb.inflight)

	// A 的请求很慢(不 done);B、C 各释放一个 → 在途 A=2,B=1,C=1。
	lb.done(nodes[1]) // B
	lb.done(nodes[2]) // C
	require.Equal(t, []int{2, 1, 1}, lb.inflight)

	// 接下来两个请求避开忙节点 A,落到更闲的 B、C。
	assert.Equal(t, "B", lb.pick().Name) // min(2,1,1) → B
	assert.Equal(t, "C", lb.pick().Name) // min(2,2,1) → C
	assert.Equal(t, 2, lb.inflight[0], "A 始终维持 2 在途,未被新请求选中")
}

// TestP2C_BalancesBetterThanRandom 验证 P2C 负载比纯随机均衡得多。
// 验证:同样开 count 个请求不释放,P2C 各节点在途的极差(max-min)≤1,且明显小于纯随机的极差。
// 为何更均:纯随机命中数偏差 ~√count(本例 88);P2C 每次在 2 个候选里取更少者,把偏差压到接近 0。
func TestP2C_BalancesBetterThanRandom(t *testing.T) {
	const count = 9000

	p := NewP2C(newNodes(), rand.New(rand.NewSource(42)))
	for i := 0; i < count; i++ {
		p.pick()
	}
	p2cGap := slices.Max(p.inflight) - slices.Min(p.inflight)

	// 纯随机分配 count 个请求,统计命中数偏差作对照。
	r := &Random{rng: rand.New(rand.NewSource(42)), nodes: newNodes()}
	counts, _ := runStrategy(r, 3, count)
	randGap := slices.Max(counts) - slices.Min(counts)

	t.Logf("负载偏差(max-min):P2C=%d,Random=%d", p2cGap, randGap)
	assert.LessOrEqual(t, p2cGap, 1, "P2C 负载应高度均衡")
	assert.Less(t, p2cGap, randGap, "P2C 偏差应明显小于纯随机")
}

// TestP2C_Trace 开 logf 观察 P2C 的"二选一"决策过程(重在可视化)。
// 验证:仅 sanity——不 done 时在途总数=发起的请求数。
// 看点:每行打印随机抽到的两个候选及各自在途,可见 P2C 总把请求推向更闲的那个。
func TestP2C_Trace(t *testing.T) {
	const count = 10
	p := NewP2C(newNodes(), rand.New(rand.NewSource(1)))
	p.logf = t.Logf
	for i := 0; i < count; i++ {
		p.pick() // 不 done,在途只增不减,可看出 P2C 如何把后续请求往低位推
	}
	require.Equal(t, count, p.inflight[0]+p.inflight[1]+p.inflight[2], "在途总数应等于发起的请求数")
}
