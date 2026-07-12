package domain

// Article 搜索领域模型（search 服务视角；向量是索引期算出的细节，不进 domain）。
type Article struct {
	Id         int64
	Title      string
	Abstract   string
	AuthorId   int64
	AuthorName string
	Status     uint8
	Category   string
	Tags       []string
	CreatedAt  int64
	// Content 正文全文，仅索引期用于生成 content_vec（不入库、搜索结果不回填），与 RecommendTags 的 embed 口径对齐。
	Content string
}

// TagCount 标签聚合计数（facet / 推荐候选），Slug 为 tag slug。
type TagCount struct {
	Slug  string
	Count int64
}

// SearchResult 搜索结果：命中文章 + 总数 + 标签 facet。
type SearchResult struct {
	Articles []Article
	Total    int64
	Facets   []TagCount
}
