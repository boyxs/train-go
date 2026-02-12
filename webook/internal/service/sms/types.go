package sms

import "context"

type SmsService interface {
	Send(ctx context.Context, templateId string, args []string, phoneNumbers ...string) error
}
