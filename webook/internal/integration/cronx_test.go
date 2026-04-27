package integration

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/webook/internal/integration/setup"
	"github.com/webook/pkg/cronx"
	cronprom "github.com/webook/pkg/cronx/prometheus"
	"github.com/webook/pkg/logger"
	"github.com/webook/pkg/redislockx"
)

// cron + Wrapper + 真锁端到端 — 验多实例下"只一个跑"、长任务靠 watchdog 续约跑完。
type CronxSuite struct {
	suite.Suite
	cmd redis.Cmdable
}

func TestCronxIntegration(t *testing.T) {
	suite.Run(t, &CronxSuite{})
}

func (s *CronxSuite) SetupSuite() {
	s.cmd = setup.InitRedis()
}

func (s *CronxSuite) TearDownTest() {
	ctx := context.Background()
	keys, _ := s.cmd.Keys(ctx, "cronx:lock:integration_*").Result()
	if len(keys) > 0 {
		s.cmd.Del(ctx, keys...)
	}
}

func (s *CronxSuite) newWrapper() *cronx.Wrapper {
	reg := prometheus.NewRegistry()
	metrics := cronprom.NewPrometheusBuilder("test", "cron", "test").Registry(reg).Build()
	return cronx.NewWrapper(redislockx.NewClient(s.cmd), metrics, logger.NewNopLogger())
}

// 3 个 Wrapper 实例（模拟 3 K8s pod）同时 Wrap 相同 task name + 同一刻调用，
// 真 Redis 锁互斥保证业务回调只跑 1 次。
func (s *CronxSuite) TestMultiWrapper_OnlyOneRuns() {
	t := s.T()
	const podCount = 3
	const taskName = "integration_multi"

	var ran int32
	fn := func(ctx context.Context, date string) error {
		atomic.AddInt32(&ran, 1)
		time.Sleep(200 * time.Millisecond) // 业务跑一会，给其他实例时间抢锁
		return nil
	}

	cbs := make([]func(), podCount)
	for i := 0; i < podCount; i++ {
		cbs[i] = s.newWrapper().Wrap(taskName, 5*time.Second, fn)
	}

	// 三个 pod 同时触发同一 tick — 用 WaitGroup 同步开跑
	var start sync.WaitGroup
	start.Add(1)
	var done sync.WaitGroup
	done.Add(podCount)
	for _, cb := range cbs {
		go func(cb func()) {
			defer done.Done()
			start.Wait()
			cb()
		}(cb)
	}
	start.Done()
	done.Wait()

	assert.Equal(t, int32(1), atomic.LoadInt32(&ran),
		"3 pod 同时触发同一 task，业务回调应只跑 1 次")
}

// 业务跑超过单次 lockTTL：默认 watchdog（ttl/3）必须真续约，业务才能跑完。
// 关键断言：在 wall-clock 过原 TTL 后查 key 仍存在 — 这才区分 watchdog ON vs OFF：
//
//	watchdog ON：续约把 TTL 推到 now+1s，1.5s 时 key 还在 ✓
//	watchdog OFF：1s 后 key 自然过期 → 1.5s 探测会 false → 测试红
func (s *CronxSuite) TestLongTask_WatchdogKeepsLockAlive() {
	t := s.T()
	w := cronx.NewWrapper(
		redislockx.NewClient(s.cmd),
		cronprom.NewPrometheusBuilder("test", "cron", "test").Registry(prometheus.NewRegistry()).Build(),
		logger.NewNopLogger(),
		cronx.WithLockTTL(1*time.Second), // 故意短 TTL；watchdog 333ms 续约
	)

	var ran int32
	var aliveAfterTTL int64 // 0=未探测，>0=探测时仍存在
	fn := func(ctx context.Context, date string) error {
		atomic.AddInt32(&ran, 1)
		time.Sleep(1500 * time.Millisecond) // 业务先跑过原 TTL=1s
		// 此刻 wall-clock=1.5s 已过原 TTL，watchdog 须续上 key 才能存在
		exists, _ := s.cmd.Exists(context.Background(), "cronx:lock:integration_long").Result()
		atomic.StoreInt64(&aliveAfterTTL, exists)
		time.Sleep(1000 * time.Millisecond) // 再跑 1s 验证持续续约
		return nil
	}
	cb := w.Wrap("integration_long", 5*time.Second, fn)
	cb()

	require.Equal(t, int32(1), atomic.LoadInt32(&ran), "业务应被调一次")
	assert.Equal(t, int64(1), atomic.LoadInt64(&aliveAfterTTL),
		"过原 TTL 后 key 必须仍存在，证明 watchdog 续上了（OFF 时 key 已自然过期）")

	// 业务跑完后锁应已释放（cronx.release 用独立 ctx 调 Unlock）
	exists, err := s.cmd.Exists(context.Background(), "cronx:lock:integration_long").Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), exists, "业务跑完 + release 后 lock key 应消失")
}

// cron 触发 + Wrapper + 真锁端到端：cron 周期触发，业务回调被多次 Wrap 调用。
func (s *CronxSuite) TestCron_Triggers_WrappedTask() {
	t := s.T()
	w := s.newWrapper()

	var ran int32
	fn := func(ctx context.Context, date string) error {
		atomic.AddInt32(&ran, 1)
		return nil
	}

	c := cron.New(cron.WithSeconds())
	_, err := c.AddFunc("*/1 * * * * *", w.Wrap("integration_cron", 5*time.Second, fn))
	require.NoError(t, err)
	c.Start()
	t.Cleanup(func() { <-c.Stop().Done() })

	// 等 cron 至少触发 2 次
	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&ran) >= 2
	}, 4*time.Second, 100*time.Millisecond, "cron 应在 4s 内触发任务 ≥2 次")
}

// graceful shutdown：cron.Stop().Done() 应等 in-flight 业务跑完才返。
func (s *CronxSuite) TestGracefulShutdown_WaitsInFlight() {
	t := s.T()
	w := s.newWrapper()

	taskStarted := make(chan struct{})
	taskDone := make(chan struct{})
	var startedOnce, doneOnce sync.Once // 防御 cron 偶发二次触发时 close 已关 chan panic
	fn := func(ctx context.Context, date string) error {
		startedOnce.Do(func() { close(taskStarted) })
		time.Sleep(800 * time.Millisecond) // in-flight 任务跑 800ms
		doneOnce.Do(func() { close(taskDone) })
		return nil
	}

	c := cron.New(cron.WithSeconds())
	_, err := c.AddFunc("*/1 * * * * *", w.Wrap("integration_shutdown", 5*time.Second, fn))
	require.NoError(t, err)
	c.Start()

	// 等任务真启动了再 Stop
	select {
	case <-taskStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("等 cron 触发首次任务超时")
	}

	stopStart := time.Now()
	<-c.Stop().Done()
	elapsed := time.Since(stopStart)

	// Stop 应等 taskDone 关闭后才返（in-flight 任务跑完）
	select {
	case <-taskDone:
		// ok
	default:
		t.Fatal("Stop 应等 in-flight 任务跑完")
	}

	assert.Greater(t, elapsed, 100*time.Millisecond, "Stop 应至少阻塞到 in-flight 跑完")
}
