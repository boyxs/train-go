package integration

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"github.com/webook/pkg/logger"
	"github.com/webook/relation/domain"
	"github.com/webook/relation/integration/setup"
	"github.com/webook/relation/repository"
	"github.com/webook/relation/repository/cache"
	"github.com/webook/relation/repository/dao"
)

// RelationRepoSuite 真实 MySQL + Redis：验证 repository 的 Cache-Aside（stats 计数缓存 + 写后失效）与 domain 转换。
type RelationRepoSuite struct {
	suite.Suite
	db   *gorm.DB
	cmd  redis.Cmdable
	repo repository.RelationRepository
}

func TestRelationRepo(t *testing.T) {
	suite.Run(t, &RelationRepoSuite{})
}

func (s *RelationRepoSuite) SetupSuite() {
	s.db = setup.InitDB()
	s.cmd = setup.InitRedis()
	s.repo = repository.NewCacheRelationRepository(
		dao.NewGormRelationDAO(s.db),
		cache.NewRedisRelationCache(s.cmd),
		logger.NewNopLogger(),
	)
}

func (s *RelationRepoSuite) SetupTest()    { s.reset() }
func (s *RelationRepoSuite) TearDownTest() { s.reset() }

func (s *RelationRepoSuite) reset() {
	require.NoError(s.T(), s.db.Exec("TRUNCATE TABLE relation_follow").Error)
	require.NoError(s.T(), s.db.Exec("TRUNCATE TABLE relation_stats").Error)
	require.NoError(s.T(), s.db.Exec("TRUNCATE TABLE relation_block").Error)
	require.NoError(s.T(), s.cmd.FlushDB(context.Background()).Err())
}

func (s *RelationRepoSuite) mustStats(ctx context.Context, uid int64) domain.RelationStats {
	st, err := s.repo.GetStats(ctx, uid)
	require.NoError(s.T(), err)
	return st
}

// GetStats Cache-Aside：首次回源并回填；绕过 repo 直改 DB 后再查仍返缓存旧值。
func (s *RelationRepoSuite) TestGetStats_CacheAside() {
	ctx := context.Background()
	_, err := s.repo.Follow(ctx, 1, 2)
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(1), s.mustStats(ctx, 2).FollowerCnt, "回源得 1 并回填缓存")

	require.NoError(s.T(), s.db.Exec("UPDATE relation_stats SET follower_cnt = 99 WHERE uid = 2").Error)
	assert.Equal(s.T(), int64(1), s.mustStats(ctx, 2).FollowerCnt, "返回缓存旧值，证明走了缓存")
}

// 取关清缓存：Unfollow 后再查走 DB 得新值 0。
func (s *RelationRepoSuite) TestWrite_InvalidatesCache() {
	ctx := context.Background()
	_, err := s.repo.Follow(ctx, 1, 2)
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(1), s.mustStats(ctx, 2).FollowerCnt) // 预热

	_, err = s.repo.Unfollow(ctx, 1, 2)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(0), s.mustStats(ctx, 2).FollowerCnt, "取关清缓存，返回 DB 新值 0")
}

// 拉黑级联清双方缓存。
func (s *RelationRepoSuite) TestBlock_InvalidatesBothSides() {
	ctx := context.Background()
	_, _ = s.repo.Follow(ctx, 1, 2)
	_, _ = s.repo.Follow(ctx, 2, 1)
	require.Equal(s.T(), int64(1), s.mustStats(ctx, 1).FollowerCnt) // 预热 1
	require.Equal(s.T(), int64(1), s.mustStats(ctx, 2).FollowerCnt) // 预热 2

	_, err := s.repo.Block(ctx, 1, 2)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(0), s.mustStats(ctx, 1).FollowerCnt, "拉黑清双方缓存")
	assert.Equal(s.T(), int64(0), s.mustStats(ctx, 2).FollowerCnt)
}

// ListFollowees 转 domain.FollowEdge（id DESC + 字段映射）。
func (s *RelationRepoSuite) TestListFollowees_Conversion() {
	ctx := context.Background()
	_, _ = s.repo.Follow(ctx, 1, 2)
	_, _ = s.repo.Follow(ctx, 1, 3)
	list, err := s.repo.ListFollowees(ctx, 1, 0, 10)
	require.NoError(s.T(), err)
	require.Len(s.T(), list, 2)
	assert.Equal(s.T(), int64(3), list[0].FolloweeId, "id DESC 最新在前")
	assert.Equal(s.T(), int64(1), list[0].FollowerId)
	assert.True(s.T(), list[0].CreatedAt > 0, "CreatedAt 已映射")
}
