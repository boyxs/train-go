package ioc

import (
	"os"
	"time"

	sms "gitee.com/train-cloud/geektime-basic-go/internal/service/sms"
	"gitee.com/train-cloud/geektime-basic-go/internal/service/sms/memory"
	"gitee.com/train-cloud/geektime-basic-go/internal/service/sms/ratelimit"
	"gitee.com/train-cloud/geektime-basic-go/internal/service/sms/tencent"
	ratelimit2 "gitee.com/train-cloud/geektime-basic-go/pkg/ratelimit"
	"github.com/redis/go-redis/v9"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	tsms "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/sms/v20210111"
)

func InitSmsService(cmd redis.Cmdable) sms.SmsService {
	//return initTencentSmsService()
	//return memory.NewSmsService()
	//这里测试需要忽略Store返回的错误
	//type CodeService interface {
	//	Send(ctx context.Context, biz string, phone string) error
	//}
	return ratelimit.NewRateLimitSmsService(
		memory.NewSmsService(),
		ratelimit2.NewRedisSlidingWindowLimiter(cmd, time.Second, 20),
	)
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
