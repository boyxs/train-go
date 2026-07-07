package web

import (
	"github.com/gin-gonic/gin"

	"github.com/webook/internal/domain"
	"github.com/webook/internal/service"
	"github.com/webook/pkg/ginx"
	"github.com/webook/pkg/slicex"
)

// RelationHandler 用户关系 HTTP 接入层：绑定请求 → 调 RelationService → domain→VO 映射。
// 聚合（relation gRPC + user 昵称 + 关系态 + 事件生产）全在 service.RelationService，本层不含业务逻辑。
type RelationHandler interface {
	RegisterRoutes(server *gin.Engine)
}

type InternalRelationHandler struct {
	svc service.RelationService
}

func NewInternalRelationHandler(svc service.RelationService) RelationHandler {
	return &InternalRelationHandler{svc: svc}
}

func (h *InternalRelationHandler) RegisterRoutes(server *gin.Engine) {
	g := server.Group("/relation")
	g.POST("/follow", ginx.WrapReq[relFolloweeReq](h.Follow))
	g.POST("/unfollow", ginx.WrapReq[relFolloweeReq](h.Unfollow))
	// followees/followers/stat 公开可读，登录态可选（OptionalPaths）→ 有 viewer 才填关系态
	g.POST("/followees", ginx.WrapReq[relListReq](h.Followees))
	g.POST("/followers", ginx.WrapReq[relListReq](h.Followers))
	g.POST("/stat", ginx.WrapReq[relStatReq](h.Stat))
	g.POST("/block", ginx.WrapReq[relTargetReq](h.Block))
	g.POST("/unblock", ginx.WrapReq[relTargetReq](h.Unblock))
	g.POST("/blocklist", ginx.WrapReq[relBlocklistReq](h.Blocklist))
}

type relFolloweeReq struct {
	FolloweeId int64 `json:"followeeId" binding:"required,gt=0"`
}
type relTargetReq struct {
	TargetId int64 `json:"targetId" binding:"required,gt=0"`
}
type relListReq struct {
	UserId int64 `json:"userId" binding:"required,gt=0"`
	Cursor int64 `json:"cursor"`
	Limit  int32 `json:"limit"`
}
type relStatReq struct {
	UserId int64 `json:"userId" binding:"required,gt=0"`
}
type relBlocklistReq struct {
	Cursor int64 `json:"cursor"`
	Limit  int32 `json:"limit"`
}

// userBriefVO 关系列表/主页的用户简介；头像前端按 name 首字母渲染（user 表无头像字段）。
type userBriefVO struct {
	Id   int64  `json:"id"`
	Name string `json:"name"`
	Bio  string `json:"bio"`
}
type followeeItemVO struct {
	userBriefVO
	IsMutual    bool  `json:"isMutual"`    // 该关注对象是否也关注了列表主人（互相关注）
	FolloweeCnt int64 `json:"followeeCnt"` // 该用户关注数
	FollowerCnt int64 `json:"followerCnt"` // 该用户粉丝数
}
type followerItemVO struct {
	userBriefVO
	IsFollowedBack bool  `json:"isFollowedBack"` // 列表主人是否已回关该粉丝
	FolloweeCnt    int64 `json:"followeeCnt"`    // 该用户关注数
	FollowerCnt    int64 `json:"followerCnt"`    // 该用户粉丝数
	CreatedAt      int64 `json:"createdAt"`      // 该粉丝关注列表主人的时间（关注了你 · X）
}
type blockItemVO struct {
	userBriefVO
	BlockedAt int64 `json:"blockedAt"`
}
type relationStatVO struct {
	FolloweeCnt int64 `json:"followeeCnt"`
	FollowerCnt int64 `json:"followerCnt"`
	IsFollowing bool  `json:"isFollowing"`
	IsMutual    bool  `json:"isMutual"`
	IsBlocked   bool  `json:"isBlocked"`   // viewer 拉黑了对方
	IsBlockedBy bool  `json:"isBlockedBy"` // 对方拉黑了 viewer（前端置灰关注按钮）
}

