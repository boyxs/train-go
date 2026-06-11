package domain

// Checkpoint 引擎层共享的 cursor 抽象。
//
// 字段与 DAO Checkpoint 一一对应，但不携带 Id / UpdatedAt 等基础设施字段。
// Source 实现按 CursorKind 解释 CursorValue：
//
//	id_range    → "PK_last_synced"     用于全量 PK 游标
//	binlog_pos  → "mysql-bin.000001/4" 用于增量 binlog 位置
//	gtid        → "UUID:1-100"         增量 GTID（v1 未实现，CanalSource 显式拒绝）
type Checkpoint struct {
	TaskId          int64
	Phase           string  // consts.PhaseFull / consts.PhaseIncr
	ShardNo         int32   // 全量分片号；增量恒 0
	CursorKind      string  // consts.CursorKindIDRange / CursorKindBinlog / CursorKindGTID
	CursorValue     string  // 按 Kind 解析
	ProgressPercent float64 // 0-100
	LastLagMs       int64   // 增量延迟（毫秒）
	Version         int64   // 乐观锁版本
}
