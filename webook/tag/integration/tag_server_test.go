package integration

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"gorm.io/gorm"

	tagv1 "github.com/boyxs/train-go/webook/api/gen/tag/v1"
	"github.com/boyxs/train-go/webook/pkg/grpcx/interceptor/errconv"
	"github.com/boyxs/train-go/webook/tag/domain"
	tagerrs "github.com/boyxs/train-go/webook/tag/errs"
	"github.com/boyxs/train-go/webook/tag/integration/setup"
)

const bufSize = 1024 * 1024

// TagServerSuite 真实 MySQL + bufconn gRPC：经 TagServiceClient 发真实请求，
// 打通 gRPC → service → repository → dao → MySQL 全链路。
type TagServerSuite struct {
	suite.Suite
	db     *gorm.DB
	rdb    redis.Cmdable
	conn   *grpc.ClientConn
	srv    *grpc.Server
	client tagv1.TagServiceClient
}

func TestTagServer(t *testing.T) {
	suite.Run(t, &TagServerSuite{})
}

func (s *TagServerSuite) SetupSuite() {
	s.db = setup.InitDB()
	s.rdb = setup.InitRedis()

	// bufconn 内存 gRPC server，装 errconv 拦截器（与生产 wire 一致：*errs.Error ↔ status）
	lis := bufconn.Listen(bufSize)
	s.srv = grpc.NewServer(grpc.UnaryInterceptor(errconv.UnaryServerInterceptor(nil)))
	tagv1.RegisterTagServiceServer(s.srv, setup.InitTagServer())
	go func() { _ = s.srv.Serve(lis) }()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(errconv.UnaryClientInterceptor()),
	)
	require.NoError(s.T(), err)
	s.conn = conn
	s.client = tagv1.NewTagServiceClient(conn)
}

func (s *TagServerSuite) TearDownSuite() {
	_ = s.conn.Close()
	s.srv.GracefulStop()
}

func (s *TagServerSuite) SetupTest()    { s.reset() }
func (s *TagServerSuite) TearDownTest() { s.reset() }
func (s *TagServerSuite) reset() {
	require.NoError(s.T(), s.db.Exec("TRUNCATE TABLE tagging").Error)
	require.NoError(s.T(), s.db.Exec("TRUNCATE TABLE tag_follow").Error)
	require.NoError(s.T(), s.db.Exec("TRUNCATE TABLE tag").Error)
	s.flushDetail()
}

// flushDetail 清标签详情缓存（tag:detail:*），避免跨用例 / 直改 DB 后串缓存。
func (s *TagServerSuite) flushDetail() {
	keys, err := s.rdb.Keys(context.Background(), "tag:detail:*").Result()
	require.NoError(s.T(), err)
	if len(keys) > 0 {
		require.NoError(s.T(), s.rdb.Del(context.Background(), keys...).Err())
	}
}

func (s *TagServerSuite) tagRows() int64 {
	var n int64
	require.NoError(s.T(), s.db.Table("tag").Count(&n).Error)
	return n
}

func (s *TagServerSuite) sync(bizId int64, names ...string) *tagv1.TagList {
	resp, err := s.client.SyncTags(context.Background(), &tagv1.SyncTagsRequest{
		Biz: "article", BizId: bizId, Names: names, Source: domain.TagSourceAuthor,
	})
	require.NoError(s.T(), err)
	return resp
}

func pbSlugs(list *tagv1.TagList) []string {
	out := make([]string, 0, len(list.GetTags()))
	for _, t := range list.GetTags() {
		out = append(out, t.GetSlug())
	}
	return out
}

// SyncTags：首次打标签 → 建 tag + tagging，返回已解析（含归一 slug）
func (s *TagServerSuite) TestSyncTags_FirstTime() {
	resp := s.sync(1, "Golang", "并发编程")
	assert.ElementsMatch(s.T(), []string{"golang", "并发编程"}, pbSlugs(resp), "名字归一为 slug")
	assert.Equal(s.T(), int64(2), s.tagRows())
}

// SyncTags：归一后同 slug 去重（Go/go/GO → 1 个）
func (s *TagServerSuite) TestSyncTags_DedupBySlug() {
	resp := s.sync(1, "Go", "go", "GO")
	require.Len(s.T(), resp.GetTags(), 1, "归一同 slug 去重")
	assert.Equal(s.T(), "go", resp.GetTags()[0].GetSlug())
}

