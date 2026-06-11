// Package sink 是迁移写端抽象。
//
// 设计：Sink 接口屏蔽"同构 vs 异构"、"MySQL/ES/CK/Mongo/Kafka"的实现差异。
// 引擎层（FullEngine / IncrEngine）只依赖接口，业务路由按 Op 分发，每种 Sink 自己保证幂等。
//
// 实现矩阵（见 architecture.md §8.5）：
//
//	MySQLSink       — 同构（跨机房 / 分库分表 / schema 演进）
//	ESSink          — 异构搜索（重构 internal/repository/dao/article_search.go）
//	ClickHouseSink  — 异构 OLAP（互动 / 点击事件报表）
//	MongoSink       — 异构文档型
//	TiDBSink        — 异构 HTAP
//	KafkaSink       — 异构事件流（下游订阅）
//
// 接入新 Sink 只需实现 Sink 接口 + ioc 注册，引擎层零改动。
package sink

import "context"

// Op 枚举，Sink 实现根据它分发写策略。
const (
	OpInsert = "insert"
	OpUpdate = "update"
	OpDelete = "delete"
)

// Sink 写端抽象。
//
//	Apply  原子写入一批 Mutation；实现必须保证幂等（同一批重放结果一致）
//	Close  释放底层连接 / 客户端
type Sink interface {
	Apply(ctx context.Context, batch []Mutation) error
	Close() error
}

// SrcSink / DstSink 是 Sink 的 named type，用于 wire 区分二实例。
//
// Repair 时 src_overwrite_dst 需要 DstSink；dst_overwrite_src 需要 SrcSink。
// 二者都通过 source.Source(s) cast 实际调用，跟 Source 二实例区分同样手法。
type SrcSink Sink
type DstSink Sink

// Mutation 一行数据变更。
//
// Version 字段用于乐观锁防"老 binlog 覆盖新值"（架构 §3.7 坑 1）：
//   - 双写场景：OLD(t=1) → NEW(t=2) → CDC 回放 OLD(t=1) 时，Sink 应判 NEW.version > t=1 跳过
//   - MySQLSink upsert 在行带 version 时启用乐观锁（VALUES(version) > version 才覆盖，否则普通 upsert）
type Mutation struct {
	Op      string // OpInsert / OpUpdate / OpDelete
	Table   string
	PK      string
	Cols    map[string]any // delete 时可为 nil
	Version int64
}
