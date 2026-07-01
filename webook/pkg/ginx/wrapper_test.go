package ginx

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/webook/pkg/errs"
)

func init() {
	gin.SetMode(gin.TestMode)
}

type fakeReq struct {
	Name string `json:"name"`
}

type fakeClaims struct {
	Uid int64
}

func runWrap(handler HandlerFunc) *httptest.ResponseRecorder {
	srv := gin.New()
	srv.GET("/x", Wrap(handler))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	srv.ServeHTTP(rec, req)
	return rec
}

func decodeResult(t *testing.T, rec *httptest.ResponseRecorder) Result {
	t.Helper()
	var got Result
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
	return got
}

func testCtx() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	return c
}

// ── 成功路径：body.code ≡ HTTP status = 200 ─────────────────────────────────

func TestWrap_Success_Code200MsgOK(t *testing.T) {
	rec := runWrap(func(ctx *gin.Context) (Result, error) {
		return Result{Data: map[string]string{"hi": "ok"}}, nil
	})
	assert.Equal(t, http.StatusOK, rec.Code)
	got := decodeResult(t, rec)
	assert.Equal(t, 200, got.Code, "成功 body.code 应 = HTTP 200")
	assert.Equal(t, "OK", got.Msg, "msg 空则默认 OK")
}

func TestWrap_Success_KeepsCustomMsg(t *testing.T) {
	rec := runWrap(func(ctx *gin.Context) (Result, error) {
		return Result{Msg: "已发布"}, nil
	})
	got := decodeResult(t, rec)
	assert.Equal(t, 200, got.Code)
	assert.Equal(t, "已发布", got.Msg)
}

// respond 按 Result.Code 作 HTTP status：命名构造器带 code 的 Result（err=nil）也走对应状态码。
func TestWrap_ResultWithCode_UsesItAsHTTPStatus(t *testing.T) {
	rec := runWrap(func(ctx *gin.Context) (Result, error) {
		return NotFound("文章不存在"), nil
	})
	assert.Equal(t, http.StatusNotFound, rec.Code)
	got := decodeResult(t, rec)
	assert.Equal(t, 404, got.Code)
	assert.Equal(t, "文章不存在", got.Msg)
}

// ── 错误路径：*errs.Error → HTTP status，其他 → 500 ─────────────────────────

func TestWrap_BizError_CodeEqualsHTTPStatus(t *testing.T) {
	rec := runWrap(func(ctx *gin.Context) (Result, error) {
		return Result{}, errs.New(404, "用户不存在")
	})
	assert.Equal(t, http.StatusNotFound, rec.Code)
	got := decodeResult(t, rec)
	assert.Equal(t, 404, got.Code)
	assert.Equal(t, "用户不存在", got.Msg)
}

func TestWrap_PlainError_Returns500(t *testing.T) {
	rec := runWrap(func(ctx *gin.Context) (Result, error) {
		return Result{}, errors.New("db down")
	})
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	got := decodeResult(t, rec)
	assert.Equal(t, 500, got.Code)
	assert.Equal(t, "系统错误", got.Msg)
}

func TestWrap_WrappedBizError_StillExtracted(t *testing.T) {
	sentinel := errs.New(404, "文章不存在")
	wrapped := fmt.Errorf("repo: %w", sentinel.WithCause(errors.New("record not found")))
	rec := runWrap(func(ctx *gin.Context) (Result, error) {
		return Result{}, wrapped
	})
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "文章不存在", decodeResult(t, rec).Msg)
}

func TestWrap_BizError_ReasonAndMetadata(t *testing.T) {
	rec := runWrap(func(ctx *gin.Context) (Result, error) {
		return Result{}, errs.New(429, "润色次数已达上限").
			WithReason("POLISH_RATE_LIMITED").WithMetadata("retry_after", "60")
	})
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	got := decodeResult(t, rec)
	assert.Equal(t, "POLISH_RATE_LIMITED", got.Reason)
	assert.Equal(t, "60", got.Metadata["retry_after"])
}

func TestWrap_BizError_NoReason_OmitsField(t *testing.T) {
	rec := runWrap(func(ctx *gin.Context) (Result, error) {
		return Result{}, errs.New(409, "邮箱已被注册")
	})
	assert.NotContains(t, rec.Body.String(), "reason")
}

// ── WrapReq：请求体绑定 ─────────────────────────────────────────────────────

func TestWrapReq_InvalidJSON_Returns400WithReason(t *testing.T) {
	srv := gin.New()
	srv.POST("/x", WrapReq(func(ctx *gin.Context, req fakeReq) (Result, error) {
		t.Fatal("不应到达 handler，应被 ShouldBindJSON 拦截")
		return Result{}, nil
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	got := decodeResult(t, rec)
	assert.Equal(t, 400, got.Code)
	assert.Equal(t, "参数错误", got.Msg)
	assert.Equal(t, "BAD_REQUEST", got.Reason)
}

func TestWrapReq_Success_Code200(t *testing.T) {
	srv := gin.New()
	srv.POST("/x", WrapReq(func(ctx *gin.Context, req fakeReq) (Result, error) {
		return Result{Data: req.Name}, nil
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", bytes.NewBufferString(`{"name":"tom"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	got := decodeResult(t, rec)
	assert.Equal(t, 200, got.Code)
	assert.Equal(t, "tom", got.Data)
}

func TestWrapReq_HandlerBizError_PassesThrough(t *testing.T) {
	srv := gin.New()
	srv.POST("/x", WrapReq(func(ctx *gin.Context, req fakeReq) (Result, error) {
		return Result{}, errs.New(409, "邮箱已被注册")
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", bytes.NewBufferString(`{"name":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Equal(t, "邮箱已被注册", decodeResult(t, rec).Msg)
}

// ── 登录态访问器 MustClaims / Claims ────────────────────────────────────────

func TestClaims_Present_ReturnsTrue(t *testing.T) {
	c := testCtx()
	c.Set(UserKey, fakeClaims{Uid: 100})
	uc, ok := Claims[fakeClaims](c)
	assert.True(t, ok)
	assert.Equal(t, int64(100), uc.Uid)
}

func TestClaims_Missing_ReturnsFalse(t *testing.T) {
	uc, ok := Claims[fakeClaims](testCtx())
	assert.False(t, ok)
	assert.Equal(t, fakeClaims{}, uc)
}

func TestClaims_WrongType_ReturnsFalse(t *testing.T) {
	c := testCtx()
	c.Set(UserKey, "not-claims")
	_, ok := Claims[fakeClaims](c)
	assert.False(t, ok)
}

func TestMustClaims_Present_Returns(t *testing.T) {
	c := testCtx()
	c.Set(UserKey, fakeClaims{Uid: 100})
	assert.Equal(t, int64(100), MustClaims[fakeClaims](c).Uid)
}

func TestMustClaims_Missing_Panics(t *testing.T) {
	assert.Panics(t, func() {
		MustClaims[fakeClaims](testCtx())
	}, "缺登录态应 panic（fail-loud）")
}