// SyncTags：空白名跳过
func (s *TagServerSuite) TestSyncTags_SkipsEmpty() {
	resp := s.sync(1, "Golang", "   ", "")
	require.Len(s.T(), resp.GetTags(), 1)
	assert.Equal(s.T(), "golang", resp.GetTags()[0].GetSlug())
}

// SyncTags：超 5 个 → 拒绝且不落库（校验在 Upsert 之前）；错误经 errconv 跨 gRPC 仍 errors.Is 命中
func (s *TagServerSuite) TestSyncTags_LimitExceeded() {
	_, err := s.client.SyncTags(context.Background(), &tagv1.SyncTagsRequest{
		Biz: "article", BizId: 1, Names: []string{"a", "b", "c", "d", "e", "f"}, Source: domain.TagSourceAuthor,
	})
	assert.ErrorIs(s.T(), err, tagerrs.ErrTagLimitExceeded)
	assert.Equal(s.T(), int64(0), s.tagRows(), "超限不创建任何标签")
}

// SyncTags：名字非法（超长）→ 拒绝
func (s *TagServerSuite) TestSyncTags_InvalidName() {
	_, err := s.client.SyncTags(context.Background(), &tagv1.SyncTagsRequest{
		Biz: "article", BizId: 1, Names: []string{strings.Repeat("x", domain.TagNameMaxLen+1)}, Source: domain.TagSourceAuthor,
	})
	assert.ErrorIs(s.T(), err, tagerrs.ErrTagNameInvalid)
}

// SyncTags：重新同步不同集合 → 增删对齐（经 TagsByBiz 验证）
func (s *TagServerSuite) TestSyncTags_Resync() {
	s.sync(1, "Golang", "并发")
	s.sync(1, "Golang", "性能")
	resp, err := s.client.TagsByBiz(context.Background(), &tagv1.TagsByBizRequest{Biz: "article", BizIds: []int64{1}})
	require.NoError(s.T(), err)
	assert.ElementsMatch(s.T(), []string{"golang", "性能"}, pbSlugs(resp.GetTags()[1]), "并发 移除、性能 加入")
}

// Detail：存在返回（名 + ref_count），缺失 → ErrTagNotFound
func (s *TagServerSuite) TestDetail() {
	s.sync(1, "Golang")
	got, err := s.client.Detail(context.Background(), &tagv1.DetailRequest{Slug: "golang"})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), "Golang", got.GetName())
	assert.Equal(s.T(), int64(1), got.GetRefCount())

	_, err = s.client.Detail(context.Background(), &tagv1.DetailRequest{Slug: "nope"})
	assert.ErrorIs(s.T(), err, tagerrs.ErrTagNotFound)
}

// Detail：本周新增计数（近 7 天关联数），滚动窗口排除 8 天前的旧关联
func (s *TagServerSuite) TestDetail_WeeklyNewCount() {
	s.sync(100, "Golang")
	s.sync(200, "Golang")
	got, err := s.client.Detail(context.Background(), &tagv1.DetailRequest{Slug: "golang"})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(2), got.GetWeeklyNewCount(), "两篇本周内打标签 → 本周新增 2")

	// 直插一条 8 天前的关联（绕过 autoCreateTime）：超出 7 天窗口不计入
	old := time.Now().Add(-8 * 24 * time.Hour).UnixMilli()
	require.NoError(s.T(), s.db.Exec(
		"INSERT INTO tagging (tag_id, biz, biz_id, source, created_at, updated_at) VALUES (?, 'article', 999, 'author', ?, ?)",
		got.GetId(), old, old).Error)
	s.flushDetail() // 直插绕过服务、缓存未失效，手动清后强制回源重算窗口
	got2, err := s.client.Detail(context.Background(), &tagv1.DetailRequest{Slug: "golang"})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(2), got2.GetWeeklyNewCount(), "8 天前的关联不计入本周新增")
}

