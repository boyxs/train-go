// Package source 是迁移读端抽象。
//
// 设计：按读取语义拆两个小接口（接口隔离 ISP，每个实现只实现自己能做的，无 NotSupported 半残）：
//   - FullSource（全量扫描）：MySQLSource / ESSource / MongoSource
//   - IncrSource（增量订阅）：CanalSource / MongoIncrSource
//
// 引擎层依赖对应小接口（FullEngine→FullSource / IncrEngine→IncrSource / VerifyEngine→两侧 FullSource）。
// SourceFactory 三方法返回精确类型（BuildFullSrc/BuildIncrSrc/BuildDst），调用方无需试探。
package source

import (
	"context"

	"github.com/webook/migrator/domain"
)

// FullSource 全量读端：按分片全量扫描。
//
//	FullScan  按 ShardSpec 全量扫描，逐行推到 out chan（实现须按 PK 升序发出：引擎游标取"最后发出的 PK"）
//	Close     释放底层连接（DB pool 等）
//
// 实现：MySQLSource / ESSource / MongoSource（全量）。可选实现 PKRanger 暴露 PK 范围供自动切片。
type FullSource interface {
	FullScan(ctx context.Context, shard ShardSpec, out chan<- Row) error
	Close() error
}

// IncrSource 增量读端：订阅 binlog / change stream。
//
//	IncrSubscribe  从 Checkpoint 续订，逐事件推到 out chan
//	Close          释放底层连接（binlog client 等）
//
// 实现：CanalSource / MongoIncrSource。可选实现 LagReporter 暴露延迟。
type IncrSource interface {
	IncrSubscribe(ctx context.Context, ckpt domain.Checkpoint, out chan<- ChangeEvent) error
	Close() error
}

// ShardSpec 全量扫描分片描述。FullEngine 根据 PK 范围切片，并发跑多个 ShardSpec。
type ShardSpec struct {
	No       int   // 分片号（0-based），对应 checkpoint.shard_no
	PKMin    int64 // PK 范围下界（包含）
	PKMax    int64 // PK 范围上界（包含）
	BatchSz  int   // 单次 SELECT 行数；0 → 用默认 1000
	QPSLimit int   // 每秒最多读多少行；0 → 不限速
}

// Row 全量扫描输出。Source 不解释字段类型，原样传 driver 返回的 map。
type Row struct {
	Table string
	PK    string
	Cols  map[string]any
}

// PKRanger 全量类 Source 可实现此接口暴露 PK 范围查询。
//
// FullEngine 启动时如果调用方未提供 ShardSpec[]，handler 调 PKRange 拿 (min, max)
// 后调 full.PlanShards 自动切片。CanalSource 不实现此接口（增量无 PK 范围概念）。
type PKRanger interface {
	PKRange(ctx context.Context) (minPK int64, maxPK int64, err error)
}

// LagReporter 增量类 Source 可实现此接口暴露延迟监控。
//
// 目前仅 CanalSource 实现（基于最近一次 binlog 事件时间戳）；
// MySQLSource 不实现（全量没有 lag 概念）。
// IncrEngine 通过 type assertion 拿到 LagReporter — 未实现时 Lag() 返回 error。
type LagReporter interface {
	Lag(taskId int64) int64 // 毫秒；-1 表示任务尚无事件
}

// ChangeEvent binlog / CDC 事件抽象。Op 枚举 insert / update / delete。
//
//	insert  → After 填，Before 空
//	update  → Before / After 都填
//	delete  → Before 填，After 空
//
// 字段语义与 BinlogEvent 完全一致（CanalSource 内只是 cast 一下名字），
// 用 alias 而非独立 struct 避免维护两份相同字段定义。
// 真要为业务层加额外字段时，把 alias 换回 struct 即可。
type ChangeEvent = BinlogEvent
