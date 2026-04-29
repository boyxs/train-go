package ginx

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/webook/pkg/errs"
	"github.com/webook/pkg/logger"
)

// L 全局 Logger，由 ioc 在启动时注入。默认 NopLogger 防 nil panic。
var L logger.LoggerX = logger.NewNopLogger()

// HandlerFunc 业务 handler 签名。约定：
//
//	成功     → return ginx.Result{Data: ...}, nil
//	业务错误 → return ginx.Result{}, errs.ErrXxx     （自动转对应 HTTP code）
//	系统错误 → return ginx.Result{}, err             （任意 err，自动 500）
type (
	HandlerFunc                          func(ctx *gin.Context) (Result, error)
	HandlerFuncReq[Req any]              func(ctx *gin.Context, req Req) (Result, error)
	HandlerFuncClaims[C any]             func(ctx *gin.Context, uc C) (Result, error)
	HandlerFuncReqClaims[Req any, C any] func(ctx *gin.Context, req Req, uc C) (Result, error)
)

// WriteError 翻译 error 为 HTTP 响应。给非 wrap 路径（SSE / 自定义流式）直接调用；
// wrap 内部也走这个。规则：
//
//	*errs.Error → HTTP e.Code + Result{Code, Msg, Metadata}（业务错误 Warn 日志）
//	其他 error  → HTTP 500 + Result{Code:500, Msg:"系统错误"}（系统错误 Error 日志，原 err 不出网）
func WriteError(ctx *gin.Context, err error) {
	if err == nil {
		return
	}
	path := ctx.Request.URL.Path
	var be *errs.Error
	if errors.As(err, &be) {
		L.Warn("业务错误", logger.String("path", path), logger.Error(err))
		ctx.JSON(be.Code, Result{Code: be.Code, Msg: be.Message, Metadata: be.Metadata})
		return
	}
	L.Error("业务处理失败", logger.String("path", path), logger.Error(err))
	ctx.JSON(http.StatusInternalServerError, Result{
		Code: http.StatusInternalServerError,
		Msg:  "系统错误",
	})
}

// respond 内部 helper：err==nil 写 200+res；否则走 WriteError。
func respond(ctx *gin.Context, res Result, err error) {
	if err != nil {
		WriteError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, res)
}

// bindBadRequest ShouldBindJSON 失败统一响应：HTTP 400 + 参数错误。
func bindBadRequest(ctx *gin.Context) {
	ctx.JSON(http.StatusBadRequest, Result{
		Code: http.StatusBadRequest,
		Msg:  "参数错误",
	})
}

// claimsOf 从 ctx 取 UserClaims；缺失或类型不匹配 → 401 abort，第二个返回值标识是否成功。
func claimsOf[C any](ctx *gin.Context, key string) (C, bool) {
	var zero C
	val, exists := ctx.Get(key)
	if !exists {
		ctx.AbortWithStatus(http.StatusUnauthorized)
		return zero, false
	}
	uc, ok := val.(C)
	if !ok {
		ctx.AbortWithStatus(http.StatusUnauthorized)
		return zero, false
	}
	return uc, true
}

// Wrap 包装最简单的 handler。
func Wrap(fn HandlerFunc) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		res, err := fn(ctx)
		respond(ctx, res, err)
	}
}

// WrapReq 反序列化请求体。失败 → 400。
func WrapReq[Req any](fn HandlerFuncReq[Req]) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var req Req
		if err := ctx.ShouldBindJSON(&req); err != nil {
			bindBadRequest(ctx)
			return
		}
		res, err := fn(ctx, req)
		respond(ctx, res, err)
	}
}

// WrapClaims 取 UserClaims。缺失/类型错 → 401。
func WrapClaims[C any](userKey string, fn HandlerFuncClaims[C]) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		uc, ok := claimsOf[C](ctx, userKey)
		if !ok {
			return
		}
		res, err := fn(ctx, uc)
		respond(ctx, res, err)
	}
}

// WrapReqClaims 反序列化 + 取 Claims；前者失败 400 短路，后者失败 401。
func WrapReqClaims[Req any, C any](userKey string, fn HandlerFuncReqClaims[Req, C]) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var req Req
		if err := ctx.ShouldBindJSON(&req); err != nil {
			bindBadRequest(ctx)
			return
		}
		uc, ok := claimsOf[C](ctx, userKey)
		if !ok {
			return
		}
		res, err := fn(ctx, req, uc)
		respond(ctx, res, err)
	}
}
