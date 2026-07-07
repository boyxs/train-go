package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"github.com/webook/relation/integration/setup"
	"github.com/webook/relation/repository/dao"
)

// RelationDAOSuite 真实 MySQL：直查 relation_follow/relation_stats/relation_block 三表断言，
// 隔离验证 DAO 的关注边翻转 + 计数维护 + 拉黑级联（计数事务/幂等只有真库测得出）。
type RelationDAOSuite struct {
	suite.Suite
	db  *gorm.DB
	dao dao.RelationDAO
}

func TestRelationDAO(t *testing.T) {
	suite.Run(t, &RelationDAOSuite{})
}

func (s *RelationDAOSuite) SetupSuite() {
	s.db = setup.InitDB()
	s.dao = dao.NewGormRelationDAO(s.db)
}

func (s *RelationDAOSuite) SetupTest()    { s.reset() }
func (s *RelationDAOSuite) TearDownTest() { s.reset() }

func (s *RelationDAOSuite) reset() {
	require.NoError(s.T(), s.db.Exec("TRUNCATE TABLE relation_follow").Error)
	require.NoError(s.T(), s.db.Exec("TRUNCATE TABLE relation_stats").Error)
	require.NoError(s.T(), s.db.Exec("TRUNCATE TABLE relation_block").Error)
}

// ── 直查断言辅助 ──────────────────────────────────────
func (s *RelationDAOSuite) followerCnt(uid int64) int64 { return s.stat(uid).FollowerCnt }
func (s *RelationDAOSuite) followeeCnt(uid int64) int64 { return s.stat(uid).FolloweeCnt }

func (s *RelationDAOSuite) stat(uid int64) dao.RelationStats {
	var st dao.RelationStats
	err := s.db.Where("uid = ?", uid).First(&st).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return dao.RelationStats{}
	}
	require.NoError(s.T(), err)
	return st
}

func (s *RelationDAOSuite) edgeStatus(follower, followee int64) (uint8, bool) {
	var e dao.FollowRelation
	err := s.db.Where("follower_id = ? AND followee_id = ?", follower, followee).First(&e).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false
	}
	require.NoError(s.T(), err)
	return e.Status, true
}

func (s *RelationDAOSuite) edgeRows(follower, followee int64) int64 {
	var n int64
	require.NoError(s.T(), s.db.Model(&dao.FollowRelation{}).
		Where("follower_id = ? AND followee_id = ?", follower, followee).Count(&n).Error)
	return n
}

// ── 用例：关注/取关计数核心 ───────────────────────────
func (s *RelationDAOSuite) TestFollow_FirstTime() {
	changed, err := s.dao.Follow(context.Background(), 1, 2)
	require.NoError(s.T(), err)
	assert.True(s.T(), changed, "首次关注应 changed=true")
	assert.Equal(s.T(), int64(1), s.followerCnt(2), "被关注者粉丝数=1")
	assert.Equal(s.T(), int64(1), s.followeeCnt(1), "关注者关注数=1")
	st, ok := s.edgeStatus(1, 2)
	assert.True(s.T(), ok)
	assert.Equal(s.T(), uint8(1), st, "关注边 status=1")
}

func (s *RelationDAOSuite) TestFollow_Idempotent() {
	_, err := s.dao.Follow(context.Background(), 1, 2)
	require.NoError(s.T(), err)
	changed, err := s.dao.Follow(context.Background(), 1, 2)
	require.NoError(s.T(), err)
	assert.False(s.T(), changed, "重复关注 changed=false")
	assert.Equal(s.T(), int64(1), s.followerCnt(2), "重复关注不重复计数")
}

func (s *RelationDAOSuite) TestUnfollow() {
	_, err := s.dao.Follow(context.Background(), 1, 2)
	require.NoError(s.T(), err)
	changed, err := s.dao.Unfollow(context.Background(), 1, 2)
	require.NoError(s.T(), err)
	assert.True(s.T(), changed)
	assert.Equal(s.T(), int64(0), s.followerCnt(2))
	assert.Equal(s.T(), int64(0), s.followeeCnt(1))
	st, _ := s.edgeStatus(1, 2)
	assert.Equal(s.T(), uint8(0), st, "取关后 status=0")
}

func (s *RelationDAOSuite) TestUnfollow_NotFollowing_Idempotent() {
	changed, err := s.dao.Unfollow(context.Background(), 1, 2)
	require.NoError(s.T(), err)
	assert.False(s.T(), changed)
	assert.Equal(s.T(), int64(0), s.followerCnt(2), "未关注取关不产生负计数")
}

