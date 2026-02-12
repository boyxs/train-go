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

type ICodeService interface {
	Send(ctx context.Context, biz string, phone string) error
	Verify(ctx context.Context, biz string, phone string, code string) (bool, error)
}

type CodeService struct {
	repo repository.ICodeRepository
	sms  sms.ISmsService
}

func (cs *CodeService) Send(ctx context.Context, biz string, phone string) error {
	code := cs.genCode()
	err := cs.repo.Store(ctx, biz, phone, code)
	if err != nil {
		return err
	}
	const templateId = "1877556"
	return cs.sms.Send(ctx, templateId, []string{code}, phone)
}

func (cs *CodeService) Verify(ctx context.Context, biz string, phone string, code string) (bool, error) {
	ok, err := cs.repo.Verify(ctx, biz, phone, code)
	if errors.Is(err, ErrCodeVerifyTooMany) {
		return false, err
	}
	return ok, nil
}

func NewCodeService(repo repository.ICodeRepository, sms sms.ISmsService) ICodeService {
	return &CodeService{
		repo: repo,
		sms:  sms,
	}
}

func (cs *CodeService) genCode() string {
	// 0-999999
	code := rand.Intn(1000000)
	return fmt.Sprintf("%06d", code)
}
