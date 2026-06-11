package dao

import (
	"gorm.io/gorm"
)

// InitTable migrator 控制库 5 张表的 AutoMigrate。
// 风格与 chat/repository/dao/init_table.go 一致：dao 层声明表清单，ioc 调用。
//
// 注：scripts/migrator.sql 是人工 DDL 参考（含索引 / 注释 / 字符集等 AutoMigrate 不保证一致的元数据），
// 生产环境推荐用 sql 显式建表；本地 / 集成测试用 AutoMigrate 加速 setup。
func InitTable(db *gorm.DB) error {
	return db.AutoMigrate(
		&Task{},
		&Checkpoint{},
		&ValidateLog{},
		&AuditLog{},
		&DeadLetter{},
	)
}
