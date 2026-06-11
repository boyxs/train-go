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
	"github.com/webook/migrator/repository"
	"github.com/webook/migrator/repository/dao"
	"github.com/webook/migrator/service"
	"github.com/webook/migrator/service/verify"
	"github.com/webook/pkg/logger"
)

// verifyTaskStub 最小 TaskService：VerifyEngine 的 Full/Sample 只调 Get；
// 其余方法不触达（嵌入 nil 接口，被调到会 panic，正好暴露非预期调用）。
type verifyTaskStub struct {
	service.TaskService
	task domain.Task
}

func (s verifyTaskStub) Get(context.Context, int64) (domain.Task, error) { return s.task, nil }

// TestMySQL_E2E_VerifyMongoDst 端到端验证「异构对账：MySQL 源 vs Mongo 目标」零假阳性 + 能检出差异。
//
// 覆盖上一轮新增的异构 verify 代码路径（单测已覆盖逻辑，这里加真 infra 闭环）：
//   - source/factory.go BuildDst 的 mongo 分支（sinkType=mongo 时复用 MongoSource 读 dst collection）；
//   - verify.go normalizeRows 去 _id（MongoSink 注入的 _id 是 PK 回显，行已按 PK 匹配，不参与字段比对）。
//
// 关键断言：干净迁移后 Full() 返 0 —— 若不丢 _id，dst 每行都会比 src 多一个 _id 字段被误报 diff。
//
// 前置：mysql.dsn 可达（含 webook_migrator_test 控制库）+ migrator.mongo.{uri,database} 指向可用 Mongo。
// 缺任一前置 → Skip（避免污染 go test ./... 全量回归）。
func TestMySQL_E2E_VerifyMongoDst(t *testing.T) {
	mongoURI := viper.GetString("migrator.mongo.uri")
	mongoDB := viper.GetString("migrator.mongo.database")
	dsnStr := viper.GetString("mysql.dsn")
	if mongoURI == "" || mongoDB == "" {
		t.Skip("migrator.mongo.{uri,database} 未配置，跳过异构 verify e2e")
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
	// 控制库表（validate_log 等），缺库直接 skip
	if err := dao.InitTable(gdb); err != nil {
		t.Skipf("migrator 控制库不可用（先建 webook_migrator_test 库）：%v", err)
	}

	mcli, err := mongo.Connect(ctx, mongoopts.Client().ApplyURI(mongoURI).SetServerSelectionTimeout(3*time.Second))
	if err != nil {
		t.Skipf("mongo connect failed: %v", err)
	}
	defer func() { _ = mcli.Disconnect(context.Background()) }()
	if err := mcli.Ping(ctx, nil); err != nil {
		t.Skipf("mongo unreachable: %v", err)
	}

	const name = "article_verify_mongo_e2e"
	const taskId int64 = 990001

	// MySQL 源表 + 2 行（只用 BIGINT + VARCHAR，避免 TEXT 列 driver 类型回环歧义）
	require.NoError(t, gdb.Exec("DROP TABLE IF EXISTS "+name).Error)
	require.NoError(t, gdb.Exec("CREATE TABLE "+name+` (
		id BIGINT PRIMARY KEY, title VARCHAR(255), author VARCHAR(128)
	) ENGINE=InnoDB CHARSET=utf8mb4`).Error)
	require.NoError(t, gdb.Exec("INSERT INTO "+name+" (id,title,author) VALUES (1,'第一篇','alice'),(2,'第二篇','bob')").Error)
	defer func() { _ = gdb.Exec("DROP TABLE IF EXISTS " + name).Error }()

	// Mongo 目标 collection（Drop 当 operability gate：连得上但操作失败如开了 auth → skip 而非 fail）
	coll := mcli.Database(mongoDB).Collection(name)
	if derr := coll.Drop(ctx); derr != nil {
		t.Skipf("mongo not usable for e2e (auth/permission?): %v", derr)
	}
	defer func() { _ = coll.Drop(context.Background()) }()

	// 清本 task 历史对账日志，保证 verify 重跑幂等（validate_log 有去重唯一索引，重复同 kind 行会撞）
	require.NoError(t, gdb.Exec("DELETE FROM validate_log WHERE task_id = ?", taskId).Error)
	defer func() { _ = gdb.Exec("DELETE FROM validate_log WHERE task_id = ?", taskId).Error }()

	l := logger.NewNopLogger()

	// 1. 迁移 MySQL→Mongo（MySQLSource → MongoSink，无 transform），把 dst 灌成与 src 一致
	migSrc := source.NewMySQLSource(gdb, name, "id", l)
	migSnk := sink.NewMongoSink(coll, "id", l)
	out := make(chan source.Row, 16)
	scanErr := make(chan error, 1)
	go func() {
		defer close(out)
		scanErr <- migSrc.FullScan(ctx, source.ShardSpec{No: 0, PKMin: 1, PKMax: 100, BatchSz: 100}, out)
	}()
	var muts []sink.Mutation
	for row := range out {
		muts = append(muts, sink.Mutation{Op: sink.OpInsert, Table: row.Table, PK: row.PK, Cols: row.Cols})
	}
	require.NoError(t, <-scanErr)
	require.Len(t, muts, 2)
	require.NoError(t, migSnk.Apply(ctx, muts))

	// 2. 构造真实 VerifyEngine（mysql + mongo 双 builder，mirror ioc.InitSourceFactory/InitSinkFactory 接线）
	mongoSrcBuilder := func(collection, _ string) (source.FullSource, error) {
		c := mcli.Database(mongoDB).Collection(collection)
		return source.NewMongoSource(source.NewGoMongoScanner(c, l), collection, l), nil
	}
	srcFactory := source.NewSourceFactory(gdb, l, source.WithMongoSourceBuilder(mongoSrcBuilder))
	heteroSink := func(_ domain.Task, tm domain.TableMapping) (sink.Sink, error) {
		c := mcli.Database(mongoDB).Collection(tm.Dst)
		return sink.NewMongoSink(c, tm.PartitionKey, l), nil
	}
	sinkFactory := sink.NewSinkFactory(gdb, l, sink.WithHeteroBuilder(heteroSink))
	reg := transform.NewRegistry()
	reg.Register(transform.TransformMongoToRelational, transform.MongoToRelationalTransformer{})

	task := domain.Task{
		Id:         taskId,
		SourceType: domain.SourceTypeMySQL,
		SinkType:   "mongo",
		TablesJSON: `[{"src":"` + name + `","dst":"` + name + `","partitionKey":"id"}]`,
	}
	eng := verify.NewVerifyEngine(
		verifyTaskStub{task: task},
		repository.NewValidateLogRepository(dao.NewGormValidateLogDAO(gdb)),
		srcFactory, sinkFactory,
		l, reg,
		verify.Config{},
	)

	// 3. 干净迁移后：异构对账零假阳性（dst Mongo 文档的 _id 被 normalize 丢弃，不误报 extra 字段）
	mismatch, err := eng.Full(ctx, taskId)
	require.NoError(t, err)
	assert.Equal(t, int64(0), mismatch, "干净的 MySQL→Mongo 迁移异构对账应零差异")

	// 4. 注入一处差异（改 Mongo dst 一篇标题）→ 再对账应检出恰好 1 处 diff
	require.NoError(t, gdb.Exec("DELETE FROM validate_log WHERE task_id = ?", taskId).Error)
	_, err = coll.UpdateOne(ctx, bson.M{"_id": "1"}, bson.M{"$set": bson.M{"title": "被改脏了"}})
	require.NoError(t, err)
	mismatch, err = eng.Full(ctx, taskId)
	require.NoError(t, err)
	assert.Equal(t, int64(1), mismatch, "dst 改一行标题后异构对账应检出 1 处差异")
}
