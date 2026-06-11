package domain

// ValidateLog 一条对账差异记录（持久化于 validate_log 表）。
//
// DiffDetail 是 JSON 串（verify 引擎 diffAndLog 写入）：
//
//	{ "src": {col: val, ...}, "dst": {col: val, ...}, "diff_fields": [...] }
//
// missing（src 有 dst 无）只含 src 键；extra（dst 有 src 无）只含 dst 键。
type ValidateLog struct {
	Id           int64
	TaskId       int64
	Direction    string // consts.DirectionSrcToDst
	BizTable     string
	BizId        string
	MismatchKind string // consts.MismatchKindMissing / MismatchKindDiff / MismatchKindExtra
	DiffDetail   string
	Repaired     int8 // 0=未修复 1=已修复
	CreatedAt    int64
	RepairedAt   int64
}
