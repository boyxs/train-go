package web

import (
	"fmt"
	"net/http"
	"time"

	"gitee.com/train-cloud/geektime-basic-go/internal/consts"
	"gitee.com/train-cloud/geektime-basic-go/internal/service"
	"gitee.com/train-cloud/geektime-basic-go/internal/web/jwt"
	"gitee.com/train-cloud/geektime-basic-go/pkg/logger"
	"gitee.com/train-cloud/geektime-basic-go/pkg/ratelimit"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
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
	server.POST("/article/polish", h.Polish)
}

type polishReq struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

func (h *AIArticlePolishHandler) Polish(ctx *gin.Context) {
	var req polishReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusOK, Result{Code: 4, Msg: "参数错误"})
		return
	}

	uc := ctx.MustGet(consts.UserKey).(jwt.UserClaims)

	// 限流：5 次/小时
	key := fmt.Sprintf(consts.PolishRateLimitPattern, uc.Userid)
	limited, limitErr := h.limiter.Limit(ctx.Request.Context(), key)
	if limitErr != nil {
		h.l.Error("润色限流检查失败", logger.Int64("uid", uc.Userid), logger.Error(limitErr))
	}
	if limited {
		ctx.JSON(http.StatusOK, Result{Code: 4, Msg: "润色次数已达上限，请稍后再试"})
		return
	}

	result, err := h.svc.Polish(ctx.Request.Context(), req.Title, req.Content)
	if err != nil {
		h.l.Error("AI 润色失败",
			logger.Int64("uid", uc.Userid),
			logger.Error(err))
		// 区分业务参数错误和系统错误
		switch err {
		case service.ErrPolishEmptyTitle, service.ErrPolishEmptyContent, service.ErrPolishContentTooLong:
			ctx.JSON(http.StatusOK, Result{Code: 4, Msg: err.Error()})
		default:
			ctx.JSON(http.StatusOK, Result{Code: 5, Msg: "润色失败，请重试"})
		}
		return
	}

	ctx.JSON(http.StatusOK, Result{Code: 0, Msg: "ok", Data: result})
}
