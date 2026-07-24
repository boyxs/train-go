package integration

import (
	"os"
	"testing"

	"github.com/spf13/viper"
)

// TestMain 加载 feed/config/test.yaml（连本机测试 Redis；feed 无 MySQL）。
func TestMain(m *testing.M) {
	viper.SetConfigFile("../config/test.yaml")
	if err := viper.ReadInConfig(); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
