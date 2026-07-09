package integration

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/boyxs/train-go/webook/internal/integration/setup"
	"github.com/boyxs/train-go/webook/pkg/redislock"
)

// 分布式锁端到端集成测试 — 真 Redis 验证获取原子性、watchdog 续约、跨实例互斥。
// 跑前置：docker compose up redis；config/test.yaml 指向该 Redis。
// miniredis 单连接共享，跨连接竞争只能用真 Redis 验。
type RedislockSuite struct {
	suite.Suite
	cmd redis.UniversalClient
}

func TestRedislockIntegration(t *testing.T) {
	suite.Run(t, &RedislockSuite{})
}

func (s *RedislockSuite) SetupSuite() {
	s.cmd = setup.InitRedis()
}

func (s *RedislockSuite) TearDownTest() {
	ctx := context.Background()
	// 真实 key 带 hash tag：redislock:{itlock:xxx}:lock / :ch / :fence
	keys, _ := s.cmd.Keys(ctx, "redislock:{itlock:*").Result()
	if len(keys) > 0 {
		s.cmd.Del(ctx, keys...)
	}
}

// 100 goroutine 抢同一锁：真 Redis 原子性保证只 1 个赢。
func (s *RedislockSuite) TestMutex_OnlyOneWins() {
	t := s.T()
	cli := redislock.NewClient(s.cmd)
	ctx := context.Background()
	key := "itlock:mutex"

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	var success, busy int32
	holders := make(chan redislock.RedisLock, goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			lock, ok, err := cli.TryLock(ctx, key, redislock.WithLeaseTime(5*time.Second))
			if err != nil {
				return
			}
			if ok {
				atomic.AddInt32(&success, 1)
				holders <- lock
				return
			}
			atomic.AddInt32(&busy, 1)
		}()
	}
	wg.Wait()
	close(holders)

	assert.Equal(t, int32(1), atomic.LoadInt32(&success), "100 goroutine 抢锁应只 1 个成功")
	assert.Equal(t, int32(goroutines-1), atomic.LoadInt32(&busy))

	for lock := range holders {
		_ = lock.Unlock(ctx)
	}
}

// 默认 watchdog（租约/3）：等 3*租约后锁仍存活，证明续约真发生（用句柄 TTL() 查）。
func (s *RedislockSuite) TestDefaultWatchdog_KeepsLockAlive() {
	t := s.T()
	cli := redislock.NewClient(s.cmd)
	ctx := context.Background()
	key := "itlock:watchdog"

	lock, ok, err := cli.TryLock(ctx, key, redislock.WithWatchdogTimeout(1*time.Second))
	require.NoError(t, err)
	require.True(t, ok)
	t.Cleanup(func() { _ = lock.Unlock(ctx) })

	time.Sleep(3 * time.Second)

	ttl, err := lock.TTL(ctx)
	require.NoError(t, err)
	assert.Greater(t, ttl, time.Duration(0), "默认 watchdog 应保持锁存活")
	assert.LessOrEqual(t, ttl, 1*time.Second, "TTL 不该超过租约")
}

// 固定租约（WithLeaseTime）：不续约，wall-clock 过租约后锁自然过期。
func (s *RedislockSuite) TestFixedLease_NaturalExpire() {
	t := s.T()
	cli := redislock.NewClient(s.cmd)
	ctx := context.Background()
	key := "itlock:fixed"

	lock, ok, err := cli.TryLock(ctx, key, redislock.WithLeaseTime(1*time.Second))
	require.NoError(t, err)
	require.True(t, ok)
	t.Cleanup(func() { _ = lock.Unlock(ctx) })

	time.Sleep(1500 * time.Millisecond)

	locked, err := lock.IsLocked(ctx)
	require.NoError(t, err)
	assert.False(t, locked, "固定租约到期后锁应自然过期")
}

// 跨 Client 实例互斥：模拟两个 pod 抢同一锁，只一个赢。
func (s *RedislockSuite) TestCrossClient_Mutex() {
	t := s.T()
	client1 := redislock.NewClient(s.cmd)
	rdb2 := setup.InitRedis() // 独立连接模拟另一进程
	t.Cleanup(func() { _ = rdb2.Close() })
	client2 := redislock.NewClient(rdb2)

	ctx := context.Background()
	key := "itlock:cross"

	lock1, ok1, err := client1.TryLock(ctx, key, redislock.WithLeaseTime(5*time.Second))
	require.NoError(t, err)
	require.True(t, ok1)
	t.Cleanup(func() { _ = lock1.Unlock(ctx) })

	_, ok2, err := client2.TryLock(ctx, key, redislock.WithLeaseTime(5*time.Second))
	require.NoError(t, err)
	assert.False(t, ok2, "另一 Client 应抢不到（跨实例互斥）")
}

