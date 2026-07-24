package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"

	articlev1 "github.com/boyxs/train-go/webook/api/gen/article/v1"
	relationv1 "github.com/boyxs/train-go/webook/api/gen/relation/v1"
	"github.com/boyxs/train-go/webook/feed/domain"
	repomocks "github.com/boyxs/train-go/webook/feed/repository/mocks"
	"github.com/boyxs/train-go/webook/feed/service"
	grpcmocks "github.com/boyxs/train-go/webook/feed/service/grpcmocks"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

func newFeedSvc(t *testing.T, ctrl *gomock.Controller) (
	service.FeedService,
	*repomocks.MockFeedRepository,
	*grpcmocks.MockRelationServiceClient,
	*grpcmocks.MockArticleReaderServiceClient,
) {
	t.Helper()
	repo := repomocks.NewMockFeedRepository(ctrl)
	rel := grpcmocks.NewMockRelationServiceClient(ctrl)
	art := grpcmocks.NewMockArticleReaderServiceClient(ctrl)
	cfg := service.Config{BigVThreshold: 1000, FanoutBatch: 50, RebuildMaxFollowees: 1000, OutboxSize: 100}
	svc := service.NewInternalFeedService(repo, rel, art, cfg, logger.NewNopLogger())
	return svc, repo, rel, art
}

func statsResp(followerCnt int64) *relationv1.GetStatsResponse {
	return &relationv1.GetStatsResponse{Stats: &relationv1.RelationStats{FollowerCnt: followerCnt}}
}

func fitem(articleId, publishedAt int64) domain.FeedItem {
	return domain.FeedItem{ArticleId: articleId, PublishedAt: publishedAt}
}

// 普通作者：追加 outbox（存在才追加）+ 单批扩散进粉丝收件箱
func TestInternalFeedService_Fanout_NormalAuthor_SingleBatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, repo, rel, _ := newFeedSvc(t, ctrl)
	wantItem := domain.FeedItem{ArticleId: 1001, PublishedAt: 5000}

	rel.EXPECT().GetStats(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *relationv1.GetStatsRequest, _ ...grpc.CallOption) (*relationv1.GetStatsResponse, error) {
			assert.Equal(t, int64(7), in.GetUid())
			return statsResp(3), nil
		})
	repo.EXPECT().AppendOutboxIfExists(gomock.Any(), int64(7), wantItem).Return(nil)
	rel.EXPECT().ListFollowers(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *relationv1.ListRequest, _ ...grpc.CallOption) (*relationv1.ListFollowResponse, error) {
			assert.Equal(t, int64(7), in.GetUid())
			assert.Equal(t, int64(0), in.GetCursor())
			assert.Equal(t, int32(50), in.GetLimit())
			return &relationv1.ListFollowResponse{
				Edges: []*relationv1.FollowEdge{{FollowerId: 11}, {FollowerId: 22}}, NextCursor: 0,
			}, nil
		})
	repo.EXPECT().AppendInbox(gomock.Any(), []int64{11, 22}, wantItem).Return(nil)

	err := svc.Fanout(context.Background(), domain.FeedArticle{ArticleId: 1001, AuthorId: 7, PublishedAt: 5000})
	assert.NoError(t, err)
}

// 普通作者：ListFollowers 游标循环，逐批扩散
func TestInternalFeedService_Fanout_NormalAuthor_MultiBatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, repo, rel, _ := newFeedSvc(t, ctrl)
	wantItem := domain.FeedItem{ArticleId: 1001, PublishedAt: 5000}

	rel.EXPECT().GetStats(gomock.Any(), gomock.Any()).Return(statsResp(3), nil)
	repo.EXPECT().AppendOutboxIfExists(gomock.Any(), int64(7), wantItem).Return(nil)
	rel.EXPECT().ListFollowers(gomock.Any(), gomock.Any()).Times(2).DoAndReturn(
		func(_ context.Context, in *relationv1.ListRequest, _ ...grpc.CallOption) (*relationv1.ListFollowResponse, error) {
			if in.GetCursor() == 0 {
				return &relationv1.ListFollowResponse{Edges: []*relationv1.FollowEdge{{FollowerId: 11}}, NextCursor: 99}, nil
			}
			assert.Equal(t, int64(99), in.GetCursor())
			return &relationv1.ListFollowResponse{Edges: []*relationv1.FollowEdge{{FollowerId: 22}}, NextCursor: 0}, nil
		})
	repo.EXPECT().AppendInbox(gomock.Any(), []int64{11}, wantItem).Return(nil)
	repo.EXPECT().AppendInbox(gomock.Any(), []int64{22}, wantItem).Return(nil)

	err := svc.Fanout(context.Background(), domain.FeedArticle{ArticleId: 1001, AuthorId: 7, PublishedAt: 5000})
	assert.NoError(t, err)
}

