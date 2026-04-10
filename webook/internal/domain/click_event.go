package domain

type ClickEvent struct {
	UserId         int64
	ArticleId      int64
	ConversationId int64
	Source         string // "ai_chat", "search", "recommendation", etc.
}

type ClickEventDashboard struct {
	TotalClicks      int64        `json:"totalClicks"`
	UniqueUsers      int64        `json:"uniqueUsers"`
	UniqueArticles   int64        `json:"uniqueArticles"`
	AvgClicksPerUser float64      `json:"avgClicksPerUser"`
	DailyTrend       []DailyTrend `json:"dailyTrend"`
	TopArticles      []TopArticle `json:"topArticles"`
}

type DailyTrend struct {
	Date   string `json:"date"`
	Clicks int64  `json:"clicks"`
}

type TopArticle struct {
	Rank        int    `json:"rank"`
	ArticleId   int64  `json:"articleId"`
	Title       string `json:"title"`
	Clicks      int64  `json:"clicks"`
	UniqueUsers int64  `json:"uniqueUsers"`
}
