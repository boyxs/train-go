package grpc

import (
	"context"

	tagv1 "github.com/boyxs/train-go/webook/api/gen/tag/v1"
	"github.com/boyxs/train-go/webook/pkg/slicex"
	"github.com/boyxs/train-go/webook/tag/domain"
	"github.com/boyxs/train-go/webook/tag/service"
)

// TagServer 把 TagService 适配成 gRPC 接口（pb↔domain 纯映射；错误 return *errs.Error 由 errconv 拦截器转 status）。
type TagServer struct {
	tagv1.UnimplementedTagServiceServer
	svc service.TagService
}

func NewTagServer(svc service.TagService) *TagServer {
	return &TagServer{svc: svc}
}

func (s *TagServer) Suggest(ctx context.Context, req *tagv1.SuggestRequest) (*tagv1.TagList, error) {
	tags, err := s.svc.Suggest(ctx, req.GetPrefix(), int(req.GetLimit()))
	if err != nil {
		return nil, err
	}
	return &tagv1.TagList{Tags: slicex.Map(tags, toPb)}, nil
}

func (s *TagServer) SyncTags(ctx context.Context, req *tagv1.SyncTagsRequest) (*tagv1.TagList, error) {
	tags, err := s.svc.SyncTags(ctx, req.GetBiz(), req.GetBizId(), req.GetNames(), req.GetSource())
	if err != nil {
		return nil, err
	}
	return &tagv1.TagList{Tags: slicex.Map(tags, toPb)}, nil
}

func (s *TagServer) Detail(ctx context.Context, req *tagv1.DetailRequest) (*tagv1.Tag, error) {
	t, err := s.svc.Detail(ctx, req.GetSlug())
	if err != nil {
		return nil, err
	}
	return toPb(t), nil
}

func (s *TagServer) BatchBySlugs(ctx context.Context, req *tagv1.BatchBySlugsRequest) (*tagv1.TagList, error) {
	tags, err := s.svc.TagsBySlugs(ctx, req.GetSlugs())
	if err != nil {
		return nil, err
	}
	return &tagv1.TagList{Tags: slicex.Map(tags, toPb)}, nil
}

func (s *TagServer) TagsByBiz(ctx context.Context, req *tagv1.TagsByBizRequest) (*tagv1.TagsByBizResponse, error) {
	m, err := s.svc.TagsByBiz(ctx, req.GetBiz(), req.GetBizIds())
	if err != nil {
		return nil, err
	}
	pbMap := make(map[int64]*tagv1.TagList, len(m))
	for bizId, tags := range m {
		pbMap[bizId] = &tagv1.TagList{Tags: slicex.Map(tags, toPb)}
	}
	return &tagv1.TagsByBizResponse{Tags: pbMap}, nil
}

func (s *TagServer) BizIdsByTag(ctx context.Context, req *tagv1.BizIdsByTagRequest) (*tagv1.BizIdsByTagResponse, error) {
	ids, total, err := s.svc.BizIdsByTag(ctx, req.GetSlug(), req.GetBiz(), int(req.GetLimit()))
	if err != nil {
		return nil, err
	}
	return &tagv1.BizIdsByTagResponse{Ids: ids, Total: total}, nil
}

func (s *TagServer) Follow(ctx context.Context, req *tagv1.FollowRequest) (*tagv1.FollowResponse, error) {
	changed, cnt, err := s.svc.Follow(ctx, req.GetUid(), req.GetSlug())
	if err != nil {
		return nil, err
	}
	return &tagv1.FollowResponse{Changed: changed, FollowerCount: cnt}, nil
}

func (s *TagServer) Unfollow(ctx context.Context, req *tagv1.FollowRequest) (*tagv1.FollowResponse, error) {
	changed, cnt, err := s.svc.Unfollow(ctx, req.GetUid(), req.GetSlug())
	if err != nil {
		return nil, err
	}
	return &tagv1.FollowResponse{Changed: changed, FollowerCount: cnt}, nil
}

func (s *TagServer) FollowStatus(ctx context.Context, req *tagv1.FollowStatusRequest) (*tagv1.FollowStatusResponse, error) {
	following, err := s.svc.IsFollowing(ctx, req.GetUid(), req.GetSlug())
	if err != nil {
		return nil, err
	}
	return &tagv1.FollowStatusResponse{IsFollowing: following}, nil
}

// toPb domain → pb 单条转换（批量走 slicex.Map）。
func toPb(t domain.Tag) *tagv1.Tag {
	return &tagv1.Tag{
		Id:             t.Id,
		Name:           t.Name,
		Slug:           t.Slug,
		Type:           t.Type,
		Description:    t.Description,
		RefCount:       t.RefCount,
		FollowCount:    t.FollowCount,
		WeeklyNewCount: t.WeeklyNewCount,
	}
}
