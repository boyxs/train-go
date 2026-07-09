package redislock

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// benchRedis 优先连真 Redis（env REDISLOCK_REDIS_ADDR/PASS，默认 127.0.0.1:6379），
// 不可达则回退 miniredis（数值仅供相对参考，无网络往返）。返回后端标签供 b.Log 记录。
func benchRedis(tb testing.TB) (redis.UniversalClient, string) {
	addr := os.Getenv("REDISLOCK_REDIS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:6379"
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr, Password: os.Getenv("REDISLOCK_REDIS_PASS")})
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err == nil {
		tb.Cleanup(func() { _ = rdb.Close() })
		return rdb, "real(" + addr + ")"
	}
	_ = rdb.Close()

	mr, err := miniredis.Run()
	if err != nil {
		tb.Fatalf("miniredis: %v", err)
	}
	tb.Cleanup(mr.Close)
	m := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	tb.Cleanup(func() { _ = m.Close() })
	return m, "miniredis"
}

// delKeys 清掉本基准用到的 lock/fence key，避免污染共享 dev Redis。
func delKeys(rdb redis.UniversalClient, callerKeys ...string) {
	ctx := context.Background()
	for _, k := range callerKeys {
		rdb.Del(ctx, lockKey(k), fenceKey(k), channelKey(k))
	}
}

// BenchmarkTryLock_Uncontended 无竞争 获取+释放 吞吐（基线）。
func BenchmarkTryLock_Uncontended(b *testing.B) {
	rdb, backend := benchRedis(b)
	b.Logf("backend=%s", backend)
	cli := NewClient(rdb)
	ctx := context.Background()
	const key = "bench:uncontended"
	delKeys(rdb, key)
	b.Cleanup(func() { delKeys(rdb, key) })

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lk, ok, err := cli.TryLock(ctx, key, WithLeaseTime(30*time.Second))
		if err != nil || !ok {
			b.Fatalf("acquire failed: ok=%v err=%v", ok, err)
		}
		if err := lk.Unlock(ctx); err != nil {
			b.Fatalf("unlock: %v", err)
		}
	}
}

// BenchmarkTryLock_Contended N goroutine 抢同 key：吞吐与失败率（busy%）。
func BenchmarkTryLock_Contended(b *testing.B) {
	for _, n := range []int{2, 8, 32, 128} {
		b.Run(fmt.Sprintf("g%d", n), func(b *testing.B) {
			rdb, _ := benchRedis(b)
			cli := NewClient(rdb)
			ctx := context.Background()
			key := fmt.Sprintf("bench:contended:%d", n)
			delKeys(rdb, key)
			b.Cleanup(func() { delKeys(rdb, key) })

			var acquired, busy, errs, idx int64
			b.ResetTimer()
			var wg sync.WaitGroup
			for w := 0; w < n; w++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for atomic.AddInt64(&idx, 1) <= int64(b.N) { // 精确跑满 b.N，不丢整除余数
						lk, ok, err := cli.TryLock(ctx, key, WithLeaseTime(5*time.Second))
						switch {
						case err != nil:
							atomic.AddInt64(&errs, 1)
						case !ok:
							atomic.AddInt64(&busy, 1)
						default:
							atomic.AddInt64(&acquired, 1)
							_ = lk.Unlock(ctx)
						}
					}
				}()
			}
			wg.Wait()
			b.StopTimer()
			total := acquired + busy + errs
			if total > 0 {
				b.ReportMetric(float64(busy)/float64(total)*100, "busy%")
				b.ReportMetric(float64(errs)/float64(total)*100, "err%")
			}
		})
	}
}

// BenchmarkWatchdogOverhead 持锁期 watchdog 后台开销：开 vs 关（每次获取/释放的差值）。
func BenchmarkWatchdogOverhead(b *testing.B) {
	rdb, backend := benchRedis(b)
	b.Logf("backend=%s", backend)
	cli := NewClient(rdb)
	ctx := context.Background()
	const key = "bench:watchdog"
	b.Cleanup(func() { delKeys(rdb, key) })

	b.Run("off", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			lk, _, err := cli.TryLock(ctx, key, WithLeaseTime(30*time.Second))
			if err != nil {
				b.Fatal(err)
			}
			_ = lk.Unlock(ctx)
		}
	})
	b.Run("on", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			lk, _, err := cli.TryLock(ctx, key, WithWatchdogTimeout(30*time.Second))
			if err != nil {
				b.Fatal(err)
			}
			_ = lk.Unlock(ctx)
		}
	})
}

