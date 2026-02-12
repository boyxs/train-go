package memory

import (
	"context"
	"log"

	"gitee.com/train-cloud/geektime-basic-go/internal/service/sms"
)

type SmsService struct {
}

func NewSmsService() sms.ISmsService {
	return &SmsService{}
}

func (s *SmsService) Send(ctx context.Context, templateId string, args []string, phoneNumbers ...string) error {
	log.Println("code ", args)
	return nil
}
