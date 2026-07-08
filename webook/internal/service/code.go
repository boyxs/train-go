package service

import (
	"context"
	"errors"
	"fmt"
	"math/rand"

	"github.com/boyxs/train-go/webook/internal/errs"
	"github.com/boyxs/train-go/webook/internal/repository"
	"github.com/boyxs/train-go/webook/internal/service/sms"
)

type CodeService interface {
	Send(ctx context.Context, biz string, phone string) error
	Verify(ctx context.Context, biz string, phone string, code string) (bool, error)
}

type SmsCodeService struct {
	repo repository.CodeRepository
	sms  sms.SmsService
}

func NewSmsCodeService(repo repository.CodeRepository, sms sms.SmsService) CodeService {
	return &SmsCodeService{
		repo: repo,
		sms:  sms,
	}
}

func (cs *SmsCodeService) Send(ctx context.Context, biz string, phone string) error {
	code := cs.genCode()
	err := cs.repo.Store(ctx, biz, phone, code)
	if err != nil {
		return err
	}
	const templateId = "1177249"
	return cs.sms.Send(ctx, templateId, []string{code}, phone)
}

func (cs *SmsCodeService) Verify(ctx context.Context, biz string, phone string, code string) (bool, error) {
	ok, err := cs.repo.Verify(ctx, biz, phone, code)
	if errors.Is(err, errs.ErrCodeVerifyTooMany) {
		return false, err
	}
	return ok, nil
}

func (cs *SmsCodeService) genCode() string {
	// 0-999999
	code := rand.Intn(1000000)
	return fmt.Sprintf("%06d", code)
}
