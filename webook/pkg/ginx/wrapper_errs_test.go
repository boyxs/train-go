package ginx

import (
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

// ── 用例 4.x：ginx Wrap 与 *errs.Error 集成 ─────────────────────────────────

func init() {
	gin.SetMode(gin.TestMode)
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

func TestWrap_NilErr_Returns200WithData(t *testing.T) {
	rec := runWrap(func(ctx *gin.Context) (Result, error) {
		return Result{Data: map[string]string{"hi": "ok"}}, nil
	})
	assert.Equal(t, http.StatusOK, rec.Code)
	got := decodeResult(t, rec)
	assert.Equal(t, 0, got.Code)
}

func TestWrap_BizErrorNotFound_Returns404(t *testing.T) {
	be := errs.New(404, "用户不存在")
	rec := runWrap(func(ctx *gin.Context) (Result, error) {
		return Result{}, be
	})
	assert.Equal(t, http.StatusNotFound, rec.Code, "*errs.Error.Code 应直通 HTTP status")
	got := decodeResult(t, rec)
	assert.Equal(t, 404, got.Code)
	assert.Equal(t, "用户不存在", got.Msg)
}

func TestWrap_BizErrorInvalidArgument_Returns400(t *testing.T) {
	be := errs.New(400, "标题不能为空")
	rec := runWrap(func(ctx *gin.Context) (Result, error) {
		return Result{}, be
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	got := decodeResult(t, rec)
	assert.Equal(t, "标题不能为空", got.Msg)
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
	wrapped := fmt.Errorf("repo layer: %w", sentinel.WithCause(errors.New("gorm: record not found")))
	rec := runWrap(func(ctx *gin.Context) (Result, error) {
		return Result{}, wrapped
	})
	assert.Equal(t, http.StatusNotFound, rec.Code, "包装多层后 errors.As 仍能抓出 *errs.Error")
	got := decodeResult(t, rec)
	assert.Equal(t, "文章不存在", got.Msg)
}

func TestWrap_BizErrorWithMetadata_PassesToBody(t *testing.T) {
	be := errs.New(404, "用户不存在").
		WithMetadata("uid", "100", "tenant", "abc")
	rec := runWrap(func(ctx *gin.Context) (Result, error) {
		return Result{}, be
	})
	assert.Equal(t, http.StatusNotFound, rec.Code)
	got := decodeResult(t, rec)
	assert.Equal(t, map[string]string{"uid": "100", "tenant": "abc"}, got.Metadata)
}
