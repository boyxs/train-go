package web

import (
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"

	"github.com/boyxs/train-go/webook/internal/domain"
	"github.com/boyxs/train-go/webook/internal/errs"
	"github.com/boyxs/train-go/webook/internal/service"
	"github.com/boyxs/train-go/webook/pkg/ginx"
	"github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/pkg/slicex"
)

type ArticleAuthorHandler interface {
	RegisterRoutes(server *gin.Engine)
}

type InternalArticleAuthorHandler struct {
	svc     service.ArticleAuthorService
	intrSvc service.InteractionService
	l       logger.LoggerX
}

func NewInternalArticleAuthorHandler(svc service.ArticleAuthorService, intrSvc service.InteractionService, l logger.LoggerX) ArticleAuthorHandler {
	return &InternalArticleAuthorHandler{
		svc:     svc,
		intrSvc: intrSvc,
		l:       l,
	}
}

type editReq struct {
	Id       int64    `json:"id"`
	Title    string   `json:"title"`
	Abstract string   `json:"abstract"`
	Content  string   `json:"content"`
	Category string   `json:"category"` // 分区（可空）
	Tags     []string `json:"tags"`     // 作者输入的标签名（≤5，发布时经 tag 服务归一，Edit 存草稿不落标签）
}

type idReq struct {
	Id int64 `json:"id"`
}

type pageReq struct {
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
}

func (h *InternalArticleAuthorHandler) RegisterRoutes(server *gin.Engine) {
	g := server.Group("/article")
	g.POST("/edit", ginx.WrapReq[editReq](h.Edit))
	g.POST("/publish", ginx.WrapReq[editReq](h.Publish))
	g.POST("/withdraw", ginx.WrapReq[idReq](h.Withdraw))
	g.POST("/detail", ginx.WrapReq[idReq](h.Detail))
	g.POST("/page", ginx.WrapReq[pageReq](h.Page))
	g.POST("/list", ginx.Wrap(h.List))
	g.POST("/delete", ginx.WrapReq[idReq](h.Delete))
}

func (h *InternalArticleAuthorHandler) Edit(ctx *gin.Context, req editReq) (ginx.Result, error) {
	uc := ginx.MustClaims[UserClaims](ctx)
	if req.Title == "" || req.Content == "" {
		return ginx.Result{}, errs.ErrArticleEmptyTitleOrContent
	}
	id, err := h.svc.Edit(ctx, domain.Article{
		Id:       req.Id,
		Title:    req.Title,
		Abstract: req.Abstract,
		Content:  req.Content,
		Category: req.Category,
		Author:   domain.Author{Id: uc.Userid},
	})
	if err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Data: id}, nil
}

func (h *InternalArticleAuthorHandler) Publish(ctx *gin.Context, req editReq) (ginx.Result, error) {
	uc := ginx.MustClaims[UserClaims](ctx)
	if req.Title == "" || req.Content == "" {
		return ginx.Result{}, errs.ErrArticleEmptyTitleOrContent
	}
	_, err := h.svc.Publish(ctx, domain.Article{
		Id:       req.Id,
		Title:    req.Title,
		Abstract: req.Abstract,
		Content:  req.Content,
		Category: req.Category,
		Author:   domain.Author{Id: uc.Userid},
		Tags:     req.Tags,
	})
	if err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Msg: "OK"}, nil
}

func (h *InternalArticleAuthorHandler) Withdraw(ctx *gin.Context, req idReq) (ginx.Result, error) {
	uc := ginx.MustClaims[UserClaims](ctx)
	if err := h.svc.Withdraw(ctx, req.Id, uc.Userid); err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Msg: "OK"}, nil
}

func (h *InternalArticleAuthorHandler) Detail(ctx *gin.Context, req idReq) (ginx.Result, error) {
	uc := ginx.MustClaims[UserClaims](ctx)
	var article domain.Article
	var intr domain.Interaction
	var eg errgroup.Group
	eg.Go(func() error {
		var e error
		article, e = h.svc.Detail(ctx, req.Id, uc.Userid)
		return e
	})
	eg.Go(func() error {
		var e error
		intr, e = h.intrSvc.FindInteraction(ctx, 0, domain.BizArticle, req.Id)
		if e != nil {
			h.l.WithContext(ctx).Error("获取文章互动数据失败",
				logger.Int64("article_id", req.Id), logger.Error(e))
		}
		return nil
	})
	if err := eg.Wait(); err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Data: AuthorDetailVO{
		Id:        article.Id,
		Title:     article.Title,
		Content:   article.Content,
		Abstract:  article.Abstract,
		Status:    article.Status.ToUint8(),
		Category:  article.Category,
		ReadCnt:   intr.ReadCount,
		Tags:      article.Tags,
		CreatedAt: article.CreatedAt,
		UpdatedAt: article.UpdatedAt,
	}}, nil
}

