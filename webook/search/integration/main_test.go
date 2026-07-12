package integration

import (
	"os"
	"testing"

	"github.com/spf13/viper"
)

// TestMain 加载 search/config/test.yaml，data.es 指向本机 ES。
func TestMain(m *testing.M) {
	viper.SetConfigFile("../config/test.yaml")
	if err := viper.ReadInConfig(); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
