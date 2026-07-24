package event

// ArticleEvent 与 core 生产端约定的文章事件（跨服务契约，两端各自定义、不共享代码）。
// feed 写扩散/移除的源头：published → 扩散进粉丝收件箱；withdrawn → DEL 作者发件箱。
type ArticleEvent struct {
	Type      string `json:"type"`      // published / withdrawn
	ArticleId int64  `json:"articleId"` // 文章 ID
	AuthorId  int64  `json:"authorId"`  // 作者 ID
	Ts        int64  `json:"ts"`        // 事件时间（Unix 毫秒），扩散时作收件箱 score
}

// TopicArticleEvents 必须与 core 生产端一致。
const TopicArticleEvents = "article_events"

// 事件类型取值（ArticleEvent.Type），与 core 一致。
const (
	ArticleTypePublished = "published"
	ArticleTypeWithdrawn = "withdrawn"
)
