package ginx

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/webook/pkg/errs"
)

// ── 用例 4.x 补充：WrapReq / WrapClaims / WrapReqClaims 各自独有路径 ────────

type fakeReq struct {
	Name string `json:"name"`
}

type fakeClaims struct {
	Userid int64
}

const userKey = "user_claims"

// WrapReq: ShouldBindJSON 失败 → HTTP 400 + Code:4 + "参数错误"
func TestWrapReq_InvalidJSON_Returns400(t *testing.T) {
	srv := gin.New()
	srv.POST("/x", WrapReq(func(ctx *gin.Context, req fakeReq) (Result, error) {
		t.Fatal("不应该到达 handler，应该被 ShouldBindJSON 拦截")
		return Result{}, nil
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	got := decodeResult(t, rec)
	assert.Equal(t, http.StatusBadRequest, got.Code)
	assert.Equal(t, "参数错误", got.Msg)
}

// WrapReq: 反序列化通过后 handler 抛 *errs.Error → 走 respond 正常路径
func TestWrapReq_HandlerBizError_PassesToRespond(t *testing.T) {
	srv := gin.New()
	srv.POST("/x", WrapReq(func(ctx *gin.Context, req fakeReq) (Result, error) {
		return Result{}, errs.New(409, "邮箱已被注册")
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", bytes.NewBufferString(`{"name":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusConflict, rec.Code)
	got := decodeResult(t, rec)
	assert.Equal(t, 409, got.Code)
	assert.Equal(t, "邮箱已被注册", got.Msg)
}

// WrapClaims: UserClaims 缺失 → HTTP 401 (AbortWithStatus)
func TestWrapClaims_MissingClaims_Returns401(t *testing.T) {
	srv := gin.New()
	srv.GET("/x", WrapClaims[fakeClaims](userKey, func(ctx *gin.Context, uc fakeClaims) (Result, error) {
		t.Fatal("不应该到达 handler")
		return Result{}, nil
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// WrapClaims: UserClaims 类型不匹配 → 401
func TestWrapClaims_WrongClaimsType_Returns401(t *testing.T) {
	srv := gin.New()
	srv.Use(func(ctx *gin.Context) {
		ctx.Set(userKey, "wrong-type-string") // 注入错误类型
	})
	srv.GET("/x", WrapClaims[fakeClaims](userKey, func(ctx *gin.Context, uc fakeClaims) (Result, error) {
		t.Fatal("不应该到达 handler")
		return Result{}, nil
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// WrapClaims: UserClaims 注入正确 → handler 正常执行
func TestWrapClaims_HappyPath_HandlerCalled(t *testing.T) {
	srv := gin.New()
	srv.Use(func(ctx *gin.Context) {
		ctx.Set(userKey, fakeClaims{Userid: 100})
	})
	srv.GET("/x", WrapClaims[fakeClaims](userKey, func(ctx *gin.Context, uc fakeClaims) (Result, error) {
		assert.Equal(t, int64(100), uc.Userid)
		return Result{Data: "ok"}, nil
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

// WrapReqClaims: 反序列化 + UserClaims 双重前置校验，任一失败短路
func TestWrapReqClaims_InvalidJSON_Returns400_BeforeClaims(t *testing.T) {
	srv := gin.New()
	// 故意不注入 claims，验证 ShouldBindJSON 失败先短路（不走到 401）
	srv.POST("/x", WrapReqClaims[fakeReq, fakeClaims](userKey, func(ctx *gin.Context, req fakeReq, uc fakeClaims) (Result, error) {
		t.Fatal("不应到达")
		return Result{}, nil
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	got := decodeResult(t, rec)
	assert.Equal(t, http.StatusBadRequest, got.Code)
}

func TestWrapReqClaims_MissingClaims_Returns401(t *testing.T) {
	srv := gin.New()
	srv.POST("/x", WrapReqClaims[fakeReq, fakeClaims](userKey, func(ctx *gin.Context, req fakeReq, uc fakeClaims) (Result, error) {
		t.Fatal("不应到达")
		return Result{}, nil
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", bytes.NewBufferString(`{"name":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}
