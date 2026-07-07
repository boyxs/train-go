package dao

import (
	"context"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// TestGormCommentDAO_BatchCount 验证 BatchCount 的 GROUP BY 聚合：只统计指定 biz+bizIds、
// 按 bizId 分组、无评论的 id 不在结果、软删行被排除。
// 用唯一 biz 隔离共享 comment 表，Unscoped 硬删清理，不污染其他数据。
//
// 跑：cd webook && go test ./comment/repository/dao/ -run TestGormCommentDAO_BatchCount -v
func TestGormCommentDAO_BatchCount(t *testing.T) {
	viper.SetConfigFile("../../config/test.yaml")
	if err := viper.ReadInConfig(); err != nil {
		t.Skipf("test.yaml 不可用：%v", err)
	}
	dsn := viper.GetString("data.mysql.dsn")
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("mysql 不可达：%v", err)
	}
	require.NoError(t, db.AutoMigrate(&Comment{}))

	const biz = "test_batchcount_relation"
	clean := func() { db.Unscoped().Where("biz = ?", biz).Delete(&Comment{}) }
	clean()
	defer clean()

	d := NewGormCommentDAO(db)
	ctx := context.Background()
	// biz_id=101 两条，102 一条，103 无
	for _, c := range []Comment{
		{Biz: biz, BizId: 101, Uid: 1, Content: "a"},
		{Biz: biz, BizId: 101, Uid: 2, Content: "b"},
		{Biz: biz, BizId: 102, Uid: 1, Content: "c"},
	} {
		require.NoError(t, db.Create(&c).Error)
	}

	got, err := d.BatchCount(ctx, biz, []int64{101, 102, 103})
	require.NoError(t, err)
	assert.Equal(t, int64(2), got[101])
	assert.Equal(t, int64(1), got[102])
	_, has103 := got[103]
	assert.False(t, has103, "无评论的 bizId 不应在结果 map 中")

	// 软删 102 的评论 → 计数排除
	require.NoError(t, db.Where("biz = ? AND biz_id = ?", biz, int64(102)).Delete(&Comment{}).Error)
	got2, err := d.BatchCount(ctx, biz, []int64{101, 102})
	require.NoError(t, err)
	assert.Equal(t, int64(2), got2[101])
	_, has102 := got2[102]
	assert.False(t, has102, "软删后 102 应从计数中排除")
}
