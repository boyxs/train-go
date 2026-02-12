package sms

import "context"

type ISmsService interface {
	Send(ctx context.Context, templateId string, args []string, phoneNumbers ...string) error
}
