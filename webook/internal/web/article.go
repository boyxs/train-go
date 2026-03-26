package web

import (
	"net/http"

	"gitee.com/train-cloud/geektime-basic-go/internal/consts"
	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"gitee.com/train-cloud/geektime-basic-go/internal/service"
	"gitee.com/train-cloud/geektime-basic-go/pkg/logger"
	"github.com/gin-gonic/gin"
)

type ArticleHandler interface {
	RegisterRoutes(server *gin.Engine)
	Edit(engine *gin.Context)
}

type InternalArticleHandler struct {
	svc service.ArticleService
	l   logger.LoggerX
}

func NewInternalArticleHandler(svc service.ArticleService, l logger.LoggerX) ArticleHandler {
	return &InternalArticleHandler{
		svc: svc,
		l:   l,
	}
}

func (h *InternalArticleHandler) RegisterRoutes(server *gin.Engine) {
	g := server.Group("/article")
	g.POST("/edit", h.Edit)
}

func (h *InternalArticleHandler) Edit(ctx *gin.Context) {
	type EditRequest struct {
		Id      int64  `json:"id"`
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	var req EditRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
	uc := ctx.MustGet(consts.UserKey).(UserClaims)
	id, err := h.svc.Edit(ctx, domain.Article{
		Id:      req.Id,
		Title:   req.Title,
		Content: req.Content,
		Author: domain.Author{
			Id: uc.Userid,
		},
	})
	if err != nil {
		ctx.JSON(http.StatusOK, Result{
			Msg: "系统错误",
		})
		h.l.Error("编辑文章数据失败",
			logger.Int64("userid", uc.Userid),
			logger.Error(err))
		return
	}
	ctx.JSON(http.StatusOK, Result{
		Data: id,
	})
}
