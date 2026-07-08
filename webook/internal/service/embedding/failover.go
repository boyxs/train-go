package embedding

import (
	"context"
	"errors"
	"fmt"

	"github.com/boyxs/train-go/webook/pkg/logger"
)

// FailoverClient 顺序尝试多个 Client，第一个成功即返回
type FailoverClient struct {
	clients []Client
	l       logger.LoggerX
}

func NewFailoverClient(clients []Client, l logger.LoggerX) Client {
	if len(clients) == 0 {
		panic("Embedding 提供方列表不能为空")
	}
	return &FailoverClient{clients: clients, l: l}
}

func (f *FailoverClient) Embed(ctx context.Context, text string) ([]float32, error) {
	var lastErr error
	for i, c := range f.clients {
		vec, err := c.Embed(ctx, text)
		switch {
		case err == nil:
			return vec, nil
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return nil, err
		}
		lastErr = err
		f.l.Warn("Embedding 提供方调用失败，尝试下一个",
			logger.Int64("providerIndex", int64(i)),
			logger.Error(err))
	}
	return nil, fmt.Errorf("所有 Embedding 提供方均失败: %w", lastErr)
}
