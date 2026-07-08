package integration

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"gorm.io/gorm"

	commentv1 "github.com/boyxs/train-go/webook/api/gen/comment/v1"
	"github.com/boyxs/train-go/webook/comment/integration/setup"
	"github.com/boyxs/train-go/webook/comment/repository/dao"
	"github.com/boyxs/train-go/webook/pkg/grpcx/interceptor/errconv"
)

const (
	bufSize = 1024 * 1024
	biz     = "article"
)

// CommentServerSuite 真实 MySQL + Redis + bufconn gRPC：经 CommentServiceClient 发真实请求，
// 打通 gRPC → service → repository → dao → DB 全链路。目录架构对齐 internal/integration。
type CommentServerSuite struct {
	suite.Suite
	db     *gorm.DB
	cmd    redis.Cmdable
	conn   *grpc.ClientConn
	srv    *grpc.Server
	client commentv1.CommentServiceClient
}

func TestCommentServer(t *testing.T) {
	suite.Run(t, &CommentServerSuite{})
}

func (s *CommentServerSuite) SetupSuite() {
	s.db = setup.InitDB()
	s.cmd = setup.InitRedis()

	// bufconn 内存 gRPC server，装 errconv 拦截器（与生产 wire 一致：*errs.Error ↔ status）
	lis := bufconn.Listen(bufSize)
	s.srv = grpc.NewServer(grpc.UnaryInterceptor(errconv.UnaryServerInterceptor(nil)))
	commentv1.RegisterCommentServiceServer(s.srv, setup.InitCommentServer())
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
	s.client = commentv1.NewCommentServiceClient(conn)
}

func (s *CommentServerSuite) TearDownSuite() {
	_ = s.conn.Close()
	s.srv.GracefulStop()
}

func (s *CommentServerSuite) TearDownTest() {
	// 自关联外键下先关 FK 检查再 TRUNCATE；Redis 清计数缓存 + 限流键，避免跨用例串
	require.NoError(s.T(), s.db.Exec("SET FOREIGN_KEY_CHECKS = 0").Error)
	require.NoError(s.T(), s.db.Exec("TRUNCATE TABLE comment").Error)
	require.NoError(s.T(), s.db.Exec("SET FOREIGN_KEY_CHECKS = 1").Error)
	require.NoError(s.T(), s.cmd.FlushDB(context.Background()).Err())
}

// ── 请求辅助 ──────────────────────────────────────────────

func (s *CommentServerSuite) create(bizId, uid int64, content string, pid int64) *commentv1.Comment {
	resp, err := s.client.CreateComment(context.Background(), &commentv1.CreateCommentRequest{
		Biz: biz, BizId: bizId, UserId: uid, Content: content, Pid: pid,
	})
	require.NoError(s.T(), err)
	return resp.GetComment()
}

func (s *CommentServerSuite) listRoots(bizId int64) []*commentv1.Comment {
	resp, err := s.client.ListComments(context.Background(), &commentv1.ListCommentsRequest{
		Biz: biz, BizId: bizId, Offset: 0, Limit: 50,
	})
	require.NoError(s.T(), err)
	return resp.GetComments()
}

func (s *CommentServerSuite) findRoot(bizId, id int64) *commentv1.Comment {
	for _, c := range s.listRoots(bizId) {
		if c.GetId() == id {
			return c
		}
	}
	s.T().Fatalf("root %d 不在一级评论列表", id)
	return nil
}

// ── 用例 ──────────────────────────────────────────────────

// 回复设置 root_id + 楼主 reply_cnt+1；深层回复继承楼主 root_id；整楼回复按时间正序
func (s *CommentServerSuite) TestReply_RootAndOrder() {
	root := s.create(1, 100, "楼主", 0)
	r1 := s.create(1, 200, "回复1", root.GetId())
	r2 := s.create(1, 300, "回复回复", r1.GetId())

	resp, err := s.client.GetReplies(context.Background(), &commentv1.GetRepliesRequest{
		RootId: root.GetId(), Offset: 0, Limit: 10,
	})
	require.NoError(s.T(), err)
	require.Len(s.T(), resp.GetReplies(), 2, "整楼回复（含嵌套）")
	assert.Equal(s.T(), r1.GetId(), resp.GetReplies()[0].GetId(), "按时间正序")
	assert.Equal(s.T(), r2.GetId(), resp.GetReplies()[1].GetId())
	assert.Equal(s.T(), root.GetId(), resp.GetReplies()[1].GetRootId(), "深层回复 root_id 继承楼主")
	assert.Equal(s.T(), r1.GetId(), resp.GetReplies()[1].GetPid(), "pid=直接父")
}

