package repository

import (
	"context"

	"gitee.com/train-cloud/geektime-basic-go/internal/repository/cache"
)

var (
	ErrCodeSendTooMany   = cache.ErrCodeSendTooMany
	ErrCodeVerifyTooMany = cache.ErrCodeVerifyTooMany
)

type ICodeRepository interface {
	Store(ctx context.Context, biz string, phone string, code string) error
	Verify(ctx context.Context, biz string, phone string, code string) (bool, error)
}
type CodeRepository struct {
	cache cache.ICodeCache
}

func (cr *CodeRepository) Store(ctx context.Context, biz string, phone string, code string) error {
	return cr.cache.Store(ctx, biz, phone, code)
}

func (cr *CodeRepository) Verify(ctx context.Context, biz string, phone string, code string) (bool, error) {
	return cr.cache.Verify(ctx, biz, phone, code)
}

func NewCodeRepository(cache cache.ICodeCache) ICodeRepository {
	return &CodeRepository{
		cache: cache,
	}
}
