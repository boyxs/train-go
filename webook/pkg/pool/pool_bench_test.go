package pool

import (
	"runtime"
	"sync"
	"testing"
)

// BenchmarkPool_Submit 串行提交 no-op 任务，隔离「submit + dispatch + run 包装」的每任务开销。
func BenchmarkPool_Submit(b *testing.B) {
	p := New(runtime.NumCPU(), 4096, WithRejectPolicy(Block))
	defer p.Shutdown()
	var wg sync.WaitGroup
	wg.Add(b.N)
	done := wg.Done
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.Submit(done)
	}
	wg.Wait()
}

// BenchmarkPool_SubmitParallel 多 goroutine 并发提交，压 tasks channel + in-flight 计数的竞争。
func BenchmarkPool_SubmitParallel(b *testing.B) {
	p := New(runtime.NumCPU(), 4096, WithRejectPolicy(Block))
	defer p.Shutdown()
	var wg sync.WaitGroup
	wg.Add(b.N)
	done := wg.Done
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			p.Submit(done)
		}
	})
	wg.Wait()
}

// BenchmarkRawGoroutine 对照组：每任务裸起一个 goroutine（无池）。
func BenchmarkRawGoroutine(b *testing.B) {
	var wg sync.WaitGroup
	wg.Add(b.N)
	done := wg.Done
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		go done()
	}
	wg.Wait()
}
