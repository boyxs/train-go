package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	searchv1 "github.com/boyxs/train-go/webook/api/gen/search/v1"
	tagv1 "github.com/boyxs/train-go/webook/api/gen/tag/v1"
	"github.com/boyxs/train-go/webook/internal/domain"
	"github.com/boyxs/train-go/webook/internal/service"
	svcmocks "github.com/boyxs/train-go/webook/internal/service/mocks"
	grpcmocks "github.com/boyxs/train-go/webook/internal/web/grpcmocks"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

// newSearchSvc 装配 GRPCArticleSearchService：搜索/标签下游 gRPC client + 互动 service 全 mock。
func newSearchSvc(t *testing.T, ctrl *gomock.Controller) (
	service.ArticleSearchService,
	*grpcmocks.MockSearchServiceClient,
	*grpcmocks.MockTagServiceClient,
	*svcmocks.MockInteractionService,
) {
	t.Helper()
	searchCli := grpcmocks.NewMockSearchServiceClient(ctrl)
	tagCli := grpcmocks.NewMockTagServiceClient(ctrl)
	intrSvc := svcmocks.NewMockInteractionService(ctrl)
	svc := service.NewGRPCArticleSearchService(searchCli, tagCli, intrSvc, logger.NewNopLogger())
	return svc, searchCli, tagCli, intrSvc
}

// Search 正常路径：命中文章 + facet；并发补标签名(tag) + 互动计数(interaction)。
func TestGRPCArticleSearchService_Search_Aggregates(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, searchCli, tagCli, intrSvc := newSearchSvc(t, ctrl)

	searchCli.EXPECT().SearchArticles(gomock.Any(), gomock.Any()).Return(&searchv1.SearchArticlesResponse{
		Articles: []*searchv1.ArticleCard{
			{Id: 1, Title: "Go 并发", AuthorId: 10, AuthorName: "张三", Tags: []string{"go"}, CreatedAt: 100},
		},
		Total:  1,
		Facets: []*searchv1.TagCount{{Slug: "go", Count: 5}, {Slug: "rust", Count: 2}},
	}, nil)
	tagCli.EXPECT().BatchBySlugs(gomock.Any(), gomock.Any()).Return(&tagv1.TagList{
		Tags: []*tagv1.Tag{{Slug: "go", Name: "Go"}, {Slug: "rust", Name: "Rust"}},
	}, nil)
	intrSvc.EXPECT().FindByBizIds(gomock.Any(), domain.BizArticle, []int64{1}).
		Return(map[int64]domain.Interaction{1: {ReadCount: 10, LikeCount: 3, CollectCount: 2}}, nil)

	res, err := svc.Search(context.Background(), "go", nil, 1, 10)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), res.Total)
	if assert.Len(t, res.Articles, 1) {
		a := res.Articles[0]
		assert.Equal(t, int64(1), a.Id)
		assert.Equal(t, "张三", a.Author.Name)
		assert.Equal(t, int64(10), a.ReadCnt)
		assert.Equal(t, int64(3), a.LikeCnt)
		assert.Equal(t, int64(2), a.CollectCnt)
		if assert.Len(t, a.Tags, 1) {
			assert.Equal(t, "Go", a.Tags[0].Name, "命中标签 slug→name 已解析")
		}
	}
	facetNames := map[string]string{}
	for _, f := range res.Facets {
		facetNames[f.Slug] = f.Name
	}
	assert.Equal(t, "Go", facetNames["go"], "facet 也补了名字")
	assert.Equal(t, "Rust", facetNames["rust"])
}

// Search：标签名解析(tag)失败 → 名字降级用 slug 占位，整体不报错。
func TestGRPCArticleSearchService_Search_TagNameDegrade(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, searchCli, tagCli, intrSvc := newSearchSvc(t, ctrl)

	searchCli.EXPECT().SearchArticles(gomock.Any(), gomock.Any()).Return(&searchv1.SearchArticlesResponse{
		Articles: []*searchv1.ArticleCard{{Id: 1, Tags: []string{"go"}}},
		Total:    1,
		Facets:   []*searchv1.TagCount{{Slug: "go", Count: 5}},
	}, nil)
	tagCli.EXPECT().BatchBySlugs(gomock.Any(), gomock.Any()).Return(nil, errors.New("tag 服务不可用"))
	intrSvc.EXPECT().FindByBizIds(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(map[int64]domain.Interaction{1: {LikeCount: 1}}, nil)

	res, err := svc.Search(context.Background(), "go", nil, 1, 10)
	assert.NoError(t, err, "标签名解析失败应降级不报错")
	if assert.Len(t, res.Articles, 1) && assert.Len(t, res.Articles[0].Tags, 1) {
		assert.Equal(t, "go", res.Articles[0].Tags[0].Name, "名字缺失用 slug 占位")
	}
	if assert.Len(t, res.Facets, 1) {
		assert.Equal(t, "go", res.Facets[0].Name)
	}
}

// Search：互动计数(interaction)失败 → 计数降级填零，整体不报错。
func TestGRPCArticleSearchService_Search_InteractionDegrade(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, searchCli, tagCli, intrSvc := newSearchSvc(t, ctrl)

	searchCli.EXPECT().SearchArticles(gomock.Any(), gomock.Any()).Return(&searchv1.SearchArticlesResponse{
		Articles: []*searchv1.ArticleCard{{Id: 1, Tags: []string{"go"}}},
		Total:    1,
	}, nil)
	tagCli.EXPECT().BatchBySlugs(gomock.Any(), gomock.Any()).Return(&tagv1.TagList{
		Tags: []*tagv1.Tag{{Slug: "go", Name: "Go"}},
	}, nil)
	intrSvc.EXPECT().FindByBizIds(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("interaction 不可用"))

	res, err := svc.Search(context.Background(), "go", nil, 1, 10)
	assert.NoError(t, err, "互动计数失败应降级填零")
	if assert.Len(t, res.Articles, 1) {
		assert.Equal(t, int64(0), res.Articles[0].ReadCnt)
		assert.Equal(t, int64(0), res.Articles[0].LikeCnt)
	}
}

// Search：下游 search 失败 → 传播错误（非降级），且不触发后续聚合。
func TestGRPCArticleSearchService_Search_DownstreamError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, searchCli, _, _ := newSearchSvc(t, ctrl)

	searchCli.EXPECT().SearchArticles(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("search 不可用"))

	_, err := svc.Search(context.Background(), "go", nil, 1, 10)
	assert.Error(t, err, "search 下游失败应传播")
}

// Index / Remove：薄透传，错误原样传播。
func TestGRPCArticleSearchService_IndexRemove(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, searchCli, _, _ := newSearchSvc(t, ctrl)

	searchCli.EXPECT().IndexArticle(gomock.Any(), gomock.Any()).
		Return(&searchv1.IndexArticleResponse{}, nil)
	assert.NoError(t, svc.Index(context.Background(), domain.Article{Id: 1, Title: "t"}))

	searchCli.EXPECT().RemoveArticle(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("boom"))
	assert.Error(t, svc.Remove(context.Background(), 1))
}
