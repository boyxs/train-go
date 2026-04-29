package integration

import (
	"os"
	"testing"

	"github.com/spf13/viper"
)

// TestMain 与主仓 internal/integration/main_test.go 同结构：加载 chat/config/test.yaml
// 测试库 dsn 指向 webook_test，与 core 共享同一个 docker compose 起的 mysql/redis 实例。
// 路径相对 chat/integration 包目录：../config/test.yaml = chat/config/test.yaml
func TestMain(m *testing.M) {
	viper.SetConfigFile("../config/test.yaml")
	if err := viper.ReadInConfig(); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
