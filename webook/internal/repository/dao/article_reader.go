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
	// FindByIds 一次性 IN 查询多条；返回的列表不保证顺序，调用方按 id 自行索引
	FindByIds(ctx context.Context, ids []int64) ([]PublishedArticle, error)
	Page(ctx context.Context, offset int, limit int) ([]PublishedArticle, error)
	Count(ctx context.Context) (int64, error)
}

// ArticleReaderNewDAO named type 让 wire 区分 OLD/NEW 两个 ArticleReaderDAO 实例。
type ArticleReaderNewDAO ArticleReaderDAO

type GormArticleReaderDAO struct {
	db        *gorm.DB
	tableName string
}

func NewGormArticleReaderDAO(db *gorm.DB) ArticleReaderDAO {
	return &GormArticleReaderDAO{db: db, tableName: "published_article"}
}

// NewGormArticleReaderNewDAO 操作迁移 NEW 侧表 published_article_v1。
func NewGormArticleReaderNewDAO(db *gorm.DB) ArticleReaderNewDAO {
	return &GormArticleReaderDAO{db: db, tableName: "published_article_v1"}
}

func (d *GormArticleReaderDAO) Upsert(ctx context.Context, article PublishedArticle) error {
	article.Abstract = ensureAbstract(article.Abstract, article.Content)
	return d.db.WithContext(ctx).Table(d.tableName).Clauses(clause.OnConflict{
		DoUpdates: clause.AssignmentColumns([]string{
			"title", "content", "abstract", "status", "category", "updated_at", "deleted_at",
		}),
	}).Create(&article).Error
}

func (d *GormArticleReaderDAO) Delete(ctx context.Context, id int64, uid int64) error {
	return d.db.WithContext(ctx).Table(d.tableName).
		Where("id = ? AND author_id = ?", id, uid).
		Delete(&PublishedArticle{}).Error
}

func (d *GormArticleReaderDAO) FindById(ctx context.Context, id int64) (PublishedArticle, error) {
	var article PublishedArticle
	err := d.db.WithContext(ctx).Table(d.tableName).Where("id = ?", id).First(&article).Error
	if err != nil {
		return PublishedArticle{}, err
	}
	return article, nil
}

func (d *GormArticleReaderDAO) FindByIds(ctx context.Context, ids []int64) ([]PublishedArticle, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var articleList []PublishedArticle
	err := d.db.WithContext(ctx).Table(d.tableName).
		Select("id, title, abstract, author_id, status, category, created_at, updated_at").
		Where("id IN ?", ids).
		Find(&articleList).Error
	return articleList, err
}

func (d *GormArticleReaderDAO) Page(ctx context.Context, offset int, limit int) ([]PublishedArticle, error) {
	var articles []PublishedArticle
	err := d.db.WithContext(ctx).Table(d.tableName).
		Select("id, title, abstract, author_id, status, category, created_at, updated_at").
		Order("id DESC").
		Offset(offset).Limit(limit).
		Find(&articles).Error
	return articles, err
}

func (d *GormArticleReaderDAO) Count(ctx context.Context) (int64, error) {
	var count int64
	// Model 给 GORM 注入 softDelete 过滤；Table 给动态表名。少 Model 会把软删行算进来。
	err := d.db.WithContext(ctx).Table(d.tableName).Model(&PublishedArticle{}).Count(&count).Error
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
	Category  string                `gorm:"type=varchar(32);default:'';index"`
	CreatedAt int64                 `gorm:"autoCreateTime:milli"`
	UpdatedAt int64                 `gorm:"autoUpdateTime:milli"`
	DeletedAt soft_delete.DeletedAt `gorm:"softDelete:milli"`
}

func (PublishedArticle) TableName() string {
	return "published_article"
}
