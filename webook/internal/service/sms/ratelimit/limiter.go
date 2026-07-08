package ratelimit

import (
	"context"
	"errors"

	"github.com/boyxs/train-go/webook/internal/service/sms"
	"github.com/boyxs/train-go/webook/pkg/ratelimit"
)

var errLimited = errors.New("触发短信限流")

type RateLimitSmsService struct {
	svc     sms.SmsService //被装饰者
	limiter ratelimit.Limiter
	key     string
}

func NewRateLimitSmsService(sms sms.SmsService, limiter ratelimit.Limiter) sms.SmsService {
	return &RateLimitSmsService{
		svc:     sms,
		limiter: limiter,
		key:     "sms-limiter",
	}
}

func (r *RateLimitSmsService) Send(ctx context.Context, templateId string, args []string, phoneNumbers ...string) error {
	limit, err := r.limiter.Limit(ctx, r.key)
	if err != nil {
		return err
	}
	if limit {
		return errLimited
	}
	return r.svc.Send(ctx, templateId, args, phoneNumbers...)
}
