package auth

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/webook/internal/service/sms"
	smsmocks "github.com/webook/internal/service/sms/mocks"
)

func TestAuthSmsService_Send(t *testing.T) {
	testCases := []struct {
		name     string
		mock     func(ctrl *gomock.Controller) sms.SmsService
		key      []byte
		tplToken func(key []byte) string

		wantErr error
	}{
		{
			name: "发送消息成功",
			key:  []byte("secret-key-123456"),
			mock: func(ctrl *gomock.Controller) sms.SmsService {
				svc := smsmocks.NewMockSmsService(ctrl)
				svc.EXPECT().Send(gomock.Any(), "tpl_id_001", []string{"123"}, "188...").
					Return(nil)
				return svc
			},
			tplToken: func(key []byte) string {
				claims := &SmsClaims{
					TemplateId: "tpl_id_001",
				}
				token := jwt.NewWithClaims(jwt.SigningMethodHS512, claims)
				tokenStr, _ := token.SignedString(key)
				return tokenStr
			},
			wantErr: nil,
		},
		{
			name: "Token过期报错",
			key:  []byte("secret-key-123456"),
			mock: func(ctrl *gomock.Controller) sms.SmsService {
				svc := smsmocks.NewMockSmsService(ctrl)
				return svc
			},
			tplToken: func(key []byte) string {
				claims := &SmsClaims{
					TemplateId: "tpl_id_001",
					RegisteredClaims: jwt.RegisteredClaims{
						// 立即过期
						ExpiresAt: jwt.NewNumericDate(time.Now()),
					},
				}
				token := jwt.NewWithClaims(jwt.SigningMethodHS512, claims)
				tokenStr, _ := token.SignedString(key)
				return tokenStr
			},
			wantErr: jwt.ErrTokenExpired,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			svc := NewAuthSmsService(tc.mock(ctrl), tc.key)
			tokenStr := tc.tplToken(tc.key)
			err := svc.Send(context.Background(), tokenStr, []string{"123"}, "188...")

			if tc.name != "Token过期报错" {
				assert.Equal(t, tc.wantErr, err)
				return
			}
			assert.ErrorContains(t, err, tc.wantErr.Error())
		})
	}
}
