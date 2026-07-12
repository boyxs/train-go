package embedding

import "context"

// EmbeddingClient 文本向量化接口。子包内接口用完整名（调用方读 embedding.EmbeddingClient 一眼分清用途）。
type EmbeddingClient interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}