// 一级评论 reply_cnt = 整楼回复数（含嵌套）
func (s *CommentServerSuite) TestReplyCnt_WholeThread() {
	root := s.create(1, 100, "楼主", 0)
	li := s.create(1, 200, "李四", root.GetId())
	s.create(1, 300, "王五回复李四", li.GetId())
	assert.Equal(s.T(), int64(2), s.findRoot(1, root.GetId()).GetReplyCnt(), "整楼回复数=2")
}

// 一级评论按时间倒序（最新在前），回复不进一级列表
func (s *CommentServerSuite) TestList_NewestFirst() {
	c1 := s.create(1, 100, "第一", 0)
	c2 := s.create(1, 100, "第二", 0)
	s.create(1, 200, "回复", c1.GetId())

	roots := s.listRoots(1)
	require.Len(s.T(), roots, 2, "只返回一级评论")
	assert.Equal(s.T(), c2.GetId(), roots[0].GetId(), "最新在前")
	assert.Equal(s.T(), c1.GetId(), roots[1].GetId())
}

// BatchGet 按 id 批量取（core 拿 interaction 热门 id 后回查）
func (s *CommentServerSuite) TestBatchGet_ByIds() {
	c1 := s.create(1, 100, "A", 0)
	s.create(1, 100, "B", 0)
	c3 := s.create(1, 100, "C", 0)

	resp, err := s.client.BatchGetComments(context.Background(), &commentv1.BatchGetCommentsRequest{
		Ids: []int64{c1.GetId(), c3.GetId()},
	})
	require.NoError(s.T(), err)
	require.Len(s.T(), resp.GetComments(), 2, "只取指定 2 条")
}

// Count 只统计指定 biz/bizId
func (s *CommentServerSuite) TestCount_ByBiz() {
	root := s.create(7, 100, "楼主", 0)
	s.create(7, 200, "回复", root.GetId())
	s.create(99, 100, "别的文章", 0)

	resp, err := s.client.CountComment(context.Background(), &commentv1.CountCommentRequest{Biz: biz, BizId: 7})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(2), resp.GetCount(), "只统计 bizId=7")
}

// Count Cache-Aside：首次回源回填，绕过 gRPC 直插一条后再查命中缓存返回旧值
func (s *CommentServerSuite) TestCount_CacheAside() {
	ctx := context.Background()
	s.create(1, 100, "A", 0)
	s.create(1, 100, "B", 0)

	resp, err := s.client.CountComment(ctx, &commentv1.CountCommentRequest{Biz: biz, BizId: 1})
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(2), resp.GetCount())

	// 绕过 repo 直接写 DB（不清缓存）
	require.NoError(s.T(), s.db.Create(&dao.Comment{Biz: biz, BizId: 1, Uid: 100, Content: "C"}).Error)

	resp2, err := s.client.CountComment(ctx, &commentv1.CountCommentRequest{Biz: biz, BizId: 1})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(2), resp2.GetCount(), "返回缓存旧值，证明走了缓存")
}

// 删除鉴权：仅作者本人可删
func (s *CommentServerSuite) TestDelete_OnlyOwner() {
	ctx := context.Background()
	c := s.create(1, 100, "我的评论", 0)

	_, err := s.client.DeleteComment(ctx, &commentv1.DeleteCommentRequest{Id: c.GetId(), UserId: 999})
	require.Error(s.T(), err, "非作者删除应失败")

	_, err = s.client.DeleteComment(ctx, &commentv1.DeleteCommentRequest{Id: c.GetId(), UserId: 100})
	require.NoError(s.T(), err, "作者删除成功")
}

