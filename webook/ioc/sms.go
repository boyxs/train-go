package ioc

import (
	"os"

	sms "gitee.com/train-cloud/geektime-basic-go/internal/service/sms"
	"gitee.com/train-cloud/geektime-basic-go/internal/service/sms/memory"
	"gitee.com/train-cloud/geektime-basic-go/internal/service/sms/tencent"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	tsms "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/sms/v20210111"
)

func InitSmsService() sms.SmsService {
	//return initTencentSmsService()
	return memory.NewSmsService()
}

func initTencentSmsService() sms.SmsService {
	secretId, ok := os.LookupEnv("SMS_SECRET_ID")
	if !ok {
		panic("找不到腾讯 SMS 的 secret id")
	}
	secretKey, ok := os.LookupEnv("SMS_SECRET_KEY")
	if !ok {
		panic("找不到腾讯 SMS 的 secret key")
	}
	c, err := tsms.NewClient(
		common.NewCredential(secretId, secretKey),
		"ap-nanjing",
		profile.NewClientProfile(),
	)
	if err != nil {
		panic(err)
	}
	return tencent.NewSmsService(c, "1400842696", "Tommy")
}
