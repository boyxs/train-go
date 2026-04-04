package service

import (
	"context"
	"errors"
	"testing"

	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository"
	repomocks "gitee.com/train-cloud/geektime-basic-go/internal/repository/mocks"
	aimocks "gitee.com/train-cloud/geektime-basic-go/internal/service/ai/mocks"
	"gitee.com/train-cloud/geektime-basic-go/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

var stubVec = make([]float32, 1024)

func TestSearchService_Search(t *testing.T) {
	articles := []domain.Article{
		{Id: 1, Title: "健身饮食", Abstract: "如何科学饮食"},
		{Id: 2, Title: "跑步技巧", Abstract: "提升跑步效率"},
	}

	testCases := []struct {
		name     string
		query    string
		page     int
		size     int
		mock     func(ctrl *gomock.Controller) (repository.ArticleSearchRepository, *aimocks.MockEmbeddingClient)
		wantList []domain.Article
		wantTotal int64
		wantErr  string
	}{
		{
			name:  "搜索成功有结果",
			query: "健身饮食",
			page:  1,
			size:  10,
			mock: func(ctrl *gomock.Controller) (repository.ArticleSearchRepository, *aimocks.MockEmbeddingClient) {
				repo := repomocks.NewMockArticleSearchRepository(ctrl)
				embed := aimocks.NewMockEmbeddingClient(ctrl)
				embed.EXPECT().Embed(gomock.Any(), "健身饮食").Return(stubVec, nil)
				repo.EXPECT().Search(gomock.Any(), "健身饮食", stubVec, 0, 10).
					Return(articles, int64(2), nil)
				return repo, embed
			},
			wantList:  articles,
			wantTotal: 2,
		},
		{
			name:  "搜索成功空结果",
			query: "量子力学",
			page:  1,
			size:  10,
			mock: func(ctrl *gomock.Controller) (repository.ArticleSearchRepository, *aimocks.MockEmbeddingClient) {
				repo := repomocks.NewMockArticleSearchRepository(ctrl)
				embed := aimocks.NewMockEmbeddingClient(ctrl)
				embed.EXPECT().Embed(gomock.Any(), "量子力学").Return(stubVec, nil)
				repo.EXPECT().Search(gomock.Any(), "量子力学", stubVec, 0, 10).
					Return(nil, int64(0), nil)
				return repo, embed
			},
			wantList:  nil,
			wantTotal: 0,
		},
		{
			name:  "embed 失败",
			query: "test",
			page:  1,
			size:  10,
			mock: func(ctrl *gomock.Controller) (repository.ArticleSearchRepository, *aimocks.MockEmbeddingClient) {
				repo := repomocks.NewMockArticleSearchRepository(ctrl)
				embed := aimocks.NewMockEmbeddingClient(ctrl)
				embed.EXPECT().Embed(gomock.Any(), "test").Return(nil, errors.New("embed error"))
				return repo, embed
			},
			wantErr: "embed error",
		},
		{
			name:  "repo.Search 失败",
			query: "test",
			page:  1,
			size:  10,
			mock: func(ctrl *gomock.Controller) (repository.ArticleSearchRepository, *aimocks.MockEmbeddingClient) {
				repo := repomocks.NewMockArticleSearchRepository(ctrl)
				embed := aimocks.NewMockEmbeddingClient(ctrl)
				embed.EXPECT().Embed(gomock.Any(), "test").Return(stubVec, nil)
				repo.EXPECT().Search(gomock.Any(), "test", stubVec, 0, 10).
					Return(nil, int64(0), errors.New("es down"))
				return repo, embed
			},
			wantErr: "es down",
		},
		{
			name:  "query 全空格视为空",
			query: "   ",
			page:  1,
			size:  10,
			mock: func(ctrl *gomock.Controller) (repository.ArticleSearchRepository, *aimocks.MockEmbeddingClient) {
				return repomocks.NewMockArticleSearchRepository(ctrl), aimocks.NewMockEmbeddingClient(ctrl)
			},
			wantErr: "搜索内容不能为空",
		},
		{
			name:  "page=0 当作 page=1",
			query: "test",
			page:  0,
			size:  10,
			mock: func(ctrl *gomock.Controller) (repository.ArticleSearchRepository, *aimocks.MockEmbeddingClient) {
				repo := repomocks.NewMockArticleSearchRepository(ctrl)
				embed := aimocks.NewMockEmbeddingClient(ctrl)
				embed.EXPECT().Embed(gomock.Any(), "test").Return(stubVec, nil)
				// page=0 → offset=0，等同 page=1
				repo.EXPECT().Search(gomock.Any(), "test", stubVec, 0, 10).
					Return(nil, int64(0), nil)
				return repo, embed
			},
		},
		{
			name:  "size=0 使用默认值 10",
			query: "test",
			page:  1,
			size:  0,
			mock: func(ctrl *gomock.Controller) (repository.ArticleSearchRepository, *aimocks.MockEmbeddingClient) {
				repo := repomocks.NewMockArticleSearchRepository(ctrl)
				embed := aimocks.NewMockEmbeddingClient(ctrl)
				embed.EXPECT().Embed(gomock.Any(), "test").Return(stubVec, nil)
				repo.EXPECT().Search(gomock.Any(), "test", stubVec, 0, 10).
					Return(nil, int64(0), nil)
				return repo, embed
			},
		},
		{
			name:  "size 超大截断到 50",
			query: "test",
			page:  1,
			size:  999,
			mock: func(ctrl *gomock.Controller) (repository.ArticleSearchRepository, *aimocks.MockEmbeddingClient) {
				repo := repomocks.NewMockArticleSearchRepository(ctrl)
				embed := aimocks.NewMockEmbeddingClient(ctrl)
				embed.EXPECT().Embed(gomock.Any(), "test").Return(stubVec, nil)
				repo.EXPECT().Search(gomock.Any(), "test", stubVec, 0, 50).
					Return(nil, int64(0), nil)
				return repo, embed
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			repo, embed := tc.mock(ctrl)
			svc := NewArticleSearchService(repo, embed, logger.NewNopLogger())

			list, total, err := svc.Search(context.Background(), tc.query, tc.page, tc.size)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantList, list)
				assert.Equal(t, tc.wantTotal, total)
			}
		})
	}
}

