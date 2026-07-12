package main

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"

	searchv1 "github.com/boyxs/train-go/webook/api/gen/search/v1"
	tagv1 "github.com/boyxs/train-go/webook/api/gen/tag/v1"
	"github.com/boyxs/train-go/webook/internal/repository/dao"
	daomocks "github.com/boyxs/train-go/webook/internal/repository/dao/mocks"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

// stubSearchClient 只实现 IndexArticle，其余方法由内嵌接口（nil）兜底——本测试不触发。
type stubSearchClient struct {
	searchv1.SearchServiceClient
	indexed []*searchv1.ArticleDoc
	failIds map[int64]bool
}

func (s *stubSearchClient) IndexArticle(_ context.Context, in *searchv1.IndexArticleRequest, _ ...grpc.CallOption) (*searchv1.IndexArticleResponse, error) {
	if s.failIds[in.GetDoc().GetId()] {
		return nil, errors.New("es down")
	}
	s.indexed = append(s.indexed, in.GetDoc())
	return &searchv1.IndexArticleResponse{}, nil
}

// stubTagClient 只实现 TagsByBiz。
type stubTagClient struct {
	tagv1.TagServiceClient
	resp *tagv1.TagsByBizResponse
	err  error
}

func (s *stubTagClient) TagsByBiz(_ context.Context, _ *tagv1.TagsByBizRequest, _ ...grpc.CallOption) (*tagv1.TagsByBizResponse, error) {
	return s.resp, s.err
}

func TestSearchBackfiller_Run(t *testing.T) {
	t.Run("按当前标签逐篇索引，两批（数据批 + 空批终止）", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		rows := []dao.PublishedArticle{
			{Id: 1, Title: "a", Abstract: "ab", AuthorId: 9, Status: 2, Category: "go", CreatedAt: 100},
			{Id: 2, Title: "b", AuthorId: 9, Status: 2},
		}
		d := daomocks.NewMockArticleReaderDAO(ctrl)
		d.EXPECT().Count(gomock.Any()).Return(int64(2), nil)
		d.EXPECT().Page(gomock.Any(), 0, backfillBatch).Return(rows, nil)
		d.EXPECT().Page(gomock.Any(), backfillBatch, backfillBatch).Return(nil, nil)

		tagCli := &stubTagClient{resp: &tagv1.TagsByBizResponse{Tags: map[int64]*tagv1.TagList{
			1: {Tags: []*tagv1.Tag{{Slug: "golang"}, {Slug: "backend"}}},
		}}}
		searchCli := &stubSearchClient{}

		err := NewSearchBackfiller(d, searchCli, tagCli, logger.NewNopLogger()).Run(context.Background())
		assert.NoError(t, err)
		assert.Len(t, searchCli.indexed, 2)
		// id1 带当前标签 + category；author_name 恒空（与发布索引语义一致）
		assert.Equal(t, []string{"golang", "backend"}, searchCli.indexed[0].GetTags())
		assert.Equal(t, "go", searchCli.indexed[0].GetCategory())
		assert.Empty(t, searchCli.indexed[0].GetAuthorName())
		assert.EqualValues(t, 100, searchCli.indexed[0].GetCreatedAt())
		// id2 无标签 → 空 slice，仍照常索引
		assert.Empty(t, searchCli.indexed[1].GetTags())
	})

	t.Run("取标签失败：整批降级不带标签，不中断", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		rows := []dao.PublishedArticle{{Id: 1, Title: "a", Status: 2}}
		d := daomocks.NewMockArticleReaderDAO(ctrl)
		d.EXPECT().Count(gomock.Any()).Return(int64(1), nil)
		d.EXPECT().Page(gomock.Any(), 0, backfillBatch).Return(rows, nil)
		d.EXPECT().Page(gomock.Any(), backfillBatch, backfillBatch).Return(nil, nil)

		tagCli := &stubTagClient{err: errors.New("tag svc down")}
		searchCli := &stubSearchClient{}

		err := NewSearchBackfiller(d, searchCli, tagCli, logger.NewNopLogger()).Run(context.Background())
		assert.NoError(t, err)
		assert.Len(t, searchCli.indexed, 1)
		assert.Empty(t, searchCli.indexed[0].GetTags())
	})

	t.Run("单篇索引失败：计数并最终返错，其余照常", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		rows := []dao.PublishedArticle{{Id: 1, Status: 2}, {Id: 2, Status: 2}}
		d := daomocks.NewMockArticleReaderDAO(ctrl)
		d.EXPECT().Count(gomock.Any()).Return(int64(2), nil)
		d.EXPECT().Page(gomock.Any(), 0, backfillBatch).Return(rows, nil)
		d.EXPECT().Page(gomock.Any(), backfillBatch, backfillBatch).Return(nil, nil)

		tagCli := &stubTagClient{resp: &tagv1.TagsByBizResponse{}}
		searchCli := &stubSearchClient{failIds: map[int64]bool{2: true}}

		err := NewSearchBackfiller(d, searchCli, tagCli, logger.NewNopLogger()).Run(context.Background())
		assert.Error(t, err)
		assert.Len(t, searchCli.indexed, 1) // 只有 id1 成功
		assert.EqualValues(t, 1, searchCli.indexed[0].GetId())
	})

	t.Run("读源库失败：致命错误直接返回", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		d := daomocks.NewMockArticleReaderDAO(ctrl)
		d.EXPECT().Count(gomock.Any()).Return(int64(0), errors.New("db down"))

		err := NewSearchBackfiller(d, &stubSearchClient{}, &stubTagClient{}, logger.NewNopLogger()).Run(context.Background())
		assert.Error(t, err)
	})
}
