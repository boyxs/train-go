package integration

import (
	"context"
	"net"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"gorm.io/gorm"

	relationv1 "github.com/boyxs/train-go/webook/api/gen/relation/v1"
	"github.com/boyxs/train-go/webook/pkg/grpcx/interceptor/errconv"
	relationerrs "github.com/boyxs/train-go/webook/relation/errs"
	relationgrpc "github.com/boyxs/train-go/webook/relation/grpc"
	"github.com/boyxs/train-go/webook/relation/integration/setup"
)

const bufSize = 1024 * 1024

// RelationServerSuite 真实 MySQL + Redis + bufconn gRPC：经 RelationServiceClient 发真实请求，
// 打通 gRPC → service → repository → dao/cache → DB/Redis 全链路。目录架构对齐 interaction/comment integration。
type RelationServerSuite struct {
	suite.Suite
	db     *gorm.DB
	cmd    redis.Cmdable
	conn   *grpc.ClientConn
	srv    *grpc.Server
	client relationv1.RelationServiceClient
}

func TestRelationServer(t *testing.T) {
	suite.Run(t, &RelationServerSuite{})
}

func (s *RelationServerSuite) SetupSuite() {
	s.db = setup.InitDB()
	s.cmd = setup.InitRedis()

	// bufconn 内存 gRPC server，装 errconv + 校验拦截器（与生产 wire 一致）
	lis := bufconn.Listen(bufSize)
	s.srv = grpc.NewServer(grpc.ChainUnaryInterceptor(
		errconv.UnaryServerInterceptor(nil),
		relationgrpc.ValidateUnaryInterceptor,
	))
	relationv1.RegisterRelationServiceServer(s.srv, setup.InitRelationServer())
	go func() { _ = s.srv.Serve(lis) }()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(errconv.UnaryClientInterceptor()),
	)
	require.NoError(s.T(), err)
	s.conn = conn
	s.client = relationv1.NewRelationServiceClient(conn)
}

func (s *RelationServerSuite) TearDownSuite() {
	_ = s.conn.Close()
	s.srv.GracefulStop()
}

func (s *RelationServerSuite) SetupTest()    { s.reset() }
func (s *RelationServerSuite) TearDownTest() { s.reset() }
func (s *RelationServerSuite) reset() {
	require.NoError(s.T(), s.db.Exec("TRUNCATE TABLE relation_follow").Error)
	require.NoError(s.T(), s.db.Exec("TRUNCATE TABLE relation_stats").Error)
	require.NoError(s.T(), s.db.Exec("TRUNCATE TABLE relation_block").Error)
	require.NoError(s.T(), s.cmd.FlushDB(context.Background()).Err())
}

func (s *RelationServerSuite) follow(follower, followee int64) *relationv1.FollowResponse {
	resp, err := s.client.Follow(context.Background(), &relationv1.FollowRequest{FollowerId: follower, FolloweeId: followee})
	require.NoError(s.T(), err)
	return resp
}

func (s *RelationServerSuite) block(uid, blocked int64) {
	_, err := s.client.Block(context.Background(), &relationv1.BlockRequest{Uid: uid, BlockedId: blocked})
	require.NoError(s.T(), err)
}

// Follow：自关注拒绝
func (s *RelationServerSuite) TestFollow_Self() {
	_, err := s.client.Follow(context.Background(), &relationv1.FollowRequest{FollowerId: 1, FolloweeId: 1})
	assert.ErrorIs(s.T(), err, relationerrs.ErrFollowSelf)
}

// Follow：想关注已被自己拉黑的人 → 拒绝
func (s *RelationServerSuite) TestFollow_BlockedTarget() {
	s.block(1, 2)
	_, err := s.client.Follow(context.Background(), &relationv1.FollowRequest{FollowerId: 1, FolloweeId: 2})
	assert.ErrorIs(s.T(), err, relationerrs.ErrBlockedTarget)
}

