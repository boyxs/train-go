package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/boyxs/train-go/webook/internal/domain"
	repomocks "github.com/boyxs/train-go/webook/internal/repository/mocks"
)

func TestAIClickEventService_RecordClick(t *testing.T) {
	testCases := []struct {
		name    string
		mock    func(ctrl *gomock.Controller) *repomocks.MockClickEventRepository
		wantErr error
	}{
		{
			name: "成功",
			mock: func(ctrl *gomock.Controller) *repomocks.MockClickEventRepository {
				repo := repomocks.NewMockClickEventRepository(ctrl)
				repo.EXPECT().RecordClick(gomock.Any(), domain.ClickEvent{
					UserId: 1, ArticleId: 100, ConversationId: 10, Source: "ai_chat",
				}).Return(nil)
				return repo
			},
		},
		{
			name: "失败透传",
			mock: func(ctrl *gomock.Controller) *repomocks.MockClickEventRepository {
				repo := repomocks.NewMockClickEventRepository(ctrl)
				repo.EXPECT().RecordClick(gomock.Any(), gomock.Any()).Return(errors.New("repo error"))
				return repo
			},
			wantErr: errors.New("repo error"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			repo := tc.mock(ctrl)
			svc := NewAIClickEventService(repo)
			err := svc.RecordClick(context.Background(), 1, 100, 10, "ai_chat")
			assert.Equal(t, tc.wantErr, err)
		})
	}
}

func TestAIClickEventService_Dashboard(t *testing.T) {
	testCases := []struct {
		name     string
		mock     func(ctrl *gomock.Controller) *repomocks.MockClickEventRepository
		wantData domain.ClickEventDashboard
		wantErr  error
	}{
		{
			name: "成功返回数据",
			mock: func(ctrl *gomock.Controller) *repomocks.MockClickEventRepository {
				repo := repomocks.NewMockClickEventRepository(ctrl)
				repo.EXPECT().Dashboard(gomock.Any()).Return(domain.ClickEventDashboard{
					TotalClicks: 100, UniqueUsers: 20, UniqueArticles: 10,
					AvgClicksPerUser: 5.0,
				}, nil)
				return repo
			},
			wantData: domain.ClickEventDashboard{
				TotalClicks: 100, UniqueUsers: 20, UniqueArticles: 10,
				AvgClicksPerUser: 5.0,
			},
		},
		{
			name: "失败透传",
			mock: func(ctrl *gomock.Controller) *repomocks.MockClickEventRepository {
				repo := repomocks.NewMockClickEventRepository(ctrl)
				repo.EXPECT().Dashboard(gomock.Any()).Return(domain.ClickEventDashboard{}, errors.New("fail"))
				return repo
			},
			wantErr: errors.New("fail"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			repo := tc.mock(ctrl)
			svc := NewAIClickEventService(repo)
			data, err := svc.Dashboard(context.Background())
			assert.Equal(t, tc.wantErr, err)
			if err == nil {
				assert.Equal(t, tc.wantData, data)
			}
		})
	}
}
