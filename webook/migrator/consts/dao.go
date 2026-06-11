// Package consts 集中 migrator 服务内跨包共享的枚举与常量。
// DAO / Service / Handler / 业务侧 SDK 一律 import 本包，避免分散在各 dao 文件造成耦合。
package consts

// ── Phase 阶段 ────────────────────────────────────────────
const (
	PhaseFull = "full"
	PhaseIncr = "incr"
)

// ── CursorKind 游标类型 ───────────────────────────────────
const (
	CursorKindIDRange     = "id_range"
	CursorKindBinlog      = "binlog_pos"
	CursorKindGTID        = "gtid"
	CursorKindResumeToken = "resume_token" // Mongo Change Stream resume token
)

// ── Direction 对账方向 ────────────────────────────────────
const (
	DirectionSrcToDst = "src_to_dst"
	DirectionDstToSrc = "dst_to_src"
)

// ── MismatchKind 对账差异类型 ─────────────────────────────
const (
	MismatchKindMissing = "missing"
	MismatchKindExtra   = "extra"
	MismatchKindDiff    = "diff"
)

// ── AuditAction 审计动作（扁平字符串，self-describing）────
const (
	AuditActionCreate           = "create"
	AuditActionPreflight        = "preflight"
	AuditActionStartFull        = "start_full"
	AuditActionStartIncr        = "start_incr"
	AuditActionPause            = "pause"
	AuditActionThrottle         = "throttle"
	AuditActionSetGray          = "set_gray"
	AuditActionSetStageSRCFirst = "set_stage_SRC_FIRST"
	AuditActionCutoverPropose   = "cutover_propose"
	AuditActionCutoverApprove   = "cutover_approve"
	AuditActionRollback         = "rollback"
	AuditActionVerify           = "verify"
	AuditActionRepair           = "repair"
	AuditActionReplayDL         = "replay_dl"
)

// ── AuditResult 审计结果 ──────────────────────────────────
const (
	AuditResultSuccess = "success"
	AuditResultFail    = "fail"
)

// ── DLOp 死信操作类型 ────────────────────────────────────
const (
	DLOpInsert = "insert"
	DLOpUpdate = "update"
	DLOpDelete = "delete"
)
