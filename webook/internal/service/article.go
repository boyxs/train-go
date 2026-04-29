package service

import (
	"context"
	"time"

	"github.com/webook/internal/domain"
	"github.com/webook/internal/repository"
	"github.com/webook/pkg/logger"
)

// ── 作者端 ─────────────────────────────────────────────────────────────────

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
	searchSvc  ArticleSearchService
	l          logger.LoggerX
}

func NewInternalArticleAuthorService(
	authorRepo repository.ArticleAuthorRepository,
	readerRepo repository.ArticleReaderRepository,
	searchSvc ArticleSearchService,
	l logger.LoggerX,
) ArticleAuthorService {
	return &InternalArticleAuthorService{
		authorRepo: authorRepo,
		readerRepo: readerRepo,
		searchSvc:  searchSvc,
		l:          l,
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
	// 从 DB 回查完整数据（含 AuthorName / CreatedAt），再写入 ES
	go func(articleId, uid int64) {
		bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		complete, err := s.authorRepo.FindById(bgCtx, articleId, uid)
		if err != nil {
			s.l.Error("索引文章：回查完整数据失败",
				logger.Int64("articleId", articleId), logger.Error(err))
			return
		}
		if err := s.searchSvc.IndexArticle(bgCtx, complete); err != nil {
			s.l.Error("索引文章失败", logger.Int64("articleId", articleId), logger.Error(err))
		}
	}(id, article.Author.Id)
	return id, nil
}

func (s *InternalArticleAuthorService) Withdraw(ctx context.Context, id int64, uid int64) error {
	// UpdateStatus 自带权限校验（WHERE author_id=uid AND status=from），RowsAffected=0 即失败
	err := s.authorRepo.UpdateStatus(ctx, id, uid,
		domain.ArticleStatusPublished.ToUint8(),
		domain.ArticleStatusPrivate.ToUint8())
	if err != nil {
		return err
	}
	if err := s.readerRepo.Delete(ctx, id, uid); err != nil {
		return err
	}
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.searchSvc.RemoveArticle(bgCtx, id); err != nil {
			s.l.Error("移除搜索索引失败", logger.Int64("articleId", id), logger.Error(err))
		}
	}()
	return nil
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
	if err := s.authorRepo.Delete(ctx, id, uid); err != nil {
		return err
	}
	if err := s.readerRepo.Delete(ctx, id, uid); err != nil {
		return err
	}
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.searchSvc.RemoveArticle(bgCtx, id); err != nil {
			s.l.Error("移除搜索索引失败", logger.Int64("articleId", id), logger.Error(err))
		}
	}()
	return nil
}

// ── 读者端 ─────────────────────────────────────────────────────────────────

type ArticleReaderService interface {
	Detail(ctx context.Context, id int64) (domain.Article, error)
	// BatchDetail 批量取文章详情；不存在的 id 静默跳过，返回保留入参顺序
	BatchDetail(ctx context.Context, ids []int64) ([]domain.Article, error)
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

func (s *InternalArticleReaderService) BatchDetail(ctx context.Context, ids []int64) ([]domain.Article, error) {
	return s.readerRepo.FindByIds(ctx, ids)
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
