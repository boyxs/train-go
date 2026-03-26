package dao

import "time"

// PublishedArticle 线上库模型
type PublishedArticle struct {
	Id        int64     `gorm:"primaryKey"`
	Title     string    `gorm:"type=varchar(4096)"`
	Content   string    `gorm:"type=BLOB"`
	AuthorId  int64     `gorm:"index"`
	Status    uint8
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (PublishedArticle) TableName() string {
	return "published_article"
}
