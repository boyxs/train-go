package web

import (
	"github.com/webook/internal/consts"
	"github.com/webook/internal/domain"
	"github.com/webook/internal/service"
	"github.com/webook/pkg/ginx"
	"github.com/webook/pkg/logger"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
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
	Id       int64  `json:"id"`
	Title    string `json:"title"`
	Abstract string `json:"abstract"`
	Content  string `json:"content"`
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
	g.POST("/edit", ginx.WrapReqClaims[editReq, UserClaims](consts.UserKey, h.Edit))
	g.POST("/publish", ginx.WrapReqClaims[editReq, UserClaims](consts.UserKey, h.Publish))
	g.POST("/withdraw", ginx.WrapReqClaims[idReq, UserClaims](consts.UserKey, h.Withdraw))
	g.POST("/detail", ginx.WrapReqClaims[idReq, UserClaims](consts.UserKey, h.Detail))
	g.POST("/page", ginx.WrapReqClaims[pageReq, UserClaims](consts.UserKey, h.Page))
	g.POST("/list", ginx.WrapClaims[UserClaims](consts.UserKey, h.List))
	g.POST("/delete", ginx.WrapReqClaims[idReq, UserClaims](consts.UserKey, h.Delete))
}

func (h *InternalArticleAuthorHandler) Edit(ctx *gin.Context, req editReq, uc UserClaims) (ginx.Result, error) {
	if req.Title == "" || req.Content == "" {
		return ginx.Result{Code: 4, Msg: "标题和内容不能为空"}, nil
	}
	id, err := h.svc.Edit(ctx, domain.Article{
		Id:       req.Id,
		Title:    req.Title,
		Abstract: req.Abstract,
		Content:  req.Content,
		Author:   domain.Author{Id: uc.Userid},
	})
	if err != nil {
		return ginx.Result{Msg: "系统错误"}, err
	}
	return ginx.Result{Data: id}, nil
}

func (h *InternalArticleAuthorHandler) Publish(ctx *gin.Context, req editReq, uc UserClaims) (ginx.Result, error) {
	if req.Title == "" || req.Content == "" {
		return ginx.Result{Code: 4, Msg: "标题和内容不能为空"}, nil
	}
	_, err := h.svc.Publish(ctx, domain.Article{
		Id:       req.Id,
		Title:    req.Title,
		Abstract: req.Abstract,
		Content:  req.Content,
		Author:   domain.Author{Id: uc.Userid},
	})
	if err != nil {
		return ginx.Result{Msg: "系统错误"}, err
	}
	return ginx.Result{Msg: "OK"}, nil
}

func (h *InternalArticleAuthorHandler) Withdraw(ctx *gin.Context, req idReq, uc UserClaims) (ginx.Result, error) {
	if err := h.svc.Withdraw(ctx, req.Id, uc.Userid); err != nil {
		return ginx.Result{Msg: "系统错误"}, err
	}
	return ginx.Result{Msg: "OK"}, nil
}

