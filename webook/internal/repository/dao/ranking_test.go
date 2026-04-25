package dao

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func newMockGorm(t *testing.T, sqldb *sql.DB) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqldb,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	assert.NoError(t, err)
	return db
}

func TestGormArticleRankingDAO_InsertSnapshot(t *testing.T) {
	testCases := []struct {
		name    string
		mock    func() *sql.DB
		date    string
		dim     string
		cat     string
		items   []ArticleRanking
		wantErr error
	}{
		{
			name: "批量插入成功",
			mock: func() *sql.DB {
				db, mock, err := sqlmock.New()
				assert.NoError(t, err)
				mock.ExpectBegin()
				mock.ExpectExec("INSERT INTO `article_ranking`").
					WillReturnResult(sqlmock.NewResult(1, 3))
				mock.ExpectCommit()
				return db
			},
			date: "2026-04-21", dim: "hot", cat: "",
			items: []ArticleRanking{
				{Rank: 1, ArticleId: 100, Score: 9832},
				{Rank: 2, ArticleId: 200, Score: 7621},
				{Rank: 3, ArticleId: 300, Score: 6890},
			},
		},
		{
			name: "空列表跳过",
			mock: func() *sql.DB {
				db, _, err := sqlmock.New()
				assert.NoError(t, err)
				return db
			},
			date: "2026-04-21", dim: "hot", cat: "",
			items: nil,
		},
		{
			name: "DB错误透传",
			mock: func() *sql.DB {
				db, mock, err := sqlmock.New()
				assert.NoError(t, err)
				mock.ExpectBegin()
				mock.ExpectExec("INSERT INTO `article_ranking`").
					WillReturnError(errors.New("db boom"))
				mock.ExpectRollback()
				return db
			},
			date: "2026-04-21", dim: "hot", cat: "",
			items: []ArticleRanking{
				{Rank: 1, ArticleId: 100, Score: 100},
			},
			wantErr: errors.New("db boom"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sqldb := tc.mock()
			dao := NewGormArticleRankingDAO(newMockGorm(t, sqldb))
			err := dao.InsertSnapshot(context.Background(), tc.date, tc.dim, tc.cat, tc.items)
			if tc.wantErr != nil {
				assert.EqualError(t, err, tc.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGormArticleRankingDAO_ListByDate(t *testing.T) {
	t.Run("返回列表", func(t *testing.T) {
		sqldb, mock, err := sqlmock.New()
		assert.NoError(t, err)
		rows := sqlmock.NewRows([]string{"id", "date", "dimension", "category", "rank", "article_id", "score", "snapshot", "created_at"}).
			AddRow(1, "2026-04-21", "hot", "", 1, 100, 9832.0, "{}", int64(1000)).
			AddRow(2, "2026-04-21", "hot", "", 2, 200, 7621.0, "{}", int64(1000))
		mock.ExpectQuery("SELECT .* FROM `article_ranking`").
			WithArgs("2026-04-21", "hot", "").
			WillReturnRows(rows)

		dao := NewGormArticleRankingDAO(newMockGorm(t, sqldb))
		items, err := dao.ListByDate(context.Background(), "2026-04-21", "hot", "")
		assert.NoError(t, err)
		assert.Len(t, items, 2)
		assert.Equal(t, int64(100), items[0].ArticleId)
	})

	t.Run("无结果返空slice", func(t *testing.T) {
		sqldb, mock, err := sqlmock.New()
		assert.NoError(t, err)
		mock.ExpectQuery("SELECT .* FROM `article_ranking`").
			WillReturnRows(sqlmock.NewRows([]string{"id", "date", "dimension", "category", "rank", "article_id", "score", "snapshot", "created_at"}))

		dao := NewGormArticleRankingDAO(newMockGorm(t, sqldb))
		items, err := dao.ListByDate(context.Background(), "2026-04-21", "hot", "")
		assert.NoError(t, err)
		assert.Len(t, items, 0)
	})
}

func TestGormArticleRankingDAO_ListArchiveDates(t *testing.T) {
	t.Run("返回去重日期降序", func(t *testing.T) {
		sqldb, mock, err := sqlmock.New()
		assert.NoError(t, err)
		rows := sqlmock.NewRows([]string{"date"}).
			AddRow("2026-04-21").
			AddRow("2026-04-20")
		mock.ExpectQuery("SELECT DISTINCT .*date.* FROM `article_ranking`").
			WillReturnRows(rows)

		dao := NewGormArticleRankingDAO(newMockGorm(t, sqldb))
		dates, err := dao.ListArchiveDates(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, []string{"2026-04-21", "2026-04-20"}, dates)
	})
}
