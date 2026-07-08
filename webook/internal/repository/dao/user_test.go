package dao

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/boyxs/train-go/webook/internal/errs"
)

func TestGormUserDAO_Insert(t *testing.T) {
	testCases := []struct {
		name string
		mock func(ctrl *gomock.Controller) *sql.DB

		ctx context.Context
		u   User

		wantErr error
	}{
		{
			name: "插入成功",
			mock: func(ctrl *gomock.Controller) *sql.DB {
				db, mock, err := sqlmock.New()
				assert.NoError(t, err)
				mockRes := sqlmock.NewResult(1000, 1)
				mock.ExpectExec("INSERT INTO .*").
					WillReturnResult(mockRes)
				return db
			},
			ctx: context.Background(),
			u: User{
				Nickname: "Tommy",
			},
		},
		{
			name: "邮箱冲突",
			mock: func(ctrl *gomock.Controller) *sql.DB {
				db, mock, err := sqlmock.New()
				assert.NoError(t, err)
				mock.ExpectExec("INSERT INTO .*").
					WillReturnError(&mysqlDriver.MySQLError{Number: 1062})
				return db
			},
			ctx: context.Background(),
			u: User{
				Nickname: "Tommy",
			},
			wantErr: errs.ErrDuplicateUser,
		},
		{
			name: "数据库错误",
			mock: func(ctrl *gomock.Controller) *sql.DB {
				db, mock, err := sqlmock.New()
				assert.NoError(t, err)
				mock.ExpectExec("INSERT INTO .*").
					WillReturnError(errors.New("mysql error"))
				return db
			},
			ctx: context.Background(),
			u: User{
				Nickname: "Tommy",
			},
			wantErr: errors.New("mysql error"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			sqlDB := tc.mock(ctrl)
			db, err := gorm.Open(mysql.New(mysql.Config{
				Conn:                      sqlDB,
				SkipInitializeWithVersion: true,
			}), &gorm.Config{
				DisableAutomaticPing:   true,
				SkipDefaultTransaction: true,
			})
			userDAO := NewGormUserDAO(db)
			err = userDAO.Insert(tc.ctx, tc.u)
			assert.Equal(t, tc.wantErr, err)
		})
	}
}