// BenchmarkFencingOverhead WithFencing 的额外 INCR 开销：关 vs 开。
func BenchmarkFencingOverhead(b *testing.B) {
	rdb, backend := benchRedis(b)
	b.Logf("backend=%s", backend)
	cli := NewClient(rdb)
	ctx := context.Background()
	const key = "bench:fencing"
	b.Cleanup(func() { delKeys(rdb, key) })

	b.Run("off", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			lk, _, err := cli.TryLock(ctx, key, WithLeaseTime(30*time.Second))
			if err != nil {
				b.Fatal(err)
			}
			_ = lk.Unlock(ctx)
		}
	})
	b.Run("on", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			lk, _, err := cli.TryLock(ctx, key, WithLeaseTime(30*time.Second), WithFencing())
			if err != nil {
				b.Fatal(err)
			}
			_ = lk.Unlock(ctx)
		}
	})
}

// BenchmarkReentrant 重入 N 层的开销：非重入基线（每次全新获取+释放）vs 同 ownerId 连续
// acquire N 次再 release N 次。量化重入每多一层的额外 hincrby 往返成本。
func BenchmarkReentrant(b *testing.B) {
	rdb, backend := benchRedis(b)
	b.Logf("backend=%s", backend)
	cli := NewClient(rdb)
	ctx := context.Background()
	const key = "bench:reentrant"
	b.Cleanup(func() { delKeys(rdb, key) })

	b.Run("non-reentrant", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			lk, ok, err := cli.TryLock(ctx, key, WithLeaseTime(30*time.Second))
			if err != nil || !ok {
				b.Fatalf("acquire: ok=%v err=%v", ok, err)
			}
			_ = lk.Unlock(ctx)
		}
	})

	for _, depth := range []int{2, 4, 8} {
		b.Run(fmt.Sprintf("depth%d", depth), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				locks := make([]RedisLock, 0, depth)
				for d := 0; d < depth; d++ {
					lk, ok, err := cli.TryLock(ctx, key, WithReentrant("bench-owner"), WithLeaseTime(30*time.Second))
					if err != nil || !ok {
						b.Fatalf("reentrant acquire d=%d: ok=%v err=%v", d, ok, err)
					}
					locks = append(locks, lk)
				}
				for _, lk := range locks {
					_ = lk.Unlock(ctx)
				}
			}
		})
	}
}

// BenchmarkBlockingLock_PubSubVsPoll 阻塞获取的 hand-off 延迟：pub/sub 唤醒 vs 纯轮询（§7.1，证 §3.4 收益）。
// 每次迭代：持有者占锁、holdMs 后释放；测等待者从"开始等"到"拿到"的耗时（ns/op = 平均 hand-off 延迟）。
// poll 臂手动轮询（模拟 P4-9 前）；pubsub 臂用 Lock（retryInterval 设大，确保靠 pub/sub 而非轮询唤醒）。
func BenchmarkBlockingLock_PubSubVsPoll(b *testing.B) {
	rdb, backend := benchRedis(b)
	b.Logf("backend=%s", backend)
	cli := NewClient(rdb)
	ctx := context.Background()
	const key = "bench:handoff"
	const holdMs = 5 * time.Millisecond
	const pollInterval = 50 * time.Millisecond
	b.Cleanup(func() { delKeys(rdb, key) })

	// handoff 起持有者→holdMs 后释放→等待者阻塞获取。acquire 由各臂提供（轮询 / pub-sub）。
	handoff := func(b *testing.B, acquire func()) {
		for i := 0; i < b.N; i++ {
			h, ok, err := cli.TryLock(ctx, key, WithLeaseTime(5*time.Second))
			if err != nil || !ok {
				b.Fatalf("holder: ok=%v err=%v", ok, err)
			}
			rel := make(chan struct{})
			go func() { time.Sleep(holdMs); _ = h.Unlock(ctx); close(rel) }()
			acquire() // 阻塞直到拿到
			<-rel
		}
	}

	b.Run("poll", func(b *testing.B) {
		handoff(b, func() {
			for {
				lk, ok, err := cli.TryLock(ctx, key, WithLeaseTime(5*time.Second))
				if err != nil {
					b.Fatal(err)
				}
				if ok {
					_ = lk.Unlock(ctx)
					return
				}
				time.Sleep(pollInterval)
			}
		})
	})

	b.Run("pubsub", func(b *testing.B) {
		handoff(b, func() {
			lk, err := cli.Lock(ctx, key, WithLeaseTime(5*time.Second), WithRetryInterval(1*time.Second))
			if err != nil {
				b.Fatal(err)
			}
			_ = lk.Unlock(ctx)
		})
	})
}