func (h *InternalRelationHandler) Follow(ctx *gin.Context, req relFolloweeReq) (ginx.Result, error) {
	uc := ginx.MustClaims[UserClaims](ctx)
	if err := h.svc.Follow(ctx.Request.Context(), uc.Userid, req.FolloweeId); err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Msg: "OK"}, nil
}

func (h *InternalRelationHandler) Unfollow(ctx *gin.Context, req relFolloweeReq) (ginx.Result, error) {
	uc := ginx.MustClaims[UserClaims](ctx)
	if err := h.svc.Unfollow(ctx.Request.Context(), uc.Userid, req.FolloweeId); err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Msg: "OK"}, nil
}

func (h *InternalRelationHandler) Block(ctx *gin.Context, req relTargetReq) (ginx.Result, error) {
	uc := ginx.MustClaims[UserClaims](ctx)
	if err := h.svc.Block(ctx.Request.Context(), uc.Userid, req.TargetId); err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Msg: "OK"}, nil
}

func (h *InternalRelationHandler) Unblock(ctx *gin.Context, req relTargetReq) (ginx.Result, error) {
	uc := ginx.MustClaims[UserClaims](ctx)
	if err := h.svc.Unblock(ctx.Request.Context(), uc.Userid, req.TargetId); err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Msg: "OK"}, nil
}

func (h *InternalRelationHandler) Followees(ctx *gin.Context, req relListReq) (ginx.Result, error) {
	items, next, err := h.svc.Followees(ctx.Request.Context(), req.UserId, req.Cursor, req.Limit)
	if err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Data: gin.H{"list": slicex.Map(items, toFolloweeVO), "nextCursor": next}}, nil
}

func (h *InternalRelationHandler) Followers(ctx *gin.Context, req relListReq) (ginx.Result, error) {
	items, next, err := h.svc.Followers(ctx.Request.Context(), req.UserId, req.Cursor, req.Limit)
	if err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Data: gin.H{"list": slicex.Map(items, toFollowerVO), "nextCursor": next}}, nil
}

func (h *InternalRelationHandler) Blocklist(ctx *gin.Context, req relBlocklistReq) (ginx.Result, error) {
	uc := ginx.MustClaims[UserClaims](ctx)
	items, next, err := h.svc.Blocklist(ctx.Request.Context(), uc.Userid, req.Cursor, req.Limit)
	if err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Data: gin.H{"list": slicex.Map(items, toBlockVO), "nextCursor": next}}, nil
}

func (h *InternalRelationHandler) Stat(ctx *gin.Context, req relStatReq) (ginx.Result, error) {
	st, err := h.svc.Stat(ctx.Request.Context(), optionalUid(ctx), req.UserId)
	if err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Data: toStatVO(st)}, nil
}

// ── domain.RelationUser/RelationStat → VO 纯映射 ─────────────────────────────

func toFolloweeVO(u domain.RelationUser) followeeItemVO {
	return followeeItemVO{
		userBriefVO: userBriefVO{Id: u.Id, Name: u.Name, Bio: u.Bio},
		IsMutual:    u.IsMutual,
		FolloweeCnt: u.FolloweeCnt,
		FollowerCnt: u.FollowerCnt,
	}
}

func toFollowerVO(u domain.RelationUser) followerItemVO {
	return followerItemVO{
		userBriefVO:    userBriefVO{Id: u.Id, Name: u.Name, Bio: u.Bio},
		IsFollowedBack: u.IsFollowedBack,
		FolloweeCnt:    u.FolloweeCnt,
		FollowerCnt:    u.FollowerCnt,
		CreatedAt:      u.CreatedAt,
	}
}

func toBlockVO(u domain.RelationUser) blockItemVO {
	return blockItemVO{
		userBriefVO: userBriefVO{Id: u.Id, Name: u.Name, Bio: u.Bio},
		BlockedAt:   u.CreatedAt,
	}
}

func toStatVO(st domain.RelationStat) relationStatVO {
	return relationStatVO{
		FolloweeCnt: st.FolloweeCnt,
		FollowerCnt: st.FollowerCnt,
		IsFollowing: st.IsFollowing,
		IsMutual:    st.IsMutual,
		IsBlocked:   st.IsBlocked,
		IsBlockedBy: st.IsBlockedBy,
	}
}