// 删一级评论 → 整楼级联软删：一级列表消失、整楼回复消失、Count 归零
func (s *CommentServerSuite) TestDelete_RootCascadesReplies() {
	ctx := context.Background()
	root := s.create(1, 100, "楼主", 0)
	r1 := s.create(1, 200, "子回复", root.GetId())
	s.create(1, 300, "回复回复", r1.GetId())

	_, err := s.client.DeleteComment(ctx, &commentv1.DeleteCommentRequest{Id: root.GetId(), UserId: 100})
	require.NoError(s.T(), err)

	assert.Empty(s.T(), s.listRoots(1), "一级列表不再有该楼")

	replies, err := s.client.GetReplies(ctx, &commentv1.GetRepliesRequest{RootId: root.GetId(), Offset: 0, Limit: 10})
	require.NoError(s.T(), err)
	assert.Empty(s.T(), replies.GetReplies(), "整楼回复级联删除")

	cnt, err := s.client.CountComment(ctx, &commentv1.CountCommentRequest{Biz: biz, BizId: 1})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(0), cnt.GetCount(), "整楼删除后计数归零")
}

// 删「有子回复的楼内回复」→ 只删自身，子回复保留，楼根 reply_cnt 只减 1
func (s *CommentServerSuite) TestDelete_ReplyKeepsItsChildren() {
	ctx := context.Background()
	root := s.create(1, 100, "楼主", 0)
	mid := s.create(1, 200, "中层回复", root.GetId())
	s.create(1, 300, "回复中层", mid.GetId())

	_, err := s.client.DeleteComment(ctx, &commentv1.DeleteCommentRequest{Id: mid.GetId(), UserId: 200})
	require.NoError(s.T(), err)

	replies, err := s.client.GetReplies(ctx, &commentv1.GetRepliesRequest{RootId: root.GetId(), Offset: 0, Limit: 10})
	require.NoError(s.T(), err)
	require.Len(s.T(), replies.GetReplies(), 1, "中层删除，其子回复保留")
	assert.Equal(s.T(), mid.GetId(), replies.GetReplies()[0].GetPid(), "子回复 pid 仍指向已删中层")
	assert.Equal(s.T(), int64(1), s.findRoot(1, root.GetId()).GetReplyCnt(), "reply_cnt 只减中层这 1 条")
}

// 删「无子回复的回复」→ 递减直接父 reply_cnt（对称 Insert 的 +1）
func (s *CommentServerSuite) TestDelete_DecrementsParentReplyCnt() {
	ctx := context.Background()
	root := s.create(1, 100, "楼主", 0)
	reply := s.create(1, 200, "唯一回复", root.GetId())
	require.Equal(s.T(), int64(1), s.findRoot(1, root.GetId()).GetReplyCnt(), "写回复后楼主 reply_cnt=1")

	_, err := s.client.DeleteComment(ctx, &commentv1.DeleteCommentRequest{Id: reply.GetId(), UserId: 200})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(0), s.findRoot(1, root.GetId()).GetReplyCnt(), "删唯一回复后递减为 0")
}

// 删除清计数缓存：预热后删除 → 再查走 DB 得新值（否则返回缓存旧值）
func (s *CommentServerSuite) TestDelete_InvalidatesCountCache() {
	ctx := context.Background()
	c := s.create(1, 100, "x", 0)
	r1, err := s.client.CountComment(ctx, &commentv1.CountCommentRequest{Biz: biz, BizId: 1})
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(1), r1.GetCount())

	_, err = s.client.DeleteComment(ctx, &commentv1.DeleteCommentRequest{Id: c.GetId(), UserId: 100})
	require.NoError(s.T(), err)

	r2, err := s.client.CountComment(ctx, &commentv1.CountCommentRequest{Biz: biz, BizId: 1})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(0), r2.GetCount(), "删除清了计数缓存，返回 DB 新值")
}

// 内容校验：空内容 / 超长内容被拒
func (s *CommentServerSuite) TestCreate_RejectsInvalidContent() {
	ctx := context.Background()
	_, err := s.client.CreateComment(ctx, &commentv1.CreateCommentRequest{Biz: biz, BizId: 1, UserId: 100, Content: "   "})
	require.Error(s.T(), err, "空内容应拒绝")

	_, err = s.client.CreateComment(ctx, &commentv1.CreateCommentRequest{Biz: biz, BizId: 1, UserId: 100, Content: strings.Repeat("x", 501)})
	require.Error(s.T(), err, "超长内容应拒绝")
}
