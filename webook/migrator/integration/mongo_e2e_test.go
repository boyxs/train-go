package integration

import (
	"context"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	mongoopts "go.mongodb.org/mongo-driver/mongo/options"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/webook/migrator/pipeline/sink"
	"github.com/webook/migrator/pipeline/source"
	"github.com/webook/migrator/pipeline/transform"
	"github.com/webook/pkg/logger"
)

// TestMongo_E2E_FullToMySQL 端到端验证 Mongo→MySQL 全量：
// MongoSource 流式扫集合 → MongoToRelationalTransformer 把文档拍平（嵌套子文档/数组→JSON 列）→ MySQLSink 落库。
//
// 前置：migrator.mongo.{uri,database} 已配（config/test.yaml）+ mysql.dsn 可达。
// 缺任一前置 → t.Skip（避免污染 go test ./... 全量回归）。
//
// 注：直接拼 Source→transform→Sink 三段（与 canal_e2e 直测 client 同思路）；
// full 引擎的 transform 接入由 service/full 单测覆盖。
func TestMongo_E2E_FullToMySQL(t *testing.T) {
	mongoURI := viper.GetString("migrator.mongo.uri")
	mongoDB := viper.GetString("migrator.mongo.database")
	dsnStr := viper.GetString("mysql.dsn")
	if mongoURI == "" || mongoDB == "" {
		t.Skip("migrator.mongo.{uri,database} 未配置，跳过 mongo e2e")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// 1. 连 Mongo（探活，不可达 skip）
	mcli, err := mongo.Connect(ctx, mongoopts.Client().ApplyURI(mongoURI).SetServerSelectionTimeout(3*time.Second))
	if err != nil {
		t.Skipf("mongo connect failed: %v", err)
	}
	defer func() { _ = mcli.Disconnect(context.Background()) }()
	if err := mcli.Ping(ctx, nil); err != nil {
		t.Skipf("mongo unreachable: %v", err)
	}

	// 2. 连 MySQL（gorm；不可达 skip）
	gdb, err := gorm.Open(gormmysql.Open(dsnStr), &gorm.Config{})
	if err != nil {
		t.Skipf("mysql open failed: %v", err)
	}
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	defer func() { _ = sqlDB.Close() }()
	if err := sqlDB.Ping(); err != nil {
		t.Skipf("mysql unreachable: %v", err)
	}

	const collName = "user_mongo_e2e"
	coll := mcli.Database(mongoDB).Collection(collName)

	// 3. 准备 Mongo 数据：清 + 插 2 文档（含嵌套子文档 + 数组）
	// Drop 当 operability gate：连得上但操作失败（如开了 auth）→ skip 而非 fail
	if derr := coll.Drop(ctx); derr != nil {
		t.Skipf("mongo not usable for e2e (auth/permission?): %v", derr)
	}
	_, err = coll.InsertMany(ctx, []any{
		bson.M{"_id": "u1", "name": "alice", "profile": bson.M{"city": "SG", "age": 30}, "tags": bson.A{"go", "db"}},
		bson.M{"_id": "u2", "name": "bob", "profile": bson.M{"city": "NY"}, "tags": bson.A{"py"}},
	})
	require.NoError(t, err)
	defer func() { _ = coll.Drop(context.Background()) }()

	// 4. 准备 MySQL 目标表（列与拍平后字段对齐：_id / name / profile(JSON) / tags(JSON)）
	require.NoError(t, gdb.Exec("DROP TABLE IF EXISTS "+collName).Error)
	require.NoError(t, gdb.Exec(`CREATE TABLE `+collName+` (
		_id     VARCHAR(64) PRIMARY KEY,
		name    VARCHAR(255),
		profile TEXT,
		tags    TEXT
	) ENGINE=InnoDB CHARSET=utf8mb4`).Error)
	defer func() { _ = gdb.Exec("DROP TABLE IF EXISTS " + collName).Error }()

	// 5. 跑全量管道：MongoSource → MongoToRelationalTransformer → MySQLSink
	src := source.NewMongoSource(source.NewGoMongoScanner(coll, logger.NewNopLogger()), collName, logger.NewNopLogger())
	snk := sink.NewMySQLSink(gdb, collName, "_id", logger.NewNopLogger())
	tf := transform.MongoToRelationalTransformer{}

	out := make(chan source.Row, 16)
	scanErr := make(chan error, 1)
	go func() {
		defer close(out)
		scanErr <- src.FullScan(ctx, source.ShardSpec{BatchSz: 100}, out)
	}()

	var muts []sink.Mutation
	for row := range out {
		m, terr := tf.Transform(sink.Mutation{Op: sink.OpInsert, Table: row.Table, PK: row.PK, Cols: row.Cols})
		require.NoError(t, terr)
		muts = append(muts, m)
	}
	require.NoError(t, <-scanErr)
	require.Len(t, muts, 2)
	require.NoError(t, snk.Apply(ctx, muts))

	// 6. 断言 MySQL 落库 + 嵌套字段成 JSON 列
	var count int64
	require.NoError(t, gdb.Raw("SELECT COUNT(*) FROM "+collName).Scan(&count).Error)
	assert.Equal(t, int64(2), count)

	var alice struct {
		ID      string `gorm:"column:_id"`
		Name    string `gorm:"column:name"`
		Profile string `gorm:"column:profile"`
		Tags    string `gorm:"column:tags"`
	}
	require.NoError(t, gdb.Raw("SELECT _id, name, profile, tags FROM "+collName+" WHERE _id = ?", "u1").Scan(&alice).Error)
	assert.Equal(t, "alice", alice.Name)
	assert.JSONEq(t, `{"city":"SG","age":30}`, alice.Profile) // 嵌套子文档 → JSON 列
	assert.JSONEq(t, `["go","db"]`, alice.Tags)               // 数组 → JSON 列
}
