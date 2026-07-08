package dao

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/boyxs/train-go/webook/migrator/errs"
)

func TestGormTaskDAO_Insert(t *testing.T) {
	testCases := []struct {
		name string
		mock func() *sql.DB

		ctx  context.Context
		task Task

		wantId  int64
		wantErr error
	}{
		{
			name: "插入成功",
			mock: func() *sql.DB {
				db, mock, err := sqlmock.New()
				assert.NoError(t, err)
				mockRes := sqlmock.NewResult(1, 1)
				mock.ExpectExec("INSERT INTO `task`").
					WillReturnResult(mockRes)
				return db
			},
			ctx: context.Background(),
			task: Task{
				Name:         "article_to_es_v1",
				Mode:         "cdc",
				Kind:         "heterogeneous",
				SourceDsnRef: "vault:webook/db/source",
				SinkType:     "es",
				SinkDsnRef:   "vault:webook/es/cluster",
				TablesJSON:   `[{"src":"article"}]`,
			},
			wantId: 1,
		},
		{
			name: "name 重复（unique constraint）",
			mock: func() *sql.DB {
				db, mock, err := sqlmock.New()
				assert.NoError(t, err)
				mock.ExpectExec("INSERT INTO `task`").
					WillReturnError(&mysqlDriver.MySQLError{Number: 1062, Message: "Duplicate entry 'foo' for key 'uni_task_name'"})
				return db
			},
			ctx: context.Background(),
			task: Task{
				Name: "duplicate-name",
				Mode: "cdc",
				Kind: "heterogeneous",
			},
			wantErr: errs.ErrDuplicateTaskName,
		},
		{
			name: "数据库错误",
			mock: func() *sql.DB {
				db, mock, err := sqlmock.New()
				assert.NoError(t, err)
				mock.ExpectExec("INSERT INTO `task`").
					WillReturnError(errors.New("mysql error"))
				return db
			},
			ctx: context.Background(),
			task: Task{
				Name: "any",
				Mode: "cdc",
				Kind: "heterogeneous",
			},
			wantErr: errors.New("mysql error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sqlDB := tc.mock()
			db, err := gorm.Open(mysql.New(mysql.Config{
				Conn:                      sqlDB,
				SkipInitializeWithVersion: true,
			}), &gorm.Config{
				DisableAutomaticPing:   true,
				SkipDefaultTransaction: true,
			})
			assert.NoError(t, err)

			dao := NewGormTaskDAO(db)
			gotId, err := dao.Insert(tc.ctx, tc.task)

			if tc.wantErr != nil {
				assert.EqualError(t, err, tc.wantErr.Error())
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.wantId, gotId)
		})
	}
}

func openMockGorm(t *testing.T, sqlDB *sql.DB) *gorm.DB {
	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
	})
	assert.NoError(t, err)
	return db
}

func TestGormTaskDAO_FindById(t *testing.T) {
	t.Run("命中", func(t *testing.T) {
		sqlDB, mock, err := sqlmock.New()
		assert.NoError(t, err)
		rows := sqlmock.NewRows([]string{
			"id", "name", "mode", "kind", "source_dsn_ref", "sink_type", "sink_dsn_ref",
			"tables_json", "status", "gray_percent", "consistency",
			"created_at", "updated_at", "deleted_at",
		}).AddRow(1, "article_to_es_v1", "cdc", "heterogeneous", "vault:src", "es", "vault:dst",
			`[{"src":"article"}]`, 3, 50, "eventual", int64(1000), int64(1000), int64(0))
		mock.ExpectQuery("SELECT \\* FROM `task`").WillReturnRows(rows)
		dao := NewGormTaskDAO(openMockGorm(t, sqlDB))
		got, err := dao.FindById(context.Background(), 1)
		assert.NoError(t, err)
		assert.Equal(t, int64(1), got.Id)
		assert.Equal(t, "article_to_es_v1", got.Name)
		assert.Equal(t, int8(3), got.Status)
		assert.Equal(t, int16(50), got.GrayPercent)
	})

	t.Run("未找到", func(t *testing.T) {
		sqlDB, mock, err := sqlmock.New()
		assert.NoError(t, err)
		rows := sqlmock.NewRows([]string{"id"})
		mock.ExpectQuery("SELECT \\* FROM `task`").WillReturnRows(rows)
		dao := NewGormTaskDAO(openMockGorm(t, sqlDB))
		_, err = dao.FindById(context.Background(), 999)
		assert.ErrorIs(t, err, errs.ErrTaskNotFound)
	})
}

