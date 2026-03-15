package integration

import (
	"os"
	"testing"

	"github.com/spf13/viper"
)

func TestMain(m *testing.M) {
	viper.SetConfigFile("../../config/dev.yaml")
	if err := viper.ReadInConfig(); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