func TestSearchService_IndexArticle(t *testing.T) {
	article := domain.Article{
		Id: 1, Title: "健身饮食", Abstract: "如何科学饮食",
		Author: domain.Author{Id: 1, Name: "张三"},
	}
	articleNoAbstract := domain.Article{
		Id: 2, Title: "无摘要文章", Abstract: "",
		Author: domain.Author{Id: 1, Name: "张三"},
	}

	testCases := []struct {
		name    string
		article domain.Article
		mock    func(ctrl *gomock.Controller) (repository.ArticleSearchRepository, *aimocks.MockEmbeddingClient)
		wantErr bool
	}{
		{
			name:    "成功索引",
			article: article,
			mock: func(ctrl *gomock.Controller) (repository.ArticleSearchRepository, *aimocks.MockEmbeddingClient) {
				repo := repomocks.NewMockArticleSearchRepository(ctrl)
				embed := aimocks.NewMockEmbeddingClient(ctrl)
				embed.EXPECT().Embed(gomock.Any(), "健身饮食 如何科学饮食").Return(stubVec, nil)
				repo.EXPECT().Index(gomock.Any(), article, stubVec).Return(nil)
				return repo, embed
			},
		},
		{
			name:    "abstract 为空只 embed title",
			article: articleNoAbstract,
			mock: func(ctrl *gomock.Controller) (repository.ArticleSearchRepository, *aimocks.MockEmbeddingClient) {
				repo := repomocks.NewMockArticleSearchRepository(ctrl)
				embed := aimocks.NewMockEmbeddingClient(ctrl)
				embed.EXPECT().Embed(gomock.Any(), "无摘要文章").Return(stubVec, nil)
				repo.EXPECT().Index(gomock.Any(), articleNoAbstract, stubVec).Return(nil)
				return repo, embed
			},
		},
		{
			name:    "embed 失败只记日志不报错",
			article: article,
			mock: func(ctrl *gomock.Controller) (repository.ArticleSearchRepository, *aimocks.MockEmbeddingClient) {
				repo := repomocks.NewMockArticleSearchRepository(ctrl)
				embed := aimocks.NewMockEmbeddingClient(ctrl)
				embed.EXPECT().Embed(gomock.Any(), gomock.Any()).Return(nil, errors.New("embed down"))
				return repo, embed
			},
			wantErr: false, // 不阻塞发布
		},
		{
			name:    "repo.Index 失败只记日志不报错",
			article: article,
			mock: func(ctrl *gomock.Controller) (repository.ArticleSearchRepository, *aimocks.MockEmbeddingClient) {
				repo := repomocks.NewMockArticleSearchRepository(ctrl)
				embed := aimocks.NewMockEmbeddingClient(ctrl)
				embed.EXPECT().Embed(gomock.Any(), gomock.Any()).Return(stubVec, nil)
				repo.EXPECT().Index(gomock.Any(), article, stubVec).Return(errors.New("es down"))
				return repo, embed
			},
			wantErr: false, // 不阻塞发布
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			repo, embed := tc.mock(ctrl)
			svc := NewArticleSearchService(repo, embed, logger.NewNopLogger())

			err := svc.IndexArticle(context.Background(), tc.article)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSearchService_RemoveArticle(t *testing.T) {
	testCases := []struct {
		name    string
		id      int64
		mock    func(ctrl *gomock.Controller) (repository.ArticleSearchRepository, *aimocks.MockEmbeddingClient)
		wantErr string
	}{
		{
			name: "成功删除",
			id:   1,
			mock: func(ctrl *gomock.Controller) (repository.ArticleSearchRepository, *aimocks.MockEmbeddingClient) {
				repo := repomocks.NewMockArticleSearchRepository(ctrl)
				embed := aimocks.NewMockEmbeddingClient(ctrl)
				repo.EXPECT().Remove(gomock.Any(), int64(1)).Return(nil)
				return repo, embed
			},
		},
		{
			name: "repo 失败透传错误",
			id:   1,
			mock: func(ctrl *gomock.Controller) (repository.ArticleSearchRepository, *aimocks.MockEmbeddingClient) {
				repo := repomocks.NewMockArticleSearchRepository(ctrl)
				embed := aimocks.NewMockEmbeddingClient(ctrl)
				repo.EXPECT().Remove(gomock.Any(), int64(1)).Return(errors.New("es down"))
				return repo, embed
			},
			wantErr: "es down",
		},
		{
			name: "文档不存在幂等返回 nil",
			id:   999,
			mock: func(ctrl *gomock.Controller) (repository.ArticleSearchRepository, *aimocks.MockEmbeddingClient) {
				repo := repomocks.NewMockArticleSearchRepository(ctrl)
				embed := aimocks.NewMockEmbeddingClient(ctrl)
				repo.EXPECT().Remove(gomock.Any(), int64(999)).Return(repository.ErrSearchDocNotFound)
				return repo, embed
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			repo, embed := tc.mock(ctrl)
			svc := NewArticleSearchService(repo, embed, logger.NewNopLogger())

			err := svc.RemoveArticle(context.Background(), tc.id)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
