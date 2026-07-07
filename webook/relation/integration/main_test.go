package integration

import (
	"os"
	"testing"

	"github.com/spf13/viper"
)

// TestMain 加载 relation/config/test.yaml，测试库 dsn 指向本机 webook_test。
func TestMain(m *testing.M) {
	viper.SetConfigFile("../config/test.yaml")
	if err := viper.ReadInConfig(); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
