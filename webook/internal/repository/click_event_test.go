package repository

import (
	"context"
	"errors"
	"testing"

	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository/cache"
	cachemocks "gitee.com/train-cloud/geektime-basic-go/internal/repository/cache/mocks"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository/dao"
	daomocks "gitee.com/train-cloud/geektime-basic-go/internal/repository/dao/mocks"
	"gitee.com/train-cloud/geektime-basic-go/pkg/logger"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestCacheAIClickEventRepository_RecordClick(t *testing.T) {
	testCases := []struct {
		name    string
		mock    func(ctrl *gomock.Controller) (dao.ClickEventDAO, cache.ClickEventCache)
		click   domain.ClickEvent
		wantErr error
	}{
		{
			name: "成功写入并清缓存",
			mock: func(ctrl *gomock.Controller) (dao.ClickEventDAO, cache.ClickEventCache) {
				d := daomocks.NewMockClickEventDAO(ctrl)
				c := cachemocks.NewMockClickEventCache(ctrl)
				d.EXPECT().Insert(gomock.Any(), gomock.Any()).Return(nil)
				c.EXPECT().DelDashboard(gomock.Any()).Return(nil)
				return d, c
			},
			click: domain.ClickEvent{UserId: 1, ArticleId: 100, ConversationId: 10, Source: "ai_chat"},
		},
		{
			name: "DAO失败返回错误",
			mock: func(ctrl *gomock.Controller) (dao.ClickEventDAO, cache.ClickEventCache) {
				d := daomocks.NewMockClickEventDAO(ctrl)
				c := cachemocks.NewMockClickEventCache(ctrl)
				d.EXPECT().Insert(gomock.Any(), gomock.Any()).Return(errors.New("db error"))
				return d, c
			},
			click:   domain.ClickEvent{UserId: 1, ArticleId: 100, ConversationId: 10, Source: "ai_chat"},
			wantErr: errors.New("db error"),
		},
		{
			name: "DAO成功但清缓存失败不影响返回",
			mock: func(ctrl *gomock.Controller) (dao.ClickEventDAO, cache.ClickEventCache) {
				d := daomocks.NewMockClickEventDAO(ctrl)
				c := cachemocks.NewMockClickEventCache(ctrl)
				d.EXPECT().Insert(gomock.Any(), gomock.Any()).Return(nil)
				c.EXPECT().DelDashboard(gomock.Any()).Return(errors.New("redis error"))
				return d, c
			},
			click: domain.ClickEvent{UserId: 1, ArticleId: 100, ConversationId: 10, Source: "ai_chat"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			d, c := tc.mock(ctrl)
			repo := NewCacheAIClickEventRepository(d, c, logger.NewNopLogger())
			err := repo.RecordClick(context.Background(), tc.click)
			assert.Equal(t, tc.wantErr, err)
		})
	}
}

func TestCacheAIClickEventRepository_Dashboard(t *testing.T) {
	cachedData := domain.ClickEventDashboard{
		TotalClicks:    100,
		UniqueUsers:    20,
		UniqueArticles: 10,
		AvgClicksPerUser: 5.0,
		DailyTrend:     []domain.DailyTrend{{Date: "2026-04-01", Clicks: 10}},
		TopArticles:    []domain.TopArticle{{Rank: 1, ArticleId: 1, Title: "Go", Clicks: 50, UniqueUsers: 10}},
	}

	testCases := []struct {
		name     string
		mock     func(ctrl *gomock.Controller) (dao.ClickEventDAO, cache.ClickEventCache)
		wantData domain.ClickEventDashboard
		wantErr  error
	}{
		{
			name: "缓存命中直接返回",
			mock: func(ctrl *gomock.Controller) (dao.ClickEventDAO, cache.ClickEventCache) {
				d := daomocks.NewMockClickEventDAO(ctrl)
				c := cachemocks.NewMockClickEventCache(ctrl)
				c.EXPECT().GetDashboard(gomock.Any()).Return(cachedData, nil)
				return d, c
			},
			wantData: cachedData,
		},
		{
			name: "缓存miss查DAO并回填",
			mock: func(ctrl *gomock.Controller) (dao.ClickEventDAO, cache.ClickEventCache) {
				d := daomocks.NewMockClickEventDAO(ctrl)
				c := cachemocks.NewMockClickEventCache(ctrl)
				c.EXPECT().GetDashboard(gomock.Any()).Return(domain.ClickEventDashboard{}, redis.Nil)
				d.EXPECT().Dashboard(gomock.Any(), gomock.Any(), "ai_chat").Return(
					dao.ClickEventStats{TotalClicks: 30, UniqueUsers: 10, UniqueArticles: 5},
					[]dao.ClickEventDailyTrend{{Date: "2026-04-09", Clicks: 8}},
					[]dao.ClickEventTopArticle{{ArticleId: 1, Title: "Test", Clicks: 15, UniqueUsers: 6}},
					nil,
				)
				c.EXPECT().SetDashboard(gomock.Any(), gomock.Any()).Return(nil)
				return d, c
			},
			wantData: domain.ClickEventDashboard{
				TotalClicks:      30,
				UniqueUsers:      10,
				UniqueArticles:   5,
				AvgClicksPerUser: 3.0,
				DailyTrend:       []domain.DailyTrend{{Date: "2026-04-09", Clicks: 8}},
				TopArticles:      []domain.TopArticle{{Rank: 1, ArticleId: 1, Title: "Test", Clicks: 15, UniqueUsers: 6}},
			},
		},
		{
			name: "缓存miss且DAO失败",
			mock: func(ctrl *gomock.Controller) (dao.ClickEventDAO, cache.ClickEventCache) {
				d := daomocks.NewMockClickEventDAO(ctrl)
				c := cachemocks.NewMockClickEventCache(ctrl)
				c.EXPECT().GetDashboard(gomock.Any()).Return(domain.ClickEventDashboard{}, redis.Nil)
				d.EXPECT().Dashboard(gomock.Any(), gomock.Any(), "ai_chat").Return(
					dao.ClickEventStats{}, nil, nil, errors.New("db error"),
				)
				return d, c
			},
			wantErr: errors.New("db error"),
		},
		{
			name: "缓存miss查DAO成功但回填缓存失败不影响返回",
			mock: func(ctrl *gomock.Controller) (dao.ClickEventDAO, cache.ClickEventCache) {
				d := daomocks.NewMockClickEventDAO(ctrl)
				c := cachemocks.NewMockClickEventCache(ctrl)
				c.EXPECT().GetDashboard(gomock.Any()).Return(domain.ClickEventDashboard{}, redis.Nil)
				d.EXPECT().Dashboard(gomock.Any(), gomock.Any(), "ai_chat").Return(
					dao.ClickEventStats{TotalClicks: 5, UniqueUsers: 2, UniqueArticles: 3},
					[]dao.ClickEventDailyTrend{{Date: "2026-04-10", Clicks: 5}},
					[]dao.ClickEventTopArticle{{ArticleId: 1, Title: "Go", Clicks: 5, UniqueUsers: 2}},
					nil,
				)
				c.EXPECT().SetDashboard(gomock.Any(), gomock.Any()).Return(errors.New("redis error"))
				return d, c
			},
			wantData: domain.ClickEventDashboard{
				TotalClicks:      5,
				UniqueUsers:      2,
				UniqueArticles:   3,
				AvgClicksPerUser: 2.5,
				DailyTrend:       []domain.DailyTrend{{Date: "2026-04-10", Clicks: 5}},
				TopArticles:      []domain.TopArticle{{Rank: 1, ArticleId: 1, Title: "Go", Clicks: 5, UniqueUsers: 2}},
			},
		},
		{
			name: "UniqueUsers为0时Avg不除零",
			mock: func(ctrl *gomock.Controller) (dao.ClickEventDAO, cache.ClickEventCache) {
				d := daomocks.NewMockClickEventDAO(ctrl)
				c := cachemocks.NewMockClickEventCache(ctrl)
				c.EXPECT().GetDashboard(gomock.Any()).Return(domain.ClickEventDashboard{}, redis.Nil)
				d.EXPECT().Dashboard(gomock.Any(), gomock.Any(), "ai_chat").Return(
					dao.ClickEventStats{TotalClicks: 0, UniqueUsers: 0, UniqueArticles: 0},
					[]dao.ClickEventDailyTrend{},
					[]dao.ClickEventTopArticle{},
					nil,
				)
				c.EXPECT().SetDashboard(gomock.Any(), gomock.Any()).Return(nil)
				return d, c
			},
			wantData: domain.ClickEventDashboard{
				AvgClicksPerUser: 0,
				DailyTrend:       []domain.DailyTrend{},
				TopArticles:      []domain.TopArticle{},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			d, c := tc.mock(ctrl)
			repo := NewCacheAIClickEventRepository(d, c, logger.NewNopLogger())
			data, err := repo.Dashboard(context.Background())
			assert.Equal(t, tc.wantErr, err)
			if err == nil {
				assert.Equal(t, tc.wantData, data)
			}
		})
	}
}
