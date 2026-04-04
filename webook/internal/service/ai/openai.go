package ai

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

	"gitee.com/train-cloud/geektime-basic-go/config"
)

// OpenAIClient 兼容 OpenAI 协议的通用客户端（DeepSeek、Kimi 等）
type OpenAIClient struct {
	name   string
	cfg    config.LLMProviderConfig
	url    string // 预拼接的完整 API URL
	client *http.Client
}

func NewOpenAIClient(cfg config.LLMProviderConfig) *OpenAIClient {
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
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Stream      bool          `json:"stream"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
}

// openaiStreamResponse SSE 响应中的 JSON 结构
type openaiStreamResponse struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

func (c *OpenAIClient) ChatStream(ctx context.Context, messages []ChatMessage, tools []Tool) (<-chan StreamChunk, error) {
	reqBody := openaiRequest{
		Model:    c.cfg.Model,
		Messages: messages,
		Stream:   true,
	}
	if c.cfg.MaxTokens > 0 {
		reqBody.MaxTokens = c.cfg.MaxTokens
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

// readSSE 解析 SSE 流，提取 delta.content 并发送到 channel
func (c *OpenAIClient) readSSE(ctx context.Context, body io.Reader, ch chan<- StreamChunk) {
	scanner := bufio.NewScanner(body)
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
			ch <- StreamChunk{Type: "done"}
			return
		}

		var resp openaiStreamResponse
		if err := json.Unmarshal([]byte(data), &resp); err != nil {
			continue
		}

		if len(resp.Choices) == 0 {
			continue
		}

		choice := resp.Choices[0]
		if choice.Delta.Content != "" {
			ch <- StreamChunk{Type: "text", Content: choice.Delta.Content}
		}
		if choice.FinishReason != nil && *choice.FinishReason == "stop" {
			ch <- StreamChunk{Type: "done"}
			return
		}
	}

	if err := scanner.Err(); err != nil {
		ch <- StreamChunk{Type: "error", Content: fmt.Sprintf("[%s] 读取 SSE 流失败: %v", c.name, err)}
		return
	}
	// 流正常 EOF 但未收到 [DONE] 或 stop，兜底发送 done
	ch <- StreamChunk{Type: "done"}
}
