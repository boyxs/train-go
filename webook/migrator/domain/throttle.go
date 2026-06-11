package domain

// ThrottleConfig task 级限速配置（Throttle 端点写入，下次 Start 时回读覆盖 ShardSpec）。
type ThrottleConfig struct {
	QPS          int
	ShardWorkers int
}

// Empty 两项都未设置（<=0）→ 视为清空配置（恢复默认）。
func (c ThrottleConfig) Empty() bool {
	return c.QPS <= 0 && c.ShardWorkers <= 0
}
