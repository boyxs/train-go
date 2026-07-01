package integration

import (
	"os"
	"testing"

	"github.com/spf13/viper"

	"github.com/webook/internal/consts"
	"github.com/webook/pkg/ginx"
)

func TestMain(m *testing.M) {
	// 路径相对 internal/integration 包目录：../config/test.yaml = internal/config/test.yaml
	viper.SetConfigFile("../config/test.yaml")
	if err := viper.ReadInConfig(); err != nil {
		panic(err)
	}
	// 集成测试自建 gin server（不走 ioc.InitWebServer），手动对齐 ginx.UserKey 与生产，
	// 供 handler 的 ginx.MustClaims/Claims 取到测试注入的登录态（consts.UserKey）。
	ginx.UserKey = consts.UserKey
	os.Exit(m.Run())
}
