package web

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/webook/internal/consts"
	"github.com/webook/internal/service"
	"github.com/webook/pkg/ginx"
	"github.com/webook/pkg/logger"
	"github.com/webook/pkg/ratelimit"
)

type ArticlePolishHandler interface {
	RegisterRoutes(server *gin.Engine)
}

type AIArticlePolishHandler struct {
	svc     service.ArticlePolishService
	limiter ratelimit.Limiter
	l       logger.LoggerX
}

func NewAIArticlePolishHandler(svc service.ArticlePolishService, cmd redis.Cmdable, l logger.LoggerX) ArticlePolishHandler {
	return &AIArticlePolishHandler{
		svc:     svc,
		limiter: ratelimit.NewRedisSlidingWindowLimiter(cmd, time.Hour, 5),
		l:       l,
	}
}

func (h *AIArticlePolishHandler) RegisterRoutes(server *gin.Engine) {
	server.POST("/article/polish", ginx.WrapReqClaims[polishReq, UserClaims](consts.UserKey, h.Polish))
}

type polishReq struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

func (h *AIArticlePolishHandler) Polish(ctx *gin.Context, req polishReq, uc UserClaims) (ginx.Result, error) {
	// 限流：5 次/小时
	key := fmt.Sprintf(consts.PolishRateLimitPattern, uc.Userid)
	limited, limitErr := h.limiter.Limit(ctx.Request.Context(), key)
	if limitErr != nil {
		h.l.Error("润色限流检查失败", logger.Int64("uid", uc.Userid), logger.Error(limitErr))
	}
	if limited {
		return ginx.Result{Code: 4, Msg: "润色次数已达上限，请稍后再试"}, nil
	}

	result, err := h.svc.Polish(ctx.Request.Context(), req.Title, req.Content)
	if err != nil {
		// 区分业务参数错误和系统错误
		switch err {
		case service.ErrPolishEmptyTitle, service.ErrPolishEmptyContent, service.ErrPolishContentTooLong:
			return ginx.Result{Code: 4, Msg: err.Error()}, nil
		default:
			return ginx.Result{Code: 5, Msg: "润色失败，请重试"}, err
		}
	}
	return ginx.Result{Msg: "ok", Data: result}, nil
}
