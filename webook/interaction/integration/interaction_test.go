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

	interactionv1 "github.com/webook/api/gen/interaction/v1"
	interactiongrpc "github.com/webook/interaction/grpc"
	"github.com/webook/interaction/integration/setup"
	"github.com/webook/interaction/repository/dao"
	"github.com/webook/pkg/grpcx/interceptor/errconv"
)

const (
	bufSize = 1024 * 1024
	biz     = "article"
)

// InteractionServerSuite 真实 MySQL + Redis + bufconn gRPC：经 InteractionServiceClient 发真实请求，
// 打通 gRPC → service → repository → dao/cache → DB/Redis 全链路。目录架构对齐 comment/integration。
type InteractionServerSuite struct {
	suite.Suite
	db     *gorm.DB
	cmd    redis.Cmdable
	conn   *grpc.ClientConn
	srv    *grpc.Server
	client interactionv1.InteractionServiceClient
}

func TestInteractionServer(t *testing.T) {
	suite.Run(t, &InteractionServerSuite{})
}

func (s *InteractionServerSuite) SetupSuite() {
	s.db = setup.InitDB()
	s.cmd = setup.InitRedis()

	// bufconn 内存 gRPC server，装 errconv + 校验拦截器（与生产 wire 一致）
	lis := bufconn.Listen(bufSize)
	s.srv = grpc.NewServer(grpc.ChainUnaryInterceptor(
		errconv.UnaryServerInterceptor(nil),
		interactiongrpc.ValidateUnaryInterceptor,
	))
	interactionv1.RegisterInteractionServiceServer(s.srv, setup.InitInteractionServer())
	go func() {
		_ = s.srv.Serve(lis)
	}()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(errconv.UnaryClientInterceptor()),
	)
	require.NoError(s.T(), err)
	s.conn = conn
	s.client = interactionv1.NewInteractionServiceClient(conn)
}

func (s *InteractionServerSuite) TearDownSuite() {
	_ = s.conn.Close()
	s.srv.GracefulStop()
}

// SetupTest 每个用例前清干净：interaction/user_interaction 两表与 core 集成测试共用 webook_test 库，
// 不在用例前 reset 会吃到别处残留行（首个用例无 TearDownTest 兜底）。
func (s *InteractionServerSuite) SetupTest() {
	s.reset()
}

func (s *InteractionServerSuite) TearDownTest() {
	s.reset()
}

func (s *InteractionServerSuite) reset() {
	require.NoError(s.T(), s.db.Exec("TRUNCATE TABLE interaction").Error)
	require.NoError(s.T(), s.db.Exec("TRUNCATE TABLE user_interaction").Error)
	require.NoError(s.T(), s.cmd.FlushDB(context.Background()).Err())
}

