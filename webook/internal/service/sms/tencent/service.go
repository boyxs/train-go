package tencent

import (
	"context"
	"fmt"

	"gitee.com/train-cloud/geektime-basic-go/internal/service/sms"
	"github.com/ecodeclub/ekit"
	"github.com/ecodeclub/ekit/slice"
	tsms "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/sms/v20210111"
)

type SmsService struct {
	client   *tsms.Client
	appId    *string
	signName *string
}

func NewSmsService(client *tsms.Client, appId string, signName string) sms.SmsService {
	return &SmsService{
		client:   client,
		appId:    &appId,
		signName: &signName,
	}
}

func (s *SmsService) Send(ctx context.Context, templateId string, args []string, phoneNumbers ...string) error {
	request := tsms.NewSendSmsRequest()
	request.SetContext(ctx)
	request.SmsSdkAppId = s.appId
	request.SignName = s.signName
	request.TemplateId = ekit.ToPtr[string](templateId)
	request.TemplateParamSet = s.toPtrSlice(args)
	request.PhoneNumberSet = s.toPtrSlice(phoneNumbers)
	response, err := s.client.SendSms(request)
	if err != nil {
		return err
	}
	for _, statusPtr := range response.Response.SendStatusSet {
		if statusPtr == nil {
			continue
		}
		status := *statusPtr
		if status.Code != nil || *(status.Code) != "Ok" {
			return fmt.Errorf("发送短信失败 code: %s, msg: %s", *status.Code, *status.Message)
		}
	}
	return nil
}

func (s *SmsService) toPtrSlice(data []string) []*string {
	return slice.Map[string, *string](data, func(idx int, src string) *string {
		return &src
	})
}