func (s *RelationDAOSuite) TestRefollow_ReusesRow() {
	_, err := s.dao.Follow(context.Background(), 1, 2)
	require.NoError(s.T(), err)
	_, err = s.dao.Unfollow(context.Background(), 1, 2)
	require.NoError(s.T(), err)
	changed, err := s.dao.Follow(context.Background(), 1, 2)
	require.NoError(s.T(), err)
	assert.True(s.T(), changed, "取关后再关注 changed=true")
	assert.Equal(s.T(), int64(1), s.edgeRows(1, 2), "复用原行，不新建")
	assert.Equal(s.T(), int64(1), s.followerCnt(2))
}

func (s *RelationDAOSuite) TestFollow_MultiFollower() {
	_, err := s.dao.Follow(context.Background(), 1, 3)
	require.NoError(s.T(), err)
	_, err = s.dao.Follow(context.Background(), 2, 3)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(2), s.followerCnt(3), "两人关注 → 粉丝数=2")
}

func (s *RelationDAOSuite) isBlocked(uid, blockedUid int64) bool {
	var n int64
	require.NoError(s.T(), s.db.Model(&dao.BlockRelation{}).
		Where("uid = ? AND blocked_uid = ?", uid, blockedUid).Count(&n).Error)
	return n > 0
}

// ── 用例：拉黑级联 ─────────────────────────────────────
func (s *RelationDAOSuite) TestBlock_CascadeMutual() {
	ctx := context.Background()
	_, err := s.dao.Follow(ctx, 1, 2)
	require.NoError(s.T(), err)
	_, err = s.dao.Follow(ctx, 2, 1)
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(1), s.followerCnt(2), "前置：互关")

	changed, err := s.dao.Block(ctx, 1, 2)
	require.NoError(s.T(), err)
	assert.True(s.T(), changed)
	assert.True(s.T(), s.isBlocked(1, 2), "黑名单行已建")
	st12, _ := s.edgeStatus(1, 2)
	st21, _ := s.edgeStatus(2, 1)
	assert.Equal(s.T(), uint8(0), st12, "1→2 关注解除")
	assert.Equal(s.T(), uint8(0), st21, "2→1 关注解除")
	assert.Equal(s.T(), int64(0), s.followeeCnt(1))
	assert.Equal(s.T(), int64(0), s.followerCnt(2))
	assert.Equal(s.T(), int64(0), s.followeeCnt(2))
	assert.Equal(s.T(), int64(0), s.followerCnt(1))
}

func (s *RelationDAOSuite) TestBlock_OneWayFollow() {
	ctx := context.Background()
	_, err := s.dao.Follow(ctx, 1, 2) // 仅 1 关注 2
	require.NoError(s.T(), err)
	changed, err := s.dao.Block(ctx, 1, 2)
	require.NoError(s.T(), err)
	assert.True(s.T(), changed)
	assert.True(s.T(), s.isBlocked(1, 2))
	assert.Equal(s.T(), int64(0), s.followeeCnt(1), "拉黑解除 1→2 关注")
	assert.Equal(s.T(), int64(0), s.followerCnt(2))
}

func (s *RelationDAOSuite) TestBlock_NoFollow() {
	changed, err := s.dao.Block(context.Background(), 1, 2)
	require.NoError(s.T(), err)
	assert.True(s.T(), changed, "无关注关系也能拉黑")
	assert.True(s.T(), s.isBlocked(1, 2))
}

func (s *RelationDAOSuite) TestBlock_Idempotent() {
	ctx := context.Background()
	_, err := s.dao.Block(ctx, 1, 2)
	require.NoError(s.T(), err)
	changed, err := s.dao.Block(ctx, 1, 2)
	require.NoError(s.T(), err)
	assert.False(s.T(), changed, "重复拉黑 changed=false")
}

func (s *RelationDAOSuite) TestUnblock() {
	ctx := context.Background()
	_, err := s.dao.Follow(ctx, 1, 2)
	require.NoError(s.T(), err)
	_, err = s.dao.Block(ctx, 1, 2) // 解除 1→2 关注
	require.NoError(s.T(), err)

	changed, err := s.dao.Unblock(ctx, 1, 2)
	require.NoError(s.T(), err)
	assert.True(s.T(), changed)
	assert.False(s.T(), s.isBlocked(1, 2), "黑名单行已删")
	st, _ := s.edgeStatus(1, 2)
	assert.Equal(s.T(), uint8(0), st, "取消拉黑不恢复关注")
	assert.Equal(s.T(), int64(0), s.followeeCnt(1))
}

func (s *RelationDAOSuite) TestUnblock_NotBlocked() {
	changed, err := s.dao.Unblock(context.Background(), 1, 2)
	require.NoError(s.T(), err)
	assert.False(s.T(), changed, "未拉黑取消 changed=false")
}

// ── 用例：读方法 ───────────────────────────────────────
func (s *RelationDAOSuite) TestGetStats() {
	ctx := context.Background()
	_, _ = s.dao.Follow(ctx, 1, 2)
	_, _ = s.dao.Follow(ctx, 3, 2) // 2 有 2 粉丝
	_, _ = s.dao.Follow(ctx, 2, 9) // 2 关注 1 人
	st, err := s.dao.GetStats(ctx, 2)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(2), st.FollowerCnt)
	assert.Equal(s.T(), int64(1), st.FolloweeCnt)
}

