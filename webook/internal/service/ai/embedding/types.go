package embedding

import "context"

// EmbeddingClient 文本向量化接口
type EmbeddingClient interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}
