package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/webook/internal/consts"
	"github.com/webook/internal/domain"
	svcmocks "github.com/webook/internal/service/mocks"
	"github.com/webook/internal/web/jwt"
	"github.com/webook/pkg/logger"
)

func setupClickEventRouter(handler ClickEventHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	server := gin.New()
	server.Use(func(c *gin.Context) {
		c.Set(consts.UserKey, jwt.UserClaims{Userid: 1})
		c.Next()
	})
	handler.RegisterRoutes(server)
	return server
}

func TestAIClickEventHandler_Click(t *testing.T) {
	testCases := []struct {
		name     string
		body     string
		mock     func(ctrl *gomock.Controller) *svcmocks.MockClickEventService
		wantCode int
		wantBody Result
	}{
		{
			name: "成功",
			body: `{"article_id":100,"conversation_id":10}`,
			mock: func(ctrl *gomock.Controller) *svcmocks.MockClickEventService {
				svc := svcmocks.NewMockClickEventService(ctrl)
				svc.EXPECT().RecordClick(gomock.Any(), int64(1), int64(100), int64(10), "ai_chat").Return(nil)
				return svc
			},
			wantCode: http.StatusOK,
			wantBody: Result{Code: 0, Msg: "ok"},
		},
		{
			name: "参数缺失",
			body: `{}`,
			mock: func(ctrl *gomock.Controller) *svcmocks.MockClickEventService {
				return svcmocks.NewMockClickEventService(ctrl)
			},
			wantCode: http.StatusBadRequest,
			wantBody: Result{Code: 4, Msg: "参数无效"},
		},
		{
			name: "article_id为0",
			body: `{"article_id":0,"conversation_id":10}`,
			mock: func(ctrl *gomock.Controller) *svcmocks.MockClickEventService {
				return svcmocks.NewMockClickEventService(ctrl)
			},
			wantCode: http.StatusBadRequest,
			wantBody: Result{Code: 4, Msg: "参数无效"},
		},
		{
			name: "conversation_id为0",
			body: `{"article_id":100,"conversation_id":0}`,
			mock: func(ctrl *gomock.Controller) *svcmocks.MockClickEventService {
				return svcmocks.NewMockClickEventService(ctrl)
			},
			wantCode: http.StatusBadRequest,
			wantBody: Result{Code: 4, Msg: "参数无效"},
		},
		{
			name: "JSON绑定失败",
			body: `invalid json`,
			mock: func(ctrl *gomock.Controller) *svcmocks.MockClickEventService {
				return svcmocks.NewMockClickEventService(ctrl)
			},
			wantCode: http.StatusBadRequest,
			wantBody: Result{Code: 4, Msg: "参数错误"},
		},
		{
			name: "service返回错误",
			body: `{"article_id":100,"conversation_id":10}`,
			mock: func(ctrl *gomock.Controller) *svcmocks.MockClickEventService {
				svc := svcmocks.NewMockClickEventService(ctrl)
				svc.EXPECT().RecordClick(gomock.Any(), int64(1), int64(100), int64(10), "ai_chat").
					Return(errors.New("db error"))
				return svc
			},
			wantCode: http.StatusInternalServerError,
			wantBody: Result{Code: 5, Msg: "系统错误"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			svc := tc.mock(ctrl)
			handler := NewAIClickEventHandler(svc, logger.NewNopLogger())
			server := setupClickEventRouter(handler)

			req := httptest.NewRequest(http.MethodPost, "/ai/click", bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			server.ServeHTTP(recorder, req)

			assert.Equal(t, tc.wantCode, recorder.Code)
			var res Result
			err := json.NewDecoder(recorder.Body).Decode(&res)
			assert.NoError(t, err)
			assert.Equal(t, tc.wantBody.Code, res.Code)
			assert.Equal(t, tc.wantBody.Msg, res.Msg)
		})
	}
}

func TestAIClickEventHandler_Dashboard(t *testing.T) {
	testCases := []struct {
		name     string
		mock     func(ctrl *gomock.Controller) *svcmocks.MockClickEventService
		wantCode int
		wantBody Result
	}{
		{
			name: "成功",
			mock: func(ctrl *gomock.Controller) *svcmocks.MockClickEventService {
				svc := svcmocks.NewMockClickEventService(ctrl)
				svc.EXPECT().Dashboard(gomock.Any()).Return(domain.ClickEventDashboard{
					TotalClicks:      100,
					UniqueUsers:      20,
					UniqueArticles:   10,
					AvgClicksPerUser: 5.0,
					DailyTrend:       []domain.DailyTrend{{Date: "2026-04-09", Clicks: 8}},
					TopArticles:      []domain.TopArticle{{Rank: 1, ArticleId: 1, Title: "Go", Clicks: 50, UniqueUsers: 10}},
				}, nil)
				return svc
			},
			wantCode: http.StatusOK,
			wantBody: Result{Code: 0, Msg: "ok"},
		},
		{
			name: "service返回错误",
			mock: func(ctrl *gomock.Controller) *svcmocks.MockClickEventService {
				svc := svcmocks.NewMockClickEventService(ctrl)
				svc.EXPECT().Dashboard(gomock.Any()).Return(domain.ClickEventDashboard{}, errors.New("db error"))
				return svc
			},
			wantCode: http.StatusInternalServerError,
			wantBody: Result{Code: 5, Msg: "系统错误"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			svc := tc.mock(ctrl)
			handler := NewAIClickEventHandler(svc, logger.NewNopLogger())
			server := setupClickEventRouter(handler)

			req := httptest.NewRequest(http.MethodPost, "/ai/dashboard", nil)
			recorder := httptest.NewRecorder()
			server.ServeHTTP(recorder, req)

			assert.Equal(t, tc.wantCode, recorder.Code)
			var res Result
			err := json.NewDecoder(recorder.Body).Decode(&res)
			assert.NoError(t, err)
			assert.Equal(t, tc.wantBody.Code, res.Code)
			assert.Equal(t, tc.wantBody.Msg, res.Msg)
		})
	}
}
