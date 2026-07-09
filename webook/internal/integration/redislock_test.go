package integration

import (
	"context"
	"os"
	"strings"
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
// 跑前置：config/test.yaml 现指向 3 主集群（7001-7003），需本地起集群或 CI 预置；
// 不可达则整套 skip（见 SetupSuite），不硬失败。miniredis 单连接共享，跨连接竞争只能用真 Redis 验。
type RedislockSuite struct {
	suite.Suite
	cmd redis.UniversalClient
}

func TestRedislockIntegration(t *testing.T) {
	suite.Run(t, &RedislockSuite{})
}

func (s *RedislockSuite) SetupSuite() {
	s.cmd = setup.InitRedis()
	// config/test.yaml 指向 3 主集群，不可达则整套 skip（与 RedislockClusterSuite 同款守卫），
	// 避免本地/CI 无集群时硬失败。
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.cmd.Ping(ctx).Err(); err != nil {
		_ = s.cmd.Close()
		s.T().Skipf("跳过：Redis 不可达 %v（config/test.yaml 指向 3 主集群 7001-7003，需本地起集群或 CI 预置）", err)
	}
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

// RedislockClusterSuite 真 Redis 集群集成测（3 主）。集群不可达则整套 skip。
// 跑：REDISLOCK_REDIS_PASS=xxx go test ./internal/integration/ -run TestRedislockCluster
// 地址默认 127.0.0.1:7001-7003，可用 REDISLOCK_CLUSTER_ADDRS 覆盖（逗号分隔）。
type RedislockClusterSuite struct {
	suite.Suite
	cmd *redis.ClusterClient
}

func TestRedislockCluster(t *testing.T) {
	suite.Run(t, &RedislockClusterSuite{})
}

func (s *RedislockClusterSuite) SetupSuite() {
	addrs := []string{"127.0.0.1:7001", "127.0.0.1:7002", "127.0.0.1:7003"}
	if v := os.Getenv("REDISLOCK_CLUSTER_ADDRS"); v != "" {
		addrs = strings.Split(v, ",")
	}
	cmd := redis.NewClusterClient(&redis.ClusterOptions{Addrs: addrs, Password: os.Getenv("REDISLOCK_REDIS_PASS")})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := cmd.Ping(ctx).Err(); err != nil {
		_ = cmd.Close()
		s.T().Skipf("跳过：Redis 集群不可达 %v: %v（设 REDISLOCK_CLUSTER_ADDRS/REDISLOCK_REDIS_PASS 开启）", addrs, err)
	}
	s.cmd = cmd
}

func (s *RedislockClusterSuite) TearDownSuite() {
	if s.cmd != nil {
		_ = s.cmd.Close()
	}
}

// cleanup 删一把锁的全部 key（同槽，单次 Del 安全）。
func (s *RedislockClusterSuite) cleanup(k string) {
	p := "redislock:{" + k + "}:"
	s.cmd.Del(context.Background(), p+"lock", p+"fence", p+"ch", p+"queue", p+"qts")
}

// 多 key Lua 在集群不报 CROSSSLOT：release(lock+ch) / fencing(lock+fence) / fair(lock+queue+qts) 均成功。
func (s *RedislockClusterSuite) TestMultiKeyNoCrossSlot() {
	t := s.T()
	cli := redislock.NewClient(s.cmd)
	ctx := context.Background()

	lk, ok, err := cli.TryLock(ctx, "itcluster:basic", redislock.WithLeaseTime(10*time.Second))
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, lk.Unlock(ctx), "release(lock+ch) 多 key 不应 CROSSSLOT")
	s.cleanup("itcluster:basic")

	lf, ok, err := cli.TryLock(ctx, "itcluster:fence", redislock.WithLeaseTime(10*time.Second), redislock.WithFencing())
	require.NoError(t, err, "fencing(lock+fence) 多 key 不应 CROSSSLOT")
	require.True(t, ok)
	assert.Greater(t, lf.Fence(), int64(0))
	require.NoError(t, lf.Unlock(ctx))
	s.cleanup("itcluster:fence")

	fl, ok, err := cli.TryLock(ctx, "itcluster:fair", redislock.WithFair(), redislock.WithLeaseTime(10*time.Second))
	require.NoError(t, err, "fair(lock+queue+qts) 三 key 不应 CROSSSLOT")
	require.True(t, ok)
	require.NoError(t, fl.Unlock(ctx))
	s.cleanup("itcluster:fair")
}

// 集群下跨 goroutine 互斥：100 抢 1 赢。
func (s *RedislockClusterSuite) TestMutex() {
	t := s.T()
	cli := redislock.NewClient(s.cmd)
	ctx := context.Background()
	key := "itcluster:mutex"
	s.cleanup(key)

	const n = 100
	var success int32
	var wg sync.WaitGroup
	holders := make(chan redislock.RedisLock, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if lk, ok, err := cli.TryLock(ctx, key, redislock.WithLeaseTime(5*time.Second)); err == nil && ok {
				atomic.AddInt32(&success, 1)
				holders <- lk
			}
		}()
	}
	wg.Wait()
	close(holders)
	assert.Equal(t, int32(1), atomic.LoadInt32(&success), "集群下 100 抢锁应只 1 赢")
	for lk := range holders {
		_ = lk.Unlock(ctx)
	}
	s.cleanup(key)
}

// 集群下 pub/sub 阻塞唤醒：持有者释放后，阻塞 Lock 近即时拿到（retryInterval 设大排除轮询）。
func (s *RedislockClusterSuite) TestBlockingPubSubHandsOff() {
	t := s.T()
	cli := redislock.NewClient(s.cmd)
	ctx := context.Background()
	key := "itcluster:blocking"
	s.cleanup(key)

	first, ok, err := cli.TryLock(ctx, key, redislock.WithLeaseTime(30*time.Second))
	require.NoError(t, err)
	require.True(t, ok)
	go func() {
		time.Sleep(150 * time.Millisecond)
		_ = first.Unlock(ctx)
	}()

	bg, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	start := time.Now()
	second, err := cli.Lock(bg, key, redislock.WithLeaseTime(30*time.Second), redislock.WithRetryInterval(5*time.Second))
	elapsed := time.Since(start)
	require.NoError(t, err)
	require.NotNil(t, second)
	_ = second.Unlock(ctx)
	s.cleanup(key)
	assert.Less(t, elapsed, 2*time.Second, "集群 pub/sub 广播唤醒应近即时（非等 5s 轮询）")
}
