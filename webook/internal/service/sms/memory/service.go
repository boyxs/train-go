package memory

import (
	"context"

	"gitee.com/train-cloud/geektime-basic-go/internal/service/sms"
	"gitee.com/train-cloud/geektime-basic-go/pkg/logger"
)

type SmsService struct {
	l logger.LoggerX
}

func NewSmsService(l logger.LoggerX) sms.SmsService {
	return &SmsService{l: l}
}

func (s *SmsService) Send(ctx context.Context, templateId string, args []string, phoneNumbers ...string) error {
	s.l.Debug("发送验证码", logger.Strings("code", args))
	return nil
}
