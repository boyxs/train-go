package failover

import (
	"context"
	"errors"
	"log"
	"sync/atomic"

	"gitee.com/train-cloud/geektime-basic-go/internal/service/sms"
)

type FailoverSmsService struct {
	svcs []sms.SmsService
	idx  uint64 //当前服务商下标
}

// Send 普通轮询
//func (f *FailoverSmsService) Send(ctx context.Context, templateId string, args []string, phoneNumbers ...string) error {
//	for _, svc := range f.svcs {
//		err := svc.Send(ctx, templateId, args, phoneNumbers...)
//		if err == nil {
//			return nil
//		}
//		log.Fatalln(err)
//	}
//	return errors.New("轮询所有服务商均告失败")
//}

// Send 严格轮询
func (f *FailoverSmsService) Send(ctx context.Context, templateId string, args []string, phoneNumbers ...string) error {
	svcs := f.svcs
	length := uint64(len(svcs))
	if length == 0 {
		return errors.New("没有可用的短信服务商")
	}
	globalIdx := atomic.AddUint64(&f.idx, 1)
	for i := uint64(0); i < length; i++ {
		index := (globalIdx + i - 1) % length
		svc := svcs[index]
		err := svc.Send(ctx, templateId, args, phoneNumbers...)
		switch {
		case err == nil:
			return nil
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return err
		}
		log.Printf("并发请求序号 %d 尝试服务商 %d 失败: %v", globalIdx, index, err)
	}

	return errors.New("轮询所有服务商均告失败")
}

func NewFailoverSmsService(svcs []sms.SmsService) sms.SmsService {
	return &FailoverSmsService{
		svcs: svcs,
	}
}
