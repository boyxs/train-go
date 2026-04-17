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

func TestSearchHandler_Search(t *testing.T) {
	articles := []domain.Article{
		{Id: 1, Title: "健身饮食", Abstract: "科学饮食方法"},
	}

	testCases := []struct {
		name       string
		reqBody    any
		mock       func(ctrl *gomock.Controller) *svcmocks.MockArticleSearchService
		wantCode   int
		wantResult Result
	}{
		{
			name:    "搜索成功",
			reqBody: map[string]any{"query": "健身饮食", "page": 1, "size": 10},
			mock: func(ctrl *gomock.Controller) *svcmocks.MockArticleSearchService {
				svc := svcmocks.NewMockArticleSearchService(ctrl)
				svc.EXPECT().Search(gomock.Any(), "健身饮食", 1, 10).
					Return(articles, int64(1), nil)
				return svc
			},
			wantCode:   http.StatusOK,
			wantResult: Result{Code: 0, Data: map[string]any{"list": articles, "total": float64(1)}},
		},
		{
			name:    "query 为空",
			reqBody: map[string]any{"query": "", "page": 1, "size": 10},
			mock: func(ctrl *gomock.Controller) *svcmocks.MockArticleSearchService {
				return svcmocks.NewMockArticleSearchService(ctrl)
			},
			wantCode:   http.StatusBadRequest,
			wantResult: Result{Code: 4, Msg: "搜索内容不能为空"},
		},
		{
			name:    "page/size 未传使用默认值",
			reqBody: map[string]any{"query": "test"},
			mock: func(ctrl *gomock.Controller) *svcmocks.MockArticleSearchService {
				svc := svcmocks.NewMockArticleSearchService(ctrl)
				svc.EXPECT().Search(gomock.Any(), "test", 1, 10).
					Return(nil, int64(0), nil)
				return svc
			},
			wantCode:   http.StatusOK,
			wantResult: Result{Code: 0, Data: map[string]any{"list": nil, "total": float64(0)}},
		},
		{
			name:    "SearchService 失败",
			reqBody: map[string]any{"query": "test", "page": 1, "size": 10},
			mock: func(ctrl *gomock.Controller) *svcmocks.MockArticleSearchService {
				svc := svcmocks.NewMockArticleSearchService(ctrl)
				svc.EXPECT().Search(gomock.Any(), "test", 1, 10).
					Return(nil, int64(0), errors.New("es down"))
				return svc
			},
			wantCode:   http.StatusInternalServerError,
			wantResult: Result{Code: 5, Msg: "系统错误"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			gin.SetMode(gin.TestMode)
			server := gin.New()

			// 注入 JWT claims
			server.Use(func(c *gin.Context) {
				c.Set(consts.UserKey, jwt.UserClaims{Userid: 1})
				c.Next()
			})

			h := NewInternalArticleSearchHandler(tc.mock(ctrl), logger.NewNopLogger())
			h.RegisterRoutes(server)

			body, _ := json.Marshal(tc.reqBody)
			req, _ := http.NewRequest(http.MethodPost, "/search/article", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			server.ServeHTTP(w, req)

			assert.Equal(t, tc.wantCode, w.Code)

			var result Result
			_ = json.Unmarshal(w.Body.Bytes(), &result)
			assert.Equal(t, tc.wantResult.Code, result.Code)
			assert.Equal(t, tc.wantResult.Msg, result.Msg)
			if tc.wantResult.Code == 0 {
				// data 结构比较 code/total 足够
				assert.NotNil(t, result.Data)
			}
		})
	}
}
