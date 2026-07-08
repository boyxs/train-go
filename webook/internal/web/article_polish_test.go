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

	"github.com/boyxs/train-go/webook/internal/consts"
	"github.com/boyxs/train-go/webook/internal/domain"
	"github.com/boyxs/train-go/webook/internal/errs"
	svcmocks "github.com/boyxs/train-go/webook/internal/service/mocks"
	"github.com/boyxs/train-go/webook/pkg/ginx"
	jwt "github.com/boyxs/train-go/webook/pkg/jwtx"
	"github.com/boyxs/train-go/webook/pkg/logger"
	limitmocks "github.com/boyxs/train-go/webook/pkg/ratelimit/mocks"
)

// 单测里 InitWebServer 不跑，手动对齐 ginx.UserKey 与生产（= consts.UserKey），
// 供各 handler 的 ginx.MustClaims/Claims 从 ctx 取到测试注入的登录态。
func init() {
	ginx.UserKey = consts.UserKey
}

func setupPolishRouter(handler ArticlePolishHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	server := gin.New()
	server.Use(func(c *gin.Context) {
		c.Set(consts.UserKey, jwt.UserClaims{Userid: 1})
		c.Next()
	})
	handler.RegisterRoutes(server)
	return server
}

func TestAIArticlePolishHandler_Polish(t *testing.T) {
	testCases := []struct {
		name     string
		body     string
		mock     func(ctrl *gomock.Controller) (*svcmocks.MockArticlePolishService, *limitmocks.MockLimiter)
		wantCode int
		wantBody Result
	}{
		{
			name: "成功",
			body: `{"title":"Go 并发","content":"goroutine 很好用"}`,
			mock: func(ctrl *gomock.Controller) (*svcmocks.MockArticlePolishService, *limitmocks.MockLimiter) {
				svc := svcmocks.NewMockArticlePolishService(ctrl)
				lim := limitmocks.NewMockLimiter(ctrl)
				lim.EXPECT().Limit(gomock.Any(), gomock.Any()).Return(false, nil)
				svc.EXPECT().Polish(gomock.Any(), "Go 并发", "goroutine 很好用").Return(domain.PolishResult{
					Title:    "Go 并发编程入门",
					Abstract: "介绍 goroutine",
					Content:  "Goroutine 是核心并发原语",
				}, nil)
				return svc, lim
			},
			wantCode: http.StatusOK,
			wantBody: Result{Code: 200, Msg: "ok"},
		},
		{
			name: "JSON绑定失败",
			body: "not json",
			mock: func(ctrl *gomock.Controller) (*svcmocks.MockArticlePolishService, *limitmocks.MockLimiter) {
				return svcmocks.NewMockArticlePolishService(ctrl), limitmocks.NewMockLimiter(ctrl)
			},
			wantCode: http.StatusBadRequest,
			wantBody: Result{Code: http.StatusBadRequest, Msg: "参数错误"},
		},
		{
			name: "title为空",
			body: `{"title":"","content":"有内容"}`,
			mock: func(ctrl *gomock.Controller) (*svcmocks.MockArticlePolishService, *limitmocks.MockLimiter) {
				svc := svcmocks.NewMockArticlePolishService(ctrl)
				lim := limitmocks.NewMockLimiter(ctrl)
				lim.EXPECT().Limit(gomock.Any(), gomock.Any()).Return(false, nil)
				svc.EXPECT().Polish(gomock.Any(), "", "有内容").Return(domain.PolishResult{}, errs.ErrPolishEmptyTitle)
				return svc, lim
			},
			wantCode: http.StatusBadRequest,
			wantBody: Result{Code: 400, Msg: "标题不能为空"},
		},
		{
			name: "content为空",
			body: `{"title":"标题","content":""}`,
			mock: func(ctrl *gomock.Controller) (*svcmocks.MockArticlePolishService, *limitmocks.MockLimiter) {
				svc := svcmocks.NewMockArticlePolishService(ctrl)
				lim := limitmocks.NewMockLimiter(ctrl)
				lim.EXPECT().Limit(gomock.Any(), gomock.Any()).Return(false, nil)
				svc.EXPECT().Polish(gomock.Any(), "标题", "").Return(domain.PolishResult{}, errs.ErrPolishEmptyContent)
				return svc, lim
			},
			wantCode: http.StatusBadRequest,
			wantBody: Result{Code: 400, Msg: "内容不能为空"},
		},
		{
			name: "content超长",
			body: `{"title":"标题","content":"超长内容"}`,
			mock: func(ctrl *gomock.Controller) (*svcmocks.MockArticlePolishService, *limitmocks.MockLimiter) {
				svc := svcmocks.NewMockArticlePolishService(ctrl)
				lim := limitmocks.NewMockLimiter(ctrl)
				lim.EXPECT().Limit(gomock.Any(), gomock.Any()).Return(false, nil)
				svc.EXPECT().Polish(gomock.Any(), "标题", "超长内容").Return(domain.PolishResult{}, errs.ErrPolishContentTooLong)
				return svc, lim
			},
			wantCode: http.StatusBadRequest,
			wantBody: Result{Code: 400, Msg: "内容过长，请缩减至 10000 字符以内"},
		},
		{
			name: "频率限制",
			body: `{"title":"标题","content":"内容"}`,
			mock: func(ctrl *gomock.Controller) (*svcmocks.MockArticlePolishService, *limitmocks.MockLimiter) {
				svc := svcmocks.NewMockArticlePolishService(ctrl)
				lim := limitmocks.NewMockLimiter(ctrl)
				lim.EXPECT().Limit(gomock.Any(), gomock.Any()).Return(true, nil)
				return svc, lim
			},
			wantCode: http.StatusTooManyRequests,
			wantBody: Result{Code: 429, Msg: "润色次数已达上限，请稍后再试"},
		},
		{
			name: "service返回系统错误",
			body: `{"title":"标题","content":"内容"}`,
			mock: func(ctrl *gomock.Controller) (*svcmocks.MockArticlePolishService, *limitmocks.MockLimiter) {
				svc := svcmocks.NewMockArticlePolishService(ctrl)
				lim := limitmocks.NewMockLimiter(ctrl)
				lim.EXPECT().Limit(gomock.Any(), gomock.Any()).Return(false, nil)
				svc.EXPECT().Polish(gomock.Any(), "标题", "内容").Return(domain.PolishResult{}, errors.New("LLM timeout"))
				return svc, lim
			},
			wantCode: http.StatusInternalServerError,
			wantBody: Result{Code: 500, Msg: "系统错误"},
		},
		{
			name: "限流检查Redis报错但不阻塞",
			body: `{"title":"标题","content":"内容"}`,
			mock: func(ctrl *gomock.Controller) (*svcmocks.MockArticlePolishService, *limitmocks.MockLimiter) {
				svc := svcmocks.NewMockArticlePolishService(ctrl)
				lim := limitmocks.NewMockLimiter(ctrl)
				lim.EXPECT().Limit(gomock.Any(), gomock.Any()).Return(false, errors.New("redis error"))
				svc.EXPECT().Polish(gomock.Any(), "标题", "内容").Return(domain.PolishResult{
					Title: "标题", Abstract: "摘要", Content: "内容",
				}, nil)
				return svc, lim
			},
			wantCode: http.StatusOK,
			wantBody: Result{Code: 200, Msg: "ok"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			svc, lim := tc.mock(ctrl)
			handler := &AIArticlePolishHandler{svc: svc, limiter: lim, l: logger.NewNopLogger()}
			server := setupPolishRouter(handler)

			req := httptest.NewRequest(http.MethodPost, "/article/polish", bytes.NewBufferString(tc.body))
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
