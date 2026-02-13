package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gitee.com/train-cloud/geektime-basic-go/internal/integration/setup"
	"gitee.com/train-cloud/geektime-basic-go/internal/web"
	"github.com/stretchr/testify/assert"
)

func TestInternalUserHandler_SendSMSCode(t *testing.T) {
	server := setup.InitWebServer()
	cmd := setup.InitRedis()

	getKeyFunc := func(biz string, phone string) string {
		return fmt.Sprintf("code:%s:%s", biz, phone)
	}
	testCases := []struct {
		name     string
		phone    string
		before   func(t *testing.T)
		after    func(t *testing.T)
		wantCode int
		wantBody web.Result
	}{
		{
			name:   "验证码发送成功",
			phone:  "18608261234",
			before: func(t *testing.T) {},
			after: func(t *testing.T) {
				ctx, cancelFunc := context.WithTimeout(context.Background(), time.Second*10)
				defer cancelFunc()

				key := getKeyFunc("login", "18608261234")
				code, err := cmd.Get(ctx, key).Result()
				assert.NoError(t, err)
				assert.True(t, len(code) > 0)

				ttl, err := cmd.TTL(ctx, key).Result()
				assert.NoError(t, err)
				assert.True(t, ttl > time.Minute*9+time.Second+50)

				err = cmd.Del(ctx, key).Err()
				assert.NoError(t, err)
			},
			wantCode: http.StatusOK,
			wantBody: web.Result{
				Msg: "发送成功",
			},
		},
		{
			name:     "未输入手机号",
			before:   func(t *testing.T) {},
			after:    func(t *testing.T) {},
			wantCode: http.StatusOK,
			wantBody: web.Result{
				Code: 4,
				Msg:  "请输入手机号码",
			},
		},
		{
			name:  "验证码发送太频繁",
			phone: "18608261234",
			before: func(t *testing.T) {
				ctx, cancelFunc := context.WithTimeout(context.Background(), time.Second*10)
				defer cancelFunc()

				key := getKeyFunc("login", "18608261234")

				err := cmd.Set(ctx, key, "123456", time.Minute*10).Err()
				assert.NoError(t, err)
			},
			after: func(t *testing.T) {
				ctx, cancelFunc := context.WithTimeout(context.Background(), time.Second*10)
				defer cancelFunc()

				key := getKeyFunc("login", "18608261234")
				code, err := cmd.GetDel(ctx, key).Result()
				assert.NoError(t, err)
				assert.True(t, len(code) == 6)
				assert.Equal(t, "123456", code)

			},
			wantCode: http.StatusOK,
			wantBody: web.Result{
				Code: 4,
				Msg:  "短信发送太频繁，请稍后再试",
			},
		},
		{
			name:  "系统错误",
			phone: "18608261234",
			before: func(t *testing.T) {
				ctx, cancelFunc := context.WithTimeout(context.Background(), time.Second*10)
				defer cancelFunc()

				key := getKeyFunc("login", "18608261234")

				err := cmd.Set(ctx, key, "123456", 0).Err()
				assert.NoError(t, err)
			},
			after: func(t *testing.T) {
				ctx, cancelFunc := context.WithTimeout(context.Background(), time.Second*10)
				defer cancelFunc()

				key := getKeyFunc("login", "18608261234")
				code, err := cmd.GetDel(ctx, key).Result()
				assert.NoError(t, err)
				assert.Equal(t, "123456", code)
			},
			wantCode: http.StatusOK,
			wantBody: web.Result{
				Code: 5,
				Msg:  "系统错误",
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.before(t)
			defer tc.after(t)

			req, err := http.NewRequest(
				http.MethodPost, "/user/login_sms/code/send",
				bytes.NewReader([]byte(fmt.Sprintf(`{"phone": "%s"}`, tc.phone))))
			req.Header.Add("Content-Type", "application/json")
			assert.NoError(t, err)
			recorder := httptest.NewRecorder()
			server.ServeHTTP(recorder, req)

			//if tc.wantCode != http.StatusOK {
			//	return
			//}
			var result web.Result
			err = json.NewDecoder(recorder.Body).Decode(&result)
			assert.NoError(t, err)
			assert.Equal(t, tc.wantBody, result)
		})
	}
}
