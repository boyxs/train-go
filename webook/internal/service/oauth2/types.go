package oauth2

import (
	"context"

	"github.com/boyxs/train-go/webook/internal/domain"
)

type OAuth2Service interface {
	AuthURL(ctx context.Context, state string) (string, error)
	VerifyCode(ctx context.Context, code string) (domain.WechatAuth, error)
}
