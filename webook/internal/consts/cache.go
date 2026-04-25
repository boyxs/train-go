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

	// ── 文章榜单 ────────────────────────────────────────────────────────
	// Top N：每个榜单最多保留条数
	ArticleRankingTopN = 100
	// 日榜 ZSet：member=articleId, score=HotScore / publishMs / WilsonLB
	// date 是 YYYY-MM-DD 字符串，作为日分区键
	ArticleRankingZSetPattern = "article_ranking:%s:%s" // article_ranking:{yyyy-mm-dd}:{dim}
	// 分区榜 ZSet：按 category 切片
	ArticleRankingCategoryZSetPattern = "article_ranking:%s:cat:%s" // article_ranking:{yyyy-mm-dd}:cat:{category}
	// 条目详情 Hash：field=articleId, value=JSON(title/author/stats/category)
	ArticleRankingDetailPattern = "article_ranking:%s:detail" // article_ranking:{yyyy-mm-dd}:detail
	// 上一次 rank 快照 Hash：field=articleId, value=上一次 rank，算趋势用
	// 分区榜 5 个 category 各自独立快照，避免互相覆盖
	ArticleRankingPrevRankPattern = "article_ranking:%s:prevrank:%s:%s" // article_ranking:{yyyy-mm-dd}:prevrank:{dim}:{cat}  cat="" 代表总榜
	// 分布式锁：cron 任务抢占
	ArticleRankingLockPattern = "article_ranking:lock:%s:%s" // article_ranking:lock:{dim}:{yyyy-mm-dd}
)

var (
	// 日榜 Redis key 统一 TTL：25h，跨天冗余 1h，由 archive cron 在 00:10 落库后清理
	ArticleRankingTTL = 25 * time.Hour
	// 分布式锁 TTL：55s，保证下一 tick 前自动释放
	ArticleRankingLockTTL = 55 * time.Second
)

var ClickEventDashboardTTL = 10 * time.Minute
var ChatStreamTTL = 5 * time.Minute // 生成完成后 Stream 保留 5 分钟供重连
