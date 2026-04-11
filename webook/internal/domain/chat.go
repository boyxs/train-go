package domain

// Conversation AI 客服对话
type Conversation struct {
	Id        int64  `json:"id"`
	UserId    int64  `json:"userId"`
	Title     string `json:"title"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

// Message 对话消息
type Message struct {
	Id             int64  `json:"id"`
	ConversationId int64  `json:"conversationId"`
	Role           string `json:"role"`
	Content        string `json:"content"`
	ToolCalls      string `json:"toolCalls,omitempty"`
	TokenUsed      int    `json:"tokenUsed"`
	CreatedAt      int64  `json:"createdAt"`
}

// ChatEvent SSE 事件
type ChatEvent struct {
	Type    string `json:"type"`    // "delta" | "tool_call" | "tool_result" | "error" | "done"
	Content string `json:"content"`
	Data    any    `json:"data,omitempty"`
}

// ArticleCard 工具结果中携带的文章摘要（SSE tool_result 事件）
type ArticleCard struct {
	Id       int64  `json:"id"`
	Title    string `json:"title"`
	Abstract string `json:"abstract"`
	Url      string `json:"url"`
}

// ToolResultData tool_result SSE 事件中的结构化数据
type ToolResultData struct {
	CallId   string        `json:"callId"`
	Name     string        `json:"name"`
	Articles []ArticleCard `json:"articles,omitempty"`
	Error    string        `json:"error,omitempty"`
}
