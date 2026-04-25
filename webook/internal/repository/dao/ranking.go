package dao

import (
	"context"

	"gorm.io/gorm"
)

type RankingDAO interface {
	InsertSnapshot(ctx context.Context, date, dim, cat string, items []ArticleRanking) error
	ListByDate(ctx context.Context, date, dim, cat string) ([]ArticleRanking, error)
	ListArchiveDates(ctx context.Context) ([]string, error)
}

type GormArticleRankingDAO struct {
	db *gorm.DB
}

func NewGormArticleRankingDAO(db *gorm.DB) RankingDAO {
	return &GormArticleRankingDAO{db: db}
}

func (d *GormArticleRankingDAO) InsertSnapshot(ctx context.Context, date, dim, cat string, items []ArticleRanking) error {
	if len(items) == 0 {
		return nil
	}
	for i := range items {
		items[i].Date = date
		items[i].Dimension = dim
		items[i].Category = cat
	}
	return d.db.WithContext(ctx).Create(&items).Error
}

func (d *GormArticleRankingDAO) ListByDate(ctx context.Context, date, dim, cat string) ([]ArticleRanking, error) {
	var items []ArticleRanking
	// `date` `rank` 是 MySQL 保留字，必须用反引号
	err := d.db.WithContext(ctx).
		Where("`date` = ? AND `dimension` = ? AND `category` = ?", date, dim, cat).
		Order("`rank` ASC").
		Find(&items).Error
	return items, err
}

func (d *GormArticleRankingDAO) ListArchiveDates(ctx context.Context) ([]string, error) {
	var dates []string
	err := d.db.WithContext(ctx).
		Model(&ArticleRanking{}).
		Distinct("`date`").
		Order("`date` DESC").
		Pluck("`date`", &dates).Error
	return dates, err
}

type ArticleRanking struct {
	Id        int64  `gorm:"primaryKey,autoIncrement"`
	Date      string `gorm:"type:varchar(10);not null;uniqueIndex:uk_article_ranking_date_dim_cat_rank,priority:1;index:idx_article_ranking_date"`
	Dimension string `gorm:"type:varchar(16);not null;uniqueIndex:uk_article_ranking_date_dim_cat_rank,priority:2"`
	Category  string `gorm:"type:varchar(32);not null;default:'';uniqueIndex:uk_article_ranking_date_dim_cat_rank,priority:3"`
	Rank      int    `gorm:"not null;uniqueIndex:uk_article_ranking_date_dim_cat_rank,priority:4"`
	ArticleId int64  `gorm:"not null"`
	Score     float64
	Snapshot  string `gorm:"type:json"`
	CreatedAt int64  `gorm:"autoCreateTime:milli"`
}

func (ArticleRanking) TableName() string {
	return "article_ranking"
}