func (s *RelationDAOSuite) TestGetStats_Unknown() {
	st, err := s.dao.GetStats(context.Background(), 999)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(0), st.FollowerCnt)
	assert.Equal(s.T(), int64(0), st.FolloweeCnt)
}

func (s *RelationDAOSuite) TestBatchGetStats() {
	ctx := context.Background()
	_, _ = s.dao.Follow(ctx, 1, 2)
	_, _ = s.dao.Follow(ctx, 1, 3)
	got, err := s.dao.BatchGetStats(ctx, []int64{2, 3, 999})
	require.NoError(s.T(), err)
	require.Len(s.T(), got, 2, "999 无记录不返回")
	m := make(map[int64]dao.RelationStats, len(got))
	for _, st := range got {
		m[st.Uid] = st
	}
	assert.Equal(s.T(), int64(1), m[2].FollowerCnt)
	assert.Equal(s.T(), int64(1), m[3].FollowerCnt)
}

func (s *RelationDAOSuite) TestListFollowees_CursorDesc() {
	ctx := context.Background()
	for _, fe := range []int64{2, 3, 4} {
		_, err := s.dao.Follow(ctx, 1, fe)
		require.NoError(s.T(), err)
	}
	_, _ = s.dao.Unfollow(ctx, 1, 3) // 取关 3，不入列表
	page1, err := s.dao.ListFollowees(ctx, 1, 0, 2)
	require.NoError(s.T(), err)
	require.Len(s.T(), page1, 2)
	assert.Equal(s.T(), int64(4), page1[0].FolloweeId, "id DESC 最新在前")
	assert.Equal(s.T(), int64(2), page1[1].FolloweeId)
	page2, err := s.dao.ListFollowees(ctx, 1, page1[1].Id, 2)
	require.NoError(s.T(), err)
	assert.Empty(s.T(), page2, "游标续拉无更多")
}

func (s *RelationDAOSuite) TestListFollowers() {
	ctx := context.Background()
	_, _ = s.dao.Follow(ctx, 5, 1)
	_, _ = s.dao.Follow(ctx, 6, 1)
	list, err := s.dao.ListFollowers(ctx, 1, 0, 10)
	require.NoError(s.T(), err)
	require.Len(s.T(), list, 2)
	assert.Equal(s.T(), int64(6), list[0].FollowerId, "id DESC")
}

func (s *RelationDAOSuite) TestListBlocks() {
	ctx := context.Background()
	_, _ = s.dao.Block(ctx, 1, 7)
	_, _ = s.dao.Block(ctx, 1, 8)
	list, err := s.dao.ListBlocks(ctx, 1, 0, 10)
	require.NoError(s.T(), err)
	require.Len(s.T(), list, 2)
	assert.Equal(s.T(), int64(8), list[0].BlockedUid, "id DESC")
}

func (s *RelationDAOSuite) TestFindFolloweesIn() {
	ctx := context.Background()
	_, _ = s.dao.Follow(ctx, 1, 2)
	_, _ = s.dao.Follow(ctx, 1, 3)
	_, _ = s.dao.Follow(ctx, 1, 4)
	_, _ = s.dao.Unfollow(ctx, 1, 4) // 取关的不算
	got, err := s.dao.FindFolloweesIn(ctx, 1, []int64{2, 3, 4, 5})
	require.NoError(s.T(), err)
	assert.ElementsMatch(s.T(), []int64{2, 3}, got)
}

func (s *RelationDAOSuite) TestFindFollowersIn() {
	ctx := context.Background()
	_, _ = s.dao.Follow(ctx, 2, 1)
	_, _ = s.dao.Follow(ctx, 3, 1)
	got, err := s.dao.FindFollowersIn(ctx, 1, []int64{2, 3, 4})
	require.NoError(s.T(), err)
	assert.ElementsMatch(s.T(), []int64{2, 3}, got)
}

func (s *RelationDAOSuite) TestFindBlockedIn_And_By() {
	ctx := context.Background()
	_, _ = s.dao.Block(ctx, 1, 2) // 1 拉黑 2
	_, _ = s.dao.Block(ctx, 3, 1) // 3 拉黑 1
	blocked, err := s.dao.FindBlockedIn(ctx, 1, []int64{2, 3})
	require.NoError(s.T(), err)
	assert.ElementsMatch(s.T(), []int64{2}, blocked, "1 拉黑了 2")
	blockedBy, err := s.dao.FindBlockedByIn(ctx, 1, []int64{2, 3})
	require.NoError(s.T(), err)
	assert.ElementsMatch(s.T(), []int64{3}, blockedBy, "3 拉黑了 1")
}
