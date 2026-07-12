package setup

import (
	"context"
	"strings"

	"github.com/boyxs/train-go/webook/search/service"
)

// vec1024 生成 1024 维 one-hot 向量（pos 处为 1）：同 pos 余弦=1、异 pos=0，kNN 断言确定。
func vec1024(pos int) []float32 {
	v := make([]float32, 1024)
	v[pos%1024] = 1
	return v
}

// stubEmbedder 文本相关的确定向量：含 "rust" → pos2 簇，其余 → pos1 簇。
// 隔离外部 embedding API，同时保留 kNN 可分性——Go 系文（含查询）落 pos1、Rust 系落 pos2，正交。
// 于是经 gRPC IndexArticle 入库的文档向量与真实种子（Go=pos1 / Rust=pos2）一致，e2e 仍能断言 kNN 判别。
type stubEmbedder struct{}

func (stubEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	if strings.Contains(strings.ToLower(text), "rust") {
		return vec1024(2), nil
	}
	return vec1024(1), nil
}

// InitStubEmbedder 供 wire 注入 service.Embedder（集成测试不调真实 embedding API）。
func InitStubEmbedder() service.Embedder {
	return stubEmbedder{}
}
