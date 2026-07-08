package errs

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
	e2 := e.WithMetadata("c", "3", "a", "overridden")
	assert.Equal(t, map[string]string{"a": "overridden", "b": "2", "c": "3"}, e2.Metadata)
}

// ── 用例 2.x：Reason 业务原因码 ──────────────────────────────────────────────

func TestWithReason_SetsReason_DoesNotMutateSentinel(t *testing.T) {
	sentinel := New(429, "润色次数已达上限")
	got := sentinel.WithReason("POLISH_RATE_LIMITED")
	assert.Equal(t, "POLISH_RATE_LIMITED", got.Reason)
	assert.Equal(t, "", sentinel.Reason, "原 sentinel.Reason 不应被污染")
}

func TestWithReason_PreservesOtherFields(t *testing.T) {
	e := New(429, "msg").WithMetadata("a", "1").WithCause(io.EOF).WithReason("R")
	assert.Equal(t, 429, e.Code)
	assert.Equal(t, "msg", e.Message)
	assert.Equal(t, "R", e.Reason)
	assert.Equal(t, map[string]string{"a": "1"}, e.Metadata)
	assert.Equal(t, io.EOF, e.Unwrap())
}

// reason 优先：身份只看 reason，Code/Message 不同也命中
func TestIs_SameReason_DifferentCodeAndMessage(t *testing.T) {
	a := New(429, "润色次数已达上限").WithReason("POLISH_RATE_LIMITED")
	b := New(400, "文案换了").WithReason("POLISH_RATE_LIMITED")
	assert.True(t, errors.Is(a, b))
	assert.True(t, errors.Is(b, a))
}

// reason 不同 → 不命中，即使 Code+Message 完全相同
func TestIs_DifferentReason_SameCodeAndMessage(t *testing.T) {
	a := New(429, "操作太频繁").WithReason("POLISH_RATE_LIMITED")
	b := New(429, "操作太频繁").WithReason("COMMENT_RATE_LIMITED")
	assert.False(t, errors.Is(a, b))
}

// 一方有 reason 一方无 → 回退 Code+Message（迁移期新旧 sentinel 混用）
func TestIs_OneReasonEmpty_FallsBackToCodeMessage(t *testing.T) {
	withReason := New(404, "用户不存在").WithReason("USER_NOT_FOUND")
	legacy := New(404, "用户不存在")
	assert.True(t, errors.Is(withReason, legacy), "回退 Code+Message，应命中")
	assert.False(t, errors.Is(withReason, New(404, "文章不存在")))
}

// ── 用例 3.x：GRPCStatus / FromError 往返 ───────────────────────────────────

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

// 跨 gRPC 往返保真 reason + metadata
func TestFromError_GRPCRoundTrip_PreservesReasonAndMetadata(t *testing.T) {
	e := New(429, "润色次数已达上限").
		WithReason("POLISH_RATE_LIMITED").
		WithMetadata("retry_after", "60")
	got := FromError(e.GRPCStatus().Err())
	assert.Equal(t, 429, got.Code)
	assert.Equal(t, "POLISH_RATE_LIMITED", got.Reason)
	assert.Equal(t, "润色次数已达上限", got.Message)
	assert.Equal(t, "60", got.Metadata["retry_after"])
	assert.NotContains(t, got.Metadata, metaHTTPCode, "内部 _http key 不该外泄到业务 Metadata")
}

// 不在映射表的 HTTP code（如 422 敏感词）跨 gRPC 也精确还原，不退化成 500。
func TestFromError_GRPCRoundTrip_PreservesUnmappedHTTPCode(t *testing.T) {
	e := New(422, "内容包含敏感词").WithReason("COMMENT_CONTENT_SENSITIVE")
	got := FromError(e.GRPCStatus().Err())
	assert.Equal(t, 422, got.Code, "422 应精确还原，而非退化成 500")
	assert.Equal(t, "COMMENT_CONTENT_SENSITIVE", got.Reason)
	assert.Equal(t, "内容包含敏感词", got.Message)
	assert.NotContains(t, got.Metadata, metaHTTPCode)
}

