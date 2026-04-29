package cache

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/webook/internal/errs"
)

//go:embed lua/store_code.lua
var luaStoreCode string

//go:embed lua/verify_code.lua
var luaVerifyCode string

type CodeCache interface {
	Store(ctx context.Context, biz string, phone string, code string) error
	Verify(ctx context.Context, biz string, phone string, code string) (bool, error)
}

type RedisCodeCache struct {
	cmd redis.Cmdable
}

func NewRedisCodeCache(cmd redis.Cmdable) CodeCache {
	return &RedisCodeCache{
		cmd: cmd,
	}
}

func (cc *RedisCodeCache) Store(ctx context.Context, biz string, phone string, code string) error {
	result, err := cc.cmd.Eval(ctx, luaStoreCode, []string{cc.getKey(biz, phone)}, code).Int()
	if err != nil {
		return err
	}
	switch result {
	case -2:
		return errs.ErrCodeInvalid
	case -1:
		return errs.ErrCodeSendTooMany
	default:
		return nil
	}
}

func (cc *RedisCodeCache) Verify(ctx context.Context, biz string, phone string, code string) (bool, error) {
	result, err := cc.cmd.Eval(ctx, luaVerifyCode, []string{cc.getKey(biz, phone)}, code).Int()
	if err != nil {
		return false, err
	}
	switch result {
	case -3:
		return false, nil
	case -2:
		return false, errs.ErrCodeVerifyTooMany
	case -1:
		return false, errs.ErrCodeInvalid
	default:
		return true, nil
	}
}

func (cc *RedisCodeCache) getKey(biz string, phone string) string {
	return fmt.Sprintf("code:%s:%s", biz, phone)
}
