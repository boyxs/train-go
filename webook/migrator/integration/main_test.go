package integration

import (
	"os"
	"testing"

	"github.com/spf13/viper"
)

// TestMain 与 chat/integration/main_test.go 同构：加载 migrator/config/test.yaml。
// 路径相对 migrator/integration 包目录：../config/test.yaml = migrator/config/test.yaml
//
// 前置条件（本机 docker compose 起着）：
//  1. mysql 跑着 + 库 webook_migrator_test 已建
//  2. redis 跑着 + 密码与 test.yaml 一致
//
// 跑法：
//
//	cd webook && go test ./migrator/integration/...
func TestMain(m *testing.M) {
	viper.SetConfigFile("../config/test.yaml")
	if err := viper.ReadInConfig(); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
