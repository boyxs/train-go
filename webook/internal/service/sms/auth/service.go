package auth

import (
	"context"

	"gitee.com/train-cloud/geektime-basic-go/internal/service/sms"
	"github.com/golang-jwt/jwt/v5"
)

type AuthSmsService struct {
	svc sms.SmsService
	key []byte
}

func (a *AuthSmsService) Send(ctx context.Context, tplToken string, args []string, phoneNumbers ...string) error {
	var claims SmsClaims
	_, err := jwt.ParseWithClaims(tplToken, &claims, func(t *jwt.Token) (any, error) {
		return a.key, nil
	})
	if err != nil {
		return err
	}
	return a.svc.Send(ctx, claims.TemplateId, args, phoneNumbers...)
}

func NewAuthSmsService(svc sms.SmsService, key []byte) sms.SmsService {
	return &AuthSmsService{
		svc: svc,
		key: key,
	}
}

type SmsClaims struct {
	jwt.RegisteredClaims
	TemplateId string
}
