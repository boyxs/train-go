package sink

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/boyxs/train-go/webook/migrator/domain"
	"github.com/boyxs/train-go/webook/migrator/pipeline/dsn"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

// SinkFactory 按 task + 表下标动态构造 Sink 实例。
//
// 一个 task 可承载多张表，调用方按 task.Tables() 长度遍历 tableIdx 调 BuildSrc/BuildDst。
// 异构 sink 切换：按 task.SinkType（mysql/es/clickhouse/mongo/kafka）在工厂内部分发到对应实现。
type SinkFactory interface {
	BuildSrc(ctx context.Context, task domain.Task, tableIdx int) (Sink, error)
	BuildDst(ctx context.Context, task domain.Task, tableIdx int) (Sink, error)
}

// HeteroSinkBuilder 异构 sink builder：按 task.SinkType / TableMapping 构造对应 Sink。
// 由 ioc 注入（持有 ES client / Kafka producer 等共享资源）；nil 时未启用异构 dispatch。
type HeteroSinkBuilder func(task domain.Task, tm domain.TableMapping) (Sink, error)

// SinkFactoryOption functional option 模式配置 InternalSinkFactory。
type SinkFactoryOption func(f *InternalSinkFactory)

// WithHeteroBuilder 注入异构 Sink builder（ioc 层装配 ES/CK/Mongo/Kafka client）。
func WithHeteroBuilder(b HeteroSinkBuilder) SinkFactoryOption {
	return func(f *InternalSinkFactory) { f.heteroBuilder = b }
}

// WithDBResolver 让工厂按 task 解析独立的 src/dst db 连接（默认 nil → 回退用 NewSinkFactory 传入的 db）。
func WithDBResolver(r dsn.Resolver) SinkFactoryOption {
	return func(f *InternalSinkFactory) { f.resolver = r }
}

// InternalSinkFactory 是 SinkFactory 的默认实现，按 task.SinkType 分发 mysql/es/clickhouse/mongo/kafka（异构 builder 由 ioc 注入）。
// task.SinkType == "mysql" / 空 时构造 MySQLSink；其它 sink_type 调 heteroBuilder（未注入则 error）。
type InternalSinkFactory struct {
	db            *gorm.DB // fallback db（resolver=nil 时用）
	l             logger.LoggerX
	heteroBuilder HeteroSinkBuilder
	resolver      dsn.Resolver
}

// NewSinkFactory 构造 SinkFactory；opts 配置可选行为（如 WithHeteroBuilder 启用异构分发 / WithDBResolver 按 task 解析）。
func NewSinkFactory(db *gorm.DB, l logger.LoggerX, opts ...SinkFactoryOption) SinkFactory {
	f := &InternalSinkFactory{db: db, l: l}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

func (f *InternalSinkFactory) BuildSrc(ctx context.Context, task domain.Task, tableIdx int) (Sink, error) {
	tm, err := task.PickTable(tableIdx)
	if err != nil {
		return nil, err
	}
	db, err := f.resolveSrcDB(ctx, task)
	if err != nil {
		return nil, err
	}
	// Src Sink：源端恒为 MySQL（异构只发生在 dst 侧），故 src Sink 始终是 MySQLSink
	return NewMySQLSink(db, tm.Src, tm.PartitionKey, f.l), nil
}

func (f *InternalSinkFactory) BuildDst(ctx context.Context, task domain.Task, tableIdx int) (Sink, error) {
	tm, err := task.PickTable(tableIdx)
	if err != nil {
		return nil, err
	}
	if task.SinkType == "" || task.SinkType == "mysql" {
		db, derr := f.resolveDstDB(ctx, task)
		if derr != nil {
			return nil, derr
		}
		return NewMySQLSink(db, tm.Dst, tm.PartitionKey, f.l), nil
	}
	if f.heteroBuilder == nil {
		return nil, fmt.Errorf("hetero sink builder not configured for sink_type %q", task.SinkType)
	}
	return f.heteroBuilder(task, tm)
}

// resolveSrcDB 优先 resolver；fallback f.db。
func (f *InternalSinkFactory) resolveSrcDB(ctx context.Context, task domain.Task) (*gorm.DB, error) {
	if f.resolver != nil {
		db, err := f.resolver.ResolveSrc(ctx, task)
		if err != nil {
			return nil, fmt.Errorf("resolve src db: %w", err)
		}
		return db, nil
	}
	return f.db, nil
}

func (f *InternalSinkFactory) resolveDstDB(ctx context.Context, task domain.Task) (*gorm.DB, error) {
	if f.resolver != nil {
		db, err := f.resolver.ResolveDst(ctx, task)
		if err != nil {
			return nil, fmt.Errorf("resolve dst db: %w", err)
		}
		return db, nil
	}
	return f.db, nil
}
