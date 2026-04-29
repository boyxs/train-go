package grpc

import (
	"context"
	"strings"

	searchv1 "github.com/webook/api/gen/search/v1"
	"github.com/webook/internal/errs"
	"github.com/webook/internal/service"
)

// SearchServer 把内部 ArticleSearchService 适配成 gRPC 接口，给 chat 等下游调用。
// 错误处理：return *errs.Error，由 grpcx server interceptor 统一转 status.Status。
type SearchServer struct {
	searchv1.UnimplementedSearchServiceServer
	svc service.ArticleSearchService
}

func NewSearchServer(svc service.ArticleSearchService) *SearchServer {
	return &SearchServer{svc: svc}
}

func (s *SearchServer) SearchArticles(ctx context.Context, req *searchv1.SearchArticlesRequest) (*searchv1.SearchArticlesResponse, error) {
	query := strings.TrimSpace(req.GetQuery())
	if query == "" {
		return nil, errs.ErrSearchQueryEmpty
	}
	page := int(req.GetPage())
	if page <= 0 {
		page = 1
	}
	size := int(req.GetSize())
	if size <= 0 || size > 50 {
		size = 10
	}

	articles, total, err := s.svc.Search(ctx, query, page, size)
	if err != nil {
		return nil, err
	}

	cards := make([]*searchv1.ArticleCard, 0, len(articles))
	for _, a := range articles {
		cards = append(cards, &searchv1.ArticleCard{
			Id:       a.Id,
			Title:    a.Title,
			Abstract: a.Abstract,
		})
	}
	return &searchv1.SearchArticlesResponse{Articles: cards, Total: total}, nil
}
