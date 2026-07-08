package setup

import (
	"github.com/spf13/viper"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/boyxs/train-go/webook/internal/repository/dao"
	"github.com/boyxs/train-go/webook/shared/confkey"
)

func InitDB() *gorm.DB {
	db, err := gorm.Open(mysql.Open(viper.GetString(confkey.DataMySQLDSN)), &gorm.Config{})
	if err != nil {
		// 数据库都连接不上，就不要启动服务了
		panic("failed to connect database")
	}
	err = dao.InitTable(db)
	if err != nil {
		panic(err)
	}
	return db
}
