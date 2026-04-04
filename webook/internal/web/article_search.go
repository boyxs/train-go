package web

import (
	"net/http"
	"strings"

	"gitee.com/train-cloud/geektime-basic-go/internal/service"
	"gitee.com/train-cloud/geektime-basic-go/pkg/logger"
	"github.com/gin-gonic/gin"
)

type ArticleSearchHandler interface {
	RegisterRoutes(server *gin.Engine)
}

type InternalArticleSearchHandler struct {
	svc service.ArticleSearchService
	l   logger.LoggerX
}

func NewInternalArticleSearchHandler(svc service.ArticleSearchService, l logger.LoggerX) ArticleSearchHandler {
	return &InternalArticleSearchHandler{svc: svc, l: l}
}

func (h *InternalArticleSearchHandler) RegisterRoutes(server *gin.Engine) {
	server.POST("/search/article", h.Search)
}

type searchReq struct {
	Query string `json:"query"`
	Page  int    `json:"page"`
	Size  int    `json:"size"`
}

func (h *InternalArticleSearchHandler) Search(ctx *gin.Context) {
	var req searchReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.Query) == "" {
		ctx.JSON(http.StatusOK, Result{Code: 4, Msg: "搜索内容不能为空"})
		return
	}
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.Size <= 0 {
		req.Size = 10
	}

	list, total, err := h.svc.Search(ctx.Request.Context(), req.Query, req.Page, req.Size)
	if err != nil {
		ctx.JSON(http.StatusOK, Result{Code: 5, Msg: "系统错误"})
		h.l.Error("搜索文章失败", logger.Error(err))
		return
	}

	ctx.JSON(http.StatusOK, Result{
		Data: map[string]any{
			"list":  list,
			"total": total,
		},
	})
}
