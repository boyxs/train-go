package web

import (
	"os"

	"github.com/gin-gonic/gin"

	"github.com/webook/internal/consts"
	"github.com/webook/internal/errs"
	"github.com/webook/internal/service"
	"github.com/webook/pkg/ginx"
	"github.com/webook/pkg/logger"
)

type ClickEventHandler interface {
	RegisterRoutes(server *gin.Engine)
}

type AIClickEventHandler struct {
	svc service.ClickEventService
	l   logger.LoggerX
}

func NewAIClickEventHandler(svc service.ClickEventService, l logger.LoggerX) ClickEventHandler {
	return &AIClickEventHandler{svc: svc, l: l}
}

func (h *AIClickEventHandler) RegisterRoutes(server *gin.Engine) {
	g := server.Group("/ai")
	g.POST("/click", ginx.WrapReqClaims[clickReq, UserClaims](consts.UserKey, h.Click))
	// dashboard 是运营/调试接口（聚合点击数据 + Top 文章），生产环境禁用避免外泄；
	// 同 ranking.Archive 模式：DEPLOY_ENV=prod 时不注册路由，本地/dev/staging 保留以便调试
	if os.Getenv("DEPLOY_ENV") != "prod" {
		g.POST("/dashboard", ginx.Wrap(h.Dashboard))
	}
}

type clickReq struct {
	ArticleId      int64 `json:"article_id"`
	ConversationId int64 `json:"conversation_id"`
}

func (h *AIClickEventHandler) Click(ctx *gin.Context, req clickReq, uc UserClaims) (ginx.Result, error) {
	if req.ArticleId <= 0 || req.ConversationId <= 0 {
		return ginx.Result{}, errs.ErrClickInvalidArgs
	}
	err := h.svc.RecordClick(ctx, uc.Userid, req.ArticleId, req.ConversationId, "ai_chat")
	if err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Msg: "ok"}, nil
}

func (h *AIClickEventHandler) Dashboard(ctx *gin.Context) (ginx.Result, error) {
	data, err := h.svc.Dashboard(ctx)
	if err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Msg: "ok", Data: data}, nil
}
