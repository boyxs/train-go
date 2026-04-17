package ratelimit

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/webook/internal/service/sms"
	smsmocks "github.com/webook/internal/service/sms/mocks"
	"github.com/webook/pkg/ratelimit"
	limitmocks "github.com/webook/pkg/ratelimit/mocks"
)

func TestRateLimitSmsService_Send(t *testing.T) {
	testCases := []struct {
		name    string
		mock    func(ctrl *gomock.Controller) (sms.SmsService, ratelimit.Limiter)
		wantErr error
	}{
		{
			name: "未触发限流",
			mock: func(ctrl *gomock.Controller) (sms.SmsService, ratelimit.Limiter) {
				limiter := limitmocks.NewMockLimiter(ctrl)
				smsService := smsmocks.NewMockSmsService(ctrl)

				limiter.EXPECT().
					Limit(gomock.Any(), gomock.Any()).
					Return(false, nil)
				smsService.EXPECT().
					Send(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)
				return smsService, limiter
			},
			wantErr: nil,
		},
		{
			name: "触发限流",
			mock: func(ctrl *gomock.Controller) (sms.SmsService, ratelimit.Limiter) {
				limiter := limitmocks.NewMockLimiter(ctrl)
				smsService := smsmocks.NewMockSmsService(ctrl)

				limiter.EXPECT().
					Limit(gomock.Any(), gomock.Any()).
					Return(true, nil)
				return smsService, limiter
			},
			wantErr: errLimited,
		},
		{
			name: "限流出错",
			mock: func(ctrl *gomock.Controller) (sms.SmsService, ratelimit.Limiter) {
				limiter := limitmocks.NewMockLimiter(ctrl)
				smsService := smsmocks.NewMockSmsService(ctrl)

				limiter.EXPECT().
					Limit(gomock.Any(), gomock.Any()).
					Return(true, errors.New("limit error"))
				return smsService, limiter
			},
			wantErr: errors.New("limit error"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			svc := NewRateLimitSmsService(tc.mock(ctrl))
			err := svc.Send(context.Background(), "tpl", []string{"123"}, "188...")
			assert.Equal(t, tc.wantErr, err)
		})
	}
}
