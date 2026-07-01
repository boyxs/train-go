package pool

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Shutdown 优雅关闭：排空已入队任务。
func TestPool_RunsAndDrainsOnShutdown(t *testing.T) {
	var n atomic.Int32
	p := New(4, 100)
	for i := 0; i < 50; i++ {
		require.True(t, p.Submit(func() { n.Add(1) }))
	}
	p.Shutdown()
	assert.Equal(t, int32(50), n.Load())
}

// 单 task panic 不拖垮 worker：onPanic 回调，后续任务照常。
func TestPool_PanicRecovered(t *testing.T) {
	var panics, done atomic.Int32
	p := New(1, 10, WithPanicHandler(func(any) { panics.Add(1) }))
	require.True(t, p.Submit(func() { panic("boom") }))
	require.True(t, p.Submit(func() { done.Add(1) }))
	p.Shutdown()
	assert.Equal(t, int32(1), panics.Load())
	assert.Equal(t, int32(1), done.Load())
}

// Discard（默认）：队列满 Submit 返回 false。
func TestPool_DiscardWhenFull(t *testing.T) {
	started, block := make(chan struct{}), make(chan struct{})
	p := New(1, 1)
	require.True(t, p.Submit(func() { close(started); <-block }))
	<-started
	require.True(t, p.Submit(func() {})) // 占满队列
	assert.False(t, p.Submit(func() {})) // 满 → 丢弃
	close(block)
	p.Shutdown()
}

// CallerRuns：队列满时由调用方 goroutine 直接执行（同步、返回 true）。
func TestPool_CallerRunsWhenFull(t *testing.T) {
	started, block := make(chan struct{}), make(chan struct{})
	p := New(1, 1, WithRejectPolicy(CallerRuns))
	require.True(t, p.Submit(func() { close(started); <-block }))
	<-started
	require.True(t, p.Submit(func() {})) // 占满队列
	var ranInCaller bool
	require.True(t, p.Submit(func() { ranInCaller = true })) // 满 → 调用方直接跑
	assert.True(t, ranInCaller)                              // 同步执行完才返回
	close(block)
	p.Shutdown()
}

// Block + 超时：队列满阻塞等待，超时后返回 false。
func TestPool_BlockTimesOut(t *testing.T) {
	started, block := make(chan struct{}), make(chan struct{})
	p := New(1, 1, WithRejectPolicy(Block), WithBlockTimeout(40*time.Millisecond))
	require.True(t, p.Submit(func() { close(started); <-block }))
	<-started
	require.True(t, p.Submit(func() {})) // 占满队列
	start := time.Now()
	assert.False(t, p.Submit(func() {})) // 满 + 阻塞 → 超时丢弃
	assert.GreaterOrEqual(t, time.Since(start), 40*time.Millisecond)
	close(block)
	p.Shutdown()
}

// ShutdownNow：丢弃队列中未开始的任务并返回丢弃数；在途任务跑完。
func TestPool_ShutdownNowDropsQueued(t *testing.T) {
	started, block := make(chan struct{}), make(chan struct{})
	var ran atomic.Int32
	p := New(1, 100)
	require.True(t, p.Submit(func() { close(started); <-block; ran.Add(1) }))
	<-started
	for i := 0; i < 20; i++ {
		require.True(t, p.Submit(func() { ran.Add(1) }))
	}
	// 延迟放开在途任务：保证 ShutdownNow 先 close(done)，worker 醒来时已是关闭态，不再拾取队列
	go func() {
		time.Sleep(20 * time.Millisecond)
		close(block)
	}()
	dropped := p.ShutdownNow()
	assert.Equal(t, 20, dropped)          // 20 个排队全丢
	assert.Equal(t, int32(1), ran.Load()) // 只有在途那个跑了
}

// 安全：关闭后 Submit 返回 false 且绝不 panic（task channel 永不 close）。
func TestPool_SubmitAfterShutdownIsSafe(t *testing.T) {
	p := New(2, 10)
	p.Shutdown()
	assert.NotPanics(t, func() {
		assert.False(t, p.Submit(func() {}))
	})
}

// Running / Queued 运行态可观测。
func TestPool_Introspection(t *testing.T) {
	started, block := make(chan struct{}), make(chan struct{})
	p := New(1, 10)
	require.True(t, p.Submit(func() { close(started); <-block }))
	<-started
	require.True(t, p.Submit(func() {}))
	require.True(t, p.Submit(func() {}))
	assert.Equal(t, 1, p.InFlight()) // 在途 1
	assert.Equal(t, 2, p.Queued())   // 排队 2
	close(block)
	p.Shutdown()
}