// 大V（粉丝数 >= 阈值）：只追加 outbox，跳过写扩散（不调 ListFollowers / AppendInbox）
func TestInternalFeedService_Fanout_BigV_SkipsFanout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, repo, rel, _ := newFeedSvc(t, ctrl)
	wantItem := domain.FeedItem{ArticleId: 1001, PublishedAt: 5000}

	rel.EXPECT().GetStats(gomock.Any(), gomock.Any()).Return(statsResp(1000), nil) // == 阈值
	repo.EXPECT().AppendOutboxIfExists(gomock.Any(), int64(7), wantItem).Return(nil)
	// 不设 ListFollowers / AppendInbox 的 EXPECT：若被调用 gomock 报 unexpected call

	err := svc.Fanout(context.Background(), domain.FeedArticle{ArticleId: 1001, AuthorId: 7, PublishedAt: 5000})
	assert.NoError(t, err)
}

// 空粉丝：ListFollowers 返回空 → 不 AppendInbox
func TestInternalFeedService_Fanout_NoFollowers(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, repo, rel, _ := newFeedSvc(t, ctrl)
	wantItem := domain.FeedItem{ArticleId: 1001, PublishedAt: 5000}

	rel.EXPECT().GetStats(gomock.Any(), gomock.Any()).Return(statsResp(0), nil)
	repo.EXPECT().AppendOutboxIfExists(gomock.Any(), int64(7), wantItem).Return(nil)
	rel.EXPECT().ListFollowers(gomock.Any(), gomock.Any()).Return(&relationv1.ListFollowResponse{Edges: nil, NextCursor: 0}, nil)

	err := svc.Fanout(context.Background(), domain.FeedArticle{ArticleId: 1001, AuthorId: 7, PublishedAt: 5000})
	assert.NoError(t, err)
}

// GetStats 失败 → 整体失败（consumer 整批重投，扩散幂等安全）
func TestInternalFeedService_Fanout_GetStatsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, _, rel, _ := newFeedSvc(t, ctrl)

	rel.EXPECT().GetStats(gomock.Any(), gomock.Any()).Return(nil, errors.New("relation 不可用"))

	err := svc.Fanout(context.Background(), domain.FeedArticle{ArticleId: 1001, AuthorId: 7, PublishedAt: 5000})
	assert.Error(t, err)
}

// 撤回 = 读时过滤（inbox 不摘除）+ DEL 作者 outbox
func TestInternalFeedService_Remove(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, repo, _, _ := newFeedSvc(t, ctrl)

	repo.EXPECT().DelOutbox(gomock.Any(), int64(7)).Return(nil)

	err := svc.Remove(context.Background(), 1001, 7)
	assert.NoError(t, err)
}

// 关系变更失效重建：转交 repo.Invalidate
func TestInternalFeedService_InvalidateInboxes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, repo, _, _ := newFeedSvc(t, ctrl)

	repo.EXPECT().Invalidate(gomock.Any(), []int64{1, 2, 3}).Return(nil)

	err := svc.InvalidateInboxes(context.Background(), []int64{1, 2, 3})
	assert.NoError(t, err)
}

// NewCount：built 时返回收件箱 since 以来的候选 id；未 built 返回 nil（不查）
func TestInternalFeedService_NewCount(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, repo, _, _ := newFeedSvc(t, ctrl)

	repo.EXPECT().InboxBuilt(gomock.Any(), int64(1)).Return(true, nil)
	repo.EXPECT().InboxSince(gomock.Any(), int64(1), int64(5000), gomock.Any()).Return([]int64{300, 200}, nil)
	ids, err := svc.NewCount(context.Background(), 1, 5000)
	assert.NoError(t, err)
	assert.Equal(t, []int64{300, 200}, ids)
}

func TestInternalFeedService_NewCount_NotBuilt(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, repo, _, _ := newFeedSvc(t, ctrl)

	repo.EXPECT().InboxBuilt(gomock.Any(), int64(1)).Return(false, nil)
	// 未 built 不应调 InboxSince
	ids, err := svc.NewCount(context.Background(), 1, 5000)
	assert.NoError(t, err)
	assert.Empty(t, ids)
}

