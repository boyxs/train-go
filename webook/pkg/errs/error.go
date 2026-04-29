// Package errs 业务错误类型 + HTTP/gRPC 双向映射工具（项目通用，不含具体业务 sentinel）。
//
// 设计：
//
//	Error 自带 HTTP code + 人类可读 Message + 可选 Metadata + 底层 cause
//	Is 按 Code+Message 比对，单进程内等价指针比对、跨 gRPC 后仍可命中
//	GRPCStatus 让 gRPC 客户端拿到 *Error 时 status.Code(err) 直通底层
//
// 包结构：
//
//	pkg/errs        — Error 类型 + HTTP↔gRPC 映射 + FromError（项目通用，pkg 无依赖）
//	internal/errs   — webook-core 业务 sentinel（auth/article/code/...）
//	chat/errs       — webook-chat 业务 sentinel
//
// **重要**：errs.New(...) 有两种正确用法，区别在于是否会被 errors.Is 比对：
//
//  1. **sentinel 用法**（会被 errors.Is 比对）：必须用 `var ErrXxx = errs.New(...)`
//     定义在包级别。Is 按 Code+Message 比对（非指针），两个独立 sentinel 如果碰巧
//     Code+Message 字面相同会假命中 —— 这跟标准库 io.EOF / sql.ErrNoRows 指针唯一
//     语义不同，所以 Message 必须全局唯一。
//
//  2. **抛出用法**（不被 errors.Is 比对，仅给 framework 翻译成响应）：函数内
//     `return nil, errs.New(400, "...")` 临时构造可以接受。例如 gRPC server 内
//     参数校验报错抛出去给 grpcx interceptor 转 status，调用方拿到 *errs.Error
//     不做 errors.Is 而是读 Code/Message 字段。
package errs

import (
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Error 业务错误。各字段语义见 package doc。
type Error struct {
	Code     int
	Message  string
	Metadata map[string]string
	cause    error
}

// New 构造业务错误（Code 用 HTTP status：400/401/403/404/409/429/500/...）
func New(code int, message string) *Error {
	return &Error{Code: code, Message: message}
}

// Error 实现 error 接口；带 cause 时附加在末尾，方便日志排查
func (e *Error) Error() string {
	if e.cause == nil {
		return fmt.Sprintf("[%d] %s", e.Code, e.Message)
	}
	return fmt.Sprintf("[%d] %s: %v", e.Code, e.Message, e.cause)
}

// Unwrap 暴露底层 cause 给 errors.Unwrap / errors.As 链式追溯
func (e *Error) Unwrap() error { return e.cause }

// Is 按 Code+Message 比对；让 sentinel vs WithCause 副本、server vs FromError 重建实例都能命中
func (e *Error) Is(target error) bool {
	var t *Error
	if !errors.As(target, &t) {
		return false
	}
	return e.Code == t.Code && e.Message == t.Message
}

// WithCause 返回带底层错误的新副本（不污染原 sentinel）
func (e *Error) WithCause(cause error) *Error {
	cp := *e
	cp.cause = cause
	return &cp
}

// WithMetadata 接受 (key, value, key, value, ...) 平铺参数，返回带 Metadata 的新副本。
// 多次调用为 **merge** 语义（同 key 后调用覆盖前调用，不同 key 累积）。奇数个参数丢弃末位 key。
//
// 例：errs.New(404, "x").WithMetadata("a", "1").WithMetadata("b", "2")
//
//	→ Metadata = {"a": "1", "b": "2"}
func (e *Error) WithMetadata(kv ...string) *Error {
	cp := *e
	if len(kv) < 2 {
		return &cp
	}
	// 拷贝原 Metadata 防修改原 sentinel；nil map 自动 alloc
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

// GRPCStatus 实现 status.GRPCStatus 接口，gRPC 客户端 status.Code(err) 直通业务 code。
// 注：当前 status 不带 details，跨进程后 Metadata 会丢失（仅保留 Code+Message）。
func (e *Error) GRPCStatus() *status.Status {
	return status.New(httpToGRPC(e.Code), e.Message)
}

// FromError 边界转换：把任意 error 归一到 *Error。
// gRPC client interceptor 收到 status.Error 时调用本函数转回业务错误，让上层 errors.As/Is 透明。
//
//	nil           → nil
//	*Error / 包装  → errors.As 抓出，原样返回
//	status.Error  → Code=grpcToHTTP(s.Code()), Message=s.Message(), cause=原 err
//	其他 error    → 兜底 Code=500，cause 保留方便 Unwrap 排查
func FromError(err error) *Error {
	if err == nil {
		return nil
	}
	var be *Error
	if errors.As(err, &be) {
		return be
	}
	if s, ok := status.FromError(err); ok {
		return &Error{Code: grpcToHTTP(s.Code()), Message: s.Message(), cause: err}
	}
	return &Error{Code: 500, Message: err.Error(), cause: err}
}

// httpGRPC 是单一真相源，httpToGRPC / grpcToHTTP 双向查这张表（参考 google.rpc.Code）。
// 一处维护两边对齐：加新 code 只改这里，不会忘记同步反向映射。
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

// grpcHTTP 由 httpGRPC 反向构造（init 一次，运行时 O(1) 查表）
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