// Follow：想关注拉黑了自己的人 → 拒绝
func (s *RelationServerSuite) TestFollow_BlockedByTarget() {
	s.block(2, 1)
	_, err := s.client.Follow(context.Background(), &relationv1.FollowRequest{FollowerId: 1, FolloweeId: 2})
	assert.ErrorIs(s.T(), err, relationerrs.ErrBlockedByTarget)
}

// Follow：首次真翻转 changed=true，重复 changed=false（幂等不重复计数）
func (s *RelationServerSuite) TestFollow_OKThenIdempotent() {
	assert.True(s.T(), s.follow(1, 2).GetChanged(), "首次关注真翻转")
	assert.False(s.T(), s.follow(1, 2).GetChanged(), "重复关注不再翻转")
}

// Unfollow：真取关 changed=true，重复 changed=false
func (s *RelationServerSuite) TestUnfollow() {
	s.follow(1, 2)
	un, err := s.client.Unfollow(context.Background(), &relationv1.FollowRequest{FollowerId: 1, FolloweeId: 2})
	require.NoError(s.T(), err)
	assert.True(s.T(), un.GetChanged())
	un, err = s.client.Unfollow(context.Background(), &relationv1.FollowRequest{FollowerId: 1, FolloweeId: 2})
	require.NoError(s.T(), err)
	assert.False(s.T(), un.GetChanged())
}

// Block：自拉黑拒绝
func (s *RelationServerSuite) TestBlock_Self() {
	_, err := s.client.Block(context.Background(), &relationv1.BlockRequest{Uid: 1, BlockedId: 1})
	assert.ErrorIs(s.T(), err, relationerrs.ErrBlockSelf)
}

// Block：级联解除双向关注（1↔2 互关 → 1 拉黑 2 后，1 不再关注 2）
func (s *RelationServerSuite) TestBlock_CascadeUnfollow() {
	s.follow(1, 2)
	s.follow(2, 1)
	s.block(1, 2)
	resp, err := s.client.GetRelation(context.Background(), &relationv1.GetRelationRequest{ViewerId: 1, TargetIds: []int64{2}})
	require.NoError(s.T(), err)
	assert.False(s.T(), resp.GetStates()[2].GetIsFollowing(), "拉黑级联解除关注")
	assert.True(s.T(), resp.GetStates()[2].GetIsBlocked(), "1 拉黑了 2")
}

// GetStats：关注计数（followee/follower）
func (s *RelationServerSuite) TestGetStats() {
	s.follow(1, 2)
	s.follow(1, 3)
	s.follow(4, 1)
	resp, err := s.client.GetStats(context.Background(), &relationv1.GetStatsRequest{Uid: 1})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(2), resp.GetStats().GetFolloweeCnt(), "1 关注了 2、3")
	assert.Equal(s.T(), int64(1), resp.GetStats().GetFollowerCnt(), "4 关注了 1")
}

// GetRelation：互关 / 拉黑 / 被拉黑 / 无关系 四态
func (s *RelationServerSuite) TestGetRelation() {
	s.follow(1, 2)
	s.follow(2, 1) // 1、2 互关
	s.block(1, 3)  // 1 拉黑 3
	s.block(4, 1)  // 4 拉黑 1

	resp, err := s.client.GetRelation(context.Background(), &relationv1.GetRelationRequest{ViewerId: 1, TargetIds: []int64{2, 3, 4, 5}})
	require.NoError(s.T(), err)
	st := resp.GetStates()

	assert.True(s.T(), st[2].GetIsFollowing())
	assert.True(s.T(), st[2].GetIsFollowedBy(), "1、2 互关")
	assert.True(s.T(), st[3].GetIsBlocked(), "1 拉黑了 3")
	assert.True(s.T(), st[4].GetIsBlockedBy(), "4 拉黑了 1")
	if s5, ok := st[5]; ok {
		assert.False(s.T(), s5.GetIsFollowing())
		assert.False(s.T(), s5.GetIsBlocked())
	}
}

