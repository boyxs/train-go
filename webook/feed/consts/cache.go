package consts

// Redis 键模式：feed 全 Redis 投影（无 MySQL），键统一 feed: 前缀。
// cap / TTL 走 config（feed 业务段可逐环境调），不写死在此。
const (
	// InboxPattern 收件箱 ZSET：member=articleId，score=publishedAt。cap 由 inbox_cap 裁剪。
	InboxPattern = "feed:inbox:%d" // %d=uid
	// InboxBuiltPattern 收件箱已完整重建标记（String "1"）：区分「未建」vs「建了但空」。
	InboxBuiltPattern = "feed:inbox:built:%d" // %d=uid
	// BigvPattern 该用户关注中的大 V uid 集合（SET），重建时算出，读时归并其 outbox。
	BigvPattern = "feed:bigv:%d" // %d=uid
	// OutboxPattern 作者最近文章投影 ZSET：member=articleId，score=publishedAt。cache-aside。
	OutboxPattern = "feed:outbox:%d" // %d=authorId
)
