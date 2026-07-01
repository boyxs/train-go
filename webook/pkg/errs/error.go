// Package errs 业务错误类型 + HTTP/gRPC 双向映射（项目通用，不含具体业务 sentinel）。
//
// Error = HTTP code + 业务原因码 Reason + 展示 Message + 可选 Metadata + cause。
// 具体 sentinel 在各服务包（internal/errs、chat/errs …），本包只给类型与映射。
//
// 两种用法：
//   - sentinel（会被 errors.Is 比对）：var ErrXxx = New(...).WithReason(...)，Is 优先按 reason。
//   - 抛出（不比对，仅给 framework 转响应）：函数内 return New(400, "...") 临时构造即可。
package errs

import (
	"errors"
	"fmt"
	"strconv"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// metaHTTPCode 在 gRPC details 里携带原始 HTTP code，跨 gRPC 精确还原（gRPC code 空间小、映射有损，如 422→Unknown）。
const metaHTTPCode = "_http"

// Error 业务错误。各字段语义见 package doc。
type Error struct {
	Code     int
	Reason   string
	Message  string
	Metadata map[string]string
	cause    error
}

// New 构造业务错误，Code 用 HTTP status。
func New(code int, message string) *Error {
	return &Error{Code: code, Message: message}
}

// Error 实现 error 接口；带 cause 时附加在末尾。
func (e *Error) Error() string {
	if e.cause == nil {
		return fmt.Sprintf("[%d] %s", e.Code, e.Message)
	}
	return fmt.Sprintf("[%d] %s: %v", e.Code, e.Message, e.cause)
}

// Unwrap 暴露 cause 供 errors.Unwrap/As 链式追溯。
func (e *Error) Unwrap() error { return e.cause }

// Is 优先按 Reason 比对（改 Message 不破坏匹配）；任一方无 Reason 则回退 Code+Message。
func (e *Error) Is(target error) bool {
	var t *Error
	if !errors.As(target, &t) {
		return false
	}
	if e.Reason != "" && t.Reason != "" {
		return e.Reason == t.Reason
	}
	return e.Code == t.Code && e.Message == t.Message
}

// WithCause 返回带 cause 的新副本（不污染原 sentinel）。
func (e *Error) WithCause(cause error) *Error {
	cp := *e
	cp.cause = cause
	return &cp
}

// WithMetadata 平铺 (k,v,k,v...) 返回带 Metadata 的新副本；多次调用 merge（同 key 覆盖），奇数参丢末位。
func (e *Error) WithMetadata(kv ...string) *Error {
	cp := *e
	if len(kv) < 2 {
		return &cp
	}
	// 拷贝防污染原 sentinel
	merged := make(map[string]string, len(e.Metadata)+len(kv)/2)
	for k, v := range e.Metadata {
		merged[k] = v
	}
	for i := 0; i+1 < len(kv); i += 2 {
		merged[kv[i]] = kv[i+1]
	}
	cp.Metadata = merged
	return &cp
}

// WithReason 返回带业务原因码的新副本（不污染原 sentinel）。
func (e *Error) WithReason(reason string) *Error {
	cp := *e
	cp.Reason = reason
	return &cp
}

// GRPCStatus 实现 status.GRPCStatus：code 直通，reason + 原始 HTTP code + metadata 进 details（跨 gRPC 保真）。
func (e *Error) GRPCStatus() *status.Status {
	st := status.New(httpToGRPC(e.Code), e.Message)
	// 始终带上原始 HTTP code（映射有损），FromError 据此精确还原
	meta := make(map[string]string, len(e.Metadata)+1)
	for k, v := range e.Metadata {
		meta[k] = v
	}
	meta[metaHTTPCode] = strconv.Itoa(e.Code)
	if d, err := st.WithDetails(&errdetails.ErrorInfo{Reason: e.Reason, Metadata: meta}); err == nil {
		return d
	}
	return st // 失败降级为仅 code+message
}

// FromError 把任意 error 归一到 *Error（gRPC client interceptor 用它转回业务错误）。
func FromError(err error) *Error {
	if err == nil {
		return nil
	}
	var be *Error
	if errors.As(err, &be) {
		return be
	}
	if s, ok := status.FromError(err); ok {
		be := &Error{Code: grpcToHTTP(s.Code()), Message: s.Message(), cause: err}
		// 有 ErrorInfo = 下游业务错误，还原 reason + 原始 HTTP code + 业务 metadata
		for _, d := range s.Details() {
			if info, ok := d.(*errdetails.ErrorInfo); ok {
				be.Reason = info.GetReason()
				be.restoreMetadata(info.GetMetadata())
				return be
			}
		}
		// 无 ErrorInfo = 传输层错误，原始 message 是内部细节，按 code 换友好文案（原始留 cause）
		switch s.Code() {
		case codes.Unavailable:
			be.Message, be.Reason = "服务暂时不可用，请稍后重试", "SERVICE_UNAVAILABLE"
		case codes.DeadlineExceeded:
			be.Message, be.Reason = "请求超时，请稍后重试", "SERVICE_TIMEOUT"
		case codes.Canceled:
			be.Message, be.Reason = "请求已取消", "REQUEST_CANCELED"
		}
		return be
	}
	return &Error{Code: 500, Message: err.Error(), cause: err}
}

// restoreMetadata 从 details metadata 还原原始 HTTP code（有则覆盖 Code）并剥掉内部 key，剩余作业务 Metadata。
func (e *Error) restoreMetadata(md map[string]string) {
	if len(md) == 0 {
		return
	}
	hc, ok := md[metaHTTPCode]
	if !ok {
		e.Metadata = md
		return
	}
	if code, err := strconv.Atoi(hc); err == nil {
		e.Code = code
	}
	rest := make(map[string]string, len(md)-1)
	for k, v := range md {
		if k != metaHTTPCode {
			rest[k] = v
		}
	}
	if len(rest) > 0 {
		e.Metadata = rest
	}
}

// httpGRPC 单一真相源，双向映射查这张表（加 code 只改这里）。
var httpGRPC = map[int]codes.Code{
	200: codes.OK,
	400: codes.InvalidArgument,
	401: codes.Unauthenticated,
	403: codes.PermissionDenied,
	404: codes.NotFound,
	409: codes.AlreadyExists,
	429: codes.ResourceExhausted,
	499: codes.Canceled,
	500: codes.Internal,
	501: codes.Unimplemented,
	503: codes.Unavailable,
	504: codes.DeadlineExceeded,
}

// grpcHTTP 由 httpGRPC 反向构造。
var grpcHTTP = func() map[codes.Code]int {
	m := make(map[codes.Code]int, len(httpGRPC))
	for h, g := range httpGRPC {
		m[g] = h
	}
	return m
}()

// httpToGRPC 未知 HTTP code → codes.Unknown
func httpToGRPC(code int) codes.Code {
	if c, ok := httpGRPC[code]; ok {
		return c
	}
	return codes.Unknown
}

// grpcToHTTP 未知 gRPC code → 500
func grpcToHTTP(c codes.Code) int {
	if h, ok := grpcHTTP[c]; ok {
		return h
	}
	return 500
}
