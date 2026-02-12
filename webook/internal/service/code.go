package service

import (
	"context"
	"errors"
	"fmt"
	"math/rand"

	"gitee.com/train-cloud/geektime-basic-go/internal/repository"
	"gitee.com/train-cloud/geektime-basic-go/internal/service/sms"
)

var (
	ErrCodeSendTooMany   = repository.ErrCodeSendTooMany
	ErrCodeVerifyTooMany = repository.ErrCodeVerifyTooMany
)

type CodeService interface {
	Send(ctx context.Context, biz string, phone string) error
	Verify(ctx context.Context, biz string, phone string, code string) (bool, error)
}

type SmsCodeService struct {
	repo repository.CodeRepository
	sms  sms.SmsService
}

func (cs *SmsCodeService) Send(ctx context.Context, biz string, phone string) error {
	code := cs.genCode()
	err := cs.repo.Store(ctx, biz, phone, code)
	if err != nil {
		return err
	}
	const templateId = "1877556"
	return cs.sms.Send(ctx, templateId, []string{code}, phone)
}

func (cs *SmsCodeService) Verify(ctx context.Context, biz string, phone string, code string) (bool, error) {
	ok, err := cs.repo.Verify(ctx, biz, phone, code)
	if errors.Is(err, ErrCodeVerifyTooMany) {
		return false, err
	}
	return ok, nil
}

func NewSmsCodeService(repo repository.CodeRepository, sms sms.SmsService) CodeService {
	return &SmsCodeService{
		repo: repo,
		sms:  sms,
	}
}

func (cs *SmsCodeService) genCode() string {
	// 0-999999
	code := rand.Intn(1000000)
	return fmt.Sprintf("%06d", code)
}
