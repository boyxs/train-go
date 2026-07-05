package setup

import (
	"github.com/spf13/viper"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/webook/interaction/repository/dao"
)

func InitDB() *gorm.DB {
	db, err := gorm.Open(mysql.Open(viper.GetString("data.mysql.dsn")), &gorm.Config{})
	if err != nil {
		// 数据库都连接不上，就不要启动测试了
		panic("failed to connect database")
	}
	if err := dao.InitTable(db); err != nil {
		panic(err)
	}
	return db
}
