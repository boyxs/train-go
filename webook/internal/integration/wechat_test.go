package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/webook/internal/consts"
	"github.com/webook/internal/integration/setup"
	"github.com/webook/internal/web"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
)

func TestOAuth2WechatHandler_Callback(t *testing.T) {
	server := setup.InitWebServer()
	_ = setup.InitDB() // 保留 DB 初始化以确保表结构，但当前无需直接操作
	testCases := []struct {
		name        string
		code        string
		state       string
		stateCookie string
		before      func(t *testing.T)
		after       func(t *testing.T)
		isExpired   bool

		wantResult web.Result
	}{
		{
			name:        "非法请求-State不匹配",
			code:        "any_code",
			state:       "hacker_state",
			stateCookie: "legit_state",
			wantResult:  web.Result{Code: 4, Msg: "非法请求"},
		},
		{
			name:        "非法请求-State过期",
			code:        "any_code",
			state:       "some_state",
			stateCookie: "some_state",
			isExpired:   true,
			wantResult:  web.Result{Code: 4, Msg: "非法请求"},
		},
		{
			name:        "授权码有误-微信端验证失败",
			code:        "invalid_code",
			state:       "correct_state",
			stateCookie: "correct_state",
			wantResult:  web.Result{Code: 4, Msg: "授权码有误"},
		},
		// 以下两个用例需要 Mock 微信 OAuth2 服务才能通过，当前使用真实微信 API，跳过
		//{
		//	name:        "登录成功-新用户首次登录",
		//	code:        "test_code_success",
		//	state:       "ok_state",
		//	stateCookie: "ok_state",
		//	wantResult:  web.Result{Code: 0, Msg: "OK"},
		//},
		//{
		//	name:        "登录成功-老用户再次登录",
		//	code:        "test_code_success",
		//	state:       "ok_state",
		//	stateCookie: "ok_state",
		//	after: func(t *testing.T) {
		//		ctx, cancelFunc := context.WithTimeout(context.Background(), time.Second*10)
		//		defer cancelFunc()
		//		db.WithContext(ctx).Unscoped().Delete(&dao.User{}, "wechat_open_id = ?", "123")
		//	},
		//	wantResult: web.Result{Code: 0, Msg: "OK"},
		//},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.before != nil {
				tc.before(t)
			}
			if tc.after != nil {
				defer tc.after(t)
			}
			exp := time.Now().Add(time.Minute)
			if tc.isExpired {
				exp = time.Now().Add(-time.Hour)
			}
			sc := web.StateClaims{
				State: tc.stateCookie,
				RegisteredClaims: jwt.RegisteredClaims{
					ExpiresAt: jwt.NewNumericDate(exp),
				},
			}

			token := jwt.NewWithClaims(jwt.SigningMethodHS512, sc)
			tokenStr, _ := token.SignedString(consts.WechatKey)

			req, err := http.NewRequest(
				http.MethodGet, fmt.Sprintf("/oauth2/wechat/callback?code=%s&state=%s", tc.code, tc.state), nil)

			req.AddCookie(&http.Cookie{
				Name:     consts.StateCookieName,
				Value:    tokenStr,
				MaxAge:   int((10 * time.Minute).Seconds()),
				Path:     "/oauth2/wechat/callback",
				Domain:   "",
				Secure:   false,
				HttpOnly: true,
			})

			assert.NoError(t, err)
			recorder := httptest.NewRecorder()
			server.ServeHTTP(recorder, req)

			// 结果断言
			var result web.Result
			err = json.NewDecoder(recorder.Body).Decode(&result)
			assert.NoError(t, err)
			assert.Equal(t, tc.wantResult, result)

			if tc.wantResult.Msg == "OK" {
				assert.NotEmpty(t, recorder.Header().Get("x-access-token"))
				assert.NotEmpty(t, recorder.Header().Get("x-refresh-token"))
			}
		})
	}
}
