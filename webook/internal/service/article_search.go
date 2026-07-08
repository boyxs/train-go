package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/boyxs/train-go/webook/internal/domain"
	"github.com/boyxs/train-go/webook/internal/errs"
	"github.com/boyxs/train-go/webook/internal/repository"
	"github.com/boyxs/train-go/webook/internal/service/embedding"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

const (
	defaultSearchSize = 10
	maxSearchSize     = 50
)

type ArticleSearchService interface {
	// IndexArticle 索引文章（发布时调用）；失败只记日志，不阻塞发布
	IndexArticle(ctx context.Context, article domain.Article) error
	// RemoveArticle 移除索引（下架时调用）
	RemoveArticle(ctx context.Context, id int64) error
	// Search 语义搜索文章，返回列表和总数
	Search(ctx context.Context, query string, page, size int) ([]domain.Article, int64, error)
}

type InternalArticleSearchService struct {
	repo  repository.ArticleSearchRepository
	embed embedding.Client
	l     logger.LoggerX
}

func NewArticleSearchService(repo repository.ArticleSearchRepository, embed embedding.Client, l logger.LoggerX) ArticleSearchService {
	return &InternalArticleSearchService{repo: repo, embed: embed, l: l}
}

func (s *InternalArticleSearchService) Search(ctx context.Context, query string, page, size int) ([]domain.Article, int64, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, 0, fmt.Errorf("搜索内容不能为空")
	}
	if len([]rune(query)) > 256 {
		return nil, 0, fmt.Errorf("搜索内容过长")
	}

	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = defaultSearchSize
	}
	if size > maxSearchSize {
		size = maxSearchSize
	}
	offset := (page - 1) * size

	vec, err := s.embed.Embed(ctx, query)
	if err != nil {
		return nil, 0, fmt.Errorf("向量化查询失败: %w", err)
	}

	return s.repo.Search(ctx, query, vec, offset, size)
}

func (s *InternalArticleSearchService) IndexArticle(ctx context.Context, article domain.Article) error {
	text := article.Title
	if article.Abstract != "" {
		text = article.Title + " " + article.Abstract
	}

	vec, err := s.embed.Embed(ctx, text)
	if err != nil {
		s.l.Error("索引文章：embed 失败",
			logger.Int64("articleId", article.Id),
			logger.Error(err))
		return nil
	}

	if err = s.repo.Index(ctx, article, vec); err != nil {
		s.l.Error("索引文章：写入 ES 失败",
			logger.Int64("articleId", article.Id),
			logger.Error(err))
		return nil
	}
	return nil
}

func (s *InternalArticleSearchService) RemoveArticle(ctx context.Context, id int64) error {
	err := s.repo.Remove(ctx, id)
	if errors.Is(err, errs.ErrSearchDocNotFound) {
		return nil
	}
	return err
}
