package wechat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"gitee.com/train-cloud/geektime-basic-go/internal/service/oauth2"
)

type OAuth2Service struct {
	appId     string
	appSecret string
	client    *http.Client
}

var redirectURI = url.PathEscape("https://yourdomain/oauth2/wechat/callback")

func NewOAuth2Service(appId string, appSecret string) oauth2.OAuth2Service {
	return &OAuth2Service{
		appId:     appId,
		appSecret: appSecret,
		client:    http.DefaultClient,
	}
}

func (os *OAuth2Service) AuthURL(ctx context.Context, state string) (string, error) {
	authURLPattern := `https://open.weixin.qq.com/connect/qrconnect?appid=%s&redirect_uri=%s&response_type=code&scope=snsapi_login&state=%s#wechat_redirect`
	return fmt.Sprintf(authURLPattern, os.appId, redirectURI, state), nil
}

func (os *OAuth2Service) VerifyCode(ctx context.Context, code string) (domain.WechatAuth, error) {
	accessTokenURLPattern := fmt.Sprintf(`https://api.weixin.qq.com/sns/oauth2/access_token?appid=%s&secret=%s&code=%s&grant_type=authorization_code`, os.appId, os.appSecret, code)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, accessTokenURLPattern, nil)
	if err != nil {
		return domain.WechatAuth{}, err
	}
	httpResp, err := os.client.Do(req)
	if err != nil {
		return domain.WechatAuth{}, err
	}

	var result Result
	err = json.NewDecoder(httpResp.Body).Decode(&result)
	if err != nil {
		return domain.WechatAuth{}, err
	}
	if result.ErrCode != 0 {
		return domain.WechatAuth{},
			fmt.Errorf("调用微信接口失败 errcode %d, errmsg %s", result.ErrCode, result.ErrMsg)
	}
	return domain.WechatAuth{UnionId: result.UnionId, OpenId: result.OpenId}, nil
}

type Result struct {
	AccessToken string `json:"access_token"`
	// access_token接口调用凭证超时时间，单位（秒）
	ExpiresIn int64 `json:"expires_in"`
	// 用户刷新access_token
	RefreshToken string `json:"refresh_token"`
	// 授权用户唯一标识
	OpenId string `json:"openid"`
	// 用户授权的作用域，使用逗号（,）分隔
	Scope string `json:"scope"`
	// 当且仅当该网站应用已获得该用户的userinfo授权时，才会出现该字段
	UnionId string `json:"unionid"`
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}
