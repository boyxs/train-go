package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/boyxs/train-go/webook/internal/consts"
	"github.com/boyxs/train-go/webook/internal/domain"
	"github.com/boyxs/train-go/webook/internal/errs"
	"github.com/boyxs/train-go/webook/internal/repository/cache"
	cachemocks "github.com/boyxs/train-go/webook/internal/repository/cache/mocks"
	"github.com/boyxs/train-go/webook/internal/repository/dao"
	daomocks "github.com/boyxs/train-go/webook/internal/repository/dao/mocks"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

func TestRedisUserRepository_FindById(t *testing.T) {
	mockNow := time.Now().UnixMilli()
	userid := int64(101)
	birthdayTime, _ := time.ParseInLocation(consts.DateOnly, "2026-02-13", time.UTC)
	birthday := birthdayTime.UnixMilli()
	testCases := []struct {
		name string
		mock func(ctrl *gomock.Controller) (dao.UserDAO, cache.UserCache)

		ctx    context.Context
		userid int64

		wantUser domain.User
		wantErr  error
	}{
		{
			name: "查找成功，缓存未命中",
			mock: func(ctrl *gomock.Controller) (dao.UserDAO, cache.UserCache) {
				userDAO := daomocks.NewMockUserDAO(ctrl)
				userCache := cachemocks.NewMockUserCache(ctrl)
				userCache.EXPECT().Get(gomock.Any(), userid).
					Return(domain.User{}, errs.ErrKeyNotExist)
				userDAO.EXPECT().FindById(gomock.Any(), userid).
					Return(dao.User{
						Id: userid,
						Email: sql.NullString{
							String: "123456@qq.com",
							Valid:  true,
						},
						Password:  "$2a$10$vWf3.tFGTv7OMhK5HZKrquNKqH5rBp1tlevur4a7HPVu0IizhkB0e",
						Birthday:  birthday,
						AboutMe:   "say my name",
						CreatedAt: mockNow,
						UpdatedAt: mockNow,
					}, nil)
				userCache.EXPECT().Set(gomock.Any(), domain.User{
					Id:        userid,
					Email:     "123456@qq.com",
					Password:  "$2a$10$vWf3.tFGTv7OMhK5HZKrquNKqH5rBp1tlevur4a7HPVu0IizhkB0e",
					Birthday:  birthday,
					AboutMe:   "say my name",
					CreatedAt: mockNow,
					UpdatedAt: mockNow,
				}).
					Return(nil)
				return userDAO, userCache
			},
			ctx:    context.Background(),
			userid: userid,
			wantUser: domain.User{
				Id:        userid,
				Email:     "123456@qq.com",
				Password:  "$2a$10$vWf3.tFGTv7OMhK5HZKrquNKqH5rBp1tlevur4a7HPVu0IizhkB0e",
				Birthday:  birthday,
				AboutMe:   "say my name",
				CreatedAt: mockNow,
				UpdatedAt: mockNow,
			},
			wantErr: nil,
		},
		{
			name: "缓存命中",
			mock: func(ctrl *gomock.Controller) (dao.UserDAO, cache.UserCache) {
				userCache := cachemocks.NewMockUserCache(ctrl)
				userCache.EXPECT().Get(gomock.Any(), userid).
					Return(domain.User{
						Id:        userid,
						Email:     "123456@qq.com",
						Password:  "$2a$10$vWf3.tFGTv7OMhK5HZKrquNKqH5rBp1tlevur4a7HPVu0IizhkB0e",
						Birthday:  birthday,
						AboutMe:   "say my name",
						CreatedAt: mockNow,
						UpdatedAt: mockNow,
					}, nil)
				return nil, userCache
			},
			ctx:    context.Background(),
			userid: userid,
			wantUser: domain.User{
				Id:        userid,
				Email:     "123456@qq.com",
				Password:  "$2a$10$vWf3.tFGTv7OMhK5HZKrquNKqH5rBp1tlevur4a7HPVu0IizhkB0e",
				Birthday:  birthday,
				AboutMe:   "say my name",
				CreatedAt: mockNow,
				UpdatedAt: mockNow,
			},
			wantErr: nil,
		},
		{
			name: "未找到用户",
			mock: func(ctrl *gomock.Controller) (dao.UserDAO, cache.UserCache) {
				userDAO := daomocks.NewMockUserDAO(ctrl)
				userCache := cachemocks.NewMockUserCache(ctrl)
				userCache.EXPECT().Get(gomock.Any(), userid).
					Return(domain.User{}, errs.ErrKeyNotExist)
				userDAO.EXPECT().FindById(gomock.Any(), userid).
					Return(dao.User{}, errs.ErrRecordNotFound)
				return userDAO, userCache
			},
			ctx:      context.Background(),
			userid:   userid,
			wantUser: domain.User{},
			wantErr:  errs.ErrRecordNotFound,
		},
		{
			name: "回写缓存失败",
			mock: func(ctrl *gomock.Controller) (dao.UserDAO, cache.UserCache) {
				userDAO := daomocks.NewMockUserDAO(ctrl)
				userCache := cachemocks.NewMockUserCache(ctrl)
				userCache.EXPECT().Get(gomock.Any(), userid).
					Return(domain.User{}, errs.ErrKeyNotExist)
				userDAO.EXPECT().FindById(gomock.Any(), userid).
					Return(dao.User{
						Id: userid,
						Email: sql.NullString{
							String: "123456@qq.com",
							Valid:  true,
						},
						Password:  "$2a$10$vWf3.tFGTv7OMhK5HZKrquNKqH5rBp1tlevur4a7HPVu0IizhkB0e",
						Birthday:  birthday,
						AboutMe:   "say my name",
						CreatedAt: mockNow,
						UpdatedAt: mockNow,
					}, nil)
				userCache.EXPECT().Set(gomock.Any(), domain.User{
					Id:        userid,
					Email:     "123456@qq.com",
					Password:  "$2a$10$vWf3.tFGTv7OMhK5HZKrquNKqH5rBp1tlevur4a7HPVu0IizhkB0e",
					Birthday:  birthday,
					AboutMe:   "say my name",
					CreatedAt: mockNow,
					UpdatedAt: mockNow,
				}).
					Return(errors.New("cache error"))
				return userDAO, userCache
			},
			ctx:    context.Background(),
			userid: userid,
			wantUser: domain.User{
				Id:        userid,
				Email:     "123456@qq.com",
				Password:  "$2a$10$vWf3.tFGTv7OMhK5HZKrquNKqH5rBp1tlevur4a7HPVu0IizhkB0e",
				Birthday:  birthday,
				AboutMe:   "say my name",
				CreatedAt: mockNow,
				UpdatedAt: mockNow,
			},
			wantErr: nil, //这里没有返回错误，使用查出来的数据
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			userDAO, userCache := tc.mock(ctrl)
			repo := NewRedisUserRepository(userDAO, userCache, logger.NewNopLogger())
			user, err := repo.FindById(tc.ctx, tc.userid)
			assert.Equal(t, tc.wantErr, err)
			assert.Equal(t, tc.wantUser, user)
		})
	}
}
