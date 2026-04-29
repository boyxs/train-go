package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Config 多 provider 列表配置（ioc 层从 yaml 读出后传给 InitLLMClient）
type Config struct {
	Providers []ProviderConfig
}

// ProviderConfig 单个 OpenAI 兼容 provider 的连接参数
type ProviderConfig struct {
	Name      string
	ApiKey    string
	BaseUrl   string
	Model     string
	MaxTokens int
	Timeout   int // 秒
}

// OpenAIClient 兼容 OpenAI 协议的通用客户端（DeepSeek、Kimi 等）
type OpenAIClient struct {
	name   string
	cfg    ProviderConfig
	url    string // 预拼接的完整 API URL
	client *http.Client
}

func NewOpenAIClient(cfg ProviderConfig) *OpenAIClient {
	timeout := time.Duration(cfg.Timeout) * time.Second
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	return &OpenAIClient{
		name: cfg.Name,
		cfg:  cfg,
		url:  strings.TrimRight(cfg.BaseUrl, "/") + "/chat/completions",
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 5,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// openaiRequest OpenAI 兼容 API 请求体
type openaiRequest struct {
	Model         string         `json:"model"`
	Messages      []ChatMessage  `json:"messages"`
	Stream        bool           `json:"stream"`
	StreamOptions *streamOptions `json:"stream_options,omitempty"`
	MaxTokens     int            `json:"max_tokens,omitempty"`
	Temperature   float64        `json:"temperature,omitempty"`
	Tools         []openaiTool   `json:"tools,omitempty"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// openaiTool OpenAI tools 数组元素格式
type openaiTool struct {
	Type     string `json:"type"` // "function"
	Function Tool   `json:"function"`
}

// openaiStreamResponse SSE 响应中的 JSON 结构
type openaiStreamResponse struct {
	Choices []struct {
		Delta struct {
			Content   string                 `json:"content"`
			ToolCalls []openaiStreamToolCall `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *openaiUsage `json:"usage"`
}

type openaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// openaiStreamToolCall tool_call 增量 chunk
type openaiStreamToolCall struct {
	Index    int    `json:"index"`
	Id       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// openaiSyncResponse 同步（非流式）调用的响应结构
type openaiSyncResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (c *OpenAIClient) Chat(ctx context.Context, messages []ChatMessage) (string, error) {
	reqBody := openaiRequest{
		Model:    c.cfg.Model,
		Messages: messages,
		Stream:   false,
	}
	if c.cfg.MaxTokens > 0 {
		reqBody.MaxTokens = c.cfg.MaxTokens
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("[%s] marshal request: %w", c.name, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("[%s] create request: %w", c.name, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.ApiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("[%s] do request: %w", c.name, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return "", fmt.Errorf("[%s] read response: %w", c.name, err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("[%s] API error: status=%d, body=%s", c.name, resp.StatusCode, string(respBody))
	}

	var result openaiSyncResponse
	if err = json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("[%s] unmarshal response: %w", c.name, err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("[%s] empty choices", c.name)
	}
	return result.Choices[0].Message.Content, nil
}

func (c *OpenAIClient) ChatStream(ctx context.Context, messages []ChatMessage, tools []Tool) (<-chan StreamChunk, error) {
	reqBody := openaiRequest{
		Model:         c.cfg.Model,
		Messages:      messages,
		Stream:        true,
		StreamOptions: &streamOptions{IncludeUsage: true},
	}
	if c.cfg.MaxTokens > 0 {
		reqBody.MaxTokens = c.cfg.MaxTokens
	}
	if len(tools) > 0 {
		reqBody.Tools = make([]openaiTool, len(tools))
		for i, t := range tools {
			reqBody.Tools[i] = openaiTool{Type: "function", Function: t}
		}
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("[%s] marshal request: %w", c.name, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("[%s] create request: %w", c.name, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.ApiKey)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("[%s] do request: %w", c.name, err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		// 限制读取 4KB，防止异常响应撑爆内存
		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if readErr != nil {
			return nil, fmt.Errorf("[%s] API error: status=%d, read body failed: %w", c.name, resp.StatusCode, readErr)
		}
		return nil, fmt.Errorf("[%s] API error: status=%d, body=%s", c.name, resp.StatusCode, string(respBody))
	}

	ch := make(chan StreamChunk, 16)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		c.readSSE(ctx, resp.Body, ch)
	}()

	return ch, nil
}

// pendingToolCall 累积 streaming tool_call 增量
type pendingToolCall struct {
	id   string
	name string
	args strings.Builder
}

// readSSE 解析 SSE 流，提取 delta.content 和 tool_calls 并发送到 channel
func (c *OpenAIClient) readSSE(ctx context.Context, body io.Reader, ch chan<- StreamChunk) {
	scanner := bufio.NewScanner(body)
	// pendingCalls 按 index 累积工具调用增量
	pendingCalls := make(map[int]*pendingToolCall)
	// lastUsage 保存最后一个带 usage 的 chunk（stream_options include_usage=true 时出现）
	var lastUsage *StreamUsage

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			ch <- StreamChunk{Type: "error", Content: "请求被取消"}
			return
		default:
		}

		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			ch <- StreamChunk{Type: "done", Usage: lastUsage}
			return
		}

		var resp openaiStreamResponse
		if err := json.Unmarshal([]byte(data), &resp); err != nil {
			continue
		}

		// 捕获 usage（stream_options include_usage=true 时，最后一个 chunk 只有 usage 没有 choices）
		if resp.Usage != nil {
			lastUsage = &StreamUsage{
				PromptTokens:     resp.Usage.PromptTokens,
				CompletionTokens: resp.Usage.CompletionTokens,
			}
		}

		if len(resp.Choices) == 0 {
			continue
		}

		choice := resp.Choices[0]

		// 累积 tool_call 增量
		for _, tc := range choice.Delta.ToolCalls {
			p, ok := pendingCalls[tc.Index]
			if !ok {
				p = &pendingToolCall{}
				pendingCalls[tc.Index] = p
			}
			if tc.Id != "" {
				p.id = tc.Id
			}
			if tc.Function.Name != "" {
				p.name = tc.Function.Name
			}
			p.args.WriteString(tc.Function.Arguments)
		}

		if choice.Delta.Content != "" {
			ch <- StreamChunk{Type: "text", Content: choice.Delta.Content}
		}

		if choice.FinishReason == nil {
			continue
		}
		reason := *choice.FinishReason
		switch reason {
		case "tool_calls", "function_call":
			calls := c.assemblePendingCalls(pendingCalls)
			ch <- StreamChunk{Type: "tool_call", ToolCalls: calls, Usage: lastUsage}
			return
		case "stop":
			// 如果有累积的 tool_call 但 finish_reason 是 stop（某些模型的行为），也要处理
			if len(pendingCalls) > 0 {
				calls := c.assemblePendingCalls(pendingCalls)
				ch <- StreamChunk{Type: "tool_call", ToolCalls: calls, Usage: lastUsage}
				return
			}
			ch <- StreamChunk{Type: "done", Usage: lastUsage}
			return
		default:
			// 未知 finish_reason 但有 pending tool_calls
			if len(pendingCalls) > 0 {
				calls := c.assemblePendingCalls(pendingCalls)
				ch <- StreamChunk{Type: "tool_call", ToolCalls: calls, Usage: lastUsage}
				return
			}
			ch <- StreamChunk{Type: "done", Usage: lastUsage}
			return
		}
	}

	if err := scanner.Err(); err != nil {
		ch <- StreamChunk{Type: "error", Content: fmt.Sprintf("[%s] 读取 SSE 流失败: %v", c.name, err)}
		return
	}
	ch <- StreamChunk{Type: "done", Usage: lastUsage}
}

// assemblePendingCalls 将累积的工具调用 map 转为有序切片
func (c *OpenAIClient) assemblePendingCalls(pending map[int]*pendingToolCall) []StreamToolCall {
	calls := make([]StreamToolCall, 0, len(pending))
	for i := 0; i < len(pending); i++ {
		p, ok := pending[i]
		if !ok {
			continue
		}
		var args map[string]any
		if err := json.Unmarshal([]byte(p.args.String()), &args); err != nil {
			args = make(map[string]any)
		}
		calls = append(calls, StreamToolCall{Id: p.id, Name: p.name, Args: args})
	}
	return calls
}
