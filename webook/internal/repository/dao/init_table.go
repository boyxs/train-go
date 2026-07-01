package dao

import "gorm.io/gorm"

func InitTable(db *gorm.DB) error {
	// interaction / user_interaction 表归 webook-interaction 独立服务建表，core 不再 AutoMigrate
	return db.AutoMigrate(&User{}, &Article{}, &PublishedArticle{}, &ClickEvent{}, &ArticleRanking{})
}