func (h *InternalArticleAuthorHandler) Detail(ctx *gin.Context, req idReq, uc UserClaims) (ginx.Result, error) {
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
			h.l.Error("获取文章互动数据失败",
				logger.Int64("article_id", req.Id), logger.Error(e))
		}
		return nil
	})
	if err := eg.Wait(); err != nil {
		return ginx.Result{Msg: "系统错误"}, err
	}
	return ginx.Result{Data: AuthorDetailVO{
		Id:        article.Id,
		Title:     article.Title,
		Content:   article.Content,
		Abstract:  article.Abstract,
		Status:    article.Status.ToUint8(),
		ReadCnt:   intr.ReadCount,
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
	Id        int64  `json:"id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Abstract  string `json:"abstract"`
	Status    uint8  `json:"status"`
	ReadCnt   int64  `json:"readCnt"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

// ReaderDetailVO 读者视角文章详情
type ReaderDetailVO struct {
	Id        int64  `json:"id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Abstract  string `json:"abstract"`
	AuthorId  int64  `json:"authorId"`
	ReadCnt   int64  `json:"readCnt"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

func (h *InternalArticleAuthorHandler) Page(ctx *gin.Context, req pageReq, uc UserClaims) (ginx.Result, error) {
	articles, total, err := h.svc.Page(ctx, uc.Userid, req.Page, req.PageSize)
	if err != nil {
		return ginx.Result{Msg: "系统错误"}, err
	}
	ids := make([]int64, 0, len(articles))
	for _, a := range articles {
		ids = append(ids, a.Id)
	}
	intrMap, intrErr := h.intrSvc.FindByBizIds(ctx, domain.BizArticle, ids)
	if intrErr != nil {
		h.l.Error("批量获取文章互动数据失败", logger.Error(intrErr))
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
	return ginx.Result{Data: gin.H{"list": list, "total": total}}, nil
}

func (h *InternalArticleAuthorHandler) List(ctx *gin.Context, uc UserClaims) (ginx.Result, error) {
	articles, err := h.svc.List(ctx, uc.Userid)
	if err != nil {
		return ginx.Result{Msg: "系统错误"}, err
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

func (h *InternalArticleAuthorHandler) Delete(ctx *gin.Context, req idReq, uc UserClaims) (ginx.Result, error) {
	if err := h.svc.Delete(ctx, req.Id, uc.Userid); err != nil {
		return ginx.Result{Msg: "系统错误"}, err
	}
	return ginx.Result{Msg: "OK"}, nil
}

// ===== 读者端（公开，无需登录） =====

type ArticleReaderHandler interface {
	RegisterRoutes(server *gin.Engine)
}

type InternalArticleReaderHandler struct {
	svc     service.ArticleReaderService
	intrSvc service.InteractionService
	l       logger.LoggerX
}

func NewInternalArticleReaderHandler(svc service.ArticleReaderService, intrSvc service.InteractionService, l logger.LoggerX) ArticleReaderHandler {
	return &InternalArticleReaderHandler{svc: svc, intrSvc: intrSvc, l: l}
}

func (h *InternalArticleReaderHandler) RegisterRoutes(server *gin.Engine) {
	g := server.Group("/article/reader")
	g.POST("/detail", ginx.WrapReq[idReq](h.Detail))
	g.POST("/page", ginx.WrapReq[pageReq](h.Page))
}

// ReaderArticleVO 读者视角的文章简要信息
type ReaderArticleVO struct {
	Id        int64  `json:"id"`
	Title     string `json:"title"`
	Abstract  string `json:"abstract"`
	AuthorId  int64  `json:"authorId"`
	ReadCnt   int64  `json:"readCnt"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

func abstractFromContent(content string, maxLen int) string {
	r := []rune(content)
	if len(r) <= maxLen {
		return content
	}
	return string(r[:maxLen]) + "..."
}

func (h *InternalArticleReaderHandler) Detail(ctx *gin.Context, req idReq) (ginx.Result, error) {
	var article domain.Article
	var intr domain.Interaction
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
			h.l.Error("获取文章互动数据失败",
				logger.Int64("article_id", req.Id), logger.Error(e))
		}
		return nil
	})
	if err := eg.Wait(); err != nil {
		return ginx.Result{Msg: "文章不存在"}, err
	}
	abstract := article.Abstract
	if abstract == "" {
		abstract = abstractFromContent(article.Content, 128)
	}
	return ginx.Result{Data: ReaderDetailVO{
		Id:        article.Id,
		Title:     article.Title,
		Content:   article.Content,
		Abstract:  abstract,
		AuthorId:  article.Author.Id,
		ReadCnt:   intr.ReadCount,
		CreatedAt: article.CreatedAt,
		UpdatedAt: article.UpdatedAt,
	}}, nil
}

func (h *InternalArticleReaderHandler) Page(ctx *gin.Context, req pageReq) (ginx.Result, error) {
	articles, total, err := h.svc.Page(ctx, req.Page, req.PageSize)
	if err != nil {
		return ginx.Result{Msg: "系统错误"}, err
	}
	ids := make([]int64, 0, len(articles))
	for _, a := range articles {
		ids = append(ids, a.Id)
	}
	intrMap, intrErr := h.intrSvc.FindByBizIds(ctx, domain.BizArticle, ids)
	if intrErr != nil {
		h.l.Error("批量获取文章互动数据失败", logger.Error(intrErr))
		intrMap = map[int64]domain.Interaction{}
	}
	list := make([]ReaderArticleVO, 0, len(articles))
	for _, a := range articles {
		abstract := a.Abstract
		if abstract == "" {
			abstract = abstractFromContent(a.Content, 128)
		}
		list = append(list, ReaderArticleVO{
			Id:        a.Id,
			Title:     a.Title,
			Abstract:  abstract,
			AuthorId:  a.Author.Id,
			ReadCnt:   intrMap[a.Id].ReadCount,
			CreatedAt: a.CreatedAt,
			UpdatedAt: a.UpdatedAt,
		})
	}
	return ginx.Result{Data: gin.H{"list": list, "total": total}}, nil
}
