package grpc

import (
	"context"

	commentv1 "github.com/webook/api/gen/comment/v1"
	"github.com/webook/comment/domain"
	"github.com/webook/comment/service"
	"github.com/webook/pkg/errs"
	"github.com/webook/pkg/slicex"
)

// CommentServer 把内部 CommentService 适配成 gRPC 接口。
// 错误处理：return *errs.Error，由 grpcx server interceptor（errconv）统一转 status.Status。
type CommentServer struct {
	commentv1.UnimplementedCommentServiceServer
	svc service.CommentService
}

func NewCommentServer(svc service.CommentService) *CommentServer {
	return &CommentServer{svc: svc}
}

func (s *CommentServer) CreateComment(ctx context.Context, req *commentv1.CreateCommentRequest) (*commentv1.CreateCommentResponse, error) {
	if req.GetBiz() == "" || req.GetBizId() <= 0 {
		return nil, errs.New(400, "biz / bizId 不能为空")
	}
	if req.GetUserId() <= 0 {
		return nil, errs.New(401, "请先登录")
	}
	c, err := s.svc.Create(ctx, domain.Comment{
		Biz:     req.GetBiz(),
		BizId:   req.GetBizId(),
		UserId:  req.GetUserId(),
		Content: req.GetContent(),
		Pid:     req.GetPid(),
	})
	if err != nil {
		return nil, err
	}
	return &commentv1.CreateCommentResponse{Comment: toPb(c)}, nil
}

func (s *CommentServer) ListComments(ctx context.Context, req *commentv1.ListCommentsRequest) (*commentv1.ListCommentsResponse, error) {
	if req.GetBiz() == "" || req.GetBizId() <= 0 {
		return nil, errs.New(400, "biz / bizId 不能为空")
	}
	list, err := s.svc.List(ctx, req.GetBiz(), req.GetBizId(), int(req.GetOffset()), normLimit(req.GetLimit()))
	if err != nil {
		return nil, err
	}
	return &commentv1.ListCommentsResponse{Comments: slicex.Map(list, toPb)}, nil
}

func (s *CommentServer) BatchGetComments(ctx context.Context, req *commentv1.BatchGetCommentsRequest) (*commentv1.BatchGetCommentsResponse, error) {
	list, err := s.svc.BatchGet(ctx, req.GetIds())
	if err != nil {
		return nil, err
	}
	return &commentv1.BatchGetCommentsResponse{Comments: slicex.Map(list, toPb)}, nil
}

func (s *CommentServer) GetReplies(ctx context.Context, req *commentv1.GetRepliesRequest) (*commentv1.GetRepliesResponse, error) {
	if req.GetRootId() <= 0 {
		return nil, errs.New(400, "rootId 不能为空")
	}
	list, err := s.svc.GetReplies(ctx, req.GetRootId(), int(req.GetOffset()), normLimit(req.GetLimit()))
	if err != nil {
		return nil, err
	}
	return &commentv1.GetRepliesResponse{Replies: slicex.Map(list, toPb)}, nil
}

func (s *CommentServer) DeleteComment(ctx context.Context, req *commentv1.DeleteCommentRequest) (*commentv1.DeleteCommentResponse, error) {
	if req.GetId() <= 0 || req.GetUserId() <= 0 {
		return nil, errs.New(400, "id / uid 不能为空")
	}
	if err := s.svc.Delete(ctx, req.GetId(), req.GetUserId()); err != nil {
		return nil, err
	}
	return &commentv1.DeleteCommentResponse{}, nil
}

func (s *CommentServer) CountComment(ctx context.Context, req *commentv1.CountCommentRequest) (*commentv1.CountCommentResponse, error) {
	n, err := s.svc.Count(ctx, req.GetBiz(), req.GetBizId())
	if err != nil {
		return nil, err
	}
	return &commentv1.CountCommentResponse{Count: n}, nil
}

func (s *CommentServer) BatchCountComment(ctx context.Context, req *commentv1.BatchCountCommentRequest) (*commentv1.BatchCountCommentResponse, error) {
	counts, err := s.svc.BatchCount(ctx, req.GetBiz(), req.GetBizIds())
	if err != nil {
		return nil, err
	}
	return &commentv1.BatchCountCommentResponse{Counts: counts}, nil
}

// normLimit 限定分页 limit 在 (0,100]，默认 20。
func normLimit(limit int32) int {
	n := int(limit)
	if n <= 0 || n > 100 {
		return 20
	}
	return n
}

// toPb 单条 domain → pb 转换（唯一映射点；批量用 slicex.Map(list, toPb)）。
// Children 不在此填：一级评论的回复预览由调用方按 reply_preview 另组装；likeCnt/liked 由 core 聚合 interaction 填。
func toPb(c domain.Comment) *commentv1.Comment {
	return &commentv1.Comment{
		Id:        c.Id,
		Biz:       c.Biz,
		BizId:     c.BizId,
		UserId:    c.UserId,
		Content:   c.Content,
		RootId:    c.RootId,
		Pid:       c.Pid,
		ReplyCnt:  c.ReplyCnt,
		CreatedAt: c.CreatedAt,
	}
}
