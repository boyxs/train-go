package domain

// FeedItem 关注流条目：仅承载 id + 发布时间，正文/计数/标签由 core BFF 聚合。
// PublishedAt 既是 ZSET 排序 score 也是游标（Unix 毫秒）。
type FeedItem struct {
	ArticleId   int64 `json:"articleId"`
	PublishedAt int64 `json:"publishedAt"`
}

// FeedArticle 一篇待扩散/待移除的已发布文章（写路径入参）。
type FeedArticle struct {
	ArticleId   int64
	AuthorId    int64
	PublishedAt int64
}
