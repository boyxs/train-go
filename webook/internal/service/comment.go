package service

import (
	"context"
	"sort"
	"strings"

	commentv1 "github.com/boyxs/train-go/webook/api/gen/comment/v1"
	"github.com/boyxs/train-go/webook/internal/domain"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

const (
	defaultCommentLimit = 10  // 未传 limit 时的默认页大小
	commentHotWindow    = 100 // 最热取首屏窗口（P0：取一批一级评论内存排序 top N）
	commentSortHot      = "hot"
)

// CommentService 评论 core 网关业务：调 comment gRPC 拿评论树，聚合 interaction(biz="comment")
// 的 likeCnt/liked + user 昵称，hot 内存排序、count 总数。接入层只调用 + domain→VO 映射。
type CommentService interface {
	// List 一级评论分页（sort=hot 内存按热度排；否则按时间）+ 总数
	List(ctx context.Context, uid, articleId int64, sort string, offset, limit int32) ([]domain.CommentView, int64, error)
	// Replies 楼内回复懒加载
	Replies(ctx context.Context, uid, rootId int64, offset, limit int32) ([]domain.CommentView, error)
	// Create 发表评论/回复（biz 由本 service 注入）
	Create(ctx context.Context, uid, articleId int64, content string, pid int64) (domain.CommentView, error)
	// Delete 删除（鉴权由 comment 后端做）
	Delete(ctx context.Context, id, uid int64) error
}

type GRPCCommentService struct {
	client  commentv1.CommentServiceClient
	intrSvc InteractionService
	userSvc UserService
	l       logger.LoggerX
	biz     string // 评论挂载业务，固定 "article"（前端只传 articleId，core 注入）
}

func NewGRPCCommentService(client commentv1.CommentServiceClient, intrSvc InteractionService, userSvc UserService, l logger.LoggerX) CommentService {
	return &GRPCCommentService{client: client, intrSvc: intrSvc, userSvc: userSvc, l: l, biz: domain.BizArticle}
}

func (s *GRPCCommentService) List(ctx context.Context, uid, articleId int64, sortBy string, offset, limit int32) ([]domain.CommentView, int64, error) {
	limit = normalizeCommentLimit(limit)
	hot := strings.EqualFold(sortBy, commentSortHot)
	// hot 拉一批窗口后内存按热度排序；new 直接按时间分页（comment server 已倒序）
	fetchOffset, fetchLimit := offset, limit
	if hot {
		fetchOffset, fetchLimit = 0, commentHotWindow
	}
	resp, err := s.client.ListComments(ctx, &commentv1.ListCommentsRequest{
		Biz: s.biz, BizId: articleId, Offset: fetchOffset, Limit: fetchLimit,
	})
	if err != nil {
		return nil, 0, err
	}
	views := s.aggregate(ctx, uid, resp.Comments)
	if hot {
		sortViewsByHot(views)
		if int(limit) < len(views) {
			views = views[:limit]
		}
	}
	total, err := s.count(ctx, articleId)
	if err != nil {
		return nil, 0, err
	}
	return views, total, nil
}

func (s *GRPCCommentService) Replies(ctx context.Context, uid, rootId int64, offset, limit int32) ([]domain.CommentView, error) {
	resp, err := s.client.GetReplies(ctx, &commentv1.GetRepliesRequest{
		RootId: rootId, Offset: offset, Limit: normalizeCommentLimit(limit),
	})
	if err != nil {
		return nil, err
	}
	return s.aggregate(ctx, uid, resp.Replies), nil
}

func (s *GRPCCommentService) Create(ctx context.Context, uid, articleId int64, content string, pid int64) (domain.CommentView, error) {
	resp, err := s.client.CreateComment(ctx, &commentv1.CreateCommentRequest{
		Biz: s.biz, BizId: articleId, UserId: uid, Content: content, Pid: pid,
	})
	if err != nil {
		return domain.CommentView{}, err
	}
	// 新评论 likeCnt=0/liked=false，无需聚合 interaction；仅解析评论者昵称
	names := s.resolveNames(ctx, []*commentv1.Comment{resp.Comment})
	return toCommentView(resp.Comment, nil, nil, names), nil
}

func (s *GRPCCommentService) Delete(ctx context.Context, id, uid int64) error {
	_, err := s.client.DeleteComment(ctx, &commentv1.DeleteCommentRequest{Id: id, UserId: uid})
	return err
}

func (s *GRPCCommentService) count(ctx context.Context, articleId int64) (int64, error) {
	resp, err := s.client.CountComment(ctx, &commentv1.CountCommentRequest{Biz: s.biz, BizId: articleId})
	if err != nil {
		return 0, err
	}
	return resp.Count, nil
}

// aggregate 把 pb 评论树转 domain.CommentView，并批量填 interaction(biz="comment") 的 likeCnt/liked（避免 N+1）。
// 互动聚合失败降级填零，不拖垮评论列表主流程。
func (s *GRPCCommentService) aggregate(ctx context.Context, uid int64, comments []*commentv1.Comment) []domain.CommentView {
	ids := collectCommentIds(comments)
	if len(ids) == 0 {
		return []domain.CommentView{}
	}
	cntMap, err := s.intrSvc.FindByBizIds(ctx, domain.BizComment, ids)
	if err != nil {
		s.l.Error(ctx, "批量获取评论互动数据失败，降级填零", logger.Error(err))
		cntMap = nil
	}
	var likedMap map[int64]bool
	if uid > 0 {
		likedMap, err = s.intrSvc.FindUserLiked(ctx, uid, domain.BizComment, ids)
		if err != nil {
			s.l.Error(ctx, "批量获取评论点赞状态失败，降级填零", logger.Error(err))
			likedMap = nil
		}
	}
	names := s.resolveNames(ctx, comments)
	return toCommentViews(comments, cntMap, likedMap, names)
}

// resolveNames 批量解析评论者 uid→昵称（comment 服务只存 uid）。失败不阻断展示（前端首字母占位）。
func (s *GRPCCommentService) resolveNames(ctx context.Context, comments []*commentv1.Comment) map[int64]string {
	uids := collectUserIds(comments)
	if len(uids) == 0 {
		return map[int64]string{}
	}
	users, err := s.userSvc.FindByIds(ctx, uids)
	if err != nil {
		s.l.Error(ctx, "批量解析评论者昵称失败", logger.Error(err))
		return map[int64]string{}
	}
	names := make(map[int64]string, len(users))
	for uid, u := range users {
		names[uid] = u.Nickname
	}
	return names
}

func normalizeCommentLimit(limit int32) int32 {
	if limit <= 0 {
		return defaultCommentLimit
	}
	return limit
}

func sortViewsByHot(views []domain.CommentView) {
	sort.SliceStable(views, func(i, j int) bool {
		if views[i].LikeCnt != views[j].LikeCnt {
			return views[i].LikeCnt > views[j].LikeCnt
		}
		return views[i].CreatedAt > views[j].CreatedAt
	})
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

func toCommentViews(comments []*commentv1.Comment, cntMap map[int64]domain.Interaction, likedMap map[int64]bool, nameMap map[int64]string) []domain.CommentView {
	views := make([]domain.CommentView, 0, len(comments))
	for _, c := range comments {
		views = append(views, toCommentView(c, cntMap, likedMap, nameMap))
	}
	return views
}

// toCommentView 单条 pb→domain 转换（唯一映射点）；cntMap/likedMap/nameMap 可为 nil（读 nil map 返回零值）。
func toCommentView(c *commentv1.Comment, cntMap map[int64]domain.Interaction, likedMap map[int64]bool, nameMap map[int64]string) domain.CommentView {
	v := domain.CommentView{
		Id:        c.Id,
		User:      domain.CommentUser{Id: c.UserId, Name: nameMap[c.UserId]},
		Content:   c.Content,
		RootId:    c.RootId,
		Pid:       c.Pid,
		ReplyCnt:  c.ReplyCnt,
		LikeCnt:   cntMap[c.Id].LikeCount,
		Liked:     likedMap[c.Id],
		CreatedAt: c.CreatedAt,
	}
	if len(c.Children) > 0 {
		v.Children = toCommentViews(c.Children, cntMap, likedMap, nameMap)
	}
	return v
}
