package failover

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/boyxs/train-go/webook/internal/service/sms"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

type FailoverSmsService struct {
	svcs []sms.SmsService
	idx  uint64 //当前服务商下标
	l    logger.LoggerX
}

func NewFailoverSmsService(svcs []sms.SmsService, l logger.LoggerX) sms.SmsService {
	return &FailoverSmsService{
		svcs: svcs,
		l:    l,
	}
}

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
		f.l.WithContext(ctx).Warn("短信服务商发送失败",
			logger.Uint64("requestIdx", globalIdx),
			logger.Uint64("providerIdx", index),
			logger.Error(err))
	}

	return errors.New("轮询所有服务商均告失败")
}
