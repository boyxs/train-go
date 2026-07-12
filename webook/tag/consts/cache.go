package consts

import "time"

const (
	// TagDetailTTL 标签详情缓存过期。含 weekly_new_count 时窗字段（无写触发失效），短 TTL 把漂移兜到 ≤15min。
	TagDetailTTL = 10 * time.Minute

	// TagDetailPattern tag:detail:{slug}
	TagDetailPattern = "tag:detail:%s"
)
