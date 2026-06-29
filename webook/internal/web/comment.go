package web

import (
	"context"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	commentv1 "github.com/webook/api/gen/comment/v1"
	"github.com/webook/internal/consts"
	"github.com/webook/internal/domain"
	"github.com/webook/internal/service"
	"github.com/webook/pkg/ginx"
	"github.com/webook/pkg/logger"
)

const (
	defaultCommentLimit = 10  // 未传 limit 时的默认页大小
	hotWindow           = 100 // 最热取首屏窗口（P0：取一批一级评论内存排序 top N）
	sortHot             = "hot"
)

// CommentHandler 评论 HTTP 网关：调 comment gRPC server 拿评论树，
// 聚合 core 内部 interaction(biz="comment") 的 likeCnt/liked 填入 VO。
type CommentHandler interface {
	RegisterRoutes(server *gin.Engine)
}

type InternalCommentHandler struct {
	client  commentv1.CommentServiceClient
	intrSvc service.InteractionService
	userSvc service.UserService // 解析评论者 uid→昵称（comment 服务只存 uid）
	l       logger.LoggerX
	biz     string // 评论挂载业务，固定 "article"（前端只传 articleId，core 注入）
}

func NewInternalCommentHandler(client commentv1.CommentServiceClient, intrSvc service.InteractionService, userSvc service.UserService, l logger.LoggerX) CommentHandler {
	return &InternalCommentHandler{client: client, intrSvc: intrSvc, userSvc: userSvc, l: l, biz: domain.BizArticle}
}

func (h *InternalCommentHandler) RegisterRoutes(server *gin.Engine) {
	// 前端走 /api/comment/*，dev rewrite + nginx 剥掉 /api 前缀，核心路由对齐 /interaction、/article 等无 /api
	g := server.Group("/comment")
	// list/replies 公开可读，登录态可选（OptionalPaths）→ 有 uid 才聚合 liked
	g.POST("/list", ginx.WrapReq[commentListReq](h.List))
	g.POST("/replies", ginx.WrapReq[commentRepliesReq](h.Replies))
	g.POST("/create", ginx.WrapReqClaims[commentCreateReq, UserClaims](consts.UserKey, h.Create))
	g.POST("/delete", ginx.WrapReqClaims[commentDeleteReq, UserClaims](consts.UserKey, h.Delete))
}

type commentListReq struct {
	ArticleId int64  `json:"articleId"`
	Sort      string `json:"sort"` // "hot" | "new"(默认)
	Offset    int32  `json:"offset"`
	Limit     int32  `json:"limit"`
}

type commentRepliesReq struct {
	RootId int64 `json:"rootId"`
	Offset int32 `json:"offset"`
	Limit  int32 `json:"limit"`
}

type commentCreateReq struct {
	ArticleId int64  `json:"articleId"`
	Content   string `json:"content"`
	Pid       int64  `json:"pid"` // 回复目标父评论；0=一级评论
}

type commentDeleteReq struct {
	Id int64 `json:"id"`
}

// CommentVO 对外评论视图，比 proto Comment 多 likeCnt/liked（core 聚合 interaction 填入）
type CommentVO struct {
	Id        int64         `json:"id"`
	User      commentUserVO `json:"user"`
	Content   string        `json:"content"`
	RootId    int64         `json:"rootId"`
	Pid       int64         `json:"pid"`
	ReplyCnt  int64         `json:"replyCnt"`
	LikeCnt   int64         `json:"likeCnt"`
	Liked     bool          `json:"liked"`
	Deleted   bool          `json:"deleted"` // 已删除占位（前端渲染「该评论已删除」，子回复仍在）
	CreatedAt int64         `json:"createdAt"`
	Children  []CommentVO   `json:"children,omitempty"`
}

type commentUserVO struct {
	Id   int64  `json:"id"`
	Name string `json:"name"`
}

