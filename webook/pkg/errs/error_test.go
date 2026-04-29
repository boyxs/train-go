package errs

import (
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ── 用例 1.x：Error 类型核心行为 ────────────────────────────────────────────

func TestNew_FillsFields(t *testing.T) {
	e := New(404, "用户不存在")
	require.NotNil(t, e)
	assert.Equal(t, 404, e.Code)
	assert.Equal(t, "用户不存在", e.Message)
	assert.Nil(t, e.cause)
	assert.Nil(t, e.Metadata)
}

func TestError_Format_NoCause(t *testing.T) {
	e := New(404, "用户不存在")
	assert.Equal(t, "[404] 用户不存在", e.Error())
}

func TestError_Format_WithCause(t *testing.T) {
	e := New(500, "数据库异常").WithCause(io.EOF)
	assert.Equal(t, "[500] 数据库异常: EOF", e.Error())
}

func TestUnwrap_ReturnsCause(t *testing.T) {
	e := New(500, "x").WithCause(io.EOF)
	assert.Equal(t, io.EOF, e.Unwrap())
}

func TestUnwrap_NoCause_ReturnsNil(t *testing.T) {
	e := New(404, "x")
	assert.Nil(t, e.Unwrap())
}

func TestIs_SameCodeAndMessage_DifferentInstances(t *testing.T) {
	a := New(404, "用户不存在")
	b := New(404, "用户不存在") // 不同实例，Code+Message 相同
	assert.True(t, errors.Is(a, b))
	assert.True(t, errors.Is(b, a))
}

func TestIs_DifferentMessage(t *testing.T) {
	a := New(404, "用户不存在")
	b := New(404, "文章不存在")
	assert.False(t, errors.Is(a, b))
}

func TestIs_DifferentCode(t *testing.T) {
	a := New(404, "x")
	b := New(409, "x")
	assert.False(t, errors.Is(a, b))
}

func TestIs_NonErrorTarget_ReturnsFalse(t *testing.T) {
	e := New(404, "x")
	assert.False(t, errors.Is(e, io.EOF))
	assert.False(t, errors.Is(e, errors.New("plain")))
}

func TestErrorsIs_ThroughWrapChain(t *testing.T) {
	sentinel := New(404, "用户不存在")
	wrapped := sentinel.WithCause(io.EOF)
	// 包装后 errors.Is 仍能命中 sentinel（Code+Message 相同）
	assert.True(t, errors.Is(wrapped, sentinel))
}

func TestWithCause_DoesNotMutateSentinel(t *testing.T) {
	sentinel := New(404, "x")
	_ = sentinel.WithCause(io.EOF)
	assert.Nil(t, sentinel.cause, "原 sentinel 不应被污染")
}

func TestWithMetadata_DoesNotMutateSentinel(t *testing.T) {
	sentinel := New(404, "x")
	_ = sentinel.WithMetadata("uid", "100")
	assert.Nil(t, sentinel.Metadata, "原 sentinel.Metadata 不应被污染")
}

func TestWithMetadata_MultipleCallsMerge(t *testing.T) {
	e := New(404, "x").WithMetadata("a", "1", "b", "2")
	assert.Equal(t, map[string]string{"a": "1", "b": "2"}, e.Metadata)
	// merge 语义：累积不同 key + 同 key 覆盖
	e2 := e.WithMetadata("c", "3", "a", "overridden")
	assert.Equal(t, map[string]string{"a": "overridden", "b": "2", "c": "3"}, e2.Metadata)
}

func TestGRPCStatus_CodeAndMessage(t *testing.T) {
	e := New(404, "用户不存在")
	s := e.GRPCStatus()
	require.NotNil(t, s)
	assert.Equal(t, codes.NotFound, s.Code())
	assert.Equal(t, "用户不存在", s.Message())
}

func TestGRPCStatus_RoundTrip_StatusFromError(t *testing.T) {
	e := New(404, "用户不存在")
	grpcErr := e.GRPCStatus().Err()
	s, ok := status.FromError(grpcErr)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, s.Code())
	assert.Equal(t, "用户不存在", s.Message())
}
