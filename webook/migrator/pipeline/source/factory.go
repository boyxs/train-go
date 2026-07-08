package source

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/boyxs/train-go/webook/migrator/domain"
	"github.com/boyxs/train-go/webook/migrator/pipeline/dsn"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

// SourceFactory 按 task + 表下标动态构造读端实例。方法返回类型即语义，调用方无需试探。
//
// 一个 task 可承载多张表，调用方按 task.Tables() 长度遍历 tableIdx 调相应 Build* 方法。
//
// 三方法按「读取语义 + SourceType/SinkType」分发：
//
//	BuildFullSrc  全量扫描 src，按 task.SourceType 分发（mysql→MySQLSource / mongo→MongoSource）→ FullSource
//	BuildIncrSrc  增量订阅 src，mysql cdc→Canal / mongo→change stream → IncrSource
//	BuildDst   对账全量读 dst，按 task.SinkType 分发（mysql→MySQLSource / es→ESSource / mongo→MongoSource）→ FullSource
type SourceFactory interface {
	BuildFullSrc(ctx context.Context, task domain.Task, tableIdx int) (FullSource, error)
	BuildIncrSrc(ctx context.Context, task domain.Task, tableIdx int) (IncrSource, error)
	BuildDst(ctx context.Context, task domain.Task, tableIdx int) (FullSource, error)
}

// CanalClientBuilder 按 task 配置构造 BinlogClient（用于 cdc 模式工厂分发）。
// 由 ioc 注入；nil 表示未启用 Canal，mysql 增量（BuildIncrSrc）将报错（mysql 增量必须 Canal）。
type CanalClientBuilder func(task domain.Task) (BinlogClient, error)

// ESSourceBuilder 按 indexName + pkField 构造 ESSource（用于 sinkType=es 的 BuildDst 分发）。
// 由 ioc 注入；nil 时 sinkType=es 的 BuildDst 返 error。
type ESSourceBuilder func(indexName, pkField string) (FullSource, error)

// MongoSourceBuilder 按 collection + pkField 构造全量 MongoSource（BuildFullSrc / BuildDst 用）。
// 由 ioc 注入；nil 时 sourceType/sinkType=mongo 返 error。
type MongoSourceBuilder func(collection, pkField string) (FullSource, error)

// MongoIncrSourceBuilder 按 collection + pkField 构造增量 MongoSource（Change Stream，BuildIncrSrc 用）。
// 由 ioc 注入；nil 时 sourceType=mongo 的 BuildIncrSrc 返 error。
type MongoIncrSourceBuilder func(collection, pkField string) (IncrSource, error)

// SourceFactoryOption functional option 模式配置 InternalSourceFactory（避免链式 With* 破坏「ctor 返接口」规范）。
type SourceFactoryOption func(f *InternalSourceFactory)

// WithCanalClient BuildIncrSrc 在 task.Mode == cdc 时优先返 CanalSource。
func WithCanalClient(b CanalClientBuilder) SourceFactoryOption {
	return func(f *InternalSourceFactory) { f.canalClientBuilder = b }
}

// WithDBResolver 让工厂按 task 解析独立的 src/dst db 连接（默认 nil → 回退用 NewSourceFactory 传入的 db）。
func WithDBResolver(r dsn.Resolver) SourceFactoryOption {
	return func(f *InternalSourceFactory) { f.resolver = r }
}

// WithESSourceBuilder 让 BuildDst 在 task.SinkType=es 时返 ESSource。
// 未注入时 sinkType=es 的 BuildDst 返 error。
func WithESSourceBuilder(b ESSourceBuilder) SourceFactoryOption {
	return func(f *InternalSourceFactory) { f.esBuilder = b }
}

// WithMongoSourceBuilder 让 BuildFullSrc 在 task.SourceType=mongo 时返 MongoSource。
// 未注入时 sourceType=mongo 的 BuildFullSrc 返 error。
func WithMongoSourceBuilder(b MongoSourceBuilder) SourceFactoryOption {
	return func(f *InternalSourceFactory) { f.mongoSrcBuilder = b }
}

// WithMongoIncrSourceBuilder 让 BuildIncrSrc 在 task.SourceType=mongo 时返增量 MongoSource（Change Stream）。
func WithMongoIncrSourceBuilder(b MongoIncrSourceBuilder) SourceFactoryOption {
	return func(f *InternalSourceFactory) { f.mongoIncrBuilder = b }
}

// InternalSourceFactory 是 SourceFactory 的默认实现，按 task.SourceType 分发 mysql/mongo（异构 builder 由 ioc 注入）。
type InternalSourceFactory struct {
	db                 *gorm.DB // fallback db（resolver=nil 时用，本地演示模式）
	l                  logger.LoggerX
	canalClientBuilder CanalClientBuilder
	resolver           dsn.Resolver
	esBuilder          ESSourceBuilder        // sinkType=es 时 BuildDst 走这个
	mongoSrcBuilder    MongoSourceBuilder     // sourceType=mongo 时 BuildFullSrc 走这个
	mongoIncrBuilder   MongoIncrSourceBuilder // sourceType=mongo 时 BuildIncrSrc 走这个
}

