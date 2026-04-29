package dao

import "gorm.io/gorm"

// InitTable 与主仓 internal/repository/dao/init_table.go 风格一致：
// 表迁移声明集中在 dao 层，ioc 只负责调用。
func InitTable(db *gorm.DB) error {
	return db.AutoMigrate(&Conversation{}, &Message{})
}
