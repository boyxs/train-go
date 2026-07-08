package ioc

import (
	"os"

	"github.com/redis/go-redis/v9"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	tsms "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/sms/v20210111"

	sms "github.com/boyxs/train-go/webook/internal/service/sms"
	"github.com/boyxs/train-go/webook/internal/service/sms/memory"
	"github.com/boyxs/train-go/webook/internal/service/sms/tencent"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

func InitSmsService(cmd redis.Cmdable, l logger.LoggerX) sms.SmsService {
	//return initTencentSmsService()
	//return initLocalTencentSmsService()
	return memory.NewSmsService(l)
	//这里测试需要忽略Store返回的错误
	//type CodeService interface {
	//	Send(ctx context.Context, biz string, phone string) error
	//}
	//return ratelimit.NewRateLimitSmsService(
	//	memory.NewSmsService(),
	//	ratelimit2.NewRedisSlidingWindowLimiter(cmd, time.Second, 20),
	//)
}
func initLocalTencentSmsService() sms.SmsService {
	secretId := "AKIDNbYFenfGZbMp1wbtynMgla9SWKsACyUn"
	secretKey := "5uTu50f4E8mAJDrGQL0PYS7WfYCbkdHg"
	c, err := tsms.NewClient(
		common.NewCredential(secretId, secretKey),
		"ap-guangzhou",
		profile.NewClientProfile(),
	)
	if err != nil {
		panic(err)
	}
	return tencent.NewSmsService(c, "1400587666", "传智播客")
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
		"ap-guangzhou",
		profile.NewClientProfile(),
	)
	if err != nil {
		panic(err)
	}
	return tencent.NewSmsService(c, "1400587666", "传智播客")
}
