package consumer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/boyxs/train-go/webook/pkg/saramax"

	feedv1 "github.com/boyxs/train-go/webook/api/gen/feed/v1"
	"github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/worker/consumer/event"
)

// fakeFeedClient 捕获 feed gRPC 调用，供 feed 两个消费者的 handleBatch 单测共用（package consumer 内共享）。
type fakeFeedClient struct {
	fanout      []*feedv1.FanoutArticleRequest
	removed     []*feedv1.RemoveArticleRequest
	invalidated [][]int64
	err         error
}

func (f *fakeFeedClient) FanoutArticle(_ context.Context, in *feedv1.FanoutArticleRequest, _ ...grpc.CallOption) (*feedv1.FanoutArticleResponse, error) {
	f.fanout = append(f.fanout, in)
	return &feedv1.FanoutArticleResponse{}, f.err
}
func (f *fakeFeedClient) RemoveArticle(_ context.Context, in *feedv1.RemoveArticleRequest, _ ...grpc.CallOption) (*feedv1.RemoveArticleResponse, error) {
	f.removed = append(f.removed, in)
	return &feedv1.RemoveArticleResponse{}, f.err
}
func (f *fakeFeedClient) InvalidateInboxes(_ context.Context, in *feedv1.InvalidateInboxesRequest, _ ...grpc.CallOption) (*feedv1.InvalidateInboxesResponse, error) {
	f.invalidated = append(f.invalidated, in.GetUids())
	return &feedv1.InvalidateInboxesResponse{}, f.err
}
func (f *fakeFeedClient) ListFeed(_ context.Context, _ *feedv1.ListFeedRequest, _ ...grpc.CallOption) (*feedv1.ListFeedResponse, error) {
	return &feedv1.ListFeedResponse{}, nil
}
func (f *fakeFeedClient) NewCount(_ context.Context, _ *feedv1.NewCountRequest, _ ...grpc.CallOption) (*feedv1.NewCountResponse, error) {
	return &feedv1.NewCountResponse{}, nil
}

// published→FanoutArticle，withdrawn→RemoveArticle
func TestFeedArticleConsumer_handleBatch(t *testing.T) {
	fake := &fakeFeedClient{}
	c := NewFeedArticleConsumer(nil, fake, saramax.GroupConfig{GroupId: "g"}, logger.NewNopLogger())

	err := c.handleBatch(context.Background(), nil, []event.ArticleEvent{
		{Type: event.ArticleTypePublished, ArticleId: 100, AuthorId: 7, Ts: 2000},
		{Type: event.ArticleTypeWithdrawn, ArticleId: 200, AuthorId: 8},
	})
	require.NoError(t, err)
	require.Len(t, fake.fanout, 1)
	assert.Equal(t, int64(100), fake.fanout[0].GetArticleId())
	assert.Equal(t, int64(7), fake.fanout[0].GetAuthorId())
	assert.Equal(t, int64(2000), fake.fanout[0].GetPublishedAt())
	require.Len(t, fake.removed, 1)
	assert.Equal(t, int64(200), fake.removed[0].GetArticleId())
	assert.Equal(t, int64(8), fake.removed[0].GetAuthorId())
}

// 独立 group：构造时以基础 group 派生 -feed-article 后缀
func TestNewFeedArticleConsumer_DerivesGroup(t *testing.T) {
	c := NewFeedArticleConsumer(nil, &fakeFeedClient{}, saramax.GroupConfig{GroupId: "webook-worker"}, logger.NewNopLogger())
	assert.Equal(t, "webook-worker-feed-article", c.cfg.GroupId)
}
