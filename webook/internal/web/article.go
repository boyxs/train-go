package web

import (
	"net/http"

	"gitee.com/train-cloud/geektime-basic-go/internal/consts"
	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"gitee.com/train-cloud/geektime-basic-go/internal/service"
	"gitee.com/train-cloud/geektime-basic-go/pkg/logger"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
)

type ArticleAuthorHandler interface {
	RegisterRoutes(server *gin.Engine)
	Edit(ctx *gin.Context)
	Publish(ctx *gin.Context)
	Withdraw(ctx *gin.Context)
	Detail(ctx *gin.Context)
	Page(ctx *gin.Context)
	List(ctx *gin.Context)
	Delete(ctx *gin.Context)
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

func (h *InternalArticleAuthorHandler) RegisterRoutes(server *gin.Engine) {
	g := server.Group("/article")
	g.POST("/edit", h.Edit)
	g.POST("/publish", h.Publish)
	g.POST("/withdraw", h.Withdraw)
	g.POST("/detail", h.Detail)
	g.POST("/page", h.Page)
	g.POST("/list", h.List)
	g.POST("/delete", h.Delete)
}

func (h *InternalArticleAuthorHandler) Edit(ctx *gin.Context) {
	type EditRequest struct {
		Id       int64  `json:"id"`
		Title    string `json:"title"`
		Abstract string `json:"abstract"`
		Content  string `json:"content"`
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
		Id:       req.Id,
		Title:    req.Title,
		Abstract: req.Abstract,
		Content:  req.Content,
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

func (h *InternalArticleAuthorHandler) Publish(ctx *gin.Context) {
	type PublishRequest struct {
		Id       int64  `json:"id"`
		Title    string `json:"title"`
		Abstract string `json:"abstract"`
		Content  string `json:"content"`
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
		Id:       req.Id,
		Title:    req.Title,
		Abstract: req.Abstract,
		Content:  req.Content,
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

func (h *InternalArticleAuthorHandler) Withdraw(ctx *gin.Context) {
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

func (h *InternalArticleAuthorHandler) Detail(ctx *gin.Context) {
	type DetailRequest struct {
		Id int64 `json:"id"`
	}
	var req DetailRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
	uc := ctx.MustGet(consts.UserKey).(UserClaims)
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
		return nil // 互动查询失败不阻塞
	})
	if err := eg.Wait(); err != nil {
		ctx.JSON(http.StatusOK, Result{Msg: "系统错误"})
		h.l.Error("获取文章详情失败",
			logger.Int64("userid", uc.Userid),
			logger.Int64("article_id", req.Id),
			logger.Error(err))
		return
	}
	ctx.JSON(http.StatusOK, Result{
		Data: AuthorDetailVO{
			Id:        article.Id,
			Title:     article.Title,
			Content:   article.Content,
			Abstract:  article.Abstract,
			Status:    article.Status.ToUint8(),
			ReadCnt:   intr.ReadCount,
			CreatedAt: article.CreatedAt,
			UpdatedAt: article.UpdatedAt,
		},
	})
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

func (h *InternalArticleAuthorHandler) Page(ctx *gin.Context) {
	type PageRequest struct {
		Page     int `json:"page"`
		PageSize int `json:"pageSize"`
	}
	var req PageRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
	uc := ctx.MustGet(consts.UserKey).(UserClaims)
	articles, total, err := h.svc.Page(ctx, uc.Userid, req.Page, req.PageSize)
	if err != nil {
		ctx.JSON(http.StatusOK, Result{
			Msg: "系统错误",
		})
		h.l.Error("分页获取文章失败",
			logger.Int64("userid", uc.Userid),
			logger.Error(err))
		return
	}
	// 批量查阅读量
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
	ctx.JSON(http.StatusOK, Result{
		Data: gin.H{
			"list":  list,
			"total": total,
		},
	})
}

func (h *InternalArticleAuthorHandler) List(ctx *gin.Context) {
	uc := ctx.MustGet(consts.UserKey).(UserClaims)
	articles, err := h.svc.List(ctx, uc.Userid)
	if err != nil {
		ctx.JSON(http.StatusOK, Result{
			Msg: "系统错误",
		})
		h.l.Error("获取全部文章失败",
			logger.Int64("userid", uc.Userid),
			logger.Error(err))
		return
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
	ctx.JSON(http.StatusOK, Result{
		Data: list,
	})
}

func (h *InternalArticleAuthorHandler) Delete(ctx *gin.Context) {
	type DeleteRequest struct {
		Id int64 `json:"id"`
	}
	var req DeleteRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
	uc := ctx.MustGet(consts.UserKey).(UserClaims)
	err := h.svc.Delete(ctx, req.Id, uc.Userid)
	if err != nil {
		ctx.JSON(http.StatusOK, Result{
			Msg: "系统错误",
		})
		h.l.Error("删除文章失败",
			logger.Int64("userid", uc.Userid),
			logger.Int64("article_id", req.Id),
			logger.Error(err))
		return
	}
	ctx.JSON(http.StatusOK, Result{
		Msg: "OK",
	})
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
	g.POST("/detail", h.Detail)
	g.POST("/page", h.Page)
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

func (h *InternalArticleReaderHandler) Detail(ctx *gin.Context) {
	type DetailRequest struct {
		Id int64 `json:"id"`
	}
	var req DetailRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
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
		return nil // 互动查询失败不阻塞
	})
	if err := eg.Wait(); err != nil {
		ctx.JSON(http.StatusOK, Result{Msg: "文章不存在"})
		h.l.Error("获取公开文章详情失败",
			logger.Int64("article_id", req.Id),
			logger.Error(err))
		return
	}
	abstract := article.Abstract
	if abstract == "" {
		abstract = abstractFromContent(article.Content, 128)
	}
	ctx.JSON(http.StatusOK, Result{Data: ReaderDetailVO{
		Id:        article.Id,
		Title:     article.Title,
		Content:   article.Content,
		Abstract:  abstract,
		AuthorId:  article.Author.Id,
		ReadCnt:   intr.ReadCount,
		CreatedAt: article.CreatedAt,
		UpdatedAt: article.UpdatedAt,
	}})
}

func (h *InternalArticleReaderHandler) Page(ctx *gin.Context) {
	type PageRequest struct {
		Page     int `json:"page"`
		PageSize int `json:"pageSize"`
	}
	var req PageRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
	articles, total, err := h.svc.Page(ctx, req.Page, req.PageSize)
	if err != nil {
		ctx.JSON(http.StatusOK, Result{Msg: "系统错误"})
		h.l.Error("获取公开文章列表失败", logger.Error(err))
		return
	}
	// 批量查阅读量
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
	ctx.JSON(http.StatusOK, Result{
		Data: gin.H{"list": list, "total": total},
	})
}
