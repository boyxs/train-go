package repository

import (
	"context"

	"gitee.com/train-cloud/geektime-basic-go/internal/repository/cache"
)

var (
	ErrCodeSendTooMany   = cache.ErrCodeSendTooMany
	ErrCodeVerifyTooMany = cache.ErrCodeVerifyTooMany
)

type CodeRepository interface {
	Store(ctx context.Context, biz string, phone string, code string) error
	Verify(ctx context.Context, biz string, phone string, code string) (bool, error)
}
type RedisCodeRepository struct {
	cache cache.CodeCache
}

func NewRedisCodeRepository(cache cache.CodeCache) CodeRepository {
	return &RedisCodeRepository{
		cache: cache,
	}
}

func (cr *RedisCodeRepository) Store(ctx context.Context, biz string, phone string, code string) error {
	return cr.cache.Store(ctx, biz, phone, code)
}

func (cr *RedisCodeRepository) Verify(ctx context.Context, biz string, phone string, code string) (bool, error) {
	return cr.cache.Verify(ctx, biz, phone, code)
}
