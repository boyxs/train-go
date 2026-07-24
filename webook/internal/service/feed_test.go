package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"

	commentv1 "github.com/boyxs/train-go/webook/api/gen/comment/v1"
	feedv1 "github.com/boyxs/train-go/webook/api/gen/feed/v1"
	"github.com/boyxs/train-go/webook/internal/domain"
	"github.com/boyxs/train-go/webook/internal/service"
	svcmocks "github.com/boyxs/train-go/webook/internal/service/mocks"
	grpcmocks "github.com/boyxs/train-go/webook/internal/web/grpcmocks"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

type feedMocks struct {
	feed    *grpcmocks.MockFeedServiceClient
	article *svcmocks.MockArticleReaderService
	intr    *svcmocks.MockInteractionService
	comment *grpcmocks.MockCommentServiceClient
	tag     *svcmocks.MockTagService
	user    *svcmocks.MockUserService
}

func newFeedBFF(t *testing.T, ctrl *gomock.Controller) (service.FeedService, feedMocks) {
	t.Helper()
	m := feedMocks{
		feed:    grpcmocks.NewMockFeedServiceClient(ctrl),
		article: svcmocks.NewMockArticleReaderService(ctrl),
		intr:    svcmocks.NewMockInteractionService(ctrl),
		comment: grpcmocks.NewMockCommentServiceClient(ctrl),
		tag:     svcmocks.NewMockTagService(ctrl),
		user:    svcmocks.NewMockUserService(ctrl),
	}
	svc := service.NewGRPCFeedService(m.feed, m.article, m.intr, m.comment, m.tag, m.user, logger.NewNopLogger())
	return svc, m
}

// 五源聚合：按 feed 顺序返回卡片，标题/摘要/昵称/点赞/收藏/评论/标签齐全
func TestGRPCFeedService_List_Aggregates(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, m := newFeedBFF(t, ctrl)

	m.feed.EXPECT().ListFeed(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *feedv1.ListFeedRequest, _ ...grpc.CallOption) (*feedv1.ListFeedResponse, error) {
			assert.Equal(t, int64(42), in.GetUid())
			assert.Equal(t, int32(10), in.GetLimit())
			return &feedv1.ListFeedResponse{
				Items:      []*feedv1.FeedItem{{ArticleId: 200, PublishedAt: 2000}, {ArticleId: 100, PublishedAt: 1000}},
				NextCursor: 1000, HasMore: true,
			}, nil
		})
	m.article.EXPECT().BatchDetail(gomock.Any(), []int64{200, 100}).Return([]domain.Article{
		{Id: 100, Title: "T100", Abstract: "A100", Author: domain.Author{Id: 7}},
		{Id: 200, Title: "T200", Abstract: "A200", Author: domain.Author{Id: 8}},
	}, nil)
	m.intr.EXPECT().FindByBizIds(gomock.Any(), domain.BizArticle, []int64{200, 100}).
		Return(map[int64]domain.Interaction{
			200: {LikeCount: 12, CollectCount: 3},
			100: {LikeCount: 5, CollectCount: 1},
		}, nil)
	m.comment.EXPECT().BatchCountComment(gomock.Any(), gomock.Any()).
		Return(&commentv1.BatchCountCommentResponse{Counts: map[int64]int64{200: 4, 100: 2}}, nil)
	m.tag.EXPECT().TagsByBiz(gomock.Any(), domain.BizArticle, []int64{200, 100}).
		Return(map[int64][]domain.Tag{200: {{Id: 3, Name: "Go"}}}, nil)
	m.user.EXPECT().FindByIds(gomock.Any(), gomock.Any()).
		Return(map[int64]domain.User{7: {Id: 7, Nickname: "阿哲"}, 8: {Id: 8, Nickname: "小明"}}, nil)

	items, next, hasMore, err := svc.List(context.Background(), 42, 0, 10)
	require.NoError(t, err)
	require.Len(t, items, 2)
	// feed 顺序：200 在前
	assert.Equal(t, int64(200), items[0].ArticleId)
	assert.Equal(t, "T200", items[0].Title)
	assert.Equal(t, int64(2000), items[0].PublishedAt)
	assert.Equal(t, "小明", items[0].Author.Name)
	assert.Equal(t, int64(12), items[0].LikeCnt)
	assert.Equal(t, int64(3), items[0].CollectCnt)
	assert.Equal(t, int64(4), items[0].CommentCnt)
	assert.Equal(t, []domain.Tag{{Id: 3, Name: "Go"}}, items[0].Tags)
	assert.Equal(t, int64(100), items[1].ArticleId)
	assert.Equal(t, "阿哲", items[1].Author.Name)
	assert.Equal(t, int64(1000), next)
	assert.True(t, hasMore)
}

