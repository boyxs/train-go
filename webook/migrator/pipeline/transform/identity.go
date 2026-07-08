package transform

import "github.com/boyxs/train-go/webook/migrator/pipeline/sink"

// IdentityTransformer 原样返回，用于同构迁移（源与目标 schema 一致）。
type IdentityTransformer struct{}

func (IdentityTransformer) Transform(in sink.Mutation) (sink.Mutation, error) {
	return in, nil
}
