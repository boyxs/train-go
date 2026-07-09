// Package loadtest 是 redislock 的真实并发压测 harness（§7.2）：多 goroutine 真 Redis 抢锁，
// 校验互斥 / 单调不变量（MutexViolations / FenceMonotonicBreaks 必须 = 0），量化 QPS 与获取延迟分位。
// 持久留用、可复跑（非一次性脚本）；入口见 loadtest_test.go 的 TestLoad。
//
// 与 cmd/loadserver（JMeter HTTP 壳）互补：这里是进程内 Go 并发驱动，可直接 go test 跑；
// loadserver 供外部协议压测工具（JMeter）驱动。两者同一套不变量口径。
package loadtest

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/boyxs/train-go/webook/pkg/redislock"
)

// Config 一次压测的参数。KeyCount 越小争用越强（1=全员抢同一把锁）。
type Config struct {
	Concurrency int           // 并发 goroutine 数
	Duration    time.Duration // 持续时长
	KeyCount    int           // 竞争 key 数（<1 视为 1）
	WaitTime    time.Duration // TryLock 等待上限；>0 走阻塞（pub/sub 唤醒 / 公平排队）
	LeaseTime   time.Duration // 固定租约；0 走 watchdog
	Fair        bool          // 公平锁
	Fencing     bool          // fencing
	HoldTime    time.Duration // 临界区持有时长（模拟真实工作）
}

// Report 压测结果。MutexViolations / FenceMonotonicBreaks 必须为 0，否则锁不正确。
type Report struct {
	Acquired, Busy, Errors int64
	QPS                    float64
	P50, P90, P99          time.Duration
	MutexViolations        int64 // ★ 同一 key 同时 >1 持有者的次数 —— 真互斥的铁证
	FenceMonotonicBreaks   int64 // ★ fencing 令牌非单调递增的次数
}

// keyState 每 key 的不变量跟踪：holders=当前持有者数，lastFence=已见最大令牌。
type keyState struct {
	holders   int32
	lastFence int64
}

func (c Config) opts() []redislock.Options {
	var opts []redislock.Options
	if c.LeaseTime > 0 {
		opts = append(opts, redislock.WithLeaseTime(c.LeaseTime))
	}
	if c.WaitTime > 0 {
		opts = append(opts, redislock.WithWaitTime(c.WaitTime))
	}
	if c.Fair {
		opts = append(opts, redislock.WithFair())
	}
	if c.Fencing {
		opts = append(opts, redislock.WithFencing())
	}
	return opts
}

// Run 跑一次压测。每个 goroutine 循环：抢锁 → 进临界区（原子 +1 持有计数，>1 即互斥破坏；
// fencing 校验令牌单调）→ 持有 HoldTime → 退出计数（先减再释放，避免释放后被抢的假阳性）→ 释放。
func (c Config) Run(ctx context.Context, cli redislock.Client) (Report, error) {
	if c.KeyCount < 1 {
		c.KeyCount = 1
	}
	states := make([]*keyState, c.KeyCount)
	names := make([]string, c.KeyCount)
	for i := range states {
		states[i] = &keyState{}
		names[i] = fmt.Sprintf("loadtest:key:%d", i)
	}

	var rep Report
	runCtx, cancel := context.WithTimeout(ctx, c.Duration)
	defer cancel()

	latencies := make([][]time.Duration, c.Concurrency)
	var wg sync.WaitGroup
	start := time.Now()
	for g := 0; g < c.Concurrency; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			opts := c.opts()
			ki := g % c.KeyCount // 多 goroutine 落同一 key 才有争用
			key, ks := names[ki], states[ki]
			var local []time.Duration
			for runCtx.Err() == nil {
				t0 := time.Now()
				lk, ok, err := cli.TryLock(runCtx, key, opts...)
				lat := time.Since(t0)
				switch {
				case err != nil:
					if runCtx.Err() == nil { // 非收尾的真错误才计
						atomic.AddInt64(&rep.Errors, 1)
					}
					continue
				case !ok:
					atomic.AddInt64(&rep.Busy, 1)
					continue
				}
				local = append(local, lat)
				atomic.AddInt64(&rep.Acquired, 1)

				if atomic.AddInt32(&ks.holders, 1) > 1 { // 同 key 第二个持有者 → 互斥破坏
					atomic.AddInt64(&rep.MutexViolations, 1)
				}
				if f := lk.Fence(); f > 0 {
					if f <= atomic.LoadInt64(&ks.lastFence) {
						atomic.AddInt64(&rep.FenceMonotonicBreaks, 1)
					} else {
						atomic.StoreInt64(&ks.lastFence, f)
					}
				}
				if c.HoldTime > 0 {
					time.Sleep(c.HoldTime)
				}
				atomic.AddInt32(&ks.holders, -1)                        // 先退计数，此时 Redis 锁仍在，无假阳性
				if err := lk.Unlock(context.Background()); err != nil { // 独立 ctx：runCtx 可能已到点
					atomic.AddInt64(&rep.Errors, 1)
				}
			}
			latencies[g] = local
		}(g)
	}
	wg.Wait()
	elapsed := time.Since(start)

	var all []time.Duration
	for _, l := range latencies {
		all = append(all, l...)
	}
	sort.Slice(all, func(i, j int) bool { return all[i] < all[j] })
	rep.P50, rep.P90, rep.P99 = percentile(all, 50), percentile(all, 90), percentile(all, 99)
	if elapsed > 0 {
		rep.QPS = float64(rep.Acquired) / elapsed.Seconds()
	}
	return rep, nil
}

// percentile 取已排序切片的第 p 百分位（空切片返 0）。
func percentile(sorted []time.Duration, p int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	i := p * len(sorted) / 100
	if i >= len(sorted) {
		i = len(sorted) - 1
	}
	return sorted[i]
}