// built=true 且无大V：只读收件箱，按 score DESC，hasMore=len==limit
func TestInternalFeedService_ListFeed_BuiltInboxOnly(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, repo, _, _ := newFeedSvc(t, ctrl)

	repo.EXPECT().InboxBuilt(gomock.Any(), int64(1)).Return(true, nil)
	repo.EXPECT().ReadInbox(gomock.Any(), int64(1), int64(0), 10).
		Return([]domain.FeedItem{fitem(300, 3000), fitem(200, 2000)}, nil)
	repo.EXPECT().ReadBigv(gomock.Any(), int64(1)).Return(nil, nil)

	items, next, hasMore, err := svc.ListFeed(context.Background(), 1, 0, 10)
	assert.NoError(t, err)
	assert.Equal(t, []domain.FeedItem{fitem(300, 3000), fitem(200, 2000)}, items)
	assert.Equal(t, int64(2000), next)
	assert.False(t, hasMore)
}

// built=true + 大V：归并收件箱与大V发件箱，score DESC
func TestInternalFeedService_ListFeed_MergeBigV(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, repo, _, _ := newFeedSvc(t, ctrl)

	repo.EXPECT().InboxBuilt(gomock.Any(), int64(1)).Return(true, nil)
	repo.EXPECT().ReadInbox(gomock.Any(), int64(1), int64(0), 10).
		Return([]domain.FeedItem{fitem(300, 3000), fitem(100, 1000)}, nil)
	repo.EXPECT().ReadBigv(gomock.Any(), int64(1)).Return([]int64{7}, nil)
	repo.EXPECT().ReadOutbox(gomock.Any(), int64(7), int64(0), 10).
		Return([]domain.FeedItem{fitem(200, 2000)}, nil)

	items, next, hasMore, err := svc.ListFeed(context.Background(), 1, 0, 10)
	assert.NoError(t, err)
	assert.Equal(t, []domain.FeedItem{fitem(300, 3000), fitem(200, 2000), fitem(100, 1000)}, items)
	assert.Equal(t, int64(1000), next)
	assert.False(t, hasMore)
}

// 游标翻页 + hasMore：归并后截断到 limit，nextCursor=末条 publishedAt
func TestInternalFeedService_ListFeed_CursorPaging(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, repo, _, _ := newFeedSvc(t, ctrl)

	repo.EXPECT().InboxBuilt(gomock.Any(), int64(1)).Return(true, nil)
	repo.EXPECT().ReadInbox(gomock.Any(), int64(1), int64(5000), 2).
		Return([]domain.FeedItem{fitem(400, 4000), fitem(300, 3000)}, nil)
	repo.EXPECT().ReadBigv(gomock.Any(), int64(1)).Return([]int64{7}, nil)
	repo.EXPECT().ReadOutbox(gomock.Any(), int64(7), int64(5000), 2).
		Return([]domain.FeedItem{fitem(350, 3500)}, nil)

	items, next, hasMore, err := svc.ListFeed(context.Background(), 1, 5000, 2)
	assert.NoError(t, err)
	assert.Equal(t, []domain.FeedItem{fitem(400, 4000), fitem(350, 3500)}, items, "取 top-2")
	assert.Equal(t, int64(3500), next)
	assert.True(t, hasMore, "归并满 limit → 还有更多")
}

// 降级：单个大V outbox 读失败 → 跳过该作者，其余照常返回，不报错
func TestInternalFeedService_ListFeed_BigVDegrades(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, repo, _, _ := newFeedSvc(t, ctrl)

	repo.EXPECT().InboxBuilt(gomock.Any(), int64(1)).Return(true, nil)
	repo.EXPECT().ReadInbox(gomock.Any(), int64(1), int64(5000), 10).
		Return([]domain.FeedItem{fitem(300, 3000)}, nil)
	repo.EXPECT().ReadBigv(gomock.Any(), int64(1)).Return([]int64{7, 8}, nil)
	repo.EXPECT().ReadOutbox(gomock.Any(), int64(7), int64(5000), 10).Return(nil, errors.New("redis 抖动"))
	repo.EXPECT().ReadOutbox(gomock.Any(), int64(8), int64(5000), 10).
		Return([]domain.FeedItem{fitem(200, 2000)}, nil)

	items, _, _, err := svc.ListFeed(context.Background(), 1, 5000, 10)
	assert.NoError(t, err, "单作者失败应降级不报错")
	assert.Equal(t, []domain.FeedItem{fitem(300, 3000), fitem(200, 2000)}, items, "跳过失败的 7，保留 8")
}