// TestPreexistingNullDeletedAtRowVisible 回归守卫：模拟「软删列加上去之前就存在、deleted_at=NULL」的既有行
// （raw insert 不写 deleted_at）。若 interaction 再被加上 gorm soft_delete，SELECT 会注入 WHERE deleted_at=0
// 把 NULL 既有行全部过滤掉 → 点赞/收藏/计数查出来全 0（本次事故根因）。interaction 无删除路径，不该用软删。
func (s *InteractionServerSuite) TestPreexistingNullDeletedAtRowVisible() {
	t := s.T()
	const nbizId = int64(987654)
	require.NoError(t, s.db.Exec(
		"INSERT INTO interaction (biz, biz_id, read_count, like_count, collect_count, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		biz, nbizId, 7, 3, 2, 1, 1).Error)

	got, err := dao.NewGormInteractionDAO(s.db).FindByBizId(context.Background(), biz, nbizId)
	require.NoError(t, err) // soft_delete 在场时 deleted_at=0 过滤排除 NULL 行 → ErrRecordNotFound → 此处 FAIL
	assert.Equal(t, int64(7), got.ReadCount)
	assert.Equal(t, int64(3), got.LikeCount)
	assert.Equal(t, int64(2), got.CollectCount)
}

// ── 请求辅助 ──────────────────────────────────────────────

func (s *InteractionServerSuite) like(uid, bizId int64) {
	_, err := s.client.Like(context.Background(), &interactionv1.LikeRequest{Uid: uid, Biz: biz, BizId: bizId})
	require.NoError(s.T(), err)
}

func (s *InteractionServerSuite) cancelLike(uid, bizId int64) {
	_, err := s.client.CancelLike(context.Background(), &interactionv1.CancelLikeRequest{Uid: uid, Biz: biz, BizId: bizId})
	require.NoError(s.T(), err)
}

func (s *InteractionServerSuite) collect(uid, bizId int64) {
	_, err := s.client.Collect(context.Background(), &interactionv1.CollectRequest{Uid: uid, Biz: biz, BizId: bizId})
	require.NoError(s.T(), err)
}

func (s *InteractionServerSuite) view(bizId int64) {
	_, err := s.client.IncrReadCount(context.Background(), &interactionv1.IncrReadCountRequest{Biz: biz, BizId: bizId})
	require.NoError(s.T(), err)
}

func (s *InteractionServerSuite) get(uid, bizId int64) *interactionv1.Interaction {
	resp, err := s.client.GetInteraction(context.Background(), &interactionv1.GetInteractionRequest{Uid: uid, Biz: biz, BizId: bizId})
	require.NoError(s.T(), err)
	return resp.GetInteraction()
}

// ── 用例 ──────────────────────────────────────────────────

// Like 增点赞数 + 置用户状态；重复点赞幂等（不重复计数）；CancelLike 递减归零
func (s *InteractionServerSuite) TestLike_CountStateIdempotent() {
	s.like(100, 1)
	s.like(100, 1) // 重复点赞幂等

	intr := s.get(100, 1)
	assert.Equal(s.T(), int64(1), intr.GetLikeCount(), "重复点赞不重复计数")
	assert.True(s.T(), intr.GetLiked(), "uid 视角已赞")

	s.cancelLike(100, 1)
	intr = s.get(100, 1)
	assert.Equal(s.T(), int64(0), intr.GetLikeCount(), "取消后归零")
	assert.False(s.T(), intr.GetLiked())
}

// 多个用户点赞同一对象，点赞数累加
func (s *InteractionServerSuite) TestLike_MultiUser() {
	s.like(100, 1)
	s.like(200, 1)
	assert.Equal(s.T(), int64(2), s.get(0, 1).GetLikeCount())
}

// Collect 增收藏数 + 置状态；CancelCollect 递减
func (s *InteractionServerSuite) TestCollect_CountState() {
	s.collect(100, 1)
	intr := s.get(100, 1)
	assert.Equal(s.T(), int64(1), intr.GetCollectCount())
	assert.True(s.T(), intr.GetCollected())

	_, err := s.client.CancelCollect(context.Background(), &interactionv1.CancelCollectRequest{Uid: 100, Biz: biz, BizId: 1})
	require.NoError(s.T(), err)
	intr = s.get(100, 1)
	assert.Equal(s.T(), int64(0), intr.GetCollectCount())
	assert.False(s.T(), intr.GetCollected())
}

// IncrReadCount 累加阅读数
func (s *InteractionServerSuite) TestView_IncrReadCount() {
	s.view(1)
	s.view(1)
	s.view(1)
	assert.Equal(s.T(), int64(3), s.get(0, 1).GetReadCount())
}

// GetUserState 只返回用户 liked/collected，不掺计数语义
func (s *InteractionServerSuite) TestGetUserState() {
	s.like(100, 1)
	s.collect(100, 1)

	resp, err := s.client.GetUserState(context.Background(), &interactionv1.GetUserStateRequest{Uid: 100, Biz: biz, BizId: 1})
	require.NoError(s.T(), err)
	assert.True(s.T(), resp.GetLiked())
	assert.True(s.T(), resp.GetCollected())

	// 另一个用户没有状态
	resp2, err := s.client.GetUserState(context.Background(), &interactionv1.GetUserStateRequest{Uid: 999, Biz: biz, BizId: 1})
	require.NoError(s.T(), err)
	assert.False(s.T(), resp2.GetLiked())
	assert.False(s.T(), resp2.GetCollected())
}

// BatchGetInteractions 按 bizIds 批量取聚合计数，key=bizId；无记录的 bizId 不在结果里
func (s *InteractionServerSuite) TestBatchGet_Counts() {
	s.like(100, 1)
	s.view(2)

	resp, err := s.client.BatchGetInteractions(context.Background(), &interactionv1.BatchGetInteractionsRequest{
		Biz: biz, BizIds: []int64{1, 2, 3},
	})
	require.NoError(s.T(), err)
	m := resp.GetInteractions()
	require.Len(s.T(), m, 2, "bizId=3 无记录不返回")
	assert.Equal(s.T(), int64(1), m[1].GetLikeCount())
	assert.Equal(s.T(), int64(1), m[2].GetReadCount())
}

// GetUserLiked 只回已赞的 bizId（收藏不算）
func (s *InteractionServerSuite) TestGetUserLiked() {
	s.like(100, 1)
	s.like(100, 3)
	s.collect(100, 2) // 收藏不应出现在 liked 结果

	resp, err := s.client.GetUserLiked(context.Background(), &interactionv1.GetUserLikedRequest{
		Uid: 100, Biz: biz, BizIds: []int64{1, 2, 3},
	})
	require.NoError(s.T(), err)
	assert.ElementsMatch(s.T(), []int64{1, 3}, resp.GetLikedBizIds())
}

// GetHotBizIds 按 read + like*3 + collect*5 加权分降序
func (s *InteractionServerSuite) TestGetHotBizIds_WeightedOrder() {
	s.like(1, 10)    // biz 10：like*3 = 3
	s.collect(1, 20) // biz 20：collect*5 = 5
	s.view(30)       // biz 30：read*1 = 1

	resp, err := s.client.GetHotBizIds(context.Background(), &interactionv1.GetHotBizIdsRequest{Biz: biz, Limit: 10})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), []int64{20, 10, 30}, resp.GetBizIds(), "5 > 3 > 1 降序")
}

