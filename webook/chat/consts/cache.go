package consts

import "time"

// chat 服务的 Redis key 模式与 TTL。与主仓 internal/consts/cache.go 保持一致。
const (
	ChatConvPattern      = "chat:conv:list:%d" // chat:conv:list:{uid}
	ChatConvItemPattern  = "chat:conv:%d:%d"   // chat:conv:{uid}:{convId} 单条对话（高频 Find 路径走它）
	ChatMsgPattern       = "chat:msg:list:%d"  // chat:msg:list:{convId}
	ChatRateLimitPattern = "chat:ratelimit:%d" // chat:ratelimit:{uid}
	ChatStreamPattern    = "chat:stream:%d"    // chat:stream:{convId} Redis Stream key
)

var ChatStreamTTL = 5 * time.Minute // 生成完成后 Stream 保留 5 分钟供重连
