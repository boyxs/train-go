package web

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/boyxs/train-go/webook/internal/consts"
	"github.com/boyxs/train-go/webook/internal/errs"
	"github.com/boyxs/train-go/webook/internal/service"
	"github.com/boyxs/train-go/webook/pkg/ginx"
	"github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/pkg/ratelimit"
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
	server.POST("/article/polish", ginx.WrapReq[polishReq](h.Polish))
}

type polishReq struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

func (h *AIArticlePolishHandler) Polish(ctx *gin.Context, req polishReq) (ginx.Result, error) {
	uc := ginx.MustClaims[UserClaims](ctx)
	// 限流：5 次/小时
	key := fmt.Sprintf(consts.PolishRateLimitPattern, uc.Userid)
	limited, limitErr := h.limiter.Limit(ctx.Request.Context(), key)
	if limitErr != nil {
		h.l.WithContext(ctx).Error("润色限流检查失败", logger.Int64("uid", uc.Userid), logger.Error(limitErr))
	}
	if limited {
		return ginx.Result{}, errs.ErrPolishRateLimit
	}

	result, err := h.svc.Polish(ctx.Request.Context(), req.Title, req.Content)
	if err != nil {
		return ginx.Result{}, err // *errs.Error 自动转对应 HTTP，其他系统错误自动 500
	}
	return ginx.Result{Msg: "ok", Data: result}, nil
}
