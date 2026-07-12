package web

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/boyxs/train-go/webook/internal/service"
	"github.com/boyxs/train-go/webook/pkg/ginx"
	"github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/pkg/slicex"
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

const maxSearchArticleSize = 50 // /search/article 每页上限

type searchReq struct {
	Query  string       `json:"query"`
	Page   int          `json:"page"`
	Size   int          `json:"size"`
	Filter searchFilter `json:"filter"` // 标签 facet 过滤（多选 AND）
}

type searchFilter struct {
	Tags []string `json:"tags"`
}

// Search 语义搜索 + 标签过滤 + facet。挡空 query + 归一 page/size（防下游 int32 截断 / 越界页）。
func (h *InternalArticleSearchHandler) Search(ctx *gin.Context, req searchReq) (ginx.Result, error) {
	if strings.TrimSpace(req.Query) == "" {
		// 空 query 前置挡（400）；搜索域校验与原因码归属 search 服务，core 不重复定义 reason（全局唯一）
		return ginx.BadRequest("搜索内容不能为空"), nil
	}
	page, size := normalizePage(req.Page, req.Size, maxSearchArticleSize)
	res, err := h.svc.Search(ctx.Request.Context(), req.Query, req.Filter.Tags, page, size)
	if err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Data: gin.H{
		"list":   slicex.Map(res.Articles, toTaggedArticleVO),
		"total":  res.Total,
		"facets": slicex.Map(res.Facets, toFacetVO),
	}}, nil
}
