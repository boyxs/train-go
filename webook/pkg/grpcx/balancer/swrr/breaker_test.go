package swrr

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeSubConn 是带标识的假 SubConn,白盒测试里用 addr 识别 Pick 选了哪个节点。
type fakeSubConn struct {
	balancer.SubConn
	addr string
}

func newBreakerPicker(addrs ...string) *breakerPicker {
	conns := make([]*conn, len(addrs))
	for i, a := range addrs {
		conns[i] = &conn{sc: fakeSubConn{addr: a}, weight: 1, available: true}
	}
	return &breakerPicker{conns: conns}
}

// pickOnce 选一次,返回命中地址 + 对本次结果回报错误的 Done 闭包。
func pickOnce(t *testing.T, p *breakerPicker) (string, func(error)) {
	t.Helper()
	res, err := p.Pick(balancer.PickInfo{})
	require.NoError(t, err)
	return res.SubConn.(fakeSubConn).addr, func(e error) {
		res.Done(balancer.DoneInfo{Err: e})
	}
}

func connByAddr(p *breakerPicker, addr string) *conn {
	for _, c := range p.conns {
		if c.sc.(fakeSubConn).addr == addr {
			return c
		}
	}
	return nil
}

// TestBreakerSWRR_NodeFailureBreaks 验证:业务错误不熔断,节点级错误连续达阈值才摘除。
func TestBreakerSWRR_NodeFailureBreaks(t *testing.T) {
	p := newBreakerPicker("A", "B", "C")

	// 业务错误(NotFound)不计入熔断
	addr, done := pickOnce(t, p)
	for i := 0; i < failThreshold+2; i++ {
		done(status.Error(codes.NotFound, "biz"))
	}
	c := connByAddr(p, addr)
	t.Logf("业务错误 ×%d 后 节点 %s: available=%v fails=%d", failThreshold+2, addr, c.available, c.fails)
	require.True(t, c.available, "业务错误不应摘除节点")
	require.Zero(t, c.fails)

	// 节点级错误(Unavailable)连续 failThreshold 次 → 摘除
	addr2, done2 := pickOnce(t, p)
	c2 := connByAddr(p, addr2)
	for i := 1; i <= failThreshold; i++ {
		done2(status.Error(codes.Unavailable, "down"))
		t.Logf("节点 %s 第 %d 次节点级失败: fails=%d available=%v", addr2, i, c2.fails, c2.available)
	}
	require.False(t, c2.available, "连续失败达阈值应摘除")
}

// TestBreakerSWRR_BreakDivertsTraffic 验证:节点被摘后,冷却期内流量全部转移到健康节点。
func TestBreakerSWRR_BreakDivertsTraffic(t *testing.T) {
	p := newBreakerPicker("A", "B", "C")
	a := connByAddr(p, "A")
	a.available, a.fails, a.downAt = false, failThreshold, nowMs() // 模拟刚摘除,在冷却期

	const n = 300
	counts := map[string]int{}
	for i := 0; i < n; i++ {
		addr, _ := pickOnce(t, p)
		counts[addr]++
	}
	t.Logf("摘除 A 后 %d 次命中分布: %v", n, counts)
	require.Zero(t, counts["A"], "被摘节点在冷却期内不应被选中")
	require.Equal(t, n, counts["B"]+counts["C"], "流量应全部转移到 B/C")
}

// TestBreakerSWRR_HalfOpenRecovers 验证:冷却到点后半开放行探活,探活成功则节点恢复。
func TestBreakerSWRR_HalfOpenRecovers(t *testing.T) {
	p := newBreakerPicker("A", "B", "C")
	a := connByAddr(p, "A")
	a.available, a.fails, a.downAt = false, failThreshold, nowMs()

	// 冷却期内 A 不参选
	cold := 0
	for i := 0; i < 60; i++ {
		if addr, _ := pickOnce(t, p); addr == "A" {
			cold++
		}
	}
	t.Logf("冷却期内 60 次 Pick,A 命中 %d 次(应为 0)", cold)
	require.Zero(t, cold)

	// 拨快时钟:downAt 推到冷却期之前,A 进入半开
	a.downAt = nowMs() - coolDown.Milliseconds() - 1
	t.Logf("冷却到点,A 进入半开")

	// 循环 Pick 到探活命中 A,done 成功 → 恢复
	recovered := false
	for i := 0; i < 100 && !recovered; i++ {
		addr, done := pickOnce(t, p)
		done(nil) // 探活成功
		if addr == "A" {
			recovered = true
		}
	}
	require.True(t, recovered, "半开期 A 应被放行探活")
	t.Logf("A 探活成功后: available=%v fails=%d", a.available, a.fails)
	require.True(t, a.available, "探活成功应恢复 available")
	require.Zero(t, a.fails)
}
