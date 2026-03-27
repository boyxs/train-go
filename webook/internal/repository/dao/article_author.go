package dao

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrArticleNotFound = errors.New("ID或创作者错误")

type ArticleAuthorDAO interface {
	Insert(ctx context.Context, article Article) (int64, error)
	Update(ctx context.Context, article Article) error
	Publish(ctx context.Context, author Article, reader PublishedArticle) (int64, error)
	Withdraw(ctx context.Context, id int64, uid int64, fromStatus uint8, toStatus uint8) error
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
	return &GormArticleAuthorDAO{
		db: db,
	}
}

func (d *GormArticleAuthorDAO) Insert(ctx context.Context, article Article) (int64, error) {
	err := d.db.WithContext(ctx).Create(&article).Error
	return article.Id, err
}

func (d *GormArticleAuthorDAO) Update(ctx context.Context, article Article) error {
	row := d.db.WithContext(ctx).
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
		return ErrArticleNotFound
	}
	return nil
}

func (d *GormArticleAuthorDAO) Publish(ctx context.Context, author Article, reader PublishedArticle) (int64, error) {
	var id int64
	err := d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 制作库: upsert
		if author.Id > 0 {
			row := tx.Model(&author).
				Where("id = ? AND author_id = ?", author.Id, author.AuthorId).
				Updates(map[string]any{
					"title":   author.Title,
					"content": author.Content,
					"status":  author.Status,
				})
			if row.Error != nil {
				return row.Error
			}
			if row.RowsAffected == 0 {
				return ErrArticleNotFound
			}
			id = author.Id
		} else {
			if err := tx.Create(&author).Error; err != nil {
				return err
			}
			id = author.Id
		}

		// 线上库: upsert（id 和制作库一致）
		reader.Id = id
		return tx.Clauses(clause.OnConflict{
			DoUpdates: clause.AssignmentColumns([]string{
				"title", "content", "status", "updated_at",
			}),
		}).Create(&reader).Error
	})
	return id, err
}

func (d *GormArticleAuthorDAO) Withdraw(ctx context.Context, id int64, uid int64, fromStatus uint8, toStatus uint8) error {
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 先确认文章存在且是本人的
		var article Article
		err := tx.Where("id = ? AND author_id = ?", id, uid).First(&article).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrArticleNotFound
			}
			return err
		}
		// 只有 fromStatus 才改成 toStatus
		if article.Status == fromStatus {
			err = tx.Model(&Article{}).
				Where("id = ?", id).
				Updates(map[string]any{"status": toStatus}).Error
			if err != nil {
				return err
			}
		}
		// 线上库: 删除（幂等，不存在也不报错）
		return tx.Where("id = ? AND author_id = ?", id, uid).Delete(&PublishedArticle{}).Error
	})
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
		Where("author_id = ?", uid).
		Order("id DESC").
		Limit(1000).
		Find(&articles).Error
	return articles, err
}

func (d *GormArticleAuthorDAO) Delete(ctx context.Context, id int64, uid int64) error {
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 制作库：删除（必须是本人的）
		row := tx.Where("id = ? AND author_id = ?", id, uid).Delete(&Article{})
		if row.Error != nil {
			return row.Error
		}
		if row.RowsAffected == 0 {
			return ErrArticleNotFound
		}
		// 线上库：删除（幂等）
		return tx.Where("id = ? AND author_id = ?", id, uid).Delete(&PublishedArticle{}).Error
	})
}

// Article 制作库模型
type Article struct {
	Id        int64     `gorm:"primaryKey,autoIncrement"`
	Title     string    `gorm:"type=varchar(4096)"`
	Content   string    `gorm:"type=BLOB"`
	AuthorId  int64     `gorm:"index"`
	Status    uint8
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (Article) TableName() string {
	return "article"
}
