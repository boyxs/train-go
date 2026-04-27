package integration

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/webook/internal/integration/setup"
)

// RankingCron 端到端 wire-up + 生命周期：cron 注册 + 启动 + graceful shutdown。
// 不等真实 cron 触发（最快 hot 任务每分钟），仅验装配链路 + cleanup 立即返回。
type RankingCronSuite struct {
	suite.Suite
	cmd redis.Cmdable
}

func TestRankingCron(t *testing.T) {
	suite.Run(t, &RankingCronSuite{})
}

func (s *RankingCronSuite) SetupSuite() {
	s.cmd = setup.InitRedis()
}

// 防御：如果测试运行时正好命中 minute 0 cron 触发，ranking_* lock key 可能 leak；
// 跨测试清一次保证幂等
func (s *RankingCronSuite) TearDownTest() {
	ctx := context.Background()
	keys, _ := s.cmd.Keys(ctx, "cronx:lock:ranking_*").Result()
	if len(keys) > 0 {
		s.cmd.Del(ctx, keys...)
	}
}

func (s *RankingCronSuite) TestInitRankingCron_RegistersFourEntries() {
	t := s.T()
	c, cleanup := setup.InitRankingCron()
	require.NotNil(t, c)
	require.NotNil(t, cleanup)
	t.Cleanup(cleanup)

	// hot / best / new / archive 四个任务全部注册
	assert.Len(t, c.Entries(), 4, "RankingJob.RegisterTo 应注册 4 个 entry")
}

// 无 in-flight 时 cleanup 应立即返回（毫秒级），不会卡死。
func (s *RankingCronSuite) TestCleanup_ReturnsImmediately_WhenIdle() {
	t := s.T()
	_, cleanup := setup.InitRankingCron()

	done := make(chan struct{})
	start := time.Now()
	go func() {
		cleanup()
		close(done)
	}()

	select {
	case <-done:
		elapsed := time.Since(start)
		assert.Less(t, elapsed, 2*time.Second,
			"无 in-flight 时 cleanup 应快速返回，实际耗时 %v", elapsed)
	case <-time.After(5 * time.Second):
		t.Fatal("cleanup 卡死超过 5s")
	}
}
