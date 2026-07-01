package consts

import "time"

const (
	// InteractionTTL 互动缓存（聚合计数 / 用户状态）过期时间
	InteractionTTL = 24 * time.Hour

	InteractionPattern      = "interaction:%s:%d"          // interaction:{biz}:{bizId}
	InteractionStatePattern = "interaction:state:%s:%d:%d" // interaction:state:{biz}:{bizId}:{uid}
)
