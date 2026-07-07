package consts

import "time"

const (
	// RelationStatsTTL 关系聚合计数缓存过期时间。
	RelationStatsTTL = 24 * time.Hour

	// RelationStatsPattern relation:stats:{uid}
	RelationStatsPattern = "relation:stats:%d"
)
