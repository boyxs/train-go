package web

import (
	"github.com/gin-gonic/gin"

	"github.com/boyxs/train-go/webook/internal/domain"
	"github.com/boyxs/train-go/webook/internal/service"
	"github.com/boyxs/train-go/webook/pkg/errs"
	"github.com/boyxs/train-go/webook/pkg/ginx"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

type InteractionHandler interface {
	RegisterRoutes(server *gin.Engine)
}

type InternalInteractionHandler struct {
	svc service.InteractionService
	l   logger.LoggerX
}

func NewInternalInteractionHandler(svc service.InteractionService, l logger.LoggerX) InteractionHandler {
	return &InternalInteractionHandler{svc: svc, l: l}
}

func (h *InternalInteractionHandler) RegisterRoutes(server *gin.Engine) {
	g := server.Group("/interaction")
	g.POST("/like", ginx.WrapReq[likeReq](h.Like))
	g.POST("/collect", ginx.WrapReq[collectReq](h.Collect))
	g.POST("/detail", ginx.WrapReq[bizReq](h.Detail))
	g.POST("/state", ginx.WrapReq[bizReq](h.State))
	g.POST("/view", ginx.WrapReq[bizReq](h.View))
}

// allowedBiz 限定可互动的业务类型，挡住任意 biz 污染 interaction 数据。
// 新增可互动实体在此登记（与 domain.Biz* 对齐）。interaction 拆独立服务后这份白名单随之迁出。
var allowedBiz = map[string]struct{}{
	domain.BizArticle: {},
	domain.BizComment: {},
}

// ErrInvalidBiz biz 不在白名单。
var ErrInvalidBiz = errs.New(400, "biz 不合法").WithReason("INTERACTION_BIZ_INVALID")

func checkBiz(biz string) error {
	if _, ok := allowedBiz[biz]; !ok {
		return ErrInvalidBiz
	}
	return nil
}

// bizReq 通用互动目标：biz 业务类型 + bizId 业务内主键（article→articleId、comment→commentId）。
// 基础字段校验走 binding tag（WrapReq 绑定时自动 400）；biz 白名单是业务规则，仍由 handler checkBiz 管。
type bizReq struct {
	Biz   string `json:"biz" binding:"required"`
	BizId int64  `json:"bizId" binding:"required,gt=0"`
}

type likeReq struct {
	Biz   string `json:"biz" binding:"required"`
	BizId int64  `json:"bizId" binding:"required,gt=0"`
	Liked bool   `json:"liked"`
}

type collectReq struct {
	Biz       string `json:"biz" binding:"required"`
	BizId     int64  `json:"bizId" binding:"required,gt=0"`
	Collected bool   `json:"collected"`
}

func (h *InternalInteractionHandler) Like(ctx *gin.Context, req likeReq) (ginx.Result, error) {
	uc := ginx.MustClaims[UserClaims](ctx)
	if err := checkBiz(req.Biz); err != nil {
		return ginx.Result{}, err
	}
	var err error
	if req.Liked {
		err = h.svc.Like(ctx, uc.Userid, req.Biz, req.BizId)
	} else {
		err = h.svc.CancelLike(ctx, uc.Userid, req.Biz, req.BizId)
	}
	if err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Msg: "OK"}, nil
}

func (h *InternalInteractionHandler) Collect(ctx *gin.Context, req collectReq) (ginx.Result, error) {
	uc := ginx.MustClaims[UserClaims](ctx)
	if err := checkBiz(req.Biz); err != nil {
		return ginx.Result{}, err
	}
	var err error
	if req.Collected {
		err = h.svc.Collect(ctx, uc.Userid, req.Biz, req.BizId)
	} else {
		err = h.svc.CancelCollect(ctx, uc.Userid, req.Biz, req.BizId)
	}
	if err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Msg: "OK"}, nil
}

// Detail 获取互动聚合计数（公开接口，不含用户个人状态）
func (h *InternalInteractionHandler) Detail(ctx *gin.Context, req bizReq) (ginx.Result, error) {
	if err := checkBiz(req.Biz); err != nil {
		return ginx.Result{}, err
	}
	intr, err := h.svc.FindInteraction(ctx, 0, req.Biz, req.BizId)
	if err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Data: intr}, nil
}

// State 获取当前用户的互动状态（liked/collected），需登录
func (h *InternalInteractionHandler) State(ctx *gin.Context, req bizReq) (ginx.Result, error) {
	uc := ginx.MustClaims[UserClaims](ctx)
	if err := checkBiz(req.Biz); err != nil {
		return ginx.Result{}, err
	}
	liked, collected, err := h.svc.FindUserState(ctx, uc.Userid, req.Biz, req.BizId)
	if err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Data: gin.H{
		"liked":     liked,
		"collected": collected,
	}}, nil
}

func (h *InternalInteractionHandler) View(ctx *gin.Context, req bizReq) (ginx.Result, error) {
	if err := checkBiz(req.Biz); err != nil {
		return ginx.Result{}, err
	}
	if err := h.svc.IncrReadCount(ctx, req.Biz, req.BizId); err != nil {
		// View 失败不影响主流程，wrapper 会记日志
		return ginx.Result{Msg: "OK"}, err
	}
	return ginx.Result{Msg: "OK"}, nil
}
