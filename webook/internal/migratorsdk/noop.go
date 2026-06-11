package migratorsdk

import "context"

// NoOpSwitchReader 始终返回 SideOld；用于未启用迁移时的默认实现。
//
// 路径上零 Redis / 零 IO，业务方法本身就是直接调 OLD 侧 DAO 的等价物。
type NoOpSwitchReader struct{}

func NewNoOpSwitchReader() SwitchReader { return NoOpSwitchReader{} }

func (NoOpSwitchReader) ChooseSide(_ context.Context, _ string, _ int64) (Side, error) {
	return SideOld, nil
}

// NoOpDualWriter 始终单写 OLD 侧；用于未启用迁移时的默认实现。
//
// 业务接入后即使 migrator 服务不部署，路径上也只是多一层函数调用——
// 比直接调 DAO 慢的开销可忽略（< 5 ns / call）。
type NoOpDualWriter struct{}

func NewNoOpDualWriter() DualWriter { return NoOpDualWriter{} }

func (NoOpDualWriter) Write(_ context.Context, _ string, fn func(side Side) error) error {
	return fn(SideOld)
}
