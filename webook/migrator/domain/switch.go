package domain

// Stage 切流四阶段（architecture.md §5.1，行业标准 DTS / gh-ost / DataX 同名）。
type Stage string

const (
	// StageSrcOnly 默认态；读写都在源。
	StageSrcOnly Stage = "SRC_ONLY"
	// StageSrcFirst 双写过渡，源主目标异步同步；切读由 gray% 控制。
	StageSrcFirst Stage = "SRC_FIRST"
	// StageDstFirst cutover 启动后的 30 秒过渡期；两侧同步双写，全读 DST。
	StageDstFirst Stage = "DST_FIRST"
	// StageDstOnly 切流完成；单写目标，OLD 停滞、不可逆（point of no return）。
	StageDstOnly Stage = "DST_ONLY"
)

// Valid 检查 Stage 值合法（用于 API 入参校验）。
func (s Stage) Valid() bool {
	switch s {
	case StageSrcOnly, StageSrcFirst, StageDstFirst, StageDstOnly:
		return true
	}
	return false
}

// CanTransitionTo 校验当前 stage → next 是否合法。
//
// 顺序：SRC_ONLY → SRC_FIRST → DST_FIRST → DST_ONLY（不允许跳跃）
// rollback（双写期 → SRC_FIRST）走独立 Rollback + Stage.CanRollback 判定，不在此函数。
func (s Stage) CanTransitionTo(next Stage) bool {
	transitions := map[Stage]Stage{
		StageSrcOnly:  StageSrcFirst,
		StageSrcFirst: StageDstFirst,
		StageDstFirst: StageDstOnly,
	}
	expected, ok := transitions[s]
	return ok && expected == next
}

// CanRollback 校验当前 stage 是否可回滚到 SRC_FIRST。
//
// 仅双写期（SRC_FIRST / DST_FIRST，OLD 仍持续双写、有全量数据）可安全回滚：
//   - SRC_ONLY：未进入双写，无可回滚
//   - DST_ONLY：单写后 OLD 停滞，切回会读到脏数据，不可逆（point of no return）
func (s Stage) CanRollback() bool {
	return s == StageSrcFirst || s == StageDstFirst
}
