package ginx

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/boyxs/train-go/webook/pkg/errs"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

// L 全局 Logger，由 ioc 启动时注入。默认 Nop 防 nil panic。
var L logger.LoggerX = logger.NewNopLogger()

// UserKey 登录态在 gin.Context 里的 key，各服务启动时设为自己的 consts.UserKey。
var UserKey = "user"

// CtxBizReason 业务原因码在 ctx 的 key：WriteError 写入，metrics 中间件读出作 reason label。
const CtxBizReason = "biz_reason"

// HandlerFunc 业务 handler 签名。约定：
//
//	成功     → return Result{Data: ...}, nil    （框架填 code=200）
//	业务错误 → return Result{}, errs.ErrXxx      （自动转对应 HTTP code）
//	系统错误 → return Result{}, err              （自动 500）
//
// 登录态用 MustClaims/Claims 从 ctx 取，不再由 wrapper 注入。
type (
	HandlerFunc             func(ctx *gin.Context) (Result, error)
	HandlerFuncReq[Req any] func(ctx *gin.Context, req Req) (Result, error)
)

// Wrap 包装无请求体 handler。
func Wrap(fn HandlerFunc) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		res, err := fn(ctx)
		respond(ctx, res, err)
	}
}

// WrapReq 反序列化 JSON 请求体后调 handler；绑定失败 → 400 BAD_REQUEST。
func WrapReq[Req any](fn HandlerFuncReq[Req]) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var req Req
		if err := ctx.ShouldBindJSON(&req); err != nil {
			WriteError(ctx, errs.New(http.StatusBadRequest, "参数错误").
				WithReason("BAD_REQUEST").WithCause(err))
			return
		}
		res, err := fn(ctx, req)
		respond(ctx, res, err)
	}
}

// MustClaims 取登录态（受保护路由用，鉴权中间件已保证存在）。
// 缺失 = 该路由漏挂鉴权中间件的 bug → panic（gin Recovery 转 500，fail-loud）。
func MustClaims[C any](ctx *gin.Context) C {
	uc, ok := Claims[C](ctx)
	if !ok {
		panic("ginx: 未找到登录态（key=" + UserKey + "，是否漏挂鉴权中间件？）")
	}
	return uc
}

// Claims 取登录态，返回是否存在；OptionalPaths（登录可选）路由用。
func Claims[C any](ctx *gin.Context) (C, bool) {
	var zero C
	val, exists := ctx.Get(UserKey)
	if !exists {
		return zero, false
	}
	uc, ok := val.(C)
	if !ok {
		return zero, false
	}
	return uc, true
}

// WriteError 翻译 error 为 HTTP 响应（wrap 内部 + SSE/流式外部都用）：
//
//	*errs.Error → HTTP e.Code + Result{Code,Reason,Msg,Metadata}（Warn 日志）
//	其他 error  → HTTP 500 + Result{Code:500, Msg:"系统错误"}（Error 日志，原 err 不出网）
func WriteError(ctx *gin.Context, err error) {
	if err == nil {
		return
	}
	path := ctx.Request.URL.Path
	var be *errs.Error
	if errors.As(err, &be) {
		L.Warn("业务错误", logger.String("path", path), logger.Error(err))
		ctx.Set(CtxBizReason, be.Reason) // 供 metrics 中间件读出作 reason label
		ctx.JSON(be.Code, Result{Code: be.Code, Reason: be.Reason, Msg: be.Message, Metadata: be.Metadata})
		return
	}
	L.Error("业务处理失败", logger.String("path", path), logger.Error(err))
	ctx.JSON(http.StatusInternalServerError, Result{Code: http.StatusInternalServerError, Msg: "系统错误"})
}

// respond：HTTP status = res.Code（默认 200，200 且 msg 空补 "OK"）；有 err 走 WriteError。
func respond(ctx *gin.Context, res Result, err error) {
	if err != nil {
		WriteError(ctx, err)
		return
	}
	if res.Code == 0 {
		res.Code = http.StatusOK
	}
	if res.Msg == "" && res.Code == http.StatusOK {
		res.Msg = "OK"
	}
	ctx.JSON(res.Code, res)
}
