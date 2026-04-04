package ai

import "context"

// LLMClient LLM 流式调用接口
type LLMClient interface {
	ChatStream(ctx context.Context, messages []ChatMessage, tools []Tool) (<-chan StreamChunk, error)
}

// ChatMessage 聊天消息
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Tool 工具定义（P1 使用）
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

// StreamChunk 流式响应块
type StreamChunk struct {
	Type    string // "text" | "tool_call" | "done" | "error"
	Content string
	Data    any
}