func (h *InternalCommentHandler) List(ctx *gin.Context, req commentListReq) (ginx.Result, error) {
	uid := optionalUid(ctx)
	limit := normalizeLimit(req.Limit)
	c := ctx.Request.Context()
	hot := strings.EqualFold(req.Sort, sortHot)

	// hot 拉一批窗口后内存按热度排序；new 直接按时间分页（comment server 已倒序）
	fetchOffset, fetchLimit := req.Offset, limit
	if hot {
		fetchOffset, fetchLimit = 0, hotWindow
	}
	resp, err := h.client.ListComments(c, &commentv1.ListCommentsRequest{
		Biz: h.biz, BizId: req.ArticleId, Offset: fetchOffset, Limit: fetchLimit,
	})
	if err != nil {
		return ginx.Result{}, err
	}
	vos, err := h.aggregate(c, uid, resp.Comments)
	if err != nil {
		return ginx.Result{}, err
	}
	if hot {
		sortByHot(vos)
		if int(limit) < len(vos) {
			vos = vos[:limit]
		}
	}
	total, err := h.count(c, req.ArticleId)
	if err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Data: ginx.PageResult{List: vos, Total: total}}, nil
}

func (h *InternalCommentHandler) Replies(ctx *gin.Context, req commentRepliesReq) (ginx.Result, error) {
	uid := optionalUid(ctx)
	c := ctx.Request.Context()
	resp, err := h.client.GetReplies(c, &commentv1.GetRepliesRequest{
		RootId: req.RootId, Offset: req.Offset, Limit: normalizeLimit(req.Limit),
	})
	if err != nil {
		return ginx.Result{}, err
	}
	vos, err := h.aggregate(c, uid, resp.Replies)
	if err != nil {
		return ginx.Result{}, err
	}
	// 楼内回复总数 = 父评论 reply_cnt（前端已知），此处 Total 返回本页条数
	return ginx.Result{Data: ginx.PageResult{List: vos, Total: int64(len(vos))}}, nil
}

func (h *InternalCommentHandler) Create(ctx *gin.Context, req commentCreateReq, uc UserClaims) (ginx.Result, error) {
	resp, err := h.client.CreateComment(ctx.Request.Context(), &commentv1.CreateCommentRequest{
		Biz: h.biz, BizId: req.ArticleId, UserId: uc.Userid, Content: req.Content, Pid: req.Pid,
	})
	if err != nil {
		return ginx.Result{}, err
	}
	// 新评论 likeCnt=0/liked=false，无需聚合 interaction；仅解析评论者昵称
	nameMap := h.resolveNames(ctx.Request.Context(), []*commentv1.Comment{resp.Comment})
	return ginx.Result{Data: gin.H{"comment": toCommentVO(resp.Comment, nil, nil, nameMap)}}, nil
}

func (h *InternalCommentHandler) Delete(ctx *gin.Context, req commentDeleteReq, uc UserClaims) (ginx.Result, error) {
	// 鉴权（仅本人）由 comment server 校验，失败经 errconv 拦截器转 *errs.Error
	_, err := h.client.DeleteComment(ctx.Request.Context(), &commentv1.DeleteCommentRequest{
		Id: req.Id, UserId: uc.Userid,
	})
	if err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Msg: "OK"}, nil
}

func normalizeLimit(limit int32) int32 {
	if limit <= 0 {
		return defaultCommentLimit
	}
	return limit
}

func sortByHot(vos []CommentVO) {
	sort.SliceStable(vos, func(i, j int) bool {
		if vos[i].LikeCnt != vos[j].LikeCnt {
			return vos[i].LikeCnt > vos[j].LikeCnt
		}
		return vos[i].CreatedAt > vos[j].CreatedAt
	})
}

func (h *InternalCommentHandler) count(ctx context.Context, articleId int64) (int64, error) {
	resp, err := h.client.CountComment(ctx, &commentv1.CountCommentRequest{Biz: h.biz, BizId: articleId})
	if err != nil {
		return 0, err
	}
	return resp.Count, nil
}

