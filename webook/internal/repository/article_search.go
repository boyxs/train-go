package repository

import (
	"context"
	"errors"

	"github.com/boyxs/train-go/webook/internal/domain"
	"github.com/boyxs/train-go/webook/internal/errs"
	"github.com/boyxs/train-go/webook/internal/repository/dao"
)

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
	if errors.Is(err, errs.ErrESDocNotFound) {
		return errs.ErrSearchDocNotFound
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
