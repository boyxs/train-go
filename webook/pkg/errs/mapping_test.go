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

// ── 用例 2.x：HTTP ↔ gRPC code 映射 ─────────────────────────────────────────

func TestHttpToGRPC(t *testing.T) {
	cases := []struct {
		http int
		grpc codes.Code
	}{
		{200, codes.OK},
		{400, codes.InvalidArgument},
		{401, codes.Unauthenticated},
		{403, codes.PermissionDenied},
		{404, codes.NotFound},
		{409, codes.AlreadyExists},
		{429, codes.ResourceExhausted},
		{500, codes.Internal},
		{999, codes.Unknown}, // 未知 HTTP code
	}
	for _, c := range cases {
		assert.Equal(t, c.grpc, httpToGRPC(c.http), "http=%d", c.http)
	}
}

func TestGrpcToHTTP_RoundTripAllStandardCodes(t *testing.T) {
	cases := []struct {
		grpc codes.Code
		http int
	}{
		{codes.OK, 200},
		{codes.InvalidArgument, 400},
		{codes.Unauthenticated, 401},
		{codes.PermissionDenied, 403},
		{codes.NotFound, 404},
		{codes.AlreadyExists, 409},
		{codes.ResourceExhausted, 429},
		{codes.Canceled, 499},
		{codes.Internal, 500},
		{codes.Unimplemented, 501},
		{codes.Unavailable, 503},
		{codes.DeadlineExceeded, 504},
	}
	for _, c := range cases {
		got := grpcToHTTP(c.grpc)
		assert.Equal(t, c.http, got, "grpc=%v → http", c.grpc)
		// 双向 round-trip
		assert.Equal(t, c.grpc, httpToGRPC(got), "round-trip http=%d → grpc", got)
	}
}

// ── 用例 3.x：FromError 边界转换 ────────────────────────────────────────────

func TestFromError_Nil_ReturnsNil(t *testing.T) {
	assert.Nil(t, FromError(nil))
}

func TestFromError_AlreadyError_PassesThrough(t *testing.T) {
	original := New(404, "用户不存在")
	got := FromError(original)
	require.NotNil(t, got)
	assert.Equal(t, 404, got.Code)
	assert.Equal(t, "用户不存在", got.Message)
}

func TestFromError_WrappedError_StillExtractable(t *testing.T) {
	sentinel := New(404, "用户不存在")
	wrapped := sentinel.WithCause(io.EOF)
	got := FromError(wrapped)
	require.NotNil(t, got)
	assert.Equal(t, 404, got.Code)
	assert.Equal(t, "用户不存在", got.Message)
}

func TestFromError_GRPCStatus_ConvertsBack(t *testing.T) {
	grpcErr := status.Error(codes.NotFound, "用户不存在")
	got := FromError(grpcErr)
	require.NotNil(t, got)
	assert.Equal(t, 404, got.Code)
	assert.Equal(t, "用户不存在", got.Message)
	assert.Equal(t, grpcErr, got.Unwrap())
}

func TestFromError_PlainError_WrapsAsInternal(t *testing.T) {
	plain := errors.New("connection refused")
	got := FromError(plain)
	require.NotNil(t, got)
	assert.Equal(t, 500, got.Code)
	assert.Equal(t, "connection refused", got.Message)
	assert.Equal(t, plain, got.Unwrap())
}
