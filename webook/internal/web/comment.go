package web

import (
	"github.com/gin-gonic/gin"

	"github.com/webook/internal/domain"
	"github.com/webook/internal/service"
	"github.com/webook/pkg/ginx"
	"github.com/webook/pkg/slicex"
)

// CommentHandler 评论 HTTP 接入层：绑定请求 → 调 CommentService → domain→VO 映射。
// 聚合（comment gRPC + interaction likeCnt/liked + user 昵称 + hot 排序 + 计数）全在 service.CommentService。
type CommentHandler interface {
	RegisterRoutes(server *gin.Engine)
}

type InternalCommentHandler struct {
	svc service.CommentService
}

func NewInternalCommentHandler(svc service.CommentService) CommentHandler {
	return &InternalCommentHandler{svc: svc}
}

func (h *InternalCommentHandler) RegisterRoutes(server *gin.Engine) {
	// 前端走 /api/comment/*，dev rewrite + nginx 剥掉 /api 前缀，核心路由对齐 /interaction、/article 等无 /api
	g := server.Group("/comment")
	// list/replies 公开可读，登录态可选（OptionalPaths）→ 有 uid 才聚合 liked
	g.POST("/list", ginx.WrapReq[commentListReq](h.List))
	g.POST("/replies", ginx.WrapReq[commentRepliesReq](h.Replies))
	g.POST("/create", ginx.WrapReq[commentCreateReq](h.Create))
	g.POST("/delete", ginx.WrapReq[commentDeleteReq](h.Delete))
}

type commentListReq struct {
	ArticleId int64  `json:"articleId" binding:"required,gt=0"`
	Sort      string `json:"sort"` // "hot" | "new"(默认)；非 hot 一律按 new，故不约束
	Offset    int32  `json:"offset"`
	Limit     int32  `json:"limit"`
}

type commentRepliesReq struct {
	RootId int64 `json:"rootId" binding:"required,gt=0"`
	Offset int32 `json:"offset"`
	Limit  int32 `json:"limit"`
}

type commentCreateReq struct {
	ArticleId int64  `json:"articleId" binding:"required,gt=0"`
	Content   string `json:"content" binding:"required"`
	Pid       int64  `json:"pid"` // 回复目标父评论；0=一级评论，故不约束
}

type commentDeleteReq struct {
	Id int64 `json:"id" binding:"required,gt=0"`
}

// CommentVO 对外评论视图，比 proto Comment 多 likeCnt/liked（service 聚合 interaction 填入）
type CommentVO struct {
	Id        int64         `json:"id"`
	User      commentUserVO `json:"user"`
	Content   string        `json:"content"`
	RootId    int64         `json:"rootId"`
	Pid       int64         `json:"pid"`
	ReplyCnt  int64         `json:"replyCnt"`
	LikeCnt   int64         `json:"likeCnt"`
	Liked     bool          `json:"liked"`
	CreatedAt int64         `json:"createdAt"`
	Children  []CommentVO   `json:"children,omitempty"`
}

type commentUserVO struct {
	Id   int64  `json:"id"`
	Name string `json:"name"`
}

func (h *InternalCommentHandler) List(ctx *gin.Context, req commentListReq) (ginx.Result, error) {
	items, total, err := h.svc.List(ctx.Request.Context(), optionalUid(ctx), req.ArticleId, req.Sort, req.Offset, req.Limit)
	if err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Data: ginx.PageResult{List: slicex.Map(items, toCommentVO), Total: total}}, nil
}

func (h *InternalCommentHandler) Replies(ctx *gin.Context, req commentRepliesReq) (ginx.Result, error) {
	items, err := h.svc.Replies(ctx.Request.Context(), optionalUid(ctx), req.RootId, req.Offset, req.Limit)
	if err != nil {
		return ginx.Result{}, err
	}
	// 楼内回复总数 = 父评论 reply_cnt（前端已知），此处 Total 返回本页条数
	return ginx.Result{Data: ginx.PageResult{List: slicex.Map(items, toCommentVO), Total: int64(len(items))}}, nil
}

func (h *InternalCommentHandler) Create(ctx *gin.Context, req commentCreateReq) (ginx.Result, error) {
	uc := ginx.MustClaims[UserClaims](ctx)
	v, err := h.svc.Create(ctx.Request.Context(), uc.Userid, req.ArticleId, req.Content, req.Pid)
	if err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Data: gin.H{"comment": toCommentVO(v)}}, nil
}

func (h *InternalCommentHandler) Delete(ctx *gin.Context, req commentDeleteReq) (ginx.Result, error) {
	uc := ginx.MustClaims[UserClaims](ctx)
	if err := h.svc.Delete(ctx.Request.Context(), req.Id, uc.Userid); err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Msg: "OK"}, nil
}

// toCommentVO domain.CommentView → VO（纯映射，递归子回复）。
func toCommentVO(v domain.CommentView) CommentVO {
	vo := CommentVO{
		Id:        v.Id,
		User:      commentUserVO{Id: v.User.Id, Name: v.User.Name},
		Content:   v.Content,
		RootId:    v.RootId,
		Pid:       v.Pid,
		ReplyCnt:  v.ReplyCnt,
		LikeCnt:   v.LikeCnt,
		Liked:     v.Liked,
		CreatedAt: v.CreatedAt,
	}
	if len(v.Children) > 0 {
		vo.Children = slicex.Map(v.Children, toCommentVO)
	}
	return vo
}

// optionalUid 取可选登录态（OptionalPaths 命中时中间件已写入），未登录返回 0。
// comment / relation 等公开可读接入层共用。
func optionalUid(ctx *gin.Context) int64 {
	if uc, ok := ginx.Claims[UserClaims](ctx); ok {
		return uc.Userid
	}
	return 0
}
