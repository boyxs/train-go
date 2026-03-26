package service

import (
	"context"

	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository"
)

type ArticleService interface {
	Edit(ctx context.Context, article domain.Article) (int64, error)
}
type InternalArticleService struct {
	repo repository.ArticleRepository
}

func NewInternalArticleService(repo repository.ArticleRepository) ArticleService {
	return &InternalArticleService{
		repo: repo,
	}
}

func (as *InternalArticleService) Edit(ctx context.Context, article domain.Article) (int64, error) {
	// 默认状态
	article.Status = domain.ArticleStatusUnpublished
	if article.Id > 0 {
		err := as.repo.Update(ctx, article)
		return article.Id, err
	}
	return as.repo.Create(ctx, article)
}
