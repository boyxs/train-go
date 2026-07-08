package failover

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"

	"github.com/boyxs/train-go/webook/internal/service/sms"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

type TimeoutFailoverSmsService struct {
	svcs      []sms.SmsService
	idx       int32 //当前使用的服务商下标
	cnt       int32 //连续超时的计数
	threshold int32 //触发切换的超时阈值
	l         logger.LoggerX

	//仅测试用：在 CAS 之前注入的钩子，生产代码此字段为 nil
	beforeCAS func()
}

func NewTimeoutFailoverSmsService(svcs []sms.SmsService, threshold int32, l logger.LoggerX) sms.SmsService {
	if len(svcs) == 0 {
		panic("短信服务商列表不能为空")
	}
	if threshold <= 0 {
		threshold = 3 //默认连续3次超时就切换
	}
	return &TimeoutFailoverSmsService{
		svcs:      svcs,
		threshold: threshold,
		l:         l,
	}
}

func (t *TimeoutFailoverSmsService) Send(ctx context.Context, templateId string, args []string, phoneNumbers ...string) error {
	idx := atomic.LoadInt32(&t.idx)
	cnt := atomic.LoadInt32(&t.cnt)
	if cnt >= t.threshold {
		newIdx := (idx + 1) % int32(len(t.svcs))
		//仅测试Start
		if t.beforeCAS != nil {
			t.beforeCAS()
		}
		//仅测试End
		if atomic.CompareAndSwapInt32(&t.idx, idx, newIdx) {
			atomic.StoreInt32(&t.cnt, 0)
			t.l.Warn("SMS主动切换成功",
				logger.Int32("consecutiveFails", cnt),
				logger.Int32("from", idx),
				logger.Int32("to", newIdx))
			idx = newIdx
		} else {
			newActualIdx := atomic.LoadInt32(&t.idx)
			t.l.Warn("SMS切换并发竞争",
				logger.Int32("expected", newIdx),
				logger.Int32("actual", newActualIdx))
			idx = newActualIdx
		}
	}
	svs := t.svcs[idx]
	err := svs.Send(ctx, templateId, args, phoneNumbers...)
	switch {
	case err == nil:
		//成功则彻底清除连续故障计数
		atomic.StoreInt32(&t.cnt, 0)
		return nil
	case errors.Is(err, context.DeadlineExceeded):
		//记录超时次数
		atomic.AddInt32(&t.cnt, 1)
		return err
	case errors.Is(err, context.Canceled):
		//客户端取消不计入故障
		return err
	default:
		if t.isCriticalError(err) {
			atomic.AddInt32(&t.cnt, 1)
		}
		t.l.Warn("SMS服务商调用失败",
			logger.Int32("providerIdx", idx),
			logger.Error(err))
	}
	return err
}

func (t *TimeoutFailoverSmsService) isCriticalError(err error) bool {
	// 生产环境建议匹配具体的 SDK Error Type
	msg := err.Error()
	criticalKeywords := []string{"EOF", "refused", "reset", "500", "502", "503", "504"}
	for _, key := range criticalKeywords {
		if strings.Contains(strings.ToUpper(msg), key) {
			return true
		}
	}
	return false
}
