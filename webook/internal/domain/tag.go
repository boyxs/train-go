package domain

// Tag 标签本体（core BFF 侧 domain，与 tag 服务 domain 字段对齐）。
type Tag struct {
	Id             int64  `json:"id"`
	Name           string `json:"name"`
	Slug           string `json:"slug"`
	Type           string `json:"type"`
	Description    string `json:"description"`
	RefCount       int64  `json:"refCount"`
	FollowCount    int64  `json:"followCount"`
	WeeklyNewCount int64  `json:"weeklyNewCount"`
}

// TagCount facet / 推荐项：标签 slug + 计数；Name 由 tag.TagsBySlugs 补齐（search 只回 slug）。
type TagCount struct {
	Slug  string `json:"slug"`
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

// TaggedArticle 搜索 / 标签页结果项：文章 + 已解析标签 + 互动计数。
// 不内嵌 Article：Article.Tags([]string slug) 与本类型 Tags([]Tag) 同名会遮蔽，显式列字段更清晰。
type TaggedArticle struct {
	Id         int64  `json:"id"`
	Title      string `json:"title"`
	Abstract   string `json:"abstract"`
	Author     Author `json:"author"`
	Category   string `json:"category"`
	CreatedAt  int64  `json:"createdAt"`
	Tags       []Tag  `json:"tags"`
	ReadCnt    int64  `json:"readCnt"`
	LikeCnt    int64  `json:"likeCnt"`
	CollectCnt int64  `json:"collectCnt"`
}

// SearchResult /search/article 与 /tag/:slug/articles 的聚合结果：列表 + 总数 + facet（搜索才有）。
type SearchResult struct {
	Articles []TaggedArticle `json:"articles"`
	Total    int64           `json:"total"`
	Facets   []TagCount      `json:"facets"`
}
