package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/boyxs/train-go/webook/internal/domain"
	"github.com/boyxs/train-go/webook/internal/errs"
	"github.com/boyxs/train-go/webook/internal/repository"
	repomocks "github.com/boyxs/train-go/webook/internal/repository/mocks"
)

func TestInternalUserService_Login(t *testing.T) {
	testCases := []struct {
		name string
		mock func(ctrl *gomock.Controller) repository.UserRepository
		//输入
		ctx      context.Context
		email    string
		password string
		//输出
		wantUser domain.User
		wantErr  error
	}{
		{
			name: "登录成功",
			mock: func(ctrl *gomock.Controller) repository.UserRepository {
				repo := repomocks.NewMockUserRepository(ctrl)
				repo.EXPECT().FindByEmail(gomock.Any(), "123456@qq.com").
					Return(domain.User{
						Email:    "123456@qq.com",
						Password: "$2a$10$2K6kqUlPf9BmOKQIHvRaNOIvxP41rvSpCpWvK.6gJcnV0VsjjJA9C",
					}, nil)
				return repo
			},
			ctx:      context.Background(),
			email:    "123456@qq.com",
			password: "@12345678a",
			wantUser: domain.User{
				Email:    "123456@qq.com",
				Password: "$2a$10$2K6kqUlPf9BmOKQIHvRaNOIvxP41rvSpCpWvK.6gJcnV0VsjjJA9C",
			},
			wantErr: nil,
		},
		{
			name: "用户未找到",
			mock: func(ctrl *gomock.Controller) repository.UserRepository {
				repo := repomocks.NewMockUserRepository(ctrl)
				repo.EXPECT().FindByEmail(gomock.Any(), "123456789@qq.com").
					Return(domain.User{}, errs.ErrRecordNotFound)
				return repo
			},
			ctx:      context.Background(),
			email:    "123456789@qq.com",
			password: "@12345678a",
			wantUser: domain.User{},
			wantErr:  errs.ErrInvalidUserOrPassword,
		},
		{
			name: "系统异常",
			mock: func(ctrl *gomock.Controller) repository.UserRepository {
				repo := repomocks.NewMockUserRepository(ctrl)
				repo.EXPECT().FindByEmail(gomock.Any(), "123456@qq.com").
					Return(domain.User{
						Email:    "123456@qq.com",
						Password: "$2a$10$2K6kqUlPf9BmOKQIHvRaNOIvxP41rvSpCpWvK.6gJcnV0VsjjJA9C",
					}, errors.New("system error"))
				return repo
			},
			ctx:      context.Background(),
			email:    "123456@qq.com",
			password: "@12345678a",
			wantUser: domain.User{},
			wantErr:  errors.New("system error"),
		},
		{
			name: "密码不匹配",
			mock: func(ctrl *gomock.Controller) repository.UserRepository {
				repo := repomocks.NewMockUserRepository(ctrl)
				repo.EXPECT().FindByEmail(gomock.Any(), "123456@qq.com").
					Return(domain.User{
						Email:    "123456@qq.com",
						Password: "$2a$10$2K6kqUlPf9BmOKQIHvRaNOIvxP41rvSpCpWvK.6gJcnV0VsjjJA9C",
					}, nil)
				return repo
			},
			ctx:      context.Background(),
			email:    "123456@qq.com",
			password: "@123",
			wantUser: domain.User{},
			wantErr:  errs.ErrInvalidUserOrPassword,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			repo := tc.mock(ctrl)
			userService := NewInternalUserService(repo)
			user, err := userService.Login(tc.ctx, tc.email, tc.password)
			assert.Equal(t, tc.wantErr, err)
			assert.Equal(t, tc.wantUser, user)

		})
	}
}
