package dao

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

type ArticleDAO interface {
	Insert(ctx context.Context, article Article) (int64, error)
	Update(ctx context.Context, article Article) error
}

type GormArticleDAO struct {
	db *gorm.DB
}

func NewGormArticleDAO(db *gorm.DB) ArticleDAO {
	return &GormArticleDAO{
		db: db,
	}
}

func (ad *GormArticleDAO) Insert(ctx context.Context, article Article) (int64, error) {
	err := ad.db.WithContext(ctx).Create(&article).Error
	return article.Id, err
}

func (ad *GormArticleDAO) Update(ctx context.Context, article Article) error {
	row := ad.db.WithContext(ctx).
		Model(&article).
		Where("id = ? AND author_id = ?", article.Id, article.AuthorId).
		Updates(map[string]any{
			"title":   article.Title,
			"content": article.Content,
			"status":  article.Status,
		})
	if row.Error != nil {
		return row.Error
	}
	if row.RowsAffected == 0 {
		return errors.New("ID或创作者错误")
	}
	return nil
}

type Article struct {
	Id        int64     `gorm:"primaryKey,autoIncrement" bson:"id,omitempty"`
	Title     string    `gorm:"type=varchar(4096)" bson:"title,omitempty"`
	Content   string    `gorm:"type=BLOB" bson:"content,omitempty"`
	AuthorId  int64     `gorm:"index" bson:"author_id,omitempty"`
	Status    uint8     `bson:"status,omitempty"`
	CreatedAt time.Time `bson:"created_at,omitempty"`
	UpdatedAt time.Time `bson:"updated_at,omitempty"`
}

func (Article) TableName() string {
	return "article"
}
