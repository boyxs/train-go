package dao

import (
	"context"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// TestRepublishAfterWithdraw_ResetsDeletedAt_OnV1Table
// 直击 NEW 侧 DAO（tableName=published_article_v1）的 Upsert 是否在撤回后重置 deleted_at。
// 绕 HTTP / SDK / DualWriter，只测 DAO 行为本身。
//
// 跑：cd webook && go test ./internal/repository/dao/ -run TestRepublishAfterWithdraw_ResetsDeletedAt_OnV1Table -v
func TestRepublishAfterWithdraw_ResetsDeletedAt_OnV1Table(t *testing.T) {
	viper.SetConfigFile("../../config/test.yaml")
	if err := viper.ReadInConfig(); err != nil {
		t.Skipf("test.yaml 不可用：%v", err)
	}
	dsn := viper.GetString("mysql.dsn")
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("mysql 不可达：%v", err)
	}

	// 确保 v1 表存在（schema 跟 OLD 同）
	if err := db.AutoMigrate(&PublishedArticle{}); err != nil {
		t.Fatal(err)
	}
	require.NoError(t, db.Exec("DROP TABLE IF EXISTS published_article_v1").Error)
	require.NoError(t, db.Exec("CREATE TABLE published_article_v1 LIKE published_article").Error)
	defer func() { _ = db.Exec("DROP TABLE IF EXISTS published_article_v1").Error }()

	newDAO := NewGormArticleReaderNewDAO(db)
	// named type → 显式 cast 回 ArticleReaderDAO 接口调用
	asReader := ArticleReaderDAO(newDAO)
	ctx := context.Background()
	now := time.Now().UnixMilli()
	const id int64 = 12345

	// 1) 首次 Upsert：新行落库，deleted_at 应是 0
	require.NoError(t, asReader.Upsert(ctx, PublishedArticle{
		Id: id, Title: "v1", Content: "c1", AuthorId: 1, Status: 2,
		CreatedAt: now, UpdatedAt: now,
	}))
	var del1 int64
	require.NoError(t, db.Raw("SELECT deleted_at FROM published_article_v1 WHERE id=?", id).Scan(&del1).Error)
	assert.Equal(t, int64(0), del1, "首次 Upsert 后 deleted_at 应为 0")

	// 2) Delete 软删：deleted_at 应 > 0
	require.NoError(t, asReader.Delete(ctx, id, 1))
	var del2 int64
	require.NoError(t, db.Raw("SELECT deleted_at FROM published_article_v1 WHERE id=?", id).Scan(&del2).Error)
	assert.Greater(t, del2, int64(0), "Delete 后 deleted_at 应 > 0（GORM softDelete:milli）")

	// 3) 重新 Upsert：deleted_at 应被 ON DUP KEY UPDATE 重置回 0
	require.NoError(t, asReader.Upsert(ctx, PublishedArticle{
		Id: id, Title: "v2", Content: "c2", AuthorId: 1, Status: 2,
		CreatedAt: now, UpdatedAt: now,
	}))
	var del3 int64
	require.NoError(t, db.Raw("SELECT deleted_at FROM published_article_v1 WHERE id=?", id).Scan(&del3).Error)
	assert.Equal(t, int64(0), del3,
		"重新 Upsert 后 deleted_at 必须回 0；当前 %d 说明 Upsert 的 DoUpdates 列表漏 deleted_at（fix 应包含 deleted_at）", del3)

	// 4) Find 必须能读到（GORM 自动注入 deleted_at=0 过滤后仍可见）
	var pub PublishedArticle
	require.NoError(t, db.Table("published_article_v1").Where("id=?", id).First(&pub).Error)
	assert.Equal(t, "v2", pub.Title)
	assert.Equal(t, "c2", pub.Content)
}
