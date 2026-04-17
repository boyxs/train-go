package ioc

import (
	"os"

	"github.com/webook/internal/service/oauth2"
	"github.com/webook/internal/service/oauth2/wechat"
)

func InitWechatOAuth2Service() oauth2.OAuth2Service {
	//return wechat.NewOAuth2Service(loadConfig())
	return wechat.NewOAuth2Service(newLocalConfig())
}

func newLocalConfig() (string, string) {
	appId := "wx8df2c2204bfa6d35"
	appSecret := "19d8999dad273b9a1396f88462117401"
	return appId, appSecret
}

func loadConfig() (string, string) {
	appId, ok := os.LookupEnv("WECHAT_APP_ID")
	if !ok {
		panic("找不到环境变量 WECHAT_APP_ID")
	}
	appSecret, ok := os.LookupEnv("WECHAT_APP_SECRET")
	if !ok {
		panic("找不到环境变量 WECHAT_APP_SECRET")
	}
	return appId, appSecret
}
