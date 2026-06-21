package balancer

import (
	"fmt"
	"hash/crc32"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 一致性哈希:与前面"按权重/负载分流"不同,它按 key 做亲和——同一 key 恒定落到同一节点。
// 用于缓存分片、会话粘滞等场景;增删节点只重映射相邻弧段的 key(≈1/N),不像 hash%N 全量重排。

// ── 一致性哈希 ────────────────────────────────────────────

// ConsistentHash 一致性哈希。
// 原理:每个真实节点哈希成 replicas 个虚拟节点落在 [0,2^32) 的环上;key 哈希后顺时针找到的
// 第一个虚拟节点即归属(越过环尾回到环首)。
// 特点:同一 key 恒定命中同节点(亲和);增删节点只重映射相邻弧段≈1/N 的 key,远优于 hash%N 全量重排;
// 虚拟节点越多分布越均,否则易倾斜。
// 适用:缓存分片、会话粘滞、有状态分片;不适用需严格等分或频繁全量再均衡的场景。
// 复杂度:pick O(log V)(V=虚拟节点总数,环上二分),建环 O(V log V)。
type ConsistentHash struct {
	replicas int
	ring     []uint32         // 升序的虚拟节点哈希
	vnodes   map[uint32]*Node // 虚拟节点哈希 → 真实节点
}

func NewConsistentHash(replicas int, nodes []*Node) *ConsistentHash {
	c := &ConsistentHash{
		replicas: replicas,
		vnodes:   make(map[uint32]*Node, replicas*len(nodes)),
	}
	for _, n := range nodes {
		c.addVNodes(n)
	}
	slices.Sort(c.ring) // 全部虚拟节点就位后只排一次序,避免逐节点重排
	return c
}

// addVNodes 把一个真实节点的 replicas 个虚拟节点加入环(只追加不排序,排序由调用方统一做)。
func (c *ConsistentHash) addVNodes(n *Node) {
	for i := 0; i < c.replicas; i++ {
		h := hashKey(fmt.Sprintf("%s#%d", n.Name, i))
		c.ring = append(c.ring, h)
		c.vnodes[h] = n
	}
}

// remove 摘掉某节点的全部虚拟节点并重建环。其它节点的虚拟节点位置不变,
// 故只有原本落在被删节点弧段的 key 会迁走,这正是一致性哈希的价值。
func (c *ConsistentHash) remove(target *Node) {
	for i := 0; i < c.replicas; i++ {
		delete(c.vnodes, hashKey(fmt.Sprintf("%s#%d", target.Name, i)))
	}
	c.ring = c.ring[:0]
	for h := range c.vnodes {
		c.ring = append(c.ring, h)
	}
	slices.Sort(c.ring)
}

func (c *ConsistentHash) pick(key string) *Node {
	if len(c.ring) == 0 {
		return nil
	}
	// 顺时针第一个 >= h 的虚拟节点;越过环尾则回到环首。
	idx, _ := slices.BinarySearch(c.ring, hashKey(key))
	if idx == len(c.ring) {
		idx = 0
	}
	return c.vnodes[c.ring[idx]]
}

func hashKey(s string) uint32 {
	return crc32.ChecksumIEEE([]byte(s))
}

// ── 测试 ──────────────────────────────────────────────────

// TestConsistentHash_Affinity 验证亲和性:同一 key 永远命中同一节点。
// 验证:同一 key 连续 pick 100 次结果恒定。
// 为何恒定:环与哈希都不变 → key 顺时针找到的第一个虚拟节点固定,这是缓存命中/会话粘滞的基础。
func TestConsistentHash_Affinity(t *testing.T) {
	c := NewConsistentHash(100, newNodes())
	const key = "user-42"
	first := c.pick(key)
	require.NotNil(t, first)
	for i := 0; i < 100; i++ {
		require.Equal(t, first, c.pick(key), "同 key 必须恒定命中同节点")
	}
}

// TestConsistentHash_Distribution 验证分布:足够虚拟节点时,大量 key 近似均摊。
// 验证:3 万个 key 散到 3 个等权节点,各占比落在 1/3 的 ±20% 内。
// 为何非精确均分:每节点 200 个虚拟节点,环上弧段长度仍有随机起伏 → 近似而非严格;虚拟节点越多越均。
func TestConsistentHash_Distribution(t *testing.T) {
	c := NewConsistentHash(200, newNodes())
	const keys = 30000
	counts := map[string]int{}
	for i := 0; i < keys; i++ {
		counts[c.pick(fmt.Sprintf("key-%d", i)).Name]++
	}
	for _, n := range newNodes() {
		t.Logf("%s 命中 %d 个 key,占比 %.2f%%", n.Name, counts[n.Name], float64(counts[n.Name])/keys*100)
		assert.InDelta(t, keys/3, counts[n.Name], keys/3*0.20, "%s 分布应近似均衡", n.Name)
	}
}

// TestConsistentHash_MinimalRemapOnRemove 验证一致性哈希的核心价值:删节点只动局部。
// 验证:删除 C 后,原属 C 的 key 全部迁走(≈1/N),而原属 A、B 的 key 归属零变动(stayedWrong=0)。
// 对比 hash%N:取模法删一个节点会让几乎所有 key 重新落点;一致性哈希只影响被删节点的弧段。
func TestConsistentHash_MinimalRemapOnRemove(t *testing.T) {
	nodes := newNodes()
	c := NewConsistentHash(200, nodes)
	const keys = 30000

	before := make([]string, keys)
	for i := 0; i < keys; i++ {
		before[i] = c.pick(fmt.Sprintf("key-%d", i)).Name
	}

	c.remove(nodes[2]) // 删 C

	moved, stayedWrong := 0, 0
	for i := 0; i < keys; i++ {
		now := c.pick(fmt.Sprintf("key-%d", i)).Name
		switch {
		case before[i] == "C":
			moved++
			require.NotEqual(t, "C", now, "C 已删除,不应再被命中")
		case before[i] != now:
			stayedWrong++
		}
	}
	t.Logf("删除 C 后:迁移 %d 个(原属 C),非 C 的 key 变动 %d 个", moved, stayedWrong)
	require.Zero(t, stayedWrong, "删节点不应扰动其它节点的 key")
	assert.InDelta(t, keys/3, moved, keys/3*0.25, "迁移量应 ≈ 原 C 的份额(≈1/N)")
}
