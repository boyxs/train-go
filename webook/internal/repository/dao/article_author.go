package dao

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/plugin/soft_delete"
)

var ErrArticleNotFound = errors.New("ID或创作者错误")

// ArticleAuthorDAO 制作库 DAO，只操作 article 表
type ArticleAuthorDAO interface {
	Insert(ctx context.Context, article Article) (int64, error)
	Update(ctx context.Context, article Article) error
	UpdateStatus(ctx context.Context, id int64, uid int64, fromStatus uint8, toStatus uint8) error
	FindByIdAndAuthor(ctx context.Context, id int64, uid int64) (Article, error)
	PageByAuthor(ctx context.Context, uid int64, offset int, limit int) ([]Article, error)
	CountByAuthor(ctx context.Context, uid int64) (int64, error)
	ListByAuthor(ctx context.Context, uid int64) ([]Article, error)
	Delete(ctx context.Context, id int64, uid int64) error
}

type GormArticleAuthorDAO struct {
	db *gorm.DB
}

func NewGormArticleAuthorDAO(db *gorm.DB) ArticleAuthorDAO {
	return &GormArticleAuthorDAO{db: db}
}

func (d *GormArticleAuthorDAO) Insert(ctx context.Context, article Article) (int64, error) {
	article.Abstract = ensureAbstract(article.Abstract, article.Content)
	err := d.db.WithContext(ctx).Create(&article).Error
	return article.Id, err
}

func (d *GormArticleAuthorDAO) Update(ctx context.Context, article Article) error {
	article.Abstract = ensureAbstract(article.Abstract, article.Content)
	row := d.db.WithContext(ctx).
		Model(&article).
		Where("id = ? AND author_id = ?", article.Id, article.AuthorId).
		Updates(map[string]any{
			"title":    article.Title,
			"content":  article.Content,
			"abstract": article.Abstract,
			"status":   article.Status,
		})
	if row.Error != nil {
		return row.Error
	}
	if row.RowsAffected == 0 {
		return ErrArticleNotFound
	}
	return nil
}

// UpdateStatus 只更新状态（用于撤回等场景）
func (d *GormArticleAuthorDAO) UpdateStatus(ctx context.Context, id int64, uid int64, fromStatus uint8, toStatus uint8) error {
	row := d.db.WithContext(ctx).
		Model(&Article{}).
		Where("id = ? AND author_id = ? AND status = ?", id, uid, fromStatus).
		Update("status", toStatus)
	if row.Error != nil {
		return row.Error
	}
	// RowsAffected=0 不算错（幂等：已经是目标状态）
	return nil
}

func (d *GormArticleAuthorDAO) FindByIdAndAuthor(ctx context.Context, id int64, uid int64) (Article, error) {
	var article Article
	err := d.db.WithContext(ctx).
		Where("id = ? AND author_id = ?", id, uid).
		First(&article).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Article{}, ErrArticleNotFound
		}
		return Article{}, err
	}
	return article, nil
}

func (d *GormArticleAuthorDAO) PageByAuthor(ctx context.Context, uid int64, offset int, limit int) ([]Article, error) {
	var articles []Article
	err := d.db.WithContext(ctx).
		Select("id, title, abstract, author_id, status, created_at, updated_at").
		Where("author_id = ?", uid).
		Order("id DESC").
		Offset(offset).Limit(limit).
		Find(&articles).Error
	return articles, err
}

func (d *GormArticleAuthorDAO) CountByAuthor(ctx context.Context, uid int64) (int64, error) {
	var count int64
	err := d.db.WithContext(ctx).
		Model(&Article{}).
		Where("author_id = ?", uid).
		Count(&count).Error
	return count, err
}

func (d *GormArticleAuthorDAO) ListByAuthor(ctx context.Context, uid int64) ([]Article, error) {
	var articles []Article
	err := d.db.WithContext(ctx).
		Select("id, title, abstract, author_id, status, created_at, updated_at").
		Where("author_id = ?", uid).
		Order("id DESC").
		Limit(1000).
		Find(&articles).Error
	return articles, err
}

func (d *GormArticleAuthorDAO) Delete(ctx context.Context, id int64, uid int64) error {
	row := d.db.WithContext(ctx).
		Where("id = ? AND author_id = ?", id, uid).
		Delete(&Article{})
	if row.Error != nil {
		return row.Error
	}
	if row.RowsAffected == 0 {
		return ErrArticleNotFound
	}
	return nil
}

// Article 制作库模型
type Article struct {
	Id        int64  `gorm:"primaryKey,autoIncrement"`
	Title     string `gorm:"type=varchar(4096)"`
	Content   string `gorm:"type=BLOB"`
	Abstract  string `gorm:"type=varchar(256)"`
	AuthorId  int64  `gorm:"index"`
	Status    uint8
	CreatedAt int64                 `gorm:"autoCreateTime:milli"`
	UpdatedAt int64                 `gorm:"autoUpdateTime:milli"`
	DeletedAt soft_delete.DeletedAt `gorm:"softDelete:milli"`
}

func (Article) TableName() string {
	return "article"
}

// ensureAbstract 若 abstract 为空，自动从 content 截取前 128 字符
func ensureAbstract(abstract, content string) string {
	if abstract != "" {
		return abstract
	}
	r := []rune(content)
	if len(r) <= 128 {
		return content
	}
	return string(r[:128]) + "..."
}
