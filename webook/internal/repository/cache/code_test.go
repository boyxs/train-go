package cache

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/webook/internal/repository/cache/redismocks"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestRedisCodeCache_Store(t *testing.T) {
	getKeyFunc := func(biz string, phone string) string {
		return fmt.Sprintf("code:%s:%s", biz, phone)
	}

	testCases := []struct {
		name string

		mock func(ctrl *gomock.Controller) redis.Cmdable

		ctx   context.Context
		biz   string
		phone string
		code  string

		wantErr error
	}{
		{
			name: "缓存成功",
			mock: func(ctrl *gomock.Controller) redis.Cmdable {
				cmd := redismocks.NewMockCmdable(ctrl)
				r := redis.NewCmd(context.Background())
				r.SetErr(nil)
				r.SetVal(int64(0))
				cmd.EXPECT().Eval(gomock.Any(), luaStoreCode,
					[]string{getKeyFunc("test", "18608261234")},
					[]any{"123456"},
				).Return(r)
				return cmd
			},
			ctx:     context.Background(),
			biz:     "test",
			phone:   "18608261234",
			code:    "123456",
			wantErr: nil,
		},
		{
			name: "redis 返回 error",
			mock: func(ctrl *gomock.Controller) redis.Cmdable {
				cmd := redismocks.NewMockCmdable(ctrl)
				r := redis.NewCmd(context.Background())
				r.SetErr(errors.New("redis error"))
				cmd.EXPECT().Eval(gomock.Any(), luaStoreCode,
					[]string{getKeyFunc("test", "18608261234")},
					[]any{"123456"},
				).Return(r)
				return cmd
			},
			ctx:     context.Background(),
			biz:     "test",
			phone:   "18608261234",
			code:    "123456",
			wantErr: errors.New("redis error"),
		},
		{
			name: "验证码不存在",
			mock: func(ctrl *gomock.Controller) redis.Cmdable {
				cmd := redismocks.NewMockCmdable(ctrl)
				r := redis.NewCmd(context.Background())
				r.SetErr(ErrCodeInvalid)
				r.SetVal(int64(-2))
				cmd.EXPECT().Eval(gomock.Any(), luaStoreCode,
					[]string{getKeyFunc("test", "18608261234")},
					[]any{"123456"},
				).Return(r)
				return cmd
			},
			ctx:     context.Background(),
			biz:     "test",
			phone:   "18608261234",
			code:    "123456",
			wantErr: ErrCodeInvalid,
		},
		{
			name: "验证码发送太频繁",
			mock: func(ctrl *gomock.Controller) redis.Cmdable {
				cmd := redismocks.NewMockCmdable(ctrl)
				r := redis.NewCmd(context.Background())
				r.SetErr(ErrCodeSendTooMany)
				r.SetVal(int64(-1))
				cmd.EXPECT().Eval(gomock.Any(), luaStoreCode,
					[]string{getKeyFunc("test", "18608261234")},
					[]any{"123456"},
				).Return(r)
				return cmd
			},
			ctx:     context.Background(),
			biz:     "test",
			phone:   "18608261234",
			code:    "123456",
			wantErr: ErrCodeSendTooMany,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			cmd := tc.mock(ctrl)
			redisCodeCache := NewRedisCodeCache(cmd)
			err := redisCodeCache.Store(tc.ctx, tc.biz, tc.phone, tc.code)
			assert.Equal(t, tc.wantErr, err)
		})
	}
}
