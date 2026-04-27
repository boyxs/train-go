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

	"github.com/webook/internal/integration/setup"
	"github.com/webook/pkg/redislockx"
)

// 分布式锁端到端集成测试 — 真 Redis 验证 SETNX 原子性、watchdog 续约、跨实例互斥。
// 跑前置：docker compose up redis；config/test.yaml 指向该 Redis。
type RedislockxSuite struct {
	suite.Suite
	cmd redis.Cmdable
}

func TestRedislockxIntegration(t *testing.T) {
	suite.Run(t, &RedislockxSuite{})
}

func (s *RedislockxSuite) SetupSuite() {
	s.cmd = setup.InitRedis()
}

func (s *RedislockxSuite) TearDownTest() {
	ctx := context.Background()
	keys, _ := s.cmd.Keys(ctx, "redislockx:integration:*").Result()
	if len(keys) > 0 {
		s.cmd.Del(ctx, keys...)
	}
}

// 100 goroutine 抢同一锁：真 Redis SETNX 原子性保证只 1 个赢。
// miniredis 是单连接共享，跨连接竞争只能用真 Redis 验。
func (s *RedislockxSuite) TestMutex_OnlyOneWins() {
	t := s.T()
	mutex := redislockx.NewClient(s.cmd)
	ctx := context.Background()
	key := "redislockx:integration:mutex"

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	var success, busy int32
	holders := make(chan redislockx.Lock, goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			lock, ok, err := mutex.TryLock(ctx, key, 5*time.Second, redislockx.WithoutWatchdog())
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

// 默认 watchdog（ttl/3）：等 3*TTL 锁仍存活，证明续约真发生。
// miniredis 不能验：虚拟时钟不会被 wall-clock sleep 推进。
func (s *RedislockxSuite) TestDefaultWatchdog_KeepsLockAlive() {
	t := s.T()
	mutex := redislockx.NewClient(s.cmd)
	ctx := context.Background()
	key := "redislockx:integration:watchdog"

	lock, ok, err := mutex.TryLock(ctx, key, 1*time.Second)
	require.NoError(t, err)
	require.True(t, ok)
	t.Cleanup(func() { _ = lock.Unlock(ctx) })

	time.Sleep(3 * time.Second)

	pttl, err := s.cmd.PTTL(ctx, key).Result()
	require.NoError(t, err)
	assert.Greater(t, pttl, time.Duration(0), "默认 watchdog 应保持锁存活")
	assert.LessOrEqual(t, pttl, 1*time.Second, "PTTL 不该超过 TTL")
}

// 显式 WithoutWatchdog：不续约，wall-clock 1.5s 后锁自然过期。
func (s *RedislockxSuite) TestWithoutWatchdog_NaturalExpire() {
	t := s.T()
	mutex := redislockx.NewClient(s.cmd)
	ctx := context.Background()
	key := "redislockx:integration:no_watchdog"

	lock, ok, err := mutex.TryLock(ctx, key, 1*time.Second, redislockx.WithoutWatchdog())
	require.NoError(t, err)
	require.True(t, ok)
	t.Cleanup(func() { _ = lock.Unlock(ctx) })

	time.Sleep(1500 * time.Millisecond)

	exists, err := s.cmd.Exists(ctx, key).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), exists, "WithoutWatchdog 后锁应自然过期")
}

// 跨 Client 实例互斥：模拟两个 K8s pod 抢同一锁，只一个赢。
func (s *RedislockxSuite) TestCrossClient_Mutex() {
	t := s.T()
	client1 := redislockx.NewClient(s.cmd)
	// 第二个实例独立连接，模拟另一进程；用 setup.InitRedis() 重新建连，
	// 避免类型断言 s.cmd.(*redis.Client) 在 InitRedis 改返回类型时静默失败
	rdb2 := setup.InitRedis()
	if closer, ok := rdb2.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	client2 := redislockx.NewClient(rdb2)

	ctx := context.Background()
	key := "redislockx:integration:cross"

	lock1, ok1, err := client1.TryLock(ctx, key, 5*time.Second, redislockx.WithoutWatchdog())
	require.NoError(t, err)
	require.True(t, ok1)
	t.Cleanup(func() { _ = lock1.Unlock(ctx) })

	_, ok2, err := client2.TryLock(ctx, key, 5*time.Second, redislockx.WithoutWatchdog())
	require.NoError(t, err)
	assert.False(t, ok2, "另一 Client 应抢不到（跨实例互斥）")
}

// Lock 阻塞模式：先占用，第二个 Lock 阻塞重试，前者 Unlock 后立即拿到。
func (s *RedislockxSuite) TestBlockingLock_HandsOff() {
	t := s.T()
	mutex := redislockx.NewClient(s.cmd)
	ctx := context.Background()
	key := "redislockx:integration:blocking"

	first, ok, err := mutex.TryLock(ctx, key, 5*time.Second, redislockx.WithoutWatchdog())
	require.NoError(t, err)
	require.True(t, ok)

	// 200ms 后释放，给 second 抢的机会
	go func() {
		time.Sleep(200 * time.Millisecond)
		_ = first.Unlock(ctx)
	}()

	bgCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	start := time.Now()
	second, err := mutex.Lock(bgCtx, key, 5*time.Second,
		redislockx.WithoutWatchdog(),
		redislockx.WithRetryInterval(50*time.Millisecond))
	elapsed := time.Since(start)

	require.NoError(t, err, "ctx 没超时应能拿到")
	require.NotNil(t, second)
	t.Cleanup(func() { _ = second.Unlock(ctx) })

	assert.GreaterOrEqual(t, elapsed, 150*time.Millisecond, "应至少阻塞到 first Unlock")
	assert.Less(t, elapsed, 1*time.Second, "first Unlock 后应及时拿到")
}
