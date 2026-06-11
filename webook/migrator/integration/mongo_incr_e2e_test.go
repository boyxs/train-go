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

	"github.com/webook/migrator/domain"
	"github.com/webook/migrator/pipeline/sink"
	"github.com/webook/migrator/pipeline/source"
	"github.com/webook/migrator/pipeline/transform"
	"github.com/webook/pkg/logger"
)

// TestMongo_E2E_IncrToMySQL 端到端验证 Mongo→MySQL 增量：
// MongoSource.IncrSubscribe（Change Stream）→ MongoToRelationalTransformer → MySQLSink。
//
// 前置：migrator.mongo.{uri,database} 已配 + mysql.dsn 可达 + **Mongo 是副本集**（Change Stream 仅副本集可用）。
// 单机 mongod（未 rs.initiate）会让 Watch 立即失败 → 本测试 Skip。
// 本地起法：./deploy.sh local 后 `docker exec webook-mongo mongosh --eval "rs.initiate()"` 一次。
func TestMongo_E2E_IncrToMySQL(t *testing.T) {
	mongoURI := viper.GetString("migrator.mongo.uri")
	mongoDB := viper.GetString("migrator.mongo.database")
	dsnStr := viper.GetString("mysql.dsn")
	if mongoURI == "" || mongoDB == "" {
		t.Skip("migrator.mongo.{uri,database} 未配置，跳过 mongo incr e2e")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mcli, err := mongo.Connect(ctx, mongoopts.Client().ApplyURI(mongoURI).SetServerSelectionTimeout(3*time.Second))
	if err != nil {
		t.Skipf("mongo connect failed: %v", err)
	}
	defer func() { _ = mcli.Disconnect(context.Background()) }()
	if err := mcli.Ping(ctx, nil); err != nil {
		t.Skipf("mongo unreachable: %v", err)
	}

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

	const collName = "user_mongo_incr_e2e"
	coll := mcli.Database(mongoDB).Collection(collName)
	// Drop 当 operability gate：连得上但操作失败（如开了 auth）→ skip 而非 fail
	if derr := coll.Drop(ctx); derr != nil {
		t.Skipf("mongo not usable for e2e (auth/permission?): %v", derr)
	}
	defer func() { _ = coll.Drop(context.Background()) }()

	require.NoError(t, gdb.Exec("DROP TABLE IF EXISTS "+collName).Error)
	require.NoError(t, gdb.Exec(`CREATE TABLE `+collName+` (
		_id     VARCHAR(64) PRIMARY KEY,
		name    VARCHAR(255),
		profile TEXT
	) ENGINE=InnoDB CHARSET=utf8mb4`).Error)
	defer func() { _ = gdb.Exec("DROP TABLE IF EXISTS " + collName).Error }()

	// 启动 Change Stream 订阅（goroutine，持续到 ctx cancel）
	src := source.NewMongoIncrSource(source.NewGoMongoWatcher(coll, logger.NewNopLogger()), collName, logger.NewNopLogger())
	eventCh := make(chan source.ChangeEvent, 16)
	subErr := make(chan error, 1)
	go func() { subErr <- src.IncrSubscribe(ctx, domain.Checkpoint{}, eventCh) }()

	// Change Stream 建立有延迟；等一下，再看是否已失败（多半 Mongo 非副本集）
	time.Sleep(time.Second)
	select {
	case e := <-subErr:
		t.Skipf("mongo change stream unavailable（需副本集，本地跑 rs.initiate() 一次）: %v", e)
	default:
	}

	// 订阅就绪后写：1 insert + 1 update（Change Stream 只捕获订阅之后的变更）
	_, err = coll.InsertOne(ctx, bson.M{"_id": "u1", "name": "alice", "profile": bson.M{"city": "SG"}})
	require.NoError(t, err)
	_, err = coll.UpdateOne(ctx, bson.M{"_id": "u1"}, bson.M{"$set": bson.M{"name": "alice2"}})
	require.NoError(t, err)

	// 收 2 个事件，逐个 transform + apply（insert/update 都走 upsert）
	snk := sink.NewMySQLSink(gdb, collName, "_id", logger.NewNopLogger())
	tf := transform.MongoToRelationalTransformer{}
	deadline := time.After(15 * time.Second)
	for got := 0; got < 2; {
		select {
		case ev := <-eventCh:
			m, terr := tf.Transform(sink.Mutation{Op: ev.Op, Table: ev.Table, PK: ev.PK, Cols: ev.After})
			require.NoError(t, terr)
			require.NoError(t, snk.Apply(ctx, []sink.Mutation{m}))
			got++
		case e := <-subErr:
			t.Fatalf("change stream ended early: %v", e)
		case <-deadline:
			t.Fatalf("timeout waiting for change events, got %d", got)
		}
	}

	// 断言 MySQL：u1 最终 name=alice2（update 事件 fullDocument 落库），profile 嵌套成 JSON 列
	var row struct {
		Name    string `gorm:"column:name"`
		Profile string `gorm:"column:profile"`
	}
	require.NoError(t, gdb.Raw("SELECT name, profile FROM "+collName+" WHERE _id = ?", "u1").Scan(&row).Error)
	assert.Equal(t, "alice2", row.Name)
	assert.JSONEq(t, `{"city":"SG"}`, row.Profile)
}