// aggregate 把 pb 评论树转 VO，并批量填 interaction(biz="comment") 的 likeCnt/liked（避免 N+1）
func (h *InternalCommentHandler) aggregate(ctx context.Context, uid int64, comments []*commentv1.Comment) ([]CommentVO, error) {
	ids := collectCommentIds(comments)
	if len(ids) == 0 {
		return []CommentVO{}, nil
	}
	cntMap, err := h.intrSvc.FindByBizIds(ctx, domain.BizComment, ids)
	if err != nil {
		return nil, err
	}
	var likedMap map[int64]bool
	if uid > 0 {
		likedMap, err = h.intrSvc.FindUserLiked(ctx, uid, domain.BizComment, ids)
		if err != nil {
			return nil, err
		}
	}
	nameMap := h.resolveNames(ctx, comments)
	return toCommentVOs(comments, cntMap, likedMap, nameMap), nil
}

// collectCommentIds 递归收集评论树（含 children）的全部 id，供批量聚合
func collectCommentIds(comments []*commentv1.Comment) []int64 {
	ids := make([]int64, 0, len(comments))
	for _, c := range comments {
		ids = append(ids, c.Id)
		ids = append(ids, collectCommentIds(c.Children)...)
	}
	return ids
}

func toCommentVOs(comments []*commentv1.Comment, cntMap map[int64]domain.Interaction, likedMap map[int64]bool, nameMap map[int64]string) []CommentVO {
	vos := make([]CommentVO, 0, len(comments))
	for _, c := range comments {
		vos = append(vos, toCommentVO(c, cntMap, likedMap, nameMap))
	}
	return vos
}

// toCommentVO 单条转换（唯一映射点）；cntMap/likedMap/nameMap 可为 nil（读 nil map 返回零值）
func toCommentVO(c *commentv1.Comment, cntMap map[int64]domain.Interaction, likedMap map[int64]bool, nameMap map[int64]string) CommentVO {
	vo := CommentVO{
		Id:        c.Id,
		Content:   c.Content,
		RootId:    c.RootId,
		Pid:       c.Pid,
		ReplyCnt:  c.ReplyCnt,
		LikeCnt:   cntMap[c.Id].LikeCount,
		Liked:     likedMap[c.Id],
		Deleted:   c.Deleted,
		CreatedAt: c.CreatedAt,
	}
	// 昵称由 core 按 uid 解析填入（comment 只回 uid；nil map 取值返回零值）
	vo.User = commentUserVO{Id: c.UserId, Name: nameMap[c.UserId]}
	if len(c.Children) > 0 {
		vo.Children = toCommentVOs(c.Children, cntMap, likedMap, nameMap)
	}
	return vo
}

// collectUserIds 递归收集评论树的全部评论者 uid（map 自然去重）
func collectUserIds(comments []*commentv1.Comment) []int64 {
	var ids []int64
	for _, c := range comments {
		if c.UserId > 0 {
			ids = append(ids, c.UserId)
		}
		ids = append(ids, collectUserIds(c.Children)...)
	}
	return ids
}

// resolveNames 批量解析评论者 uid→昵称（comment 服务只存 uid）。失败不阻断展示（前端首字母占位）。
func (h *InternalCommentHandler) resolveNames(ctx context.Context, comments []*commentv1.Comment) map[int64]string {
	uids := collectUserIds(comments)
	if len(uids) == 0 {
		return map[int64]string{}
	}
	users, err := h.userSvc.FindByIds(ctx, uids)
	if err != nil {
		h.l.Error("批量解析评论者昵称失败", logger.Error(err))
		return map[int64]string{}
	}
	names := make(map[int64]string, len(users))
	for uid, u := range users {
		names[uid] = u.Nickname
	}
	return names
}

// optionalUid 从 ctx 取可选登录态（OptionalPaths 命中时中间件已写入），未登录返回 0
func optionalUid(ctx *gin.Context) int64 {
	val, ok := ctx.Get(consts.UserKey)
	if !ok {
		return 0
	}
	uc, ok := val.(UserClaims)
	if !ok {
		return 0
	}
	return uc.Userid
}