// Lock 阻塞模式（P4-9 pub/sub）：先占用，第二个 Lock 阻塞重试，前者 Unlock 后近即时拿到。
// retryInterval 故意设 5s（远大于释放时刻），只有 pub/sub 唤醒能让它在 ~200ms 拿到——
// 真 Redis 验证 §3.4 收益（轮询要等满 5s）。
func (s *RedislockSuite) TestBlockingLock_HandsOff() {
	t := s.T()
	cli := redislock.NewClient(s.cmd)
	ctx := context.Background()
	key := "itlock:blocking"

	first, ok, err := cli.TryLock(ctx, key, redislock.WithLeaseTime(30*time.Second))
	require.NoError(t, err)
	require.True(t, ok)

	go func() {
		time.Sleep(200 * time.Millisecond)
		_ = first.Unlock(ctx)
	}()

	bgCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	start := time.Now()
	second, err := cli.Lock(bgCtx, key,
		redislock.WithLeaseTime(30*time.Second),
		redislock.WithRetryInterval(5*time.Second))
	elapsed := time.Since(start)

	require.NoError(t, err, "ctx 没超时应能拿到")
	require.NotNil(t, second)
	t.Cleanup(func() { _ = second.Unlock(ctx) })

	assert.GreaterOrEqual(t, elapsed, 150*time.Millisecond, "应至少阻塞到 first Unlock")
	assert.Less(t, elapsed, 1*time.Second, "pub/sub 唤醒应近即时拿到，而非等 5s 轮询间隔")
}

// 可重入（P3）：同 ownerId 重入 N 次、释放 N 次才真释放；不同 owner 互斥。
// 真 Redis 验 Lua 的 hincrby 重入计数（miniredis Lua 是重实现，真 Redis 兜底）。
func (s *RedislockSuite) TestReentrant_SameOwnerAndCrossOwnerMutex() {
	t := s.T()
	cli := redislock.NewClient(s.cmd)
	ctx := context.Background()
	key := "itlock:reentrant"

	// 同 owner 重入两次
	l1, ok, err := cli.TryLock(ctx, key, redislock.WithReentrant("owner-A"), redislock.WithLeaseTime(10*time.Second))
	require.NoError(t, err)
	require.True(t, ok)
	l2, ok, err := cli.TryLock(ctx, key, redislock.WithReentrant("owner-A"), redislock.WithLeaseTime(10*time.Second))
	require.NoError(t, err)
	require.True(t, ok, "同 owner 应重入成功")

	hc, err := l2.HoldCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, hc, "重入深度应为 2")

	// 不同 owner 互斥
	_, ok, err = cli.TryLock(ctx, key, redislock.WithReentrant("owner-B"), redislock.WithLeaseTime(10*time.Second))
	require.NoError(t, err)
	assert.False(t, ok, "不同 owner 应互斥")

	// 释放 N 次才真释放
	require.NoError(t, l1.Unlock(ctx))
	locked, err := l2.IsLocked(ctx)
	require.NoError(t, err)
	assert.True(t, locked, "释放一次仍应持有")

	require.NoError(t, l2.Unlock(ctx))
	locked, err = l2.IsLocked(ctx)
	require.NoError(t, err)
	assert.False(t, locked, "释放两次后应真正释放")
}

// 公平锁 FIFO（P4-10）：多个等待者按入队先后依次拿到锁。真 Redis 验队列 Lua + pub/sub 唤醒协作。
func (s *RedislockSuite) TestFair_FIFOAcrossWaiters() {
	t := s.T()
	cli := redislock.NewClient(s.cmd)
	ctx := context.Background()
	key := "itlock:fair"
	queue := "redislock:{itlock:fair}:queue" // 队列 key（hash-tag 与锁同槽）

	holder, ok, err := cli.TryLock(ctx, key, redislock.WithLeaseTime(30*time.Second))
	require.NoError(t, err)
	require.True(t, ok)

	const n = 4
	var mu sync.Mutex
	var order []int
	bgCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			lk, err := cli.Lock(bgCtx, key, redislock.WithFair(),
				redislock.WithLeaseTime(30*time.Second), redislock.WithRetryInterval(50*time.Millisecond))
			if err != nil {
				return
			}
			mu.Lock()
			order = append(order, id)
			mu.Unlock()
			time.Sleep(30 * time.Millisecond)
			_ = lk.Unlock(ctx)
		}(i)
		// 等 id 真正入队再起下一个 → 队列顺序确定
		want := int64(i + 1)
		require.Eventually(t, func() bool {
			return s.cmd.LLen(ctx, queue).Val() == want
		}, 3*time.Second, 20*time.Millisecond, "waiter %d 应入公平队列", i)
	}

	require.NoError(t, holder.Unlock(ctx)) // 放锁，队头依次获取
	wg.Wait()
	assert.Equal(t, []int{0, 1, 2, 3}, order, "公平锁应按 FIFO 入队顺序发锁")
}
