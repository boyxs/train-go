package consts

// ClickEvent Source 常量。Dashboard 按 Source 过滤/聚合时使用。
const (
	// ClickSourceAIChat AI 聊天里附带的文章卡片点击
	ClickSourceAIChat = "ai_chat"
	// ClickSourceRanking 榜单点击前缀，实际值形如 "ranking:hot:3"
	ClickSourceRanking = "ranking"
	// ClickSourceRankingFormat 榜单点击完整格式：ranking:{dim}:{rank}
	ClickSourceRankingFormat = ClickSourceRanking + ":%s:%d"
)
