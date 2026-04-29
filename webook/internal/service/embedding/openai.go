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
)

// Config OpenAI 协议的 embedding 客户端配置
type Config struct {
	BaseUrl string
	ApiKey  string
	Model   string
	Dims    int
	Timeout int // 秒
}

// OpenAIClient 兼容 OpenAI Embedding 协议（阿里百炼、SiliconFlow 等）
type OpenAIClient struct {
	name   string
	cfg    Config
	url    string
	client *http.Client
}

func NewOpenAIClient(cfg Config) *OpenAIClient {
	timeout := time.Duration(cfg.Timeout) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &OpenAIClient{
		name: cfg.Model,
		cfg:  cfg,
		url:  strings.TrimRight(cfg.BaseUrl, "/") + "/embeddings",
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

type embedRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type embedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

func (c *OpenAIClient) Embed(ctx context.Context, text string) ([]float32, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("[%s] text 不能为空", c.name)
	}

	body, err := json.Marshal(embedRequest{Model: c.cfg.Model, Input: text})
	if err != nil {
		return nil, fmt.Errorf("[%s] marshal request: %w", c.name, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("[%s] create request: %w", c.name, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.ApiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("[%s] do request: %w", c.name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if readErr != nil {
			return nil, fmt.Errorf("[%s] API error: status=%d, read body: %w", c.name, resp.StatusCode, readErr)
		}
		return nil, fmt.Errorf("[%s] API error: status=%d, body=%s", c.name, resp.StatusCode, string(respBody))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("[%s] read response: %w", c.name, err)
	}

	var result embedResponse
	if err = json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("[%s] unmarshal response: %w", c.name, err)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("[%s] empty embeddings in response", c.name)
	}

	return result.Data[0].Embedding, nil
}
