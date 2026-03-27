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
	Edit(ctx *gin.Context)
	Publish(ctx *gin.Context)
	Withdraw(ctx *gin.Context)
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
	g.POST("/publish", h.Publish)
	g.POST("/withdraw", h.Withdraw)
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
	if req.Title == "" || req.Content == "" {
		ctx.JSON(http.StatusOK, Result{
			Code: 4,
			Msg:  "标题和内容不能为空",
		})
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

func (h *InternalArticleHandler) Publish(ctx *gin.Context) {
	type PublishRequest struct {
		Id      int64  `json:"id"`
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	var req PublishRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if req.Title == "" || req.Content == "" {
		ctx.JSON(http.StatusOK, Result{
			Code: 4,
			Msg:  "标题和内容不能为空",
		})
		return
	}
	uc := ctx.MustGet(consts.UserKey).(UserClaims)
	_, err := h.svc.Publish(ctx, domain.Article{
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
		h.l.Error("发布文章失败",
			logger.Int64("userid", uc.Userid),
			logger.Error(err))
		return
	}
	ctx.JSON(http.StatusOK, Result{
		Msg: "OK",
	})
}

func (h *InternalArticleHandler) Withdraw(ctx *gin.Context) {
	type WithdrawRequest struct {
		Id int64 `json:"id"`
	}
	var req WithdrawRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
	uc := ctx.MustGet(consts.UserKey).(UserClaims)
	err := h.svc.Withdraw(ctx, req.Id, uc.Userid)
	if err != nil {
		ctx.JSON(http.StatusOK, Result{
			Msg: "系统错误",
		})
		h.l.Error("撤回文章失败",
			logger.Int64("userid", uc.Userid),
			logger.Error(err))
		return
	}
	ctx.JSON(http.StatusOK, Result{
		Msg: "OK",
	})
}