// ArticleVO 列表接口返回的简化文章结构
type ArticleVO struct {
	Id        int64  `json:"id"`
	Title     string `json:"title"`
	Status    uint8  `json:"status"`
	ReadCnt   int64  `json:"readCnt"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

// AuthorDetailVO 作者视角文章详情
type AuthorDetailVO struct {
	Id        int64    `json:"id"`
	Title     string   `json:"title"`
	Content   string   `json:"content"`
	Abstract  string   `json:"abstract"`
	Status    uint8    `json:"status"`
	Category  string   `json:"category"` // 分区回显
	ReadCnt   int64    `json:"readCnt"`
	Tags      []string `json:"tags"` // 当前标签名，供编辑器回显
	CreatedAt int64    `json:"createdAt"`
	UpdatedAt int64    `json:"updatedAt"`
}

// ReaderDetailVO 读者视角文章详情
type ReaderDetailVO struct {
	Id        int64   `json:"id"`
	Title     string  `json:"title"`
	Content   string  `json:"content"`
	Abstract  string  `json:"abstract"`
	AuthorId  int64   `json:"authorId"`
	ReadCnt   int64   `json:"readCnt"`
	Tags      []tagVO `json:"tags"` // 该文标签（阅读页展示，chip 链 /tag/:slug）
	CreatedAt int64   `json:"createdAt"`
	UpdatedAt int64   `json:"updatedAt"`
}

func (h *InternalArticleAuthorHandler) Page(ctx *gin.Context, req pageReq) (ginx.Result, error) {
	uc := ginx.MustClaims[UserClaims](ctx)
	articles, total, err := h.svc.Page(ctx, uc.Userid, req.Page, req.PageSize)
	if err != nil {
		return ginx.Result{}, err
	}
	ids := make([]int64, 0, len(articles))
	for _, a := range articles {
		ids = append(ids, a.Id)
	}
	intrMap, intrErr := h.intrSvc.FindByBizIds(ctx, domain.BizArticle, ids)
	if intrErr != nil {
		h.l.WithContext(ctx).Error("批量获取文章互动数据失败", logger.Error(intrErr))
		intrMap = map[int64]domain.Interaction{}
	}
	list := make([]ArticleVO, 0, len(articles))
	for _, a := range articles {
		list = append(list, ArticleVO{
			Id:        a.Id,
			Title:     a.Title,
			Status:    a.Status.ToUint8(),
			ReadCnt:   intrMap[a.Id].ReadCount,
			CreatedAt: a.CreatedAt,
			UpdatedAt: a.UpdatedAt,
		})
	}
	return ginx.Result{Data: ginx.PageResult{List: list, Total: total}}, nil
}

func (h *InternalArticleAuthorHandler) List(ctx *gin.Context) (ginx.Result, error) {
	uc := ginx.MustClaims[UserClaims](ctx)
	articles, err := h.svc.List(ctx, uc.Userid)
	if err != nil {
		return ginx.Result{}, err
	}
	list := make([]ArticleVO, 0, len(articles))
	for _, a := range articles {
		list = append(list, ArticleVO{
			Id:        a.Id,
			Title:     a.Title,
			Status:    a.Status.ToUint8(),
			CreatedAt: a.CreatedAt,
			UpdatedAt: a.UpdatedAt,
		})
	}
	return ginx.Result{Data: list}, nil
}

func (h *InternalArticleAuthorHandler) Delete(ctx *gin.Context, req idReq) (ginx.Result, error) {
	uc := ginx.MustClaims[UserClaims](ctx)
	if err := h.svc.Delete(ctx, req.Id, uc.Userid); err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Msg: "OK"}, nil
}

// ── 读者端（公开，无需登录） ────────────────────────────────────────────────

type ArticleReaderHandler interface {
	RegisterRoutes(server *gin.Engine)
}

type InternalArticleReaderHandler struct {
	svc     service.ArticleReaderService
	intrSvc service.InteractionService
	tagSvc  service.TagService
	l       logger.LoggerX
}

func NewInternalArticleReaderHandler(svc service.ArticleReaderService, intrSvc service.InteractionService, tagSvc service.TagService, l logger.LoggerX) ArticleReaderHandler {
	return &InternalArticleReaderHandler{svc: svc, intrSvc: intrSvc, tagSvc: tagSvc, l: l}
}

func (h *InternalArticleReaderHandler) RegisterRoutes(server *gin.Engine) {
	g := server.Group("/article/reader")
	g.POST("/detail", ginx.WrapReq[idReq](h.Detail))
	g.POST("/page", ginx.WrapReq[pageReq](h.Page))
	g.POST("/author", ginx.WrapReq[authorArticlesReq](h.Author)) // 他人主页「TA 的文章」，公开可读
}

// authorArticlesReq 他人主页「TA 的文章」分页请求
type authorArticlesReq struct {
	AuthorId int64 `json:"authorId" binding:"required,gt=0"`
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
}

// ReaderArticleVO 读者视角的文章简要信息
type ReaderArticleVO struct {
	Id         int64  `json:"id"`
	Title      string `json:"title"`
	Abstract   string `json:"abstract"`
	AuthorId   int64  `json:"authorId"`
	ReadCnt    int64  `json:"readCnt"`
	LikeCnt    int64  `json:"likeCnt"`
	CommentCnt int64  `json:"commentCnt"`
	CreatedAt  int64  `json:"createdAt"`
	UpdatedAt  int64  `json:"updatedAt"`
}

func (h *InternalArticleReaderHandler) Detail(ctx *gin.Context, req idReq) (ginx.Result, error) {
	var article domain.Article
	var intr domain.Interaction
	var tags []domain.Tag
	var eg errgroup.Group
	eg.Go(func() error {
		var e error
		article, e = h.svc.Detail(ctx, req.Id)
		return e
	})
	eg.Go(func() error {
		var e error
		intr, e = h.intrSvc.FindInteraction(ctx, 0, domain.BizArticle, req.Id)
		if e != nil {
			h.l.WithContext(ctx).Error("获取文章互动数据失败",
				logger.Int64("article_id", req.Id), logger.Error(e))
		}
		return nil
	})
	eg.Go(func() error {
		// 回显：补该文标签（阅读页 chip 链 /tag/:slug）；失败降级不带标签，不阻断详情。
		tagMap, e := h.tagSvc.TagsByBiz(ctx, domain.BizArticle, []int64{req.Id})
		if e != nil {
			h.l.WithContext(ctx).Error("阅读页：取标签失败，降级不带标签",
				logger.Int64("article_id", req.Id), logger.Error(e))
			return nil
		}
		tags = tagMap[req.Id]
		return nil
	})
	if err := eg.Wait(); err != nil {
		// errgroup 任一返 err 都视为「文章不存在」（reader 端无权限/无 detail 都按 NotFound 暴露）
		return ginx.Result{}, errs.ErrArticleNotFound.WithCause(err)
	}
	return ginx.Result{Data: ReaderDetailVO{
		Id:        article.Id,
		Title:     article.Title,
		Content:   article.Content,
		Abstract:  article.DisplayAbstract(),
		AuthorId:  article.Author.Id,
		ReadCnt:   intr.ReadCount,
		Tags:      slicex.Map(tags, toTagVO),
		CreatedAt: article.CreatedAt,
		UpdatedAt: article.UpdatedAt,
	}}, nil
}

// Author 他人主页「TA 的文章」：某作者已发布文章分页 + 获赞总数聚合。公开可读。
func (h *InternalArticleReaderHandler) Author(ctx *gin.Context, req authorArticlesReq) (ginx.Result, error) {
	items, total, likedTotal, err := h.svc.AuthorArticles(ctx, req.AuthorId, req.Page, req.PageSize)
	if err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Data: gin.H{
		"list":       slicex.Map(items, toReaderArticleVO),
		"total":      total,
		"likedTotal": likedTotal,
	}}, nil
}

// toReaderArticleVO 领域(含聚合计数) → 读者文章 VO（纯映射；摘要缺省从正文截取）。
func toReaderArticleVO(a domain.ArticleWithStats) ReaderArticleVO {
	return ReaderArticleVO{
		Id:         a.Id,
		Title:      a.Title,
		Abstract:   a.DisplayAbstract(),
		AuthorId:   a.Author.Id,
		ReadCnt:    a.ReadCnt,
		LikeCnt:    a.LikeCnt,
		CommentCnt: a.CommentCnt,
		CreatedAt:  a.CreatedAt,
		UpdatedAt:  a.UpdatedAt,
	}
}

func (h *InternalArticleReaderHandler) Page(ctx *gin.Context, req pageReq) (ginx.Result, error) {
	articles, total, err := h.svc.Page(ctx, req.Page, req.PageSize)
	if err != nil {
		return ginx.Result{}, err
	}
	ids := make([]int64, 0, len(articles))
	for _, a := range articles {
		ids = append(ids, a.Id)
	}
	intrMap, intrErr := h.intrSvc.FindByBizIds(ctx, domain.BizArticle, ids)
	if intrErr != nil {
		h.l.WithContext(ctx).Error("批量获取文章互动数据失败", logger.Error(intrErr))
		intrMap = map[int64]domain.Interaction{}
	}
	list := make([]ReaderArticleVO, 0, len(articles))
	for _, a := range articles {
		list = append(list, ReaderArticleVO{
			Id:        a.Id,
			Title:     a.Title,
			Abstract:  a.DisplayAbstract(),
			AuthorId:  a.Author.Id,
			ReadCnt:   intrMap[a.Id].ReadCount,
			CreatedAt: a.CreatedAt,
			UpdatedAt: a.UpdatedAt,
		})
	}
	return ginx.Result{Data: ginx.PageResult{List: list, Total: total}}, nil
}
