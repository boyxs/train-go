package repository

import (
	"context"
	"errors"

	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository/dao"
)

// ErrSearchDocNotFound 搜索文档不存在（幂等删除用）
var ErrSearchDocNotFound = errors.New("搜索文档不存在")

type ArticleSearchRepository interface {
	Index(ctx context.Context, article domain.Article, vec []float32) error
	Remove(ctx context.Context, id int64) error
	Search(ctx context.Context, text string, vec []float32, offset, limit int) ([]domain.Article, int64, error)
}

type ESArticleSearchRepository struct {
	dao dao.ArticleSearchDAO
}

func NewESArticleSearchRepository(d dao.ArticleSearchDAO) ArticleSearchRepository {
	return &ESArticleSearchRepository{dao: d}
}

func (r *ESArticleSearchRepository) Index(ctx context.Context, article domain.Article, vec []float32) error {
	return r.dao.Upsert(ctx, dao.ArticleESDoc{
		Id:         article.Id,
		Title:      article.Title,
		Abstract:   article.Abstract,
		AuthorId:   article.Author.Id,
		AuthorName: article.Author.Name,
		Status:     article.Status.ToUint8(),
		CreatedAt:  article.CreatedAt,
		ContentVec: vec,
	})
}

func (r *ESArticleSearchRepository) Remove(ctx context.Context, id int64) error {
	err := r.dao.Delete(ctx, id)
	if errors.Is(err, dao.ErrESDocNotFound) {
		return ErrSearchDocNotFound
	}
	return err
}

func (r *ESArticleSearchRepository) Search(ctx context.Context, text string, vec []float32, offset, limit int) ([]domain.Article, int64, error) {
	docs, total, err := r.dao.Search(ctx, text, vec, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	articles := make([]domain.Article, 0, len(docs))
	for _, d := range docs {
		articles = append(articles, domain.Article{
			Id:        d.Id,
			Title:     d.Title,
			Abstract:  d.Abstract,
			Status:    domain.ArticleStatus(d.Status),
			Author:    domain.Author{Id: d.AuthorId, Name: d.AuthorName},
			CreatedAt: d.CreatedAt,
		})
	}
	return articles, total, nil
}
