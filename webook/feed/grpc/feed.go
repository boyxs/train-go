package grpc

import (
	"context"

	feedv1 "github.com/boyxs/train-go/webook/api/gen/feed/v1"
	"github.com/boyxs/train-go/webook/feed/domain"
	"github.com/boyxs/train-go/webook/feed/service"
	"github.com/boyxs/train-go/webook/pkg/slicex"
)

// FeedServer 把 FeedService 适配成 gRPC 接口。
// 入参非空校验由 ValidateUnaryInterceptor 统一做；错误 return *errs.Error，由 errconv 拦截器转 status。
type FeedServer struct {
	feedv1.UnimplementedFeedServiceServer
	svc service.FeedService
}

func NewFeedServer(svc service.FeedService) *FeedServer {
	return &FeedServer{svc: svc}
}

func (s *FeedServer) ListFeed(ctx context.Context, req *feedv1.ListFeedRequest) (*feedv1.ListFeedResponse, error) {
	items, next, hasMore, err := s.svc.ListFeed(ctx, req.GetUid(), req.GetCursor(), int(req.GetLimit()))
	if err != nil {
		return nil, err
	}
	return &feedv1.ListFeedResponse{
		Items:      slicex.Map(items, toPb),
		NextCursor: next,
		HasMore:    hasMore,
	}, nil
}

func (s *FeedServer) NewCount(ctx context.Context, req *feedv1.NewCountRequest) (*feedv1.NewCountResponse, error) {
	ids, err := s.svc.NewCount(ctx, req.GetUid(), req.GetSinceCursor())
	if err != nil {
		return nil, err
	}
	return &feedv1.NewCountResponse{ArticleIds: ids}, nil
}

func (s *FeedServer) FanoutArticle(ctx context.Context, req *feedv1.FanoutArticleRequest) (*feedv1.FanoutArticleResponse, error) {
	err := s.svc.Fanout(ctx, domain.FeedArticle{
		ArticleId:   req.GetArticleId(),
		AuthorId:    req.GetAuthorId(),
		PublishedAt: req.GetPublishedAt(),
	})
	if err != nil {
		return nil, err
	}
	return &feedv1.FanoutArticleResponse{}, nil
}

func (s *FeedServer) RemoveArticle(ctx context.Context, req *feedv1.RemoveArticleRequest) (*feedv1.RemoveArticleResponse, error) {
	if err := s.svc.Remove(ctx, req.GetArticleId(), req.GetAuthorId()); err != nil {
		return nil, err
	}
	return &feedv1.RemoveArticleResponse{}, nil
}

func (s *FeedServer) InvalidateInboxes(ctx context.Context, req *feedv1.InvalidateInboxesRequest) (*feedv1.InvalidateInboxesResponse, error) {
	if err := s.svc.InvalidateInboxes(ctx, req.GetUids()); err != nil {
		return nil, err
	}
	return &feedv1.InvalidateInboxesResponse{}, nil
}

func toPb(it domain.FeedItem) *feedv1.FeedItem {
	return &feedv1.FeedItem{ArticleId: it.ArticleId, PublishedAt: it.PublishedAt}
}
