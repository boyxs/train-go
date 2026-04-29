package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/webook/internal/domain"
	"github.com/webook/internal/errs"
	"github.com/webook/pkg/llm"
)

const polishPrompt = `你是一个中文技术文章润色助手。用户会提供文章标题和正文，你需要：
1. 优化标题：更准确、更吸引人，保持原意
2. 生成摘要：120字以内，概括文章核心要点
3. 润色正文：修正语法错误，改善措辞和可读性，保持技术准确性，不改变原文含义

必须返回以下 JSON 格式，不要包裹 markdown code block：
{"title":"润色后的标题","abstract":"生成的摘要","content":"润色后的正文"}`

const polishMaxContentLen = 10000

type ArticlePolishService interface {
	Polish(ctx context.Context, title, content string) (domain.PolishResult, error)
}

type AIArticlePolishService struct {
	llm llm.Client
}

func NewAIArticlePolishService(llmClient llm.Client) ArticlePolishService {
	return &AIArticlePolishService{llm: llmClient}
}

func (s *AIArticlePolishService) Polish(ctx context.Context, title, content string) (domain.PolishResult, error) {
	if strings.TrimSpace(title) == "" {
		return domain.PolishResult{}, errs.ErrPolishEmptyTitle
	}
	if strings.TrimSpace(content) == "" {
		return domain.PolishResult{}, errs.ErrPolishEmptyContent
	}
	if len([]rune(content)) > polishMaxContentLen {
		return domain.PolishResult{}, errs.ErrPolishContentTooLong
	}

	userMsg := fmt.Sprintf("标题：%s\n\n正文：%s", title, content)
	messages := []llm.ChatMessage{
		{Role: "system", Content: polishPrompt},
		{Role: "user", Content: userMsg},
	}

	reply, err := s.llm.Chat(ctx, messages)
	if err != nil {
		return domain.PolishResult{}, fmt.Errorf("LLM 调用失败: %w", err)
	}

	return parsePolishResult(reply)
}

// parsePolishResult 解析 LLM 返回的 JSON，兼容 markdown code block 包裹
func parsePolishResult(raw string) (domain.PolishResult, error) {
	raw = strings.TrimSpace(raw)
	// 兜底：去掉 markdown ```json ... ``` 包裹
	if strings.HasPrefix(raw, "```") {
		if idx := strings.Index(raw[3:], "\n"); idx >= 0 {
			raw = raw[3+idx+1:]
		}
		if strings.HasSuffix(raw, "```") {
			raw = raw[:len(raw)-3]
		}
		raw = strings.TrimSpace(raw)
	}

	var result domain.PolishResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return domain.PolishResult{}, fmt.Errorf("解析润色结果失败: %w", err)
	}
	if result.Title == "" || result.Content == "" {
		return domain.PolishResult{}, errors.New("润色结果不完整")
	}
	return result, nil
}
