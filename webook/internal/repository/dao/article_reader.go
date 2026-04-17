package dao

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/plugin/soft_delete"
)

type ArticleReaderDAO interface {
	Upsert(ctx context.Context, article PublishedArticle) error
	Delete(ctx context.Context, id int64, uid int64) error
	FindById(ctx context.Context, id int64) (PublishedArticle, error)
	Page(ctx context.Context, offset int, limit int) ([]PublishedArticle, error)
	Count(ctx context.Context) (int64, error)
}

type GormArticleReaderDAO struct {
	db *gorm.DB
}

func NewGormArticleReaderDAO(db *gorm.DB) ArticleReaderDAO {
	return &GormArticleReaderDAO{db: db}
}

func (d *GormArticleReaderDAO) Upsert(ctx context.Context, article PublishedArticle) error {
	article.Abstract = ensureAbstract(article.Abstract, article.Content)
	return d.db.WithContext(ctx).Clauses(clause.OnConflict{
		DoUpdates: clause.AssignmentColumns([]string{
			"title", "content", "abstract", "status", "updated_at",
		}),
	}).Create(&article).Error
}

func (d *GormArticleReaderDAO) Delete(ctx context.Context, id int64, uid int64) error {
	return d.db.WithContext(ctx).
		Where("id = ? AND author_id = ?", id, uid).
		Delete(&PublishedArticle{}).Error
}

func (d *GormArticleReaderDAO) FindById(ctx context.Context, id int64) (PublishedArticle, error) {
	var article PublishedArticle
	err := d.db.WithContext(ctx).Where("id = ?", id).First(&article).Error
	if err != nil {
		return PublishedArticle{}, err
	}
	return article, nil
}

func (d *GormArticleReaderDAO) Page(ctx context.Context, offset int, limit int) ([]PublishedArticle, error) {
	var articles []PublishedArticle
	err := d.db.WithContext(ctx).
		Select("id, title, abstract, author_id, status, created_at, updated_at").
		Order("id DESC").
		Offset(offset).Limit(limit).
		Find(&articles).Error
	return articles, err
}

func (d *GormArticleReaderDAO) Count(ctx context.Context) (int64, error) {
	var count int64
	err := d.db.WithContext(ctx).Model(&PublishedArticle{}).Count(&count).Error
	return count, err
}

// PublishedArticle 线上库模型
type PublishedArticle struct {
	Id        int64  `gorm:"primaryKey"`
	Title     string `gorm:"type=varchar(4096)"`
	Content   string `gorm:"type=BLOB"`
	Abstract  string `gorm:"type=varchar(256)"`
	AuthorId  int64  `gorm:"index"`
	Status    uint8
	CreatedAt int64                 `gorm:"autoCreateTime:milli"`
	UpdatedAt int64                 `gorm:"autoUpdateTime:milli"`
	DeletedAt soft_delete.DeletedAt `gorm:"softDelete:milli"`
}

func (PublishedArticle) TableName() string {
	return "published_article"
}
