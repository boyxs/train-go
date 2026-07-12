package web

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/boyxs/train-go/webook/internal/domain"
	"github.com/boyxs/train-go/webook/internal/service"
	"github.com/boyxs/train-go/webook/pkg/ginx"
	"github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/pkg/slicex"
)

const (
	maxTagArticleSize = 50    // /tag/:slug/articles 每页上限（架构 §4.2）
	maxPageIndex      = 10000 // 页码上限：防下游 int32 截断 + (page-1)*size 的 int 溢出（窗口外本就空）
)

// normalizePage 归一分页：page→[1,maxPageIndex]，size 非法/超限取 10。挡住越界 page 引发的下游截断/切片溢出。
func normalizePage(page, size, maxSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if page > maxPageIndex {
		page = maxPageIndex
	}
	if size <= 0 || size > maxSize {
		size = 10
	}
	return page, size
}

type TagHandler interface {
	RegisterRoutes(server *ginx.Router)
}

type InternalTagHandler struct {
	svc service.TagService
	l   logger.LoggerX
}

func NewInternalTagHandler(svc service.TagService, l logger.LoggerX) TagHandler {
	return &InternalTagHandler{svc: svc, l: l}
}

func (h *InternalTagHandler) RegisterRoutes(server *ginx.Router) {
	server.GET("/tag/suggest", ginx.Wrap(h.Suggest))                                    // typeahead（需登录）
	server.POST("/tag/recommend", ginx.WrapReq[recommendReq](h.Recommend))              // AI 荐标签（需登录，超时豁免）
	server.Optional.GET("/tag/:slug", ginx.Wrap(h.Detail))                              // 标签详情（公开可读；登录才算 isFollowing）
	server.Public.POST("/tag/:slug/articles", ginx.WrapReq[tagArticlesReq](h.Articles)) // 标签下文章（公开：自声明）
	server.POST("/tag/:slug/follow", ginx.Wrap(h.Follow))                               // 关注标签（需登录）
	server.DELETE("/tag/:slug/follow", ginx.Wrap(h.Unfollow))                           // 取关标签（需登录）
}

func (h *InternalTagHandler) Suggest(ctx *gin.Context) (ginx.Result, error) {
	limit, _ := strconv.Atoi(ctx.Query("limit"))
	tags, err := h.svc.Suggest(ctx.Request.Context(), ctx.Query("q"), limit)
	if err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Data: slicex.Map(tags, toSuggestVO)}, nil
}

func (h *InternalTagHandler) Recommend(ctx *gin.Context, req recommendReq) (ginx.Result, error) {
	tags, err := h.svc.Recommend(ctx.Request.Context(), req.Title, req.Content)
	if err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Data: slicex.Map(tags, toTagCountNameVO)}, nil
}

func (h *InternalTagHandler) Detail(ctx *gin.Context) (ginx.Result, error) {
	t, isFollowing, err := h.svc.Detail(ctx.Request.Context(), ctx.Param("slug"), optionalUid(ctx))
	if err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Data: tagDetailVO{
		Name:           t.Name,
		Slug:           t.Slug,
		Description:    t.Description,
		RefCount:       t.RefCount,
		FollowCount:    t.FollowCount,
		WeeklyNewCount: t.WeeklyNewCount,
		IsFollowing:    isFollowing,
	}}, nil
}

func (h *InternalTagHandler) Follow(ctx *gin.Context) (ginx.Result, error) {
	uc := ginx.MustClaims[UserClaims](ctx)
	_, cnt, err := h.svc.Follow(ctx.Request.Context(), uc.Userid, ctx.Param("slug"))
	if err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Data: followVO{IsFollowing: true, FollowCount: cnt}}, nil
}

func (h *InternalTagHandler) Unfollow(ctx *gin.Context) (ginx.Result, error) {
	uc := ginx.MustClaims[UserClaims](ctx)
	_, cnt, err := h.svc.Unfollow(ctx.Request.Context(), uc.Userid, ctx.Param("slug"))
	if err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Data: followVO{IsFollowing: false, FollowCount: cnt}}, nil
}

func (h *InternalTagHandler) Articles(ctx *gin.Context, req tagArticlesReq) (ginx.Result, error) {
	page, size := normalizePage(req.Page, req.Size, maxTagArticleSize)
	res, err := h.svc.TagArticles(ctx.Request.Context(), ctx.Param("slug"), req.Sort, page, size)
	if err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Data: ginx.PageResult{
		List:  slicex.Map(res.Articles, toTaggedArticleVO),
		Total: res.Total,
	}}, nil
}

// ── 请求 / 响应 VO ──────────────────────────────────────────────────────────

type recommendReq struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

type tagArticlesReq struct {
	Page int    `json:"page"`
	Size int    `json:"size"`
	Sort string `json:"sort"` // new(默认) | hot
}

type suggestVO struct {
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	RefCount int64  `json:"refCount"`
}

type tagDetailVO struct {
	Name           string `json:"name"`
	Slug           string `json:"slug"`
	Description    string `json:"description"`
	RefCount       int64  `json:"refCount"`
	FollowCount    int64  `json:"followCount"`
	WeeklyNewCount int64  `json:"weeklyNewCount"`
	IsFollowing    bool   `json:"isFollowing"`
}

type followVO struct {
	IsFollowing bool  `json:"isFollowing"`
	FollowCount int64 `json:"followCount"`
}

type tagVO struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type facetVO struct {
	Name  string `json:"name"`
	Slug  string `json:"slug"`
	Count int64  `json:"count"`
}

type taggedArticleVO struct {
	Id         int64    `json:"id"`
	Title      string   `json:"title"`
	Abstract   string   `json:"abstract"`
	Author     authorVO `json:"author"`
	Category   string   `json:"category"`
	Tags       []tagVO  `json:"tags"`
	ReadCnt    int64    `json:"readCnt"`
	LikeCnt    int64    `json:"likeCnt"`
	CollectCnt int64    `json:"collectCnt"`
	CreatedAt  int64    `json:"createdAt"`
}

type authorVO struct {
	Id   int64  `json:"id"`
	Name string `json:"name"`
}

// ── domain → VO 单条映射（批量走 slicex.Map）─────────────────────────────────

func toSuggestVO(t domain.Tag) suggestVO {
	return suggestVO{Name: t.Name, Slug: t.Slug, RefCount: t.RefCount}
}

func toTagVO(t domain.Tag) tagVO {
	return tagVO{Name: t.Name, Slug: t.Slug}
}

func toTagCountNameVO(t domain.TagCount) tagVO {
	return tagVO{Name: t.Name, Slug: t.Slug}
}

func toFacetVO(t domain.TagCount) facetVO {
	return facetVO{Name: t.Name, Slug: t.Slug, Count: t.Count}
}

func toTaggedArticleVO(a domain.TaggedArticle) taggedArticleVO {
	return taggedArticleVO{
		Id:         a.Id,
		Title:      a.Title,
		Abstract:   a.Abstract,
		Author:     authorVO{Id: a.Author.Id, Name: a.Author.Name},
		Category:   a.Category,
		Tags:       slicex.Map(a.Tags, toTagVO),
		ReadCnt:    a.ReadCnt,
		LikeCnt:    a.LikeCnt,
		CollectCnt: a.CollectCnt,
		CreatedAt:  a.CreatedAt,
	}
}
