package service

import (
	"context"
	"errors"
	"strings"

	"github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/search/domain"
	"github.com/boyxs/train-go/webook/search/errs"
	"github.com/boyxs/train-go/webook/search/repository"
)

const (
	defaultSearchSize = 10
	maxSearchSize     = 50
	maxQueryRunes     = 256
)

// Embedder 文本向量化（search 声明自己需要的最小接口；真实实现 OpenAI/Ollama/Failover 由 ioc 注入）。
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

type ArticleService interface {
	// Index 索引文章（内部 embed title+content → 向量后写 ES，无正文回退摘要）；embed/写入失败非致命降级。
	Index(ctx context.Context, article domain.Article) error
	Remove(ctx context.Context, id int64) error
	// Search 校验 query → embed → repo.Search（分页归一），返回命中 + facet。
	Search(ctx context.Context, query string, filterTags []string, page, size int) (domain.SearchResult, error)
	// RecommendTags embed(title+content) → kNN 相似文章标签聚合作候选；无 embedding 降级空。
	RecommendTags(ctx context.Context, title, content string, k int) ([]domain.TagCount, error)
}

type InternalArticleService struct {
	repo  repository.ArticleRepository
	embed Embedder
	l     logger.LoggerX
}

func NewInternalArticleService(repo repository.ArticleRepository, embed Embedder, l logger.LoggerX) ArticleService {
	return &InternalArticleService{repo: repo, embed: embed, l: l}
}

func (s *InternalArticleService) Index(ctx context.Context, article domain.Article) error {
	// 优先用正文（与 RecommendTags 同口径 embed(title+content)，令索引/查询向量同语义空间）；
	// 无正文时回退摘要（如 backfill 只带 abstract），避免退化成 title-only。
	body := article.Content
	if body == "" {
		body = article.Abstract
	}
	text := strings.TrimSpace(article.Title + " " + body)
	vec, err := s.embed.Embed(ctx, text)
	if err != nil {
		// embedding 是外部易失依赖，失败降级：不索引、不阻断
		s.l.Error(ctx, "索引文章：embed 失败", logger.Int64("articleId", article.Id), logger.Error(err))
		return nil
	}
	if err := s.repo.Index(ctx, article, vec); err != nil {
		// 写入 ES 失败同样降级：记日志、不阻断
		s.l.Error(ctx, "索引文章：写入 ES 失败", logger.Int64("articleId", article.Id), logger.Error(err))
		return nil
	}
	return nil
}

func (s *InternalArticleService) Remove(ctx context.Context, id int64) error {
	err := s.repo.Remove(ctx, id)
	if errors.Is(err, errs.ErrSearchDocNotFound) {
		return nil // 幂等：已不存在视为成功
	}
	return err
}

func (s *InternalArticleService) Search(ctx context.Context, query string, filterTags []string, page, size int) (domain.SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return domain.SearchResult{}, errs.ErrSearchQueryEmpty
	}
	if len([]rune(query)) > maxQueryRunes {
		return domain.SearchResult{}, errs.ErrSearchQueryTooLong
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
	vec, err := s.embed.Embed(ctx, query)
	if err != nil {
		return domain.SearchResult{}, err
	}
	return s.repo.Search(ctx, query, vec, filterTags, (page-1)*size, size)
}

func (s *InternalArticleService) RecommendTags(ctx context.Context, title, content string, k int) ([]domain.TagCount, error) {
	text := strings.TrimSpace(title + " " + content)
	if text == "" {
		return nil, nil
	}
	vec, err := s.embed.Embed(ctx, text)
	if err != nil {
		// 降级：无向量则无候选，作者仍可手动输入标签
		s.l.Error(ctx, "推荐标签：embed 失败", logger.Error(err))
		return nil, nil
	}
	return s.repo.RecommendTags(ctx, vec, k)
}
