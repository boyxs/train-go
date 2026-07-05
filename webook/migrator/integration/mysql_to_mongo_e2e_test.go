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
	"github.com/webook/pkg/logger"
)

// TestMySQL_E2E_FullToMongo 端到端验证 MySQL→Mongo 全量（反向：sourceType=mysql + sinkType=mongo）：
// MySQLSource(find id_range) → 无 transform（关系行天然是平的文档）→ MongoSink（ReplaceOne upsert，_id=PK）。
//
// 前置：mysql.dsn 可达 + migrator.mongo.{uri,database} 指向可用 Mongo。缺则 Skip。
// 注：MySQL→Mongo 不需要 transform；增量走 canal binlog（见 README Step 14），与 Mongo→MySQL 的 Change Stream 不同。
func TestMySQL_E2E_FullToMongo(t *testing.T) {
	mongoURI := viper.GetString("migrator.mongo.uri")
	mongoDB := viper.GetString("migrator.mongo.database")
	dsnStr := viper.GetString("data.mysql.dsn")
	if mongoURI == "" || mongoDB == "" {
		t.Skip("migrator.mongo.{uri,database} 未配置，跳过 mysql→mongo e2e")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

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

	mcli, err := mongo.Connect(ctx, mongoopts.Client().ApplyURI(mongoURI).SetServerSelectionTimeout(3*time.Second))
	if err != nil {
		t.Skipf("mongo connect failed: %v", err)
	}
	defer func() { _ = mcli.Disconnect(context.Background()) }()
	if err := mcli.Ping(ctx, nil); err != nil {
		t.Skipf("mongo unreachable: %v", err)
	}

	const name = "article_mysql2mongo_e2e"

	// MySQL 源表 + 数据
	require.NoError(t, gdb.Exec("DROP TABLE IF EXISTS "+name).Error)
	require.NoError(t, gdb.Exec("CREATE TABLE "+name+` (
		id BIGINT PRIMARY KEY, title VARCHAR(255), content TEXT
	) ENGINE=InnoDB CHARSET=utf8mb4`).Error)
	require.NoError(t, gdb.Exec("INSERT INTO "+name+" (id,title,content) VALUES (1,'第一篇','a'),(2,'第二篇','b')").Error)
	defer func() { _ = gdb.Exec("DROP TABLE IF EXISTS " + name).Error }()

	// Mongo 目标 collection（Drop 当 operability gate：开了 auth 等连得上但操作失败 → skip）
	coll := mcli.Database(mongoDB).Collection(name)
	if derr := coll.Drop(ctx); derr != nil {
		t.Skipf("mongo not usable for e2e (auth/permission?): %v", derr)
	}
	defer func() { _ = coll.Drop(context.Background()) }()

	// 全量管道：MySQLSource → MongoSink（无 transform）
	src := source.NewMySQLSource(gdb, name, "id", logger.NewNopLogger())
	snk := sink.NewMongoSink(coll, "id", logger.NewNopLogger())

	out := make(chan source.Row, 16)
	scanErr := make(chan error, 1)
	go func() {
		defer close(out)
		scanErr <- src.FullScan(ctx, source.ShardSpec{No: 0, PKMin: 1, PKMax: 100, BatchSz: 100}, out)
	}()

	var muts []sink.Mutation
	for row := range out {
		muts = append(muts, sink.Mutation{Op: sink.OpInsert, Table: row.Table, PK: row.PK, Cols: row.Cols})
	}
	require.NoError(t, <-scanErr)
	require.Len(t, muts, 2)
	require.NoError(t, snk.Apply(ctx, muts))

	// 断言 Mongo 落库：MySQL 行 PK → 文档 _id；列 → 文档字段
	cnt, err := coll.CountDocuments(ctx, bson.M{})
	require.NoError(t, err)
	assert.Equal(t, int64(2), cnt)

	var doc bson.M
	require.NoError(t, coll.FindOne(ctx, bson.M{"_id": "1"}).Decode(&doc))
	assert.Equal(t, "第一篇", doc["title"])
}
