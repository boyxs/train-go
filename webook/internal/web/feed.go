package web

import (
	"github.com/gin-gonic/gin"

	"github.com/boyxs/train-go/webook/internal/domain"
	"github.com/boyxs/train-go/webook/internal/service"
	"github.com/boyxs/train-go/webook/pkg/ginx"
	"github.com/boyxs/train-go/webook/pkg/slicex"
)

// FeedHandler 关注流 HTTP 接入层：绑定请求 + 从 JWT 取 uid → 调 FeedService → domain→VO 映射。
// 五源聚合全在 service.FeedService，本层不含业务逻辑。
type FeedHandler interface {
	RegisterRoutes(server *gin.Engine)
}

type InternalFeedHandler struct {
	svc service.FeedService
}

func NewInternalFeedHandler(svc service.FeedService) FeedHandler {
	return &InternalFeedHandler{svc: svc}
}

func (h *InternalFeedHandler) RegisterRoutes(server *gin.Engine) {
	g := server.Group("/feed")
	// 关注流需登录：uid 取自 JWT（非 OptionalPaths，走默认鉴权中间件）
	g.POST("/list", ginx.WrapReq[feedListReq](h.List))
	g.POST("/new-count", ginx.WrapReq[feedNewCountReq](h.NewCount)) // P1 提示条轮询
}

type feedListReq struct {
	Cursor int64 `json:"cursor"` // 缺省/0=首页
	Limit  int   `json:"limit"`  // 缺省 10，后端夹取 1..20
}

type feedNewCountReq struct {
	SinceCursor int64 `json:"sinceCursor"` // 上次已见最新条目的 publishedAt
}

type feedItemVO struct {
	ArticleId   int64        `json:"articleId"`
	Title       string       `json:"title"`
	Abstract    string       `json:"abstract"`
	Author      feedAuthorVO `json:"author"`
	PublishedAt int64        `json:"publishedAt"`
	LikeCnt     int64        `json:"likeCnt"`
	CollectCnt  int64        `json:"collectCnt"`
	CommentCnt  int64        `json:"commentCnt"`
	Tags        []feedTagVO  `json:"tags"`
}

type feedAuthorVO struct {
	Id       int64  `json:"id"`
	Nickname string `json:"nickname"`
}

type feedTagVO struct {
	Id   int64  `json:"id"`
	Name string `json:"name"`
}

func (h *InternalFeedHandler) List(ctx *gin.Context, req feedListReq) (ginx.Result, error) {
	uc := ginx.MustClaims[UserClaims](ctx)
	items, next, hasMore, err := h.svc.List(ctx.Request.Context(), uc.Userid, req.Cursor, req.Limit)
	if err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Data: gin.H{
		"list":       slicex.Map(items, toFeedItemVO),
		"nextCursor": next,
		"hasMore":    hasMore,
	}}, nil
}

func (h *InternalFeedHandler) NewCount(ctx *gin.Context, req feedNewCountReq) (ginx.Result, error) {
	uc := ginx.MustClaims[UserClaims](ctx)
	count, err := h.svc.NewCount(ctx.Request.Context(), uc.Userid, req.SinceCursor)
	if err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Data: gin.H{"count": count}}, nil
}

func toFeedItemVO(it domain.FeedArticleItem) feedItemVO {
	tags := make([]feedTagVO, 0, len(it.Tags))
	for _, t := range it.Tags {
		tags = append(tags, feedTagVO{Id: t.Id, Name: t.Name})
	}
	return feedItemVO{
		ArticleId:   it.ArticleId,
		Title:       it.Title,
		Abstract:    it.Abstract,
		Author:      feedAuthorVO{Id: it.Author.Id, Nickname: it.Author.Name},
		PublishedAt: it.PublishedAt,
		LikeCnt:     it.LikeCnt,
		CollectCnt:  it.CollectCnt,
		CommentCnt:  it.CommentCnt,
		Tags:        tags,
	}
}