// Detail：Cache-Aside——命中返旧值（不查库）、写（Follow）失效后回源读新值
func (s *TagServerSuite) TestDetail_CacheAside() {
	s.sync(1, "Golang")
	ctx := context.Background()

	// 首次 Detail：回源 + 回填缓存
	got, err := s.client.Detail(ctx, &tagv1.DetailRequest{Slug: "golang"})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(1), got.GetRefCount())

	// 直改 DB refCount 绕过服务：缓存仍旧值 → 第二次 Detail 命中缓存返回旧值
	require.NoError(s.T(), s.db.Exec("UPDATE tag SET ref_count = 99 WHERE slug = 'golang'").Error)
	cached, err := s.client.Detail(ctx, &tagv1.DetailRequest{Slug: "golang"})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(1), cached.GetRefCount(), "命中缓存，返回旧 refCount（未查库）")

	// Follow 触发写失效 → 下次 Detail 回源，读到新值 + 新关注数
	_, err = s.client.Follow(ctx, &tagv1.FollowRequest{Uid: 10, Slug: "golang"})
	require.NoError(s.T(), err)
	fresh, err := s.client.Detail(ctx, &tagv1.DetailRequest{Slug: "golang"})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(99), fresh.GetRefCount(), "写失效后回源，读到新 refCount")
	assert.Equal(s.T(), int64(1), fresh.GetFollowCount(), "关注数同步更新")
}

// Suggest：前缀命中（Rust 不入）
func (s *TagServerSuite) TestSuggest() {
	s.sync(1, "Golang", "Go并发", "Rust")
	got, err := s.client.Suggest(context.Background(), &tagv1.SuggestRequest{Prefix: "go", Limit: 0})
	require.NoError(s.T(), err)
	assert.ElementsMatch(s.T(), []string{"golang", "go并发"}, pbSlugs(got), "go 前缀命中 2，Rust 不入")
}

// BatchBySlugs：批量补名，缺失 slug 不返回
func (s *TagServerSuite) TestBatchBySlugs() {
	s.sync(1, "Golang", "并发", "Rust")
	got, err := s.client.BatchBySlugs(context.Background(), &tagv1.BatchBySlugsRequest{Slugs: []string{"golang", "rust", "nope"}})
	require.NoError(s.T(), err)
	assert.ElementsMatch(s.T(), []string{"golang", "rust"}, pbSlugs(got), "nope 不存在不返回")
}

// BizIdsByTag：某标签下对象 id（created_at DESC）+ total + 窗口封顶 + 缺失 slug 空
func (s *TagServerSuite) TestBizIdsByTag() {
	s.sync(100, "Golang")
	s.sync(200, "Golang")
	s.sync(300, "Golang")
	resp, err := s.client.BizIdsByTag(context.Background(), &tagv1.BizIdsByTagRequest{Slug: "golang", Biz: "article", Limit: 10})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(3), resp.GetTotal())
	assert.Equal(s.T(), []int64{300, 200, 100}, resp.GetIds(), "created_at DESC 新在前")

	// 窗口封顶：limit=2 取前 2，total 不受 limit 影响
	page, err := s.client.BizIdsByTag(context.Background(), &tagv1.BizIdsByTagRequest{Slug: "golang", Biz: "article", Limit: 2})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(3), page.GetTotal())
	assert.Equal(s.T(), []int64{300, 200}, page.GetIds())

	// slug 不存在 → 空
	none, err := s.client.BizIdsByTag(context.Background(), &tagv1.BizIdsByTagRequest{Slug: "nope", Biz: "article", Limit: 10})
	require.NoError(s.T(), err)
	assert.Empty(s.T(), none.GetIds())
	assert.Equal(s.T(), int64(0), none.GetTotal())
}

// ref_count 跨对象累计：同标签被两篇引用 → ref_count=2；撤其一 → 1（GREATEST 防负，事务维护）
func (s *TagServerSuite) TestRefCountAcrossBiz() {
	s.sync(100, "Golang")
	s.sync(200, "Golang")
	got, err := s.client.Detail(context.Background(), &tagv1.DetailRequest{Slug: "golang"})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(2), got.GetRefCount(), "两篇引用 → ref_count=2")

	s.sync(100) // 清空 100 的关联
	got, err = s.client.Detail(context.Background(), &tagv1.DetailRequest{Slug: "golang"})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(1), got.GetRefCount(), "撤一篇 → ref_count=1")
}

