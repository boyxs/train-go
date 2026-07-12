package ginx_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/boyxs/train-go/webook/pkg/ginx"
	"github.com/boyxs/train-go/webook/pkg/jwtx"
)

// TestRouter_SelfDeclaredPublic 驱动真实请求过 jwtx 中间件 + ginx.Router，验证「路由自声明」access：
// Public 放行、Optional 无 token 也放行、未声明的 /tag/* 仍受保护（前缀 footgun 不复现）。
func TestRouter_SelfDeclaredPublic(t *testing.T) {
	gin.SetMode(gin.TestMode)

	reg := ginx.NewRouteRegistry()
	mw := jwtx.NewMiddlewareBuilder(jwtx.MiddlewareConfig{
		AccessKey: []byte("test-key"),
		UserKey:   "user",
	}).WithResolver(reg.Lookup).Build()

	engine := gin.New()
	engine.Use(mw)
	r := ginx.NewRouter(engine, reg)

	ok := func(c *gin.Context) { c.Status(http.StatusOK) }
	r.Public.GET("/tag/:slug", ok)           // 公开：自声明
	r.Public.POST("/tag/:slug/articles", ok) // 公开：自声明
	r.Optional.GET("/comment/list", ok)      // 登录可选
	r.GET("/tag/suggest", ok)                // protected（内嵌 engine，默认走 jwt）
	r.GET("/tag/:slug/edit", ok)             // protected：未声明公开的 /tag/* 不该被放行

	cases := []struct {
		name       string
		method     string
		target     string
		wantStatus int
	}{
		{"public GET 放行", http.MethodGet, "/tag/golang", http.StatusOK},
		{"public POST 放行", http.MethodPost, "/tag/golang/articles", http.StatusOK},
		{"optional 无 token 放行", http.MethodGet, "/comment/list", http.StatusOK},
		{"protected 无 token 401", http.MethodGet, "/tag/suggest", http.StatusUnauthorized},
		{"未声明的 /tag/* 无 token 401（footgun 不复现）", http.MethodGet, "/tag/golang/edit", http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.target, nil)
			engine.ServeHTTP(w, req)
			if w.Code != tc.wantStatus {
				t.Errorf("%s %s: got %d, want %d", tc.method, tc.target, w.Code, tc.wantStatus)
			}
		})
	}
}
