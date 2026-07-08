// Package transform 是迁移管道的「行变形」抽象。
//
// Transformer 夹在 Source 读出与 Sink 写入之间，把源形态的 Mutation reshape 成目标形态。
// 同构（MySQL→MySQL）用 IdentityTransformer 原样透传；异构（如 Mongo 文档→关系行）由对应 Transformer 拍平。
//
// 选择机制：按 TableMapping.Transform 名从 Registry 取；空名 → Identity。
//
// 文件组织（对齐 pipeline/source、pipeline/sink）：
//   - transform.go  仅契约：Transformer 接口
//   - registry.go   dispatch：按名查 Transformer
//   - identity.go   IdentityTransformer（同构透传）
//   - mongo.go      MongoToRelationalTransformer（Mongo 文档拍平）
package transform

import "github.com/boyxs/train-go/webook/migrator/pipeline/sink"

// Transformer 把一条 Mutation reshape 成另一条（通常只改 Cols，Op/Table/PK 透传）。
type Transformer interface {
	Transform(in sink.Mutation) (sink.Mutation, error)
}
