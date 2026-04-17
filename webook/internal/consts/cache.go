package consts

import "time"

// 缓存 TTL
var (
	CacheTTL       = time.Minute * 30
	InteractionTTL = 24 * time.Hour
	FirstPageTTL   = 3 * time.Minute
)

// Redis 键模式
const (
	UserPattern             = "user:%d"                    // user:{uid}
	UserSsidPattern         = "user:ssid:%s"               // user:ssid:{ssid}
	ArticlePattern          = "article:author:%d:%d"       // article:author:{uid}:{id}
	ArticlePubPattern       = "article:pub:%d"             // article:pub:{id}
	InteractionPattern      = "interaction:%s:%d"          // interaction:{biz}:{bizId}
	InteractionStatePattern = "interaction:state:%s:%d:%d" // interaction:state:{biz}:{bizId}:{uid}
	ReaderFirstPageKey      = "article:reader:first_page"

	ChatConvPattern      = "chat:conv:list:%d" // chat:conv:list:{uid}
	ChatMsgPattern       = "chat:msg:list:%d"  // chat:msg:list:{convId}
	ChatRateLimitPattern = "chat:ratelimit:%d" // chat:ratelimit:{uid}
	ChatStreamPattern    = "chat:stream:%d"    // chat:stream:{convId} Redis Stream

	EmbeddingCachePattern = "embedding:cache:%s" // embedding:cache:{textHash}

	ClickEventDashboardKey = "click:event:ai:dashboard" // AI 点击看板缓存

	PolishRateLimitPattern = "polish:ratelimit:%d" // polish:ratelimit:{uid}
)

var ClickEventDashboardTTL = 10 * time.Minute
var ChatStreamTTL = 5 * time.Minute // 生成完成后 Stream 保留 5 分钟供重连
