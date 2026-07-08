package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/boyxs/train-go/webook/internal/domain"
	"github.com/boyxs/train-go/webook/internal/migratorsdk"
	cachemocks "github.com/boyxs/train-go/webook/internal/repository/cache/mocks"
	"github.com/boyxs/train-go/webook/internal/repository/dao"
	daomocks "github.com/boyxs/train-go/webook/internal/repository/dao/mocks"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

func TestCacheArticleReaderRepository_Page(t *testing.T) {
	mockNow := time.Now().UnixMilli()
	testCases := []struct {
		name     string
		mock     func(ctrl *gomock.Controller) (dao.ArticleReaderDAO, *cachemocks.MockArticleCache)
		offset   int
		limit    int
		wantArts []domain.Article
		wantCnt  int64
		wantErr  error
	}{
		{
			name: "首页命中缓存",
			mock: func(ctrl *gomock.Controller) (dao.ArticleReaderDAO, *cachemocks.MockArticleCache) {
				d := daomocks.NewMockArticleReaderDAO(ctrl)
				c := cachemocks.NewMockArticleCache(ctrl)
				c.EXPECT().GetFirstPage(gomock.Any()).Return([]domain.Article{
					{Id: 3, Title: "文章3", UpdatedAt: mockNow},
					{Id: 2, Title: "文章2", UpdatedAt: mockNow},
				}, int64(5), nil)
				// 命中缓存时 total 也从缓存取，不查 DB
				return d, c
			},
			offset: 0,
			limit:  10,
			wantArts: []domain.Article{
				{Id: 3, Title: "文章3", UpdatedAt: mockNow},
				{Id: 2, Title: "文章2", UpdatedAt: mockNow},
			},
			wantCnt: 5,
		},
		{
			name: "首页缓存miss回源DB并回填",
			mock: func(ctrl *gomock.Controller) (dao.ArticleReaderDAO, *cachemocks.MockArticleCache) {
				d := daomocks.NewMockArticleReaderDAO(ctrl)
				c := cachemocks.NewMockArticleCache(ctrl)
				c.EXPECT().GetFirstPage(gomock.Any()).Return(nil, int64(0), redis.Nil)
				d.EXPECT().Page(gomock.Any(), 0, 10).Return([]dao.PublishedArticle{
					{Id: 3, Title: "文章3", AuthorId: 1, UpdatedAt: mockNow},
					{Id: 2, Title: "文章2", AuthorId: 1, UpdatedAt: mockNow},
				}, nil)
				d.EXPECT().Count(gomock.Any()).Return(int64(5), nil)
				// 回填缓存
				c.EXPECT().SetFirstPage(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				return d, c
			},
			offset: 0,
			limit:  10,
			wantArts: []domain.Article{
				{Id: 3, Title: "文章3", Author: domain.Author{Id: 1}, UpdatedAt: mockNow},
				{Id: 2, Title: "文章2", Author: domain.Author{Id: 1}, UpdatedAt: mockNow},
			},
			wantCnt: 5,
		},
		{
			name: "非首页直接查DB",
			mock: func(ctrl *gomock.Controller) (dao.ArticleReaderDAO, *cachemocks.MockArticleCache) {
				d := daomocks.NewMockArticleReaderDAO(ctrl)
				c := cachemocks.NewMockArticleCache(ctrl)
				// 非首页不调 GetFirstPage
				d.EXPECT().Page(gomock.Any(), 10, 10).Return([]dao.PublishedArticle{
					{Id: 1, Title: "文章1", AuthorId: 1, UpdatedAt: mockNow},
				}, nil)
				d.EXPECT().Count(gomock.Any()).Return(int64(15), nil)
				return d, c
			},
			offset: 10,
			limit:  10,
			wantArts: []domain.Article{
				{Id: 1, Title: "文章1", Author: domain.Author{Id: 1}, UpdatedAt: mockNow},
			},
			wantCnt: 15,
		},
		{
			name: "缓存回填失败不影响返回",
			mock: func(ctrl *gomock.Controller) (dao.ArticleReaderDAO, *cachemocks.MockArticleCache) {
				d := daomocks.NewMockArticleReaderDAO(ctrl)
				c := cachemocks.NewMockArticleCache(ctrl)
				c.EXPECT().GetFirstPage(gomock.Any()).Return(nil, int64(0), redis.Nil)
				d.EXPECT().Page(gomock.Any(), 0, 10).Return([]dao.PublishedArticle{
					{Id: 3, Title: "文章3", AuthorId: 1, UpdatedAt: mockNow},
				}, nil)
				d.EXPECT().Count(gomock.Any()).Return(int64(1), nil)
				c.EXPECT().SetFirstPage(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(errors.New("redis connection refused"))
				return d, c
			},
			offset: 0,
			limit:  10,
			wantArts: []domain.Article{
				{Id: 3, Title: "文章3", Author: domain.Author{Id: 1}, UpdatedAt: mockNow},
			},
			wantCnt: 1,
		},
		{
			name: "DB查询失败",
			mock: func(ctrl *gomock.Controller) (dao.ArticleReaderDAO, *cachemocks.MockArticleCache) {
				d := daomocks.NewMockArticleReaderDAO(ctrl)
				c := cachemocks.NewMockArticleCache(ctrl)
				c.EXPECT().GetFirstPage(gomock.Any()).Return(nil, int64(0), redis.Nil)
				d.EXPECT().Page(gomock.Any(), 0, 10).
					Return(nil, errors.New("db connection error"))
				d.EXPECT().Count(gomock.Any()).Return(int64(0), nil).AnyTimes()
				return d, c
			},
			offset:  0,
			limit:   10,
			wantErr: errors.New("db connection error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			d, c := tc.mock(ctrl)
			// 原 Page 测试不涉及 SDK 路由（Page 不走 SDK），newDAO/sw/dw 用 NoOp 占位
			repo := NewCacheArticleReaderRepository(
				d, dao.ArticleReaderNewDAO(d), c,
				migratorsdk.NewNoOpSwitchReader(),
				migratorsdk.NewNoOpDualWriter(),
				"published_article_v1",
				logger.NewNopLogger(),
			)

			arts, cnt, err := repo.Page(context.Background(), tc.offset, tc.limit)
			if tc.wantErr != nil {
				assert.EqualError(t, err, tc.wantErr.Error())
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.wantCnt, cnt)
			assert.Equal(t, tc.wantArts, arts)
		})
	}
}
