package repository

import (
	"context"

	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository/dao"
)

type ArticleRepository interface {
	Create(ctx context.Context, Article domain.Article) (int64, error)
	Update(ctx context.Context, Article domain.Article) error
}

type RedisArticleRepository struct {
	dao dao.ArticleDAO
}

func NewRedisArticleRepository(dao dao.ArticleDAO) ArticleRepository {
	return &RedisArticleRepository{
		dao: dao,
	}
}

func (ar *RedisArticleRepository) Create(ctx context.Context, Article domain.Article) (int64, error) {
	id, err := ar.dao.Insert(ctx, ar.toEntity(Article))
	if err != nil {
	}
	return id, err
}

func (ar *RedisArticleRepository) Update(ctx context.Context, Article domain.Article) error {
	err := ar.dao.Update(ctx, ar.toEntity(Article))
	if err != nil {
		return err
	}
	return err
}

func (ar *RedisArticleRepository) toDomain(a dao.Article) domain.Article {
	return domain.Article{
		Id:      a.Id,
		Title:   a.Title,
		Content: a.Content,
		Author: domain.Author{
			Id: a.AuthorId,
		},
		Status: domain.ArticleStatus(a.Status),
	}
}

func (ar *RedisArticleRepository) toEntity(a domain.Article) dao.Article {
	return dao.Article{
		Id:       a.Id,
		Title:    a.Title,
		Content:  a.Content,
		AuthorId: a.Author.Id,
		Status:   a.Status.ToUint8(),
	}
}
