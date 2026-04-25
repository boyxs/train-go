package dao

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/plugin/soft_delete"
)

type ClickEventDAO interface {
	Insert(ctx context.Context, event ClickEvent) error
	Dashboard(ctx context.Context, startMs int64, source string) (ClickEventStats, []ClickEventDailyTrend, []ClickEventTopArticle, error)
}

type GormAIClickEventDAO struct {
	db *gorm.DB
}

func NewGormAIClickEventDAO(db *gorm.DB) ClickEventDAO {
	return &GormAIClickEventDAO{db: db}
}

func (d *GormAIClickEventDAO) Insert(ctx context.Context, event ClickEvent) error {
	return d.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).
		Create(&event).Error
}

func (d *GormAIClickEventDAO) Dashboard(ctx context.Context, startMs int64, source string) (ClickEventStats, []ClickEventDailyTrend, []ClickEventTopArticle, error) {
	var stats ClickEventStats
	var trends []ClickEventDailyTrend
	var tops []ClickEventTopArticle

	db := d.db.WithContext(ctx)

	// 聚合统计
	if err := db.Model(&ClickEvent{}).
		Where("created_at >= ? AND source = ?", startMs, source).
		Select("COUNT(*) AS total_clicks, COUNT(DISTINCT user_id) AS unique_users, COUNT(DISTINCT article_id) AS unique_articles").
		Scan(&stats).Error; err != nil {
		return stats, nil, nil, err
	}

	// 每日趋势：按天分组
	if err := db.Model(&ClickEvent{}).
		Where("created_at >= ? AND source = ?", startMs, source).
		Select("FROM_UNIXTIME(created_at / 1000, '%Y-%m-%d') AS date, COUNT(*) AS clicks").
		Group("date").
		Order("date ASC").
		Scan(&trends).Error; err != nil {
		return stats, nil, nil, err
	}

	// Top10 文章：优先取 published_article 标题，兜底取 article 标题（草稿/已撤回）
	if err := db.Model(&ClickEvent{}).
		Select("ai_click_events.article_id, COALESCE(published_article.title, article.title) AS title, COUNT(*) AS clicks, COUNT(DISTINCT ai_click_events.user_id) AS unique_users").
		Joins("LEFT JOIN published_article ON ai_click_events.article_id = published_article.id").
		Joins("LEFT JOIN article ON ai_click_events.article_id = article.id").
		Where("ai_click_events.created_at >= ? AND ai_click_events.source = ?", startMs, source).
		Group("ai_click_events.article_id, title").
		Order("clicks DESC").
		Limit(10).
		Scan(&tops).Error; err != nil {
		return stats, nil, nil, err
	}

	return stats, trends, tops, nil
}

// ── DAO Models ───────────────────────────────────────────

type ClickEvent struct {
	Id             int64                 `gorm:"primaryKey,autoIncrement"`
	UserId         int64                 `gorm:"not null;uniqueIndex:uk_ai_click_events_dedup"`
	ArticleId      int64                 `gorm:"not null;uniqueIndex:uk_ai_click_events_dedup;index:idx_ai_click_events_article_id"`
	ConversationId int64                 `gorm:"not null;uniqueIndex:uk_ai_click_events_dedup"`
	Source         string                `gorm:"type:varchar(32);not null;default:'ai_chat';uniqueIndex:uk_ai_click_events_dedup"`
	CreatedAt      int64                 `gorm:"autoCreateTime:milli;index:idx_ai_click_events_created_at"`
	UpdatedAt      int64                 `gorm:"autoUpdateTime:milli"`
	DeletedAt      soft_delete.DeletedAt `gorm:"softDelete:milli"`
}

func (ClickEvent) TableName() string {
	return "ai_click_events"
}

type ClickEventStats struct {
	TotalClicks    int64
	UniqueUsers    int64
	UniqueArticles int64
}

type ClickEventDailyTrend struct {
	Date   string
	Clicks int64
}

type ClickEventTopArticle struct {
	ArticleId   int64
	Title       string
	Clicks      int64
	UniqueUsers int64
}
