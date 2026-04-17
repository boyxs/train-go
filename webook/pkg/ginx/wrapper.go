package ginx

import (
	"net/http"

	"github.com/webook/pkg/logger"
	"github.com/gin-gonic/gin"
)

// L 全局 Logger，由 ioc 在启动时注入
// 默认 NopLogger 防止 nil panic（测试环境可能没注入）
var L logger.LoggerX = logger.NewNopLogger()

// CodeToHttpStatus 将业务 Code 映射为 HTTP 状态码
// 由 ioc 在启动时注入，未注入时使用默认规则
var CodeToHttpStatus func(code int) int = defaultCodeToHttpStatus

func defaultCodeToHttpStatus(code int) int {
	switch code {
	case 0:
		return http.StatusOK // 成功
	case 4:
		return http.StatusBadRequest // 客户端错误
	case 5:
		return http.StatusInternalServerError // 服务端错误
	default:
		return http.StatusOK
	}
}

// httpStatus 决定 HTTP 状态码
// err != nil 一律 500（handler 约定：err 只用于系统错误，业务错误走 Code != 0 + err == nil）
func httpStatus(res Result, err error) int {
	if err != nil {
		return http.StatusInternalServerError
	}
	return CodeToHttpStatus(res.Code)
}

// HandlerFunc 业务 handler 签名：返回 (Result, error)
//
//	error != nil  → httpStatus 决定状态码 + 系统错误日志
//	error == nil  → CodeToHttpStatus(Result.Code) 决定状态码
type HandlerFunc func(ctx *gin.Context) (Result, error)

// HandlerFuncReq 带请求体反序列化的业务 handler 签名
type HandlerFuncReq[Req any] func(ctx *gin.Context, req Req) (Result, error)

// HandlerFuncClaims 带 UserClaims 的业务 handler 签名
type HandlerFuncClaims[C any] func(ctx *gin.Context, uc C) (Result, error)

// HandlerFuncReqClaims 同时带请求体和 UserClaims
type HandlerFuncReqClaims[Req any, C any] func(ctx *gin.Context, req Req, uc C) (Result, error)

// Wrap 包装最简单的 handler：自动写响应 + 记录 error 日志
func Wrap(fn HandlerFunc) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		res, err := fn(ctx)
		if err != nil {
			L.Error("业务处理失败",
				logger.String("path", ctx.Request.URL.Path),
				logger.Error(err))
		}
		ctx.JSON(httpStatus(res, err), res)
	}
}

// WrapReq 包装带请求体的 handler：自动反序列化 + 写响应 + 记录日志
// 反序列化失败直接 400
func WrapReq[Req any](fn HandlerFuncReq[Req]) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var req Req
		if err := ctx.ShouldBindJSON(&req); err != nil {
			ctx.JSON(http.StatusBadRequest, Result{Code: 4, Msg: "参数错误"})
			return
		}
		res, err := fn(ctx, req)
		if err != nil {
			L.Error("业务处理失败",
				logger.String("path", ctx.Request.URL.Path),
				logger.Error(err))
		}
		ctx.JSON(httpStatus(res, err), res)
	}
}

// WrapClaims 包装需要登录的 handler：自动取 UserClaims + 写响应 + 记录日志
// userKey 由调用方指定，避免 ginx 依赖业务常量
func WrapClaims[C any](userKey string, fn HandlerFuncClaims[C]) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		val, exists := ctx.Get(userKey)
		if !exists {
			ctx.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		uc, ok := val.(C)
		if !ok {
			ctx.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		res, err := fn(ctx, uc)
		if err != nil {
			L.Error("业务处理失败",
				logger.String("path", ctx.Request.URL.Path),
				logger.Error(err))
		}
		ctx.JSON(httpStatus(res, err), res)
	}
}

// WrapReqClaims 同时带请求体和 UserClaims
func WrapReqClaims[Req any, C any](userKey string, fn HandlerFuncReqClaims[Req, C]) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var req Req
		if err := ctx.ShouldBindJSON(&req); err != nil {
			ctx.JSON(http.StatusBadRequest, Result{Code: 4, Msg: "参数错误"})
			return
		}
		val, exists := ctx.Get(userKey)
		if !exists {
			ctx.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		uc, ok := val.(C)
		if !ok {
			ctx.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		res, err := fn(ctx, req, uc)
		if err != nil {
			L.Error("业务处理失败",
				logger.String("path", ctx.Request.URL.Path),
				logger.Error(err))
		}
		ctx.JSON(httpStatus(res, err), res)
	}
}