// Follow/Unfollow：翻转 + 计数维护 + 幂等（changed 门控），返回翻转后关注数
func (s *TagServerSuite) TestFollow_ToggleAndCount() {
	s.sync(1, "Golang")
	ctx := context.Background()

	// 首次关注 → changed=true，follow_count=1
	r1, err := s.client.Follow(ctx, &tagv1.FollowRequest{Uid: 10, Slug: "golang"})
	require.NoError(s.T(), err)
	assert.True(s.T(), r1.GetChanged())
	assert.Equal(s.T(), int64(1), r1.GetFollowerCount())

	// 重复关注 → 幂等 changed=false，计数不变
	r2, err := s.client.Follow(ctx, &tagv1.FollowRequest{Uid: 10, Slug: "golang"})
	require.NoError(s.T(), err)
	assert.False(s.T(), r2.GetChanged())
	assert.Equal(s.T(), int64(1), r2.GetFollowerCount())

	// 取关 → changed=true，follow_count=0
	r3, err := s.client.Unfollow(ctx, &tagv1.FollowRequest{Uid: 10, Slug: "golang"})
	require.NoError(s.T(), err)
	assert.True(s.T(), r3.GetChanged())
	assert.Equal(s.T(), int64(0), r3.GetFollowerCount())

	// 重复取关 → 幂等 changed=false，计数不负（GREATEST 防负）
	r4, err := s.client.Unfollow(ctx, &tagv1.FollowRequest{Uid: 10, Slug: "golang"})
	require.NoError(s.T(), err)
	assert.False(s.T(), r4.GetChanged())
	assert.Equal(s.T(), int64(0), r4.GetFollowerCount())
}

// Follow：多用户关注同标签累计计数；Detail 回显 follow_count
func (s *TagServerSuite) TestFollow_MultiUserCountAndDetail() {
	s.sync(1, "Golang")
	ctx := context.Background()
	_, err := s.client.Follow(ctx, &tagv1.FollowRequest{Uid: 10, Slug: "golang"})
	require.NoError(s.T(), err)
	r, err := s.client.Follow(ctx, &tagv1.FollowRequest{Uid: 20, Slug: "golang"})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(2), r.GetFollowerCount(), "两用户关注 → 2")

	got, err := s.client.Detail(ctx, &tagv1.DetailRequest{Slug: "golang"})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(2), got.GetFollowCount(), "Detail 回显 follow_count")
}

// FollowStatus：关注者 true、未关注 false、取关后 false
func (s *TagServerSuite) TestFollowStatus() {
	s.sync(1, "Golang")
	ctx := context.Background()
	_, err := s.client.Follow(ctx, &tagv1.FollowRequest{Uid: 10, Slug: "golang"})
	require.NoError(s.T(), err)

	st, err := s.client.FollowStatus(ctx, &tagv1.FollowStatusRequest{Uid: 10, Slug: "golang"})
	require.NoError(s.T(), err)
	assert.True(s.T(), st.GetIsFollowing(), "关注者 → true")

	other, err := s.client.FollowStatus(ctx, &tagv1.FollowStatusRequest{Uid: 99, Slug: "golang"})
	require.NoError(s.T(), err)
	assert.False(s.T(), other.GetIsFollowing(), "未关注者 → false")

	_, err = s.client.Unfollow(ctx, &tagv1.FollowRequest{Uid: 10, Slug: "golang"})
	require.NoError(s.T(), err)
	st2, err := s.client.FollowStatus(ctx, &tagv1.FollowStatusRequest{Uid: 10, Slug: "golang"})
	require.NoError(s.T(), err)
	assert.False(s.T(), st2.GetIsFollowing(), "取关后 → false")
}

// Follow/FollowStatus：slug 不存在 → ErrTagNotFound（跨 gRPC errconv 仍命中）
func (s *TagServerSuite) TestFollow_TagNotFound() {
	ctx := context.Background()
	_, err := s.client.Follow(ctx, &tagv1.FollowRequest{Uid: 10, Slug: "nope"})
	assert.ErrorIs(s.T(), err, tagerrs.ErrTagNotFound)
	_, err = s.client.FollowStatus(ctx, &tagv1.FollowStatusRequest{Uid: 10, Slug: "nope"})
	assert.ErrorIs(s.T(), err, tagerrs.ErrTagNotFound)
}

// TagsByBiz：批量返回每 bizId 的标签（消 N+1），无关联的 bizId 不在结果
func (s *TagServerSuite) TestTagsByBiz_Batch() {
	s.sync(100, "Golang", "并发")
	s.sync(200, "Golang")
	resp, err := s.client.TagsByBiz(context.Background(), &tagv1.TagsByBizRequest{Biz: "article", BizIds: []int64{100, 200, 999}})
	require.NoError(s.T(), err)
	m := resp.GetTags()
	require.Len(s.T(), m, 2, "999 无关联不在结果")
	assert.ElementsMatch(s.T(), []string{"golang", "并发"}, pbSlugs(m[100]))
	require.Len(s.T(), m[200].GetTags(), 1)
	assert.Equal(s.T(), "golang", m[200].GetTags()[0].GetSlug())
}
