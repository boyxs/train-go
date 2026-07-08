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

// Lock 阻塞模式：先占用，第二个 Lock 阻塞重试，前者 Unlock 后立即拿到。
func (s *RedislockSuite) TestBlockingLock_HandsOff() {
	t := s.T()
	cli := redislock.NewClient(s.cmd)
	ctx := context.Background()
	key := "itlock:blocking"

	first, ok, err := cli.TryLock(ctx, key, redislock.WithLeaseTime(5*time.Second))
	require.NoError(t, err)
	require.True(t, ok)

	go func() {
		time.Sleep(200 * time.Millisecond)
		_ = first.Unlock(ctx)
	}()

	bgCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	start := time.Now()
	second, err := cli.Lock(bgCtx, key,
		redislock.WithLeaseTime(5*time.Second),
		redislock.WithRetryInterval(50*time.Millisecond))
	elapsed := time.Since(start)

	require.NoError(t, err, "ctx 没超时应能拿到")
	require.NotNil(t, second)
	t.Cleanup(func() { _ = second.Unlock(ctx) })

	assert.GreaterOrEqual(t, elapsed, 150*time.Millisecond, "应至少阻塞到 first Unlock")
	assert.Less(t, elapsed, 1*time.Second, "first Unlock 后应及时拿到")
}
