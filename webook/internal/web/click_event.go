package web

import (
	"net/http"

	"gitee.com/train-cloud/geektime-basic-go/internal/consts"
	"gitee.com/train-cloud/geektime-basic-go/internal/service"
	"gitee.com/train-cloud/geektime-basic-go/internal/web/jwt"
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
	g.POST("/click", h.Click)
	g.POST("/dashboard", h.Dashboard)
}

type clickReq struct {
	ArticleId      int64 `json:"article_id"`
	ConversationId int64 `json:"conversation_id"`
}

func (h *AIClickEventHandler) Click(ctx *gin.Context) {
	var req clickReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusOK, Result{Code: 4, Msg: "参数错误"})
		return
	}
	if req.ArticleId <= 0 || req.ConversationId <= 0 {
		ctx.JSON(http.StatusOK, Result{Code: 4, Msg: "参数无效"})
		return
	}

	uc := ctx.MustGet(consts.UserKey).(jwt.UserClaims)
	err := h.svc.RecordClick(ctx, uc.Userid, req.ArticleId, req.ConversationId, "ai_chat")
	if err != nil {
		h.l.Error("记录点击事件失败",
			logger.Int64("uid", uc.Userid),
			logger.Int64("articleId", req.ArticleId),
			logger.Error(err))
		ctx.JSON(http.StatusOK, Result{Code: 5, Msg: "系统错误"})
		return
	}

	ctx.JSON(http.StatusOK, Result{Code: 0, Msg: "ok"})
}

func (h *AIClickEventHandler) Dashboard(ctx *gin.Context) {
	data, err := h.svc.Dashboard(ctx)
	if err != nil {
		h.l.Error("获取看板数据失败", logger.Error(err))
		ctx.JSON(http.StatusOK, Result{Code: 5, Msg: "系统错误"})
		return
	}
	ctx.JSON(http.StatusOK, Result{Code: 0, Msg: "ok", Data: data})
}
