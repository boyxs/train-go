package web

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/boyxs/train-go/webook/internal/errs"
	"github.com/boyxs/train-go/webook/internal/service"
	"github.com/boyxs/train-go/webook/pkg/ginx"
	"github.com/boyxs/train-go/webook/pkg/logger"
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
	server.POST("/search/article", ginx.WrapReq[searchReq](h.Search))
}

type searchReq struct {
	Query string `json:"query"`
	Page  int    `json:"page"`
	Size  int    `json:"size"`
}

func (h *InternalArticleSearchHandler) Search(ctx *gin.Context, req searchReq) (ginx.Result, error) {
	if strings.TrimSpace(req.Query) == "" {
		return ginx.Result{}, errs.ErrSearchQueryEmpty
	}
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.Size <= 0 {
		req.Size = 10
	}

	list, total, err := h.svc.Search(ctx.Request.Context(), req.Query, req.Page, req.Size)
	if err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Data: map[string]any{
		"list":  list,
		"total": total,
	}}, nil
}