// NewSourceFactory 构造 SourceFactory；opts 控制可选行为（WithCanalClient 启用 cdc 分发 / WithDBResolver 启用按 task DSN 解析）。
func NewSourceFactory(db *gorm.DB, l logger.LoggerX, opts ...SourceFactoryOption) SourceFactory {
	f := &InternalSourceFactory{db: db, l: l}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// BuildFullSrc 按 task.SourceType 分发全量扫描源：mysql→MySQLSource；mongo→MongoSource（需注入 builder）。
func (f *InternalSourceFactory) BuildFullSrc(ctx context.Context, task domain.Task, tableIdx int) (FullSource, error) {
	tm, err := task.PickTable(tableIdx)
	if err != nil {
		return nil, err
	}
	switch task.SourceType.Normalize() {
	case domain.SourceTypeMySQL:
		db, derr := f.resolveSrcDB(ctx, task)
		if derr != nil {
			return nil, derr
		}
		return NewMySQLSource(db, tm.Src, tm.PartitionKey, f.l), nil
	case domain.SourceTypeMongo:
		if f.mongoSrcBuilder == nil {
			return nil, fmt.Errorf("mongo source builder not configured for source_type %q", task.SourceType)
		}
		return f.mongoSrcBuilder(tm.Src, tm.PartitionKey)
	default:
		return nil, fmt.Errorf("BuildFullSrc not implemented for source_type %q (only mysql/mongo)", task.SourceType)
	}
}

// BuildIncrSrc 按 task.SourceType 分发增量订阅源：
// mysql 源走 CanalSource（要求 cdc 模式 + canal client 已注入；否则报错——mysql 增量必须 Canal，
// 不再回退 MySQLSource，接口隔离把"不支持"从运行时提前到构造期）；
// mongo 源 → change stream（需注入 mongoIncrBuilder）。
func (f *InternalSourceFactory) BuildIncrSrc(_ context.Context, task domain.Task, tableIdx int) (IncrSource, error) {
	tm, err := task.PickTable(tableIdx)
	if err != nil {
		return nil, err
	}
	switch task.SourceType.Normalize() {
	case domain.SourceTypeMySQL:
		if task.Mode != domain.ModeCDC || f.canalClientBuilder == nil {
			return nil, fmt.Errorf("mysql incremental requires cdc mode + canal client (mode=%q, canal_configured=%v)", task.Mode, f.canalClientBuilder != nil)
		}
		client, cerr := f.canalClientBuilder(task)
		if cerr != nil {
			return nil, fmt.Errorf("build canal client: %w", cerr)
		}
		return NewCanalSource(client, f.l), nil
	case domain.SourceTypeMongo:
		if f.mongoIncrBuilder == nil {
			return nil, fmt.Errorf("mongo incremental source builder not configured for source_type %q", task.SourceType)
		}
		return f.mongoIncrBuilder(tm.Src, tm.PartitionKey)
	default:
		return nil, fmt.Errorf("BuildIncrSrc not implemented for source_type %q (only mysql/mongo)", task.SourceType)
	}
}

// BuildDst 按 task.SinkType 分发对账全量读 dst 侧的 FullSource：
//
//	sinkType=es         → ESSource（读 ES 索引）
//	sinkType=mysql / 空  → MySQLSource（读 MySQL 表）
//	sinkType=mongo      → MongoSource（读 dst collection）
//	其它（ck/kafka）     → 不支持，返 error
//
// 异构对账（verify）真闭环的关键 — VerifyEngine 透明调 BuildDst，
// 拿到的 FullSource 真实指向 task.SinkType 对应的目标存储。
func (f *InternalSourceFactory) BuildDst(ctx context.Context, task domain.Task, tableIdx int) (FullSource, error) {
	tm, err := task.PickTable(tableIdx)
	if err != nil {
		return nil, err
	}
	switch task.SinkType {
	case "", "mysql":
		db, derr := f.resolveDstDB(ctx, task)
		if derr != nil {
			return nil, derr
		}
		return NewMySQLSource(db, tm.Dst, tm.PartitionKey, f.l), nil
	case "es", "elasticsearch":
		if f.esBuilder == nil {
			return nil, fmt.Errorf("es source builder not configured for sink_type %q", task.SinkType)
		}
		return f.esBuilder(tm.Dst, tm.PartitionKey)
	case "mongo", "mongodb":
		// 对账读 Mongo dst 侧：复用全量 MongoSource（find）读 dst collection
		if f.mongoSrcBuilder == nil {
			return nil, fmt.Errorf("mongo source builder not configured for sink_type %q", task.SinkType)
		}
		return f.mongoSrcBuilder(tm.Dst, tm.PartitionKey)
	default:
		return nil, fmt.Errorf("BuildDst not implemented for sink_type %q (only mysql/es/mongo)", task.SinkType)
	}
}

// resolveSrcDB 优先 resolver；fallback f.db（本地演示模式：src/dst 同库内）。
func (f *InternalSourceFactory) resolveSrcDB(ctx context.Context, task domain.Task) (*gorm.DB, error) {
	if f.resolver != nil {
		db, err := f.resolver.ResolveSrc(ctx, task)
		if err != nil {
			return nil, fmt.Errorf("resolve src db: %w", err)
		}
		return db, nil
	}
	return f.db, nil
}

func (f *InternalSourceFactory) resolveDstDB(ctx context.Context, task domain.Task) (*gorm.DB, error) {
	if f.resolver != nil {
		db, err := f.resolver.ResolveDst(ctx, task)
		if err != nil {
			return nil, fmt.Errorf("resolve dst db: %w", err)
		}
		return db, nil
	}
	return f.db, nil
}
