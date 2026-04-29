package embedding

import "context"

// Client 文本向量化接口（包名 embedding 已表达"向量化客户端"含义，类型名简洁为 Client）
type Client interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}
