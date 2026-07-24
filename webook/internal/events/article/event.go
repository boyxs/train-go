package article

const TopicArticleEvents = "article_events"

// 事件类型取值（ArticleEvent.Type）。
const (
	TypePublished = "published"
	TypeWithdrawn = "withdrawn"
)

// ArticleEvent Kafka 文章事件。core 在 Publish/Withdraw 写库成功后生产（失败仅记日志不阻断主流程）。
// 消费方（worker FeedArticleConsumer → feed 扩散/移除）定义同构结构（topic+JSON 契约，两端不共享代码，
// 由 worker 侧 contract_test 守护）。key=authorId，保证同作者 publish→withdraw 有序。
type ArticleEvent struct {
	Type      string `json:"type"`      // 见 TypePublished / TypeWithdrawn
	ArticleId int64  `json:"articleId"` // 文章 id
	AuthorId  int64  `json:"authorId"`  // 作者 id（分区 key）
	Ts        int64  `json:"ts"`        // 事件时间（Unix 毫秒）
}
