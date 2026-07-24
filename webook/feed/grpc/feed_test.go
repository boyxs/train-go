package grpc_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	googlegrpc "google.golang.org/grpc"

	feedv1 "github.com/boyxs/train-go/webook/api/gen/feed/v1"
	"github.com/boyxs/train-go/webook/feed/domain"
	"github.com/boyxs/train-go/webook/feed/errs"
	feedgrpc "github.com/boyxs/train-go/webook/feed/grpc"
	svcmocks "github.com/boyxs/train-go/webook/feed/service/mocks"
)

func newServer(t *testing.T, ctrl *gomock.Controller) (*feedgrpc.FeedServer, *svcmocks.MockFeedService) {
	t.Helper()
	svc := svcmocks.NewMockFeedService(ctrl)
	return feedgrpc.NewFeedServer(svc), svc
}

// ListFeed：透传 uid/cursor/limit，domain→pb 映射，回填 next_cursor/has_more
func TestFeedServer_ListFeed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	srv, svc := newServer(t, ctrl)

	svc.EXPECT().ListFeed(gomock.Any(), int64(1), int64(500), 10).
		Return([]domain.FeedItem{{ArticleId: 100, PublishedAt: 1000}}, int64(1000), true, nil)

	resp, err := srv.ListFeed(context.Background(), &feedv1.ListFeedRequest{Uid: 1, Cursor: 500, Limit: 10})
	assert.NoError(t, err)
	assert.Len(t, resp.GetItems(), 1)
	assert.Equal(t, int64(100), resp.GetItems()[0].GetArticleId())
	assert.Equal(t, int64(1000), resp.GetItems()[0].GetPublishedAt())
	assert.Equal(t, int64(1000), resp.GetNextCursor())
	assert.True(t, resp.GetHasMore())
}

// FanoutArticle：pb→domain.FeedArticle 映射
func TestFeedServer_FanoutArticle(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	srv, svc := newServer(t, ctrl)

	svc.EXPECT().Fanout(gomock.Any(), domain.FeedArticle{ArticleId: 1001, AuthorId: 7, PublishedAt: 5000}).Return(nil)

	resp, err := srv.FanoutArticle(context.Background(), &feedv1.FanoutArticleRequest{ArticleId: 1001, AuthorId: 7, PublishedAt: 5000})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
}

// RemoveArticle / InvalidateInboxes 透传
func TestFeedServer_RemoveAndInvalidate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	srv, svc := newServer(t, ctrl)

	svc.EXPECT().Remove(gomock.Any(), int64(1001), int64(7)).Return(nil)
	svc.EXPECT().InvalidateInboxes(gomock.Any(), []int64{1, 2}).Return(nil)

	_, err := srv.RemoveArticle(context.Background(), &feedv1.RemoveArticleRequest{ArticleId: 1001, AuthorId: 7})
	assert.NoError(t, err)
	_, err = srv.InvalidateInboxes(context.Background(), &feedv1.InvalidateInboxesRequest{Uids: []int64{1, 2}})
	assert.NoError(t, err)
}

// service 出错 → 原样返回 err（由 errconv 拦截器转 status）
func TestFeedServer_ListFeed_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	srv, svc := newServer(t, ctrl)

	svc.EXPECT().ListFeed(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, int64(0), false, errors.New("boom"))

	resp, err := srv.ListFeed(context.Background(), &feedv1.ListFeedRequest{Uid: 1, Cursor: 0, Limit: 10})
	assert.Error(t, err)
	assert.Nil(t, resp)
}

// 校验拦截器：uid<=0 拒绝，handler 不执行
func TestValidate_ListFeed_BadUid(t *testing.T) {
	called := false
	handler := func(ctx context.Context, req any) (any, error) { called = true; return nil, nil }
	_, err := feedgrpc.ValidateUnaryInterceptor(context.Background(), &feedv1.ListFeedRequest{Uid: 0}, &googlegrpc.UnaryServerInfo{}, handler)
	assert.ErrorIs(t, err, errs.ErrInvalidArg)
	assert.False(t, called)
}

// 校验拦截器：Fanout 缺 id 拒绝
func TestValidate_Fanout_MissingIds(t *testing.T) {
	handler := func(ctx context.Context, req any) (any, error) { return nil, nil }
	_, err := feedgrpc.ValidateUnaryInterceptor(context.Background(), &feedv1.FanoutArticleRequest{ArticleId: 0, AuthorId: 7}, &googlegrpc.UnaryServerInfo{}, handler)
	assert.ErrorIs(t, err, errs.ErrInvalidArg)
}

// 校验拦截器：合法请求放行 handler
func TestValidate_Valid_CallsHandler(t *testing.T) {
	called := false
	handler := func(ctx context.Context, req any) (any, error) { called = true; return "ok", nil }
	got, err := feedgrpc.ValidateUnaryInterceptor(context.Background(), &feedv1.FanoutArticleRequest{ArticleId: 1, AuthorId: 2}, &googlegrpc.UnaryServerInfo{}, handler)
	assert.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, "ok", got)
}