// miss 重建：built=false → ListFollowees + BatchGetStats 二分大V/普通 → 普通作者 outbox 回源 →
// SaveInbox(普通文章, 大V集) → 再读收件箱 + 归并大V outbox
func TestInternalFeedService_ListFeed_RebuildOnMiss(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, repo, rel, art := newFeedSvc(t, ctrl)

	repo.EXPECT().InboxBuilt(gomock.Any(), int64(1)).Return(false, nil)
	// 关注 10（普通）+ 20（大V）
	rel.EXPECT().ListFollowees(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *relationv1.ListRequest, _ ...grpc.CallOption) (*relationv1.ListFollowResponse, error) {
			assert.Equal(t, int64(1), in.GetUid())
			return &relationv1.ListFollowResponse{
				Edges: []*relationv1.FollowEdge{{FolloweeId: 10}, {FolloweeId: 20}}, NextCursor: 0,
			}, nil
		})
	rel.EXPECT().BatchGetStats(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *relationv1.BatchGetStatsRequest, _ ...grpc.CallOption) (*relationv1.BatchGetStatsResponse, error) {
			assert.ElementsMatch(t, []int64{10, 20}, in.GetUids())
			return &relationv1.BatchGetStatsResponse{Stats: map[int64]*relationv1.RelationStats{
				10: {FollowerCnt: 5}, 20: {FollowerCnt: 2000},
			}}, nil
		})
	// 普通作者 10 outbox 冷 → 回源
	repo.EXPECT().ReadOutbox(gomock.Any(), int64(10), int64(0), 100).Return(nil, nil)
	art.EXPECT().ListAuthorArticles(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *articlev1.ListAuthorArticlesRequest, _ ...grpc.CallOption) (*articlev1.ListAuthorArticlesResponse, error) {
			assert.Equal(t, int64(10), in.GetAuthorId())
			assert.Equal(t, int32(100), in.GetLimit())
			return &articlev1.ListAuthorArticlesResponse{Items: []*articlev1.FeedArticleBrief{{Id: 101, PublishedAt: 1010}}}, nil
		})
	repo.EXPECT().FillOutbox(gomock.Any(), int64(10), []domain.FeedItem{fitem(101, 1010)}).Return(nil)
	repo.EXPECT().ReadOutbox(gomock.Any(), int64(10), int64(0), 100).Return([]domain.FeedItem{fitem(101, 1010)}, nil)
	// 重建落库：普通文章进收件箱，大V 20 进 bigv 集
	repo.EXPECT().SaveInbox(gomock.Any(), int64(1), []domain.FeedItem{fitem(101, 1010)}, []int64{20}).Return(nil)
	// 重建后读
	repo.EXPECT().ReadInbox(gomock.Any(), int64(1), int64(0), 10).Return([]domain.FeedItem{fitem(101, 1010)}, nil)
	repo.EXPECT().ReadBigv(gomock.Any(), int64(1)).Return([]int64{20}, nil)
	repo.EXPECT().ReadOutbox(gomock.Any(), int64(20), int64(0), 10).Return([]domain.FeedItem{fitem(201, 2010)}, nil)

	items, next, _, err := svc.ListFeed(context.Background(), 1, 0, 10)
	assert.NoError(t, err)
	assert.Equal(t, []domain.FeedItem{fitem(201, 2010), fitem(101, 1010)}, items)
	assert.Equal(t, int64(1010), next)
}

// 空关注重建：ListFollowees 为空 → SaveInbox(nil,nil) 置 built，返回空流
func TestInternalFeedService_ListFeed_RebuildEmptyFollowees(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, repo, rel, _ := newFeedSvc(t, ctrl)

	repo.EXPECT().InboxBuilt(gomock.Any(), int64(1)).Return(false, nil)
	rel.EXPECT().ListFollowees(gomock.Any(), gomock.Any()).Return(&relationv1.ListFollowResponse{Edges: nil, NextCursor: 0}, nil)
	repo.EXPECT().SaveInbox(gomock.Any(), int64(1), nil, nil).Return(nil)
	repo.EXPECT().ReadInbox(gomock.Any(), int64(1), int64(0), 10).Return(nil, nil)
	repo.EXPECT().ReadBigv(gomock.Any(), int64(1)).Return(nil, nil)

	items, _, hasMore, err := svc.ListFeed(context.Background(), 1, 0, 10)
	assert.NoError(t, err)
	assert.Empty(t, items)
	assert.False(t, hasMore)
}
