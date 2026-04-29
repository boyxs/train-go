package llm

import "context"

// Client LLM 调用接口（包名 llm 已表达"LLM 客户端"含义，类型名简洁为 Client）
type Client interface {
	// ChatStream 流式调用，用于对话场景
	ChatStream(ctx context.Context, messages []ChatMessage, tools []Tool) (<-chan StreamChunk, error)
	// Chat 同步调用，用于润色等一次性任务，不支持 tools
	Chat(ctx context.Context, messages []ChatMessage) (string, error)
}