func TestGormTaskDAO_FindByName(t *testing.T) {
	t.Run("命中", func(t *testing.T) {
		sqlDB, mock, err := sqlmock.New()
		assert.NoError(t, err)
		rows := sqlmock.NewRows([]string{
			"id", "name", "mode", "kind", "source_dsn_ref", "sink_type", "sink_dsn_ref",
			"tables_json", "status", "gray_percent", "consistency",
			"created_at", "updated_at", "deleted_at",
		}).AddRow(7, "user_nickname_v2", "dual_write", "schema", "vault:src", "mysql", "vault:dst",
			`[{"src":"user"}]`, 3, 0, "strong", int64(1000), int64(1000), int64(0))
		mock.ExpectQuery("SELECT \\* FROM `task`").WillReturnRows(rows)
		dao := NewGormTaskDAO(openMockGorm(t, sqlDB))
		got, err := dao.FindByName(context.Background(), "user_nickname_v2")
		assert.NoError(t, err)
		assert.Equal(t, int64(7), got.Id)
	})

	t.Run("未找到", func(t *testing.T) {
		sqlDB, mock, err := sqlmock.New()
		assert.NoError(t, err)
		rows := sqlmock.NewRows([]string{"id"})
		mock.ExpectQuery("SELECT \\* FROM `task`").WillReturnRows(rows)
		dao := NewGormTaskDAO(openMockGorm(t, sqlDB))
		_, err = dao.FindByName(context.Background(), "missing")
		assert.ErrorIs(t, err, errs.ErrTaskNotFound)
	})
}

func TestGormTaskDAO_List(t *testing.T) {
	t.Run("按 status 分页正常", func(t *testing.T) {
		sqlDB, mock, err := sqlmock.New()
		assert.NoError(t, err)
		// 先 Count
		mock.ExpectQuery("SELECT count").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
		// 再 Find
		rows := sqlmock.NewRows([]string{
			"id", "name", "mode", "kind", "source_dsn_ref", "sink_type", "sink_dsn_ref",
			"tables_json", "status", "gray_percent", "consistency",
			"created_at", "updated_at", "deleted_at",
		}).
			AddRow(2, "t2", "cdc", "heterogeneous", "", "es", "", "", 3, 50, "eventual", int64(2000), int64(2000), int64(0)).
			AddRow(1, "t1", "cdc", "heterogeneous", "", "es", "", "", 3, 0, "eventual", int64(1000), int64(1000), int64(0))
		mock.ExpectQuery("SELECT \\* FROM `task`").WillReturnRows(rows)

		dao := NewGormTaskDAO(openMockGorm(t, sqlDB))
		status := int8(3)
		list, total, err := dao.List(context.Background(), &status, 0, 10)
		assert.NoError(t, err)
		assert.Equal(t, int64(2), total)
		assert.Len(t, list, 2)
		assert.Equal(t, int64(2), list[0].Id) // id DESC
	})
}

func TestGormTaskDAO_UpdateStatus(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	assert.NoError(t, err)
	mock.ExpectExec("UPDATE `task`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	dao := NewGormTaskDAO(openMockGorm(t, sqlDB))
	err = dao.UpdateStatus(context.Background(), 1, 3)
	assert.NoError(t, err)
}

func TestGormTaskDAO_UpdateGrayPercent(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	assert.NoError(t, err)
	mock.ExpectExec("UPDATE `task`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	dao := NewGormTaskDAO(openMockGorm(t, sqlDB))
	err = dao.UpdateGrayPercent(context.Background(), 1, 50)
	assert.NoError(t, err)
}

func TestGormTaskDAO_SoftDelete(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	assert.NoError(t, err)
	// GORM 软删除是 UPDATE deleted_at
	mock.ExpectExec("UPDATE `task`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	dao := NewGormTaskDAO(openMockGorm(t, sqlDB))
	err = dao.SoftDelete(context.Background(), 1)
	assert.NoError(t, err)
}
