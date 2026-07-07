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
	relationerrs "github.com/webook/relation/errs"
	"github.com/webook/relation/integration/setup"
	"github.com/webook/relation/repository"
	"github.com/webook/relation/repository/cache"
	"github.com/webook/relation/repository/dao"
	"github.com/webook/relation/service"
)

// RelationServiceSuite 真实栈：验证 service 的业务校验（自关注/自拉黑/双向拉黑门控）与关系态组装。
type RelationServiceSuite struct {
	suite.Suite
	db  *gorm.DB
	cmd redis.Cmdable
	svc service.RelationService
}

func TestRelationService(t *testing.T) {
	suite.Run(t, &RelationServiceSuite{})
}

func (s *RelationServiceSuite) SetupSuite() {
	s.db = setup.InitDB()
	s.cmd = setup.InitRedis()
	repo := repository.NewCacheRelationRepository(
		dao.NewGormRelationDAO(s.db),
		cache.NewRedisRelationCache(s.cmd),
		logger.NewNopLogger(),
	)
	s.svc = service.NewInternalRelationService(repo)
}

func (s *RelationServiceSuite) SetupTest()    { s.reset() }
func (s *RelationServiceSuite) TearDownTest() { s.reset() }

func (s *RelationServiceSuite) reset() {
	require.NoError(s.T(), s.db.Exec("TRUNCATE TABLE relation_follow").Error)
	require.NoError(s.T(), s.db.Exec("TRUNCATE TABLE relation_stats").Error)
	require.NoError(s.T(), s.db.Exec("TRUNCATE TABLE relation_block").Error)
	require.NoError(s.T(), s.cmd.FlushDB(context.Background()).Err())
}

func (s *RelationServiceSuite) TestFollow_Self() {
	_, err := s.svc.Follow(context.Background(), 1, 1)
	assert.ErrorIs(s.T(), err, relationerrs.ErrFollowSelf)
}

func (s *RelationServiceSuite) TestFollow_BlockedTarget() {
	ctx := context.Background()
	_, err := s.svc.Block(ctx, 1, 2) // 1 拉黑 2
	require.NoError(s.T(), err)
	_, err = s.svc.Follow(ctx, 1, 2) // 1 想关注已被自己拉黑的 2
	assert.ErrorIs(s.T(), err, relationerrs.ErrBlockedTarget)
}

func (s *RelationServiceSuite) TestFollow_BlockedByTarget() {
	ctx := context.Background()
	_, err := s.svc.Block(ctx, 2, 1) // 2 拉黑 1
	require.NoError(s.T(), err)
	_, err = s.svc.Follow(ctx, 1, 2) // 1 想关注拉黑了自己的 2
	assert.ErrorIs(s.T(), err, relationerrs.ErrBlockedByTarget)
}

func (s *RelationServiceSuite) TestFollow_OK() {
	changed, err := s.svc.Follow(context.Background(), 1, 2)
	require.NoError(s.T(), err)
	assert.True(s.T(), changed)
}

func (s *RelationServiceSuite) TestBlock_Self() {
	_, err := s.svc.Block(context.Background(), 1, 1)
	assert.ErrorIs(s.T(), err, relationerrs.ErrBlockSelf)
}

func (s *RelationServiceSuite) TestGetRelation() {
	ctx := context.Background()
	_, _ = s.svc.Follow(ctx, 1, 2) // 1→2
	_, _ = s.svc.Follow(ctx, 2, 1) // 2→1（互关）
	_, _ = s.svc.Block(ctx, 1, 3)  // 1 拉黑 3
	_, _ = s.svc.Block(ctx, 4, 1)  // 4 拉黑 1

	m, err := s.svc.GetRelation(ctx, 1, []int64{2, 3, 4, 5})
	require.NoError(s.T(), err)

	assert.True(s.T(), m[2].IsFollowing)
	assert.True(s.T(), m[2].IsFollowedBy)
	assert.True(s.T(), m[2].IsMutual(), "1、2 互关")

	assert.True(s.T(), m[3].IsBlocked, "1 拉黑了 3")
	assert.False(s.T(), m[3].IsFollowing)

	assert.True(s.T(), m[4].IsBlockedBy, "4 拉黑了 1")

	assert.Equal(s.T(), domain.RelationState{}, m[5], "无任何关系 → 全 false")
}