// GetCollectedBizIds 返回用户收藏的 bizId
func (s *InteractionServerSuite) TestGetCollectedBizIds() {
	s.collect(100, 1)
	s.collect(100, 2)
	s.collect(200, 3) // 别的用户

	resp, err := s.client.GetCollectedBizIds(context.Background(), &interactionv1.GetCollectedBizIdsRequest{Uid: 100, Biz: biz, Limit: 10})
	require.NoError(s.T(), err)
	assert.ElementsMatch(s.T(), []int64{1, 2}, resp.GetBizIds(), "只含 uid=100 的收藏")
}

// GetInteraction Cache-Aside：首次回源回填，绕过 gRPC 直改 DB 后再查命中缓存返回旧值
func (s *InteractionServerSuite) TestGetInteraction_CacheAside() {
	s.like(100, 1)
	require.Equal(s.T(), int64(1), s.get(0, 1).GetLikeCount(), "回源并回填缓存")

	// 绕过 repo 直接改 DB（不清缓存）
	require.NoError(s.T(), s.db.Exec("UPDATE interaction SET like_count = 99 WHERE biz = ? AND biz_id = ?", biz, 1).Error)

	assert.Equal(s.T(), int64(1), s.get(0, 1).GetLikeCount(), "返回缓存旧值，证明走了缓存")
}

// 写操作清缓存：CancelLike 后再查走 DB 得新值（否则返回缓存旧值）
func (s *InteractionServerSuite) TestWrite_InvalidatesCache() {
	s.like(100, 1)
	require.Equal(s.T(), int64(1), s.get(0, 1).GetLikeCount()) // 预热缓存

	s.cancelLike(100, 1)
	assert.Equal(s.T(), int64(0), s.get(0, 1).GetLikeCount(), "取消点赞清了缓存，返回 DB 新值")
}