// 撤回过滤：feed 返回 2 条，article 只查到 1 条（另一条撤回）→ 只出 1 张卡
func TestGRPCFeedService_List_WithdrawnFiltered(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, m := newFeedBFF(t, ctrl)

	m.feed.EXPECT().ListFeed(gomock.Any(), gomock.Any()).Return(&feedv1.ListFeedResponse{
		Items: []*feedv1.FeedItem{{ArticleId: 200, PublishedAt: 2000}, {ArticleId: 100, PublishedAt: 1000}},
	}, nil)
	// 200 已撤回 → 不在 published_article，BatchDetail 只返回 100
	m.article.EXPECT().BatchDetail(gomock.Any(), []int64{200, 100}).Return([]domain.Article{
		{Id: 100, Title: "T100", Author: domain.Author{Id: 7}},
	}, nil)
	m.intr.EXPECT().FindByBizIds(gomock.Any(), domain.BizArticle, gomock.Any()).Return(map[int64]domain.Interaction{}, nil)
	m.comment.EXPECT().BatchCountComment(gomock.Any(), gomock.Any()).Return(&commentv1.BatchCountCommentResponse{Counts: map[int64]int64{}}, nil)
	m.tag.EXPECT().TagsByBiz(gomock.Any(), domain.BizArticle, gomock.Any()).Return(map[int64][]domain.Tag{}, nil)
	m.user.EXPECT().FindByIds(gomock.Any(), gomock.Any()).Return(map[int64]domain.User{7: {Nickname: "阿哲"}}, nil)

	items, _, _, err := svc.List(context.Background(), 42, 0, 10)
	require.NoError(t, err)
	require.Len(t, items, 1, "撤回的 200 被过滤")
	assert.Equal(t, int64(100), items[0].ArticleId)
}

// 聚合源全部失败：卡片仍返回，计数填零、无标签、空昵称
func TestGRPCFeedService_List_AggregationDegrades(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, m := newFeedBFF(t, ctrl)

	m.feed.EXPECT().ListFeed(gomock.Any(), gomock.Any()).Return(&feedv1.ListFeedResponse{
		Items: []*feedv1.FeedItem{{ArticleId: 100, PublishedAt: 1000}},
	}, nil)
	m.article.EXPECT().BatchDetail(gomock.Any(), []int64{100}).Return([]domain.Article{
		{Id: 100, Title: "T100", Author: domain.Author{Id: 7}},
	}, nil)
	m.intr.EXPECT().FindByBizIds(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("intr down"))
	m.comment.EXPECT().BatchCountComment(gomock.Any(), gomock.Any()).Return(nil, errors.New("comment down"))
	m.tag.EXPECT().TagsByBiz(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("tag down"))
	m.user.EXPECT().FindByIds(gomock.Any(), gomock.Any()).Return(nil, errors.New("user down"))

	items, _, _, err := svc.List(context.Background(), 42, 0, 10)
	require.NoError(t, err, "聚合源失败应降级不报错")
	require.Len(t, items, 1)
	assert.Equal(t, int64(0), items[0].LikeCnt)
	assert.Equal(t, int64(0), items[0].CommentCnt)
	assert.Empty(t, items[0].Tags)
	assert.Empty(t, items[0].Author.Name)
}

// NewCount：feed 给候选 id，BFF 用 BatchDetail 过滤掉已撤回/软删的，返回真正可见的新文章数
func TestGRPCFeedService_NewCount(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, m := newFeedBFF(t, ctrl)

	m.feed.EXPECT().NewCount(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *feedv1.NewCountRequest, _ ...grpc.CallOption) (*feedv1.NewCountResponse, error) {
			assert.Equal(t, int64(42), in.GetUid())
			assert.Equal(t, int64(5000), in.GetSinceCursor())
			return &feedv1.NewCountResponse{ArticleIds: []int64{12, 11, 10}}, nil
		})
	// 3 个候选，12 已撤回不在线上库 → CountByIds 只数到 11、10 两条可见 → count=2
	m.article.EXPECT().CountByIds(gomock.Any(), []int64{12, 11, 10}).Return(int64(2), nil)

	n, err := svc.NewCount(context.Background(), 42, 5000)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), n)
}

// 空关注流：透传 nextCursor/hasMore，不发起聚合；limit 越界夹取到 20
func TestGRPCFeedService_List_EmptyAndLimitClamp(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, m := newFeedBFF(t, ctrl)

	m.feed.EXPECT().ListFeed(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *feedv1.ListFeedRequest, _ ...grpc.CallOption) (*feedv1.ListFeedResponse, error) {
			assert.Equal(t, int32(20), in.GetLimit(), "limit 100 夹取到 20")
			return &feedv1.ListFeedResponse{Items: nil, NextCursor: 0, HasMore: false}, nil
		})

	items, next, hasMore, err := svc.List(context.Background(), 42, 0, 100)
	require.NoError(t, err)
	assert.Empty(t, items)
	assert.Equal(t, int64(0), next)
	assert.False(t, hasMore)
}
