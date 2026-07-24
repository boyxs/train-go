package setup

import (
	"context"

	"google.golang.org/grpc"

	articlev1 "github.com/boyxs/train-go/webook/api/gen/article/v1"
)

// FakeArticleClient 集成测试桩（固定场景）：作者 7 有一篇已发布文章 101（发布时间 1010）。
type FakeArticleClient struct{}

func NewFakeArticleClient() articlev1.ArticleReaderServiceClient { return &FakeArticleClient{} }

func (f *FakeArticleClient) ListAuthorArticles(_ context.Context, in *articlev1.ListAuthorArticlesRequest, _ ...grpc.CallOption) (*articlev1.ListAuthorArticlesResponse, error) {
	if in.GetAuthorId() == 7 {
		return &articlev1.ListAuthorArticlesResponse{
			Items: []*articlev1.FeedArticleBrief{{Id: 101, PublishedAt: 1010}},
		}, nil
	}
	return &articlev1.ListAuthorArticlesResponse{}, nil
}

// 以下 feed 不调用，返回零值桩。
func (f *FakeArticleClient) GetArticle(_ context.Context, _ *articlev1.GetArticleRequest, _ ...grpc.CallOption) (*articlev1.Article, error) {
	return &articlev1.Article{}, nil
}
func (f *FakeArticleClient) BatchGetArticles(_ context.Context, _ *articlev1.BatchGetArticlesRequest, _ ...grpc.CallOption) (*articlev1.BatchGetArticlesResponse, error) {
	return &articlev1.BatchGetArticlesResponse{}, nil
}
