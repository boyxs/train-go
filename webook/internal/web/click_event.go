package web

import (
	"gitee.com/train-cloud/geektime-basic-go/internal/consts"
	"gitee.com/train-cloud/geektime-basic-go/internal/service"
	"gitee.com/train-cloud/geektime-basic-go/pkg/ginx"
	"gitee.com/train-cloud/geektime-basic-go/pkg/logger"
	"github.com/gin-gonic/gin"
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
	g.POST("/dashboard", ginx.Wrap(h.Dashboard))
}

type clickReq struct {
	ArticleId      int64 `json:"article_id"`
	ConversationId int64 `json:"conversation_id"`
}

func (h *AIClickEventHandler) Click(ctx *gin.Context, req clickReq, uc UserClaims) (ginx.Result, error) {
	if req.ArticleId <= 0 || req.ConversationId <= 0 {
		return ginx.Result{Code: 4, Msg: "参数无效"}, nil
	}
	err := h.svc.RecordClick(ctx, uc.Userid, req.ArticleId, req.ConversationId, "ai_chat")
	if err != nil {
		return ginx.Result{Code: 5, Msg: "系统错误"}, err
	}
	return ginx.Result{Msg: "ok"}, nil
}

func (h *AIClickEventHandler) Dashboard(ctx *gin.Context) (ginx.Result, error) {
	data, err := h.svc.Dashboard(ctx)
	if err != nil {
		return ginx.Result{Code: 5, Msg: "系统错误"}, err
	}
	return ginx.Result{Msg: "ok", Data: data}, nil
}
