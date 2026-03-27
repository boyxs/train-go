package service

import (
	"context"

	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository"
)

// ===== 作者端 =====

type ArticleAuthorService interface {
	Edit(ctx context.Context, article domain.Article) (int64, error)
	Publish(ctx context.Context, article domain.Article) (int64, error)
	Withdraw(ctx context.Context, id int64, uid int64) error
	Detail(ctx context.Context, id int64, uid int64) (domain.Article, error)
	Page(ctx context.Context, uid int64, page int, pageSize int) ([]domain.Article, int64, error)
	List(ctx context.Context, uid int64) ([]domain.Article, error)
	Delete(ctx context.Context, id int64, uid int64) error
}

type InternalArticleAuthorService struct {
	authorRepo repository.ArticleAuthorRepository
	readerRepo repository.ArticleReaderRepository
}

func NewInternalArticleAuthorService(
	authorRepo repository.ArticleAuthorRepository,
	readerRepo repository.ArticleReaderRepository,
) ArticleAuthorService {
	return &InternalArticleAuthorService{
		authorRepo: authorRepo,
		readerRepo: readerRepo,
	}
}

func (s *InternalArticleAuthorService) Edit(ctx context.Context, article domain.Article) (int64, error) {
	article.Status = domain.ArticleStatusUnpublished
	if article.Id > 0 {
		err := s.authorRepo.Update(ctx, article)
		return article.Id, err
	}
	return s.authorRepo.Create(ctx, article)
}

func (s *InternalArticleAuthorService) Publish(ctx context.Context, article domain.Article) (int64, error) {
	article.Status = domain.ArticleStatusPublished
	var id int64
	var err error
	if article.Id > 0 {
		err = s.authorRepo.Update(ctx, article)
		id = article.Id
	} else {
		id, err = s.authorRepo.Create(ctx, article)
	}
	if err != nil {
		return 0, err
	}
	article.Id = id
	err = s.readerRepo.Upsert(ctx, article)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (s *InternalArticleAuthorService) Withdraw(ctx context.Context, id int64, uid int64) error {
	_, err := s.authorRepo.FindById(ctx, id, uid)
	if err != nil {
		return err
	}
	err = s.authorRepo.UpdateStatus(ctx, id, uid,
		domain.ArticleStatusPublished.ToUint8(),
		domain.ArticleStatusPrivate.ToUint8())
	if err != nil {
		return err
	}
	return s.readerRepo.Delete(ctx, id, uid)
}

func (s *InternalArticleAuthorService) Detail(ctx context.Context, id int64, uid int64) (domain.Article, error) {
	return s.authorRepo.FindById(ctx, id, uid)
}

func (s *InternalArticleAuthorService) Page(ctx context.Context, uid int64, page int, pageSize int) ([]domain.Article, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize
	return s.authorRepo.Page(ctx, uid, offset, pageSize)
}

func (s *InternalArticleAuthorService) List(ctx context.Context, uid int64) ([]domain.Article, error) {
	return s.authorRepo.List(ctx, uid)
}

func (s *InternalArticleAuthorService) Delete(ctx context.Context, id int64, uid int64) error {
	err := s.authorRepo.Delete(ctx, id, uid)
	if err != nil {
		return err
	}
	return s.readerRepo.Delete(ctx, id, uid)
}

// ===== 读者端 =====

type ArticleReaderService interface {
	Detail(ctx context.Context, id int64) (domain.Article, error)
	Page(ctx context.Context, page int, pageSize int) ([]domain.Article, int64, error)
}

type InternalArticleReaderService struct {
	readerRepo repository.ArticleReaderRepository
}

func NewInternalArticleReaderService(readerRepo repository.ArticleReaderRepository) ArticleReaderService {
	return &InternalArticleReaderService{readerRepo: readerRepo}
}

func (s *InternalArticleReaderService) Detail(ctx context.Context, id int64) (domain.Article, error) {
	return s.readerRepo.FindById(ctx, id)
}

func (s *InternalArticleReaderService) Page(ctx context.Context, page int, pageSize int) ([]domain.Article, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize
	return s.readerRepo.Page(ctx, offset, pageSize)
}