// ListFollowees：列出关注的人
func (s *RelationServerSuite) TestListFollowees() {
	s.follow(1, 2)
	s.follow(1, 3)
	resp, err := s.client.ListFollowees(context.Background(), &relationv1.ListRequest{Uid: 1, Cursor: 0, Limit: 10})
	require.NoError(s.T(), err)
	followees := make([]int64, 0, len(resp.GetEdges()))
	for _, e := range resp.GetEdges() {
		followees = append(followees, e.GetFolloweeId())
	}
	assert.ElementsMatch(s.T(), []int64{2, 3}, followees)
}

func (s *RelationServerSuite) stats(uid int64) *relationv1.RelationStats {
	resp, err := s.client.GetStats(context.Background(), &relationv1.GetStatsRequest{Uid: uid})
	require.NoError(s.T(), err)
	return resp.GetStats()
}

// Unblock：物理删黑名单行、且【不恢复】此前被级联解除的关注（#6）
func (s *RelationServerSuite) TestUnblock() {
	s.follow(1, 2) // 1→2
	s.block(1, 2)  // 拉黑级联解除 1→2
	_, err := s.client.Unblock(context.Background(), &relationv1.BlockRequest{Uid: 1, BlockedId: 2})
	require.NoError(s.T(), err)

	resp, err := s.client.GetRelation(context.Background(), &relationv1.GetRelationRequest{ViewerId: 1, TargetIds: []int64{2}})
	require.NoError(s.T(), err)
	assert.False(s.T(), resp.GetStates()[2].GetIsBlocked(), "黑名单行已删")
	assert.False(s.T(), resp.GetStates()[2].GetIsFollowing(), "取消拉黑不恢复关注")
	assert.Equal(s.T(), int64(0), s.stats(1).GetFolloweeCnt(), "关注计数不恢复")
}

// Unfollow：计数递减；重复取关不产生负计数（GREATEST(0,cnt-1) 下限，#7）
func (s *RelationServerSuite) TestUnfollow_CountDecrementAndFloor() {
	s.follow(1, 2)
	assert.Equal(s.T(), int64(1), s.stats(1).GetFolloweeCnt())
	assert.Equal(s.T(), int64(1), s.stats(2).GetFollowerCnt())

	un, err := s.client.Unfollow(context.Background(), &relationv1.FollowRequest{FollowerId: 1, FolloweeId: 2})
	require.NoError(s.T(), err)
	assert.True(s.T(), un.GetChanged())
	assert.Equal(s.T(), int64(0), s.stats(1).GetFolloweeCnt(), "取关后 followee 计数回 0")
	assert.Equal(s.T(), int64(0), s.stats(2).GetFollowerCnt(), "取关后 follower 计数回 0")

	// 未关注再取关：不翻转、计数不为负
	un, err = s.client.Unfollow(context.Background(), &relationv1.FollowRequest{FollowerId: 1, FolloweeId: 2})
	require.NoError(s.T(), err)
	assert.False(s.T(), un.GetChanged())
	assert.Equal(s.T(), int64(0), s.stats(1).GetFolloweeCnt(), "不产生负计数")
}

// GetStats Cache-Aside：读回填 + 写失效（#8）。绕过缓存直改 DB 验证「读走缓存」，再触发写验证「失效回源」。
func (s *RelationServerSuite) TestStats_CacheAside() {
	s.follow(1, 2)
	require.Equal(s.T(), int64(1), s.stats(1).GetFolloweeCnt()) // 回填缓存

	// 绕过缓存直接改 DB：若下次读仍返 1，证明走了缓存（未直读 DB）
	require.NoError(s.T(), s.db.Exec("UPDATE relation_stats SET followee_cnt = 99 WHERE uid = 1").Error)
	assert.Equal(s.T(), int64(1), s.stats(1).GetFolloweeCnt(), "命中缓存旧值")

	// 再关注一人 → 写路径失效 uid=1 缓存 → 下次读回源 DB（99 + 本次 +1）
	s.follow(1, 3)
	assert.Equal(s.T(), int64(100), s.stats(1).GetFolloweeCnt(), "写后缓存失效，回源 DB")
}
