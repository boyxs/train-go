package service

import (
	"context"

	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository"
)

type ArticleService interface {
	Edit(ctx context.Context, article domain.Article) (int64, error)
	Publish(ctx context.Context, article domain.Article) (int64, error)
	Withdraw(ctx context.Context, id int64, uid int64) error
	Detail(ctx context.Context, id int64, uid int64) (domain.Article, error)
	Page(ctx context.Context, uid int64, page int, pageSize int) ([]domain.Article, int64, error)
	List(ctx context.Context, uid int64) ([]domain.Article, error)
	Delete(ctx context.Context, id int64, uid int64) error
}

type InternalArticleService struct {
	authorRepo repository.ArticleAuthorRepository
}

func NewInternalArticleService(authorRepo repository.ArticleAuthorRepository) ArticleService {
	return &InternalArticleService{
		authorRepo: authorRepo,
	}
}

func (s *InternalArticleService) Edit(ctx context.Context, article domain.Article) (int64, error) {
	article.Status = domain.ArticleStatusUnpublished
	if article.Id > 0 {
		err := s.authorRepo.Update(ctx, article)
		return article.Id, err
	}
	return s.authorRepo.Create(ctx, article)
}

func (s *InternalArticleService) Publish(ctx context.Context, article domain.Article) (int64, error) {
	article.Status = domain.ArticleStatusPublished
	return s.authorRepo.Publish(ctx, article)
}

func (s *InternalArticleService) Withdraw(ctx context.Context, id int64, uid int64) error {
	return s.authorRepo.Withdraw(ctx, id, uid)
}

func (s *InternalArticleService) Detail(ctx context.Context, id int64, uid int64) (domain.Article, error) {
	return s.authorRepo.FindById(ctx, id, uid)
}

func (s *InternalArticleService) Page(ctx context.Context, uid int64, page int, pageSize int) ([]domain.Article, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize
	return s.authorRepo.Page(ctx, uid, offset, pageSize)
}

func (s *InternalArticleService) List(ctx context.Context, uid int64) ([]domain.Article, error) {
	return s.authorRepo.List(ctx, uid)
}

func (s *InternalArticleService) Delete(ctx context.Context, id int64, uid int64) error {
	return s.authorRepo.Delete(ctx, id, uid)
}
