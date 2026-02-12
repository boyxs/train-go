package cache

import (
	"context"
	_ "embed"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
)

var (
	ErrCodeInvalid       = errors.New("验证码错误或已过期")
	ErrCodeSendTooMany   = errors.New("验证码发送太频繁")
	ErrCodeVerifyTooMany = errors.New("验证码校验太频繁")
)

//go:embed lua/store_code.lua
var luaStoreCode string

//go:embed lua/verify_code.lua
var luaVerifyCode string

type ICodeCache interface {
	Store(ctx context.Context, biz string, phone string, code string) error
	Verify(ctx context.Context, biz string, phone string, code string) (bool, error)
}

type CodeCache struct {
	cmd redis.Cmdable
}

func (cc *CodeCache) Store(ctx context.Context, biz string, phone string, code string) error {
	result, err := cc.cmd.Eval(ctx, luaStoreCode, []string{cc.getKey(biz, phone)}, code).Int()
	if err != nil {
		return err
	}
	switch result {
	case -2:
		return ErrCodeInvalid
	case -1:
		return ErrCodeSendTooMany
	default:
		return nil
	}
}

func (cc *CodeCache) Verify(ctx context.Context, biz string, phone string, code string) (bool, error) {
	result, err := cc.cmd.Eval(ctx, luaVerifyCode, []string{cc.getKey(biz, phone)}, code).Int()
	if err != nil {
		return false, err
	}
	switch result {
	case -3:
		return false, nil
	case -2:
		return false, ErrCodeVerifyTooMany
	case -1:
		return false, ErrCodeInvalid
	default:
		return true, nil
	}
}

func NewCodeCache(cmd redis.Cmdable) ICodeCache {
	return &CodeCache{
		cmd: cmd,
	}
}

func (cc *CodeCache) getKey(biz string, phone string) string {
	return fmt.Sprintf("code:%s:%s", biz, phone)
}
