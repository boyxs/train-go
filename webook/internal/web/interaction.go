package web

import (
	"github.com/gin-gonic/gin"

	"github.com/webook/internal/consts"
	"github.com/webook/internal/domain"
	"github.com/webook/internal/service"
	"github.com/webook/pkg/ginx"
	"github.com/webook/pkg/logger"
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
	g.POST("/like", ginx.WrapReqClaims[likeReq, UserClaims](consts.UserKey, h.Like))
	g.POST("/collect", ginx.WrapReqClaims[collectReq, UserClaims](consts.UserKey, h.Collect))
	g.POST("/detail", ginx.WrapReq[bizIdReq](h.Detail))
	g.POST("/state", ginx.WrapReqClaims[bizIdReq, UserClaims](consts.UserKey, h.State))
	g.POST("/view", ginx.WrapReq[bizIdReq](h.View))
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

func (h *InternalInteractionHandler) Like(ctx *gin.Context, req likeReq, uc UserClaims) (ginx.Result, error) {
	var err error
	if req.Liked {
		err = h.svc.Like(ctx, uc.Userid, h.biz, req.ArticleId)
	} else {
		err = h.svc.CancelLike(ctx, uc.Userid, h.biz, req.ArticleId)
	}
	if err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Msg: "OK"}, nil
}

func (h *InternalInteractionHandler) Collect(ctx *gin.Context, req collectReq, uc UserClaims) (ginx.Result, error) {
	var err error
	if req.Collected {
		err = h.svc.Collect(ctx, uc.Userid, h.biz, req.ArticleId)
	} else {
		err = h.svc.CancelCollect(ctx, uc.Userid, h.biz, req.ArticleId)
	}
	if err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Msg: "OK"}, nil
}

// Detail 获取互动聚合计数（公开接口，不含用户个人状态）
func (h *InternalInteractionHandler) Detail(ctx *gin.Context, req bizIdReq) (ginx.Result, error) {
	intr, err := h.svc.FindInteraction(ctx, 0, h.biz, req.ArticleId)
	if err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Data: intr}, nil
}

// State 获取当前用户的互动状态（liked/collected），需登录
func (h *InternalInteractionHandler) State(ctx *gin.Context, req bizIdReq, uc UserClaims) (ginx.Result, error) {
	liked, collected, err := h.svc.FindUserState(ctx, uc.Userid, h.biz, req.ArticleId)
	if err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Data: gin.H{
		"liked":     liked,
		"collected": collected,
	}}, nil
}

func (h *InternalInteractionHandler) View(ctx *gin.Context, req bizIdReq) (ginx.Result, error) {
	if err := h.svc.IncrReadCount(ctx, h.biz, req.ArticleId); err != nil {
		// View 失败不影响主流程，wrapper 会记日志
		return ginx.Result{Msg: "OK"}, err
	}
	return ginx.Result{Msg: "OK"}, nil
}
