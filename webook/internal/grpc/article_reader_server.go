package grpc

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	articlev1 "github.com/boyxs/train-go/webook/api/gen/article/v1"
	"github.com/boyxs/train-go/webook/internal/errs"
	"github.com/boyxs/train-go/webook/internal/service"
	pkgerrs "github.com/boyxs/train-go/webook/pkg/errs"
)

const batchGetArticlesMaxIDs = 100

// ArticleReaderServer 把内部 ArticleReaderService 适配成 gRPC 接口。
// 错误处理：return *errs.Error，由 grpcx server interceptor 统一转 status.Status。
type ArticleReaderServer struct {
	articlev1.UnimplementedArticleReaderServiceServer
	svc service.ArticleReaderService
}

func NewArticleReaderServer(svc service.ArticleReaderService) *ArticleReaderServer {
	return &ArticleReaderServer{svc: svc}
}

func (s *ArticleReaderServer) GetArticle(ctx context.Context, req *articlev1.GetArticleRequest) (*articlev1.Article, error) {
	if req.GetId() <= 0 {
		return nil, pkgerrs.New(400, "id 必须为正整数")
	}
	a, err := s.svc.Detail(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.ErrArticleNotFound.WithCause(err)
		}
		return nil, err // grpcx interceptor 兜底转 codes.Internal + 不泄漏内容
	}
	return &articlev1.Article{
		Id:       a.Id,
		Title:    a.Title,
		Abstract: a.Abstract,
	}, nil
}

// BatchGetArticles 走 service.BatchDetail：单次 IN 查询 + cache MGet，避免 N 次 RPC 雪崩 DB。
// NotFound 由 repository 层静默过滤（FindByIds 只返回查到的，不抛 ErrRecordNotFound）。
func (s *ArticleReaderServer) BatchGetArticles(ctx context.Context, req *articlev1.BatchGetArticlesRequest) (*articlev1.BatchGetArticlesResponse, error) {
	ids := req.GetIds()
	if len(ids) == 0 {
		return &articlev1.BatchGetArticlesResponse{}, nil
	}
	if len(ids) > batchGetArticlesMaxIDs {
		return nil, pkgerrs.New(400, fmt.Sprintf("ids 数量超过上限 %d", batchGetArticlesMaxIDs))
	}

	articleList, err := s.svc.BatchDetail(ctx, ids)
	if err != nil {
		return nil, err
	}

	result := make([]*articlev1.Article, 0, len(articleList))
	for _, a := range articleList {
		result = append(result, &articlev1.Article{
			Id:       a.Id,
			Title:    a.Title,
			Abstract: a.Abstract,
		})
	}
	return &articlev1.BatchGetArticlesResponse{Articles: result}, nil
}
