package llm

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/boyxs/train-go/webook/pkg/logger"
)

// FailoverClient 严格轮询：从当前 idx 开始，全部试一遍
type FailoverClient struct {
	clients []Client
	idx     uint64
	l       logger.LoggerX
}

func NewFailoverClient(clients []Client, l logger.LoggerX) Client {
	if len(clients) == 0 {
		panic("LLM 提供方列表不能为空")
	}
	return &FailoverClient{clients: clients, l: l}
}

func (f *FailoverClient) Chat(ctx context.Context, messages []ChatMessage) (string, error) {
	length := uint64(len(f.clients))
	globalIdx := atomic.AddUint64(&f.idx, 1)

	var lastErr error
	for i := uint64(0); i < length; i++ {
		index := (globalIdx + i - 1) % length
		result, err := f.clients[index].Chat(ctx, messages)
		switch {
		case err == nil:
			return result, nil
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return "", err
		}
		lastErr = err
		f.l.Warn("LLM Chat 调用失败，尝试下一个",
			logger.Uint64("providerIndex", index),
			logger.Error(err))
	}
	return "", fmt.Errorf("轮询所有 LLM 提供方均失败: %w", lastErr)
}

func (f *FailoverClient) ChatStream(ctx context.Context, messages []ChatMessage, tools []Tool) (<-chan StreamChunk, error) {
	length := uint64(len(f.clients))
	globalIdx := atomic.AddUint64(&f.idx, 1)

	var lastErr error
	for i := uint64(0); i < length; i++ {
		index := (globalIdx + i - 1) % length
		ch, err := f.clients[index].ChatStream(ctx, messages, tools)
		switch {
		case err == nil:
			return ch, nil
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return nil, err
		}
		lastErr = err
		f.l.Warn("LLM 提供方调用失败，尝试下一个",
			logger.Uint64("providerIndex", index),
			logger.Error(err))
	}
	return nil, fmt.Errorf("轮询所有 LLM 提供方均失败: %w", lastErr)
}
