package web

import (
	"net/http"

	"gitee.com/train-cloud/geektime-basic-go/internal/consts"
	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"gitee.com/train-cloud/geektime-basic-go/internal/service"
	"gitee.com/train-cloud/geektime-basic-go/pkg/logger"
	"github.com/gin-gonic/gin"
)

type InteractionHandler interface {
	RegisterRoutes(server *gin.Engine)
}

type InternalInteractionHandler struct {
	svc service.InteractionService
	l   logger.LoggerX
	biz string
}

func NewInternalInteractionHandler(svc service.InteractionService, l logger.LoggerX) InteractionHandler {
	return &InternalInteractionHandler{svc: svc, l: l, biz: domain.BizArticle}
}

func (h *InternalInteractionHandler) RegisterRoutes(server *gin.Engine) {
	g := server.Group("/interaction")
	g.POST("/like", h.Like)
	g.POST("/collect", h.Collect)
	g.POST("/detail", h.Detail)
	g.POST("/state", h.State)
	g.POST("/view", h.View)
}

type bizIdReq struct {
	ArticleId int64 `json:"articleId"` // 前端传 articleId，handler 内部映射为 bizId
}

type likeReq struct {
	ArticleId int64 `json:"articleId"`
	Liked     bool  `json:"liked"`
}

type collectReq struct {
	ArticleId int64 `json:"articleId"`
	Collected bool  `json:"collected"`
}

func (h *InternalInteractionHandler) Like(ctx *gin.Context) {
	var req likeReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
	uc := ctx.MustGet(consts.UserKey).(UserClaims)
	var err error
	if req.Liked {
		err = h.svc.Like(ctx, uc.Userid, h.biz, req.ArticleId)
	} else {
		err = h.svc.CancelLike(ctx, uc.Userid, h.biz, req.ArticleId)
	}
	if err != nil {
		ctx.JSON(http.StatusOK, Result{Code: 5, Msg: "系统错误"})
		h.l.Error("点赞操作失败",
			logger.Int64("uid", uc.Userid),
			logger.Int64("bizId", req.ArticleId),
			logger.Error(err))
		return
	}
	ctx.JSON(http.StatusOK, Result{Msg: "OK"})
}

func (h *InternalInteractionHandler) Collect(ctx *gin.Context) {
	var req collectReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
	uc := ctx.MustGet(consts.UserKey).(UserClaims)
	var err error
	if req.Collected {
		err = h.svc.Collect(ctx, uc.Userid, h.biz, req.ArticleId)
	} else {
		err = h.svc.CancelCollect(ctx, uc.Userid, h.biz, req.ArticleId)
	}
	if err != nil {
		ctx.JSON(http.StatusOK, Result{Code: 5, Msg: "系统错误"})
		h.l.Error("收藏操作失败",
			logger.Int64("uid", uc.Userid),
			logger.Int64("bizId", req.ArticleId),
			logger.Error(err))
		return
	}
	ctx.JSON(http.StatusOK, Result{Msg: "OK"})
}

// Detail 获取互动聚合计数（公开接口，不含用户个人状态）
func (h *InternalInteractionHandler) Detail(ctx *gin.Context) {
	var req bizIdReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
	intr, err := h.svc.FindInteraction(ctx, 0, h.biz, req.ArticleId)
	if err != nil {
		ctx.JSON(http.StatusOK, Result{Code: 5, Msg: "系统错误"})
		h.l.Error("获取互动数据失败",
			logger.Int64("bizId", req.ArticleId),
			logger.Error(err))
		return
	}
	ctx.JSON(http.StatusOK, Result{Data: intr})
}

// State 获取当前用户的互动状态（liked/collected），需登录
func (h *InternalInteractionHandler) State(ctx *gin.Context) {
	var req bizIdReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
	uc := ctx.MustGet(consts.UserKey).(UserClaims)
	liked, collected, err := h.svc.FindUserState(ctx, uc.Userid, h.biz, req.ArticleId)
	if err != nil {
		ctx.JSON(http.StatusOK, Result{Code: 5, Msg: "系统错误"})
		h.l.Error("获取用户互动状态失败",
			logger.Int64("uid", uc.Userid),
			logger.Int64("bizId", req.ArticleId),
			logger.Error(err))
		return
	}
	ctx.JSON(http.StatusOK, Result{Data: gin.H{
		"liked":     liked,
		"collected": collected,
	}})
}

func (h *InternalInteractionHandler) View(ctx *gin.Context) {
	var req bizIdReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if err := h.svc.IncrReadCount(ctx, h.biz, req.ArticleId); err != nil {
		h.l.Error("阅读量上报失败",
			logger.Int64("bizId", req.ArticleId),
			logger.Error(err))
	}
	ctx.JSON(http.StatusOK, Result{Msg: "OK"})
}
