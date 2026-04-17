package prometheus

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type testUser struct {
	ID   int64 `gorm:"primaryKey"`
	Name string
}

func (testUser) TableName() string { return "user" }

// newTestDB 用 sqlmock 构造 GORM DB，避免依赖真实数据库
func newTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      mockDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{
		// 跳过默认事务，简化 mock
		SkipDefaultTransaction: true,
		// 跳过自动 ping，避免 sqlmock 报多余期望
		DisableAutomaticPing: true,
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	return db, mock
}

func newTestBuilder(reg *prometheus.Registry) *PrometheusBuilder {
	return NewPrometheusBuilder("webook", "db", "query", "test").Registry(reg)
}

func TestCounter_AllTypes(t *testing.T) {
	reg := prometheus.NewRegistry()
	b := newTestBuilder(reg).WithCounter()
	db, mock := newTestDB(t)
	require.NoError(t, b.Register(db))

	mock.ExpectExec("INSERT INTO `user`").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT .* FROM `user`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "a"))
	mock.ExpectExec("UPDATE `user`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM `user`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("SELECT 1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT 2").WillReturnRows(sqlmock.NewRows([]string{"v"}).AddRow(1))

	require.NoError(t, db.Create(&testUser{ID: 1, Name: "a"}).Error)
	var u testUser
	require.NoError(t, db.First(&u, 1).Error)
	require.NoError(t, db.Model(&testUser{}).Where("id=?", 1).Update("name", "b").Error)
	require.NoError(t, db.Delete(&testUser{}, 1).Error)
	require.NoError(t, db.Exec("SELECT 1").Error)
	row := db.Raw("SELECT 2").Row()
	require.NotNil(t, row)
	var v int
	_ = row.Scan(&v)

	require.NoError(t, mock.ExpectationsWereMet())

	metrics := gatherText(t, reg)
	assert.Contains(t, metrics, `value:"create"`)
	assert.Contains(t, metrics, `value:"query"`)
	assert.Contains(t, metrics, `value:"update"`)
	assert.Contains(t, metrics, `value:"delete"`)
	assert.Contains(t, metrics, `value:"raw"`)
	assert.Contains(t, metrics, `value:"row"`)
}

func TestCounter_TableLabel(t *testing.T) {
	reg := prometheus.NewRegistry()
	b := newTestBuilder(reg).WithCounter()
	db, mock := newTestDB(t)
	require.NoError(t, b.Register(db))

	mock.ExpectExec("INSERT INTO `user`").WillReturnResult(sqlmock.NewResult(1, 1))

	require.NoError(t, db.Create(&testUser{ID: 1, Name: "a"}).Error)
	require.NoError(t, mock.ExpectationsWereMet())

	metrics := gatherText(t, reg)
	assert.Contains(t, metrics, `value:"user"`)
}

func TestCounter_TableUnknownFallback(t *testing.T) {
	reg := prometheus.NewRegistry()
	b := newTestBuilder(reg).WithCounter()
	db, mock := newTestDB(t)
	require.NoError(t, b.Register(db))

	mock.ExpectExec("SELECT 1").WillReturnResult(sqlmock.NewResult(0, 0))

	// raw SQL 没有 Statement.Table
	require.NoError(t, db.Exec("SELECT 1").Error)
	require.NoError(t, mock.ExpectationsWereMet())

	metrics := gatherText(t, reg)
	assert.Contains(t, metrics, `value:"unknown"`)
}

func TestHistogram_Observed(t *testing.T) {
	reg := prometheus.NewRegistry()
	b := newTestBuilder(reg).WithHistogram()
	db, mock := newTestDB(t)
	require.NoError(t, b.Register(db))

	mock.ExpectExec("INSERT INTO `user`").WillReturnResult(sqlmock.NewResult(1, 1))

	require.NoError(t, db.Create(&testUser{ID: 1, Name: "a"}).Error)
	require.NoError(t, mock.ExpectationsWereMet())

	metrics := gatherText(t, reg)
	assert.Contains(t, metrics, "webook_db_query_duration_seconds")
	assert.Contains(t, metrics, "type:HISTOGRAM")
	assert.Contains(t, metrics, "sample_count:1")
}

func TestSummary_Observed(t *testing.T) {
	reg := prometheus.NewRegistry()
	b := newTestBuilder(reg).WithSummary()
	db, mock := newTestDB(t)
	require.NoError(t, b.Register(db))

	mock.ExpectExec("INSERT INTO `user`").WillReturnResult(sqlmock.NewResult(1, 1))

	require.NoError(t, db.Create(&testUser{ID: 1, Name: "a"}).Error)
	require.NoError(t, mock.ExpectationsWereMet())

	metrics := gatherText(t, reg)
	assert.Contains(t, metrics, "webook_db_query_duration_seconds_summary")
	assert.Contains(t, metrics, "type:SUMMARY")
	assert.Contains(t, metrics, "sample_count:1")
}

func TestBuild_OnlyCounter(t *testing.T) {
	reg := prometheus.NewRegistry()
	b := newTestBuilder(reg).WithCounter()
	db, mock := newTestDB(t)
	require.NoError(t, b.Register(db))

	mock.ExpectExec("INSERT INTO `user`").WillReturnResult(sqlmock.NewResult(1, 1))

	require.NoError(t, db.Create(&testUser{ID: 1, Name: "a"}).Error)
	require.NoError(t, mock.ExpectationsWereMet())

	count := testutil.CollectAndCount(reg, "webook_db_query_total")
	assert.Greater(t, count, 0)
	histogramCount := testutil.CollectAndCount(reg, "webook_db_query_duration_seconds")
	assert.Equal(t, 0, histogramCount)
}

func TestBuild_NoneEnabled(t *testing.T) {
	reg := prometheus.NewRegistry()
	b := newTestBuilder(reg)
	db, mock := newTestDB(t)
	require.NoError(t, b.Register(db))

	mock.ExpectExec("INSERT INTO `user`").WillReturnResult(sqlmock.NewResult(1, 1))

	require.NoError(t, db.Create(&testUser{ID: 1, Name: "a"}).Error)
	require.NoError(t, mock.ExpectationsWereMet())

	mfs, err := reg.Gather()
	require.NoError(t, err)
	assert.Empty(t, mfs)
}

func TestSQL_FailureStillObserved(t *testing.T) {
	reg := prometheus.NewRegistry()
	b := newTestBuilder(reg).WithCounter()
	db, mock := newTestDB(t)
	require.NoError(t, b.Register(db))

	mock.ExpectExec("INSERT INTO `user`").WillReturnError(sql.ErrNoRows)

	err := db.Create(&testUser{ID: 1, Name: "a"}).Error
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	count := testutil.CollectAndCount(reg, "webook_db_query_total")
	assert.Greater(t, count, 0)
}

func gatherText(t *testing.T, reg *prometheus.Registry) string {
	mfs, err := reg.Gather()
	require.NoError(t, err)
	var sb strings.Builder
	for _, mf := range mfs {
		sb.WriteString(mf.String())
		sb.WriteString("\n")
	}
	return sb.String()
}
