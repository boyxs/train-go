package tencent

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	sms "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/sms/v20210111"
)

func TestSend(t *testing.T) {
	secretId, ok := os.LookupEnv("SMS_SECRET_ID")
	if !ok {
		t.Skip("SMS_SECRET_ID 未设置，跳过腾讯云短信真实调用测试")
	}
	secretKey, ok := os.LookupEnv("SMS_SECRET_KEY")
	if !ok {
		t.Skip("SMS_SECRET_KEY 未设置，跳过腾讯云短信真实调用测试")
	}
	c, err := sms.NewClient(common.NewCredential(secretId, secretKey),
		"ap-nanjing",
		profile.NewClientProfile())
	if err != nil {
		t.Fatal(err)
	}
	s := NewSmsService(c, "1400842696", "Tommy")
	testCases := []struct {
		name         string
		templateId   string
		params       []string
		phoneNumbers []string
		wantErr      error
	}{
		{
			name:         "发送验证码",
			templateId:   "1877556",
			params:       []string{"123456"},
			phoneNumbers: []string{"18608261234"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			er := s.Send(context.Background(), tc.templateId, tc.params, tc.phoneNumbers...)
			assert.Equal(t, tc.wantErr, er)
		})
	}
}
