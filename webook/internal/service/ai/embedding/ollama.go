package embedding

import (
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

// OllamaClient 调用本地 Ollama /api/embeddings 接口
type OllamaClient struct {
	cfg    config.OllamaEmbeddingConfig
	url    string
	client *http.Client
}

func NewOllamaClient(cfg config.OllamaEmbeddingConfig) *OllamaClient {
	if cfg.BaseUrl == "" {
		cfg.BaseUrl = "http://localhost:11434"
	}
	if cfg.Model == "" {
		cfg.Model = "bge-m3"
	}
	timeout := time.Duration(cfg.Timeout) * time.Second
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	return &OllamaClient{
		cfg: cfg,
		url: strings.TrimRight(cfg.BaseUrl, "/") + "/api/embeddings",
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        5,
				MaxIdleConnsPerHost: 5,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

type ollamaEmbedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type ollamaEmbedResponse struct {
	Embedding []float32 `json:"embedding"`
}

func (c *OllamaClient) Embed(ctx context.Context, text string) ([]float32, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("[ollama/%s] text 不能为空", c.cfg.Model)
	}

	body, err := json.Marshal(ollamaEmbedRequest{Model: c.cfg.Model, Prompt: text})
	if err != nil {
		return nil, fmt.Errorf("[ollama/%s] marshal request: %w", c.cfg.Model, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("[ollama/%s] create request: %w", c.cfg.Model, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("[ollama/%s] do request: %w", c.cfg.Model, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if readErr != nil {
			return nil, fmt.Errorf("[ollama/%s] API error: status=%d, read body: %w", c.cfg.Model, resp.StatusCode, readErr)
		}
		return nil, fmt.Errorf("[ollama/%s] API error: status=%d, body=%s", c.cfg.Model, resp.StatusCode, string(respBody))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("[ollama/%s] read response: %w", c.cfg.Model, err)
	}

	var result ollamaEmbedResponse
	if err = json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("[ollama/%s] unmarshal response: %w", c.cfg.Model, err)
	}

	if len(result.Embedding) == 0 {
		return nil, fmt.Errorf("[ollama/%s] empty embedding in response", c.cfg.Model)
	}

	return result.Embedding, nil
}