// 传输层错误无 ErrorInfo → 友好文案 + reason，原始 message 不外泄（留 cause）。
func TestFromError_TransportError_FriendlyMessageNotLeaked(t *testing.T) {
	got := FromError(status.New(codes.Unavailable, "name resolver error: produced zero addresses").Err())
	assert.Equal(t, 503, got.Code)
	assert.Equal(t, "SERVICE_UNAVAILABLE", got.Reason)
	assert.Equal(t, "服务暂时不可用，请稍后重试", got.Message, "原始 gRPC 内部 message 不该外泄给用户")
	assert.ErrorContains(t, got, "produced zero addresses", "原始细节应留 cause 供日志排查")

	// 超时
	got2 := FromError(status.New(codes.DeadlineExceeded, "context deadline exceeded").Err())
	assert.Equal(t, 504, got2.Code)
	assert.Equal(t, "SERVICE_TIMEOUT", got2.Reason)
	assert.Equal(t, "请求超时，请稍后重试", got2.Message)

	// 取消
	got3 := FromError(status.New(codes.Canceled, "context canceled").Err())
	assert.Equal(t, 499, got3.Code)
	assert.Equal(t, "REQUEST_CANCELED", got3.Reason)
	assert.Equal(t, "请求已取消", got3.Message)
}

// ── 用例 4.x：HTTP ↔ gRPC code 映射 ─────────────────────────────────────────

func TestHttpToGRPC(t *testing.T) {
	testCases := []struct {
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
	for _, c := range testCases {
		assert.Equal(t, c.grpc, httpToGRPC(c.http), "http=%d", c.http)
	}
}

func TestGrpcToHTTP_RoundTripAllStandardCodes(t *testing.T) {
	testCases := []struct {
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
	for _, c := range testCases {
		got := grpcToHTTP(c.grpc)
		assert.Equal(t, c.http, got, "grpc=%v → http", c.grpc)
		assert.Equal(t, c.grpc, httpToGRPC(got), "round-trip http=%d → grpc", got)
	}
}

// ── 用例 5.x：FromError 边界转换 ────────────────────────────────────────────

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

// ── 用例 6.x：reason 全仓强约束（扫源码，跳过 _test.go）─────────────────────

// 所有 .WithReason("X") 字面量：reason 全局唯一 + SCREAMING_SNAKE。
func TestReasonUniqueAndWellFormed(t *testing.T) {
	root := repoRoot(t)
	reasonCall := regexp.MustCompile(`\.WithReason\("([^"]*)"\)`)
	naming := regexp.MustCompile(`^[A-Z][A-Z0-9]*(_[A-Z0-9]+)*$`)

	seen := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", ".git", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		rel, _ := filepath.Rel(root, path)
		for _, m := range reasonCall.FindAllStringSubmatch(string(data), -1) {
			reason := m[1]
			if reason == "" {
				continue
			}
			assert.Truef(t, naming.MatchString(reason), "reason %q (%s) 不符合 SCREAMING_SNAKE", reason, rel)
			if first, dup := seen[reason]; dup {
				t.Errorf("reason %q 重复定义：%s 与 %s", reason, first, rel)
			} else {
				seen[reason] = rel
			}
		}
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, seen, "应至少扫到一个 reason")
	t.Logf("扫描到 %d 个唯一 reason", len(seen))
}

// 每个包级 sentinel（var X = errs.New(...)）必须 .WithReason；inline 抛错（无赋值）不在此列。
func TestAllSentinelsHaveReason(t *testing.T) {
	root := repoRoot(t)
	sentinel := regexp.MustCompile(`\b\w+\s*=\s*errs\.New\(`)
	var missing []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", ".git", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		rel, _ := filepath.Rel(root, path)
		for i, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			if sentinel.MatchString(line) && !strings.Contains(line, ".WithReason(") {
				missing = append(missing, fmt.Sprintf("%s:%d  %s", rel, i+1, strings.TrimSpace(line)))
			}
		}
		return nil
	})
	require.NoError(t, err)
	assert.Empty(t, missing, "以下 sentinel 缺 .WithReason：\n%s", strings.Join(missing, "\n"))
}

// repoRoot 从测试 cwd（.../webook/pkg/errs）向上找含 go.work 的 workspace 根（webook/）。
// 多模块化后各模块自带 go.mod，"第一个 go.mod" 只会停在 pkg/ 扫不到任何服务的 errs sentinel；
// 仅 go.work 标记全仓 workspace 根，据此锚定才能扫描全部服务的 .WithReason。
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.work")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqualf(t, parent, dir, "向上未找到 go.work")
		dir = parent
	}
}
