package llm

// ChatMessage 聊天消息，兼容 user/assistant/system/tool 四种 role
type ChatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []ToolCallData `json:"tool_calls,omitempty"`   // assistant 发出工具调用时
	ToolCallId string         `json:"tool_call_id,omitempty"` // tool role 回填结果时
}

// ToolCallData assistant 消息中携带的工具调用（对应 OpenAI tool_calls 字段）
type ToolCallData struct {
	Id       string           `json:"id"`
	Type     string           `json:"type"` // 固定 "function"
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction 工具函数名和参数
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON 字符串
}

// Tool 工具定义，传给 LLM
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

// StreamChunk 流式响应块
type StreamChunk struct {
	Type      string // "text" | "tool_call" | "done" | "error"
	Content   string
	ToolCalls []StreamToolCall // Type == "tool_call" 时有效
	Usage     *StreamUsage     // Type == "done" 时有效（如果 API 返回）
}

// StreamUsage 本次调用的 token 消耗
type StreamUsage struct {
	PromptTokens     int
	CompletionTokens int
}

// StreamToolCall LLM 流中拼接完整的单次工具调用
type StreamToolCall struct {
	Id   string         `json:"id"`
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}
