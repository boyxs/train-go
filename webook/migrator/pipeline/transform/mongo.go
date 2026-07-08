package transform

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/boyxs/train-go/webook/migrator/pipeline/sink"
)

// TransformMongoToRelational 是 MongoToRelationalTransformer 在 Registry 里的注册名。
// task 的 TableMapping.Transform 填这个值即启用 Mongo 文档→关系行变形。
const TransformMongoToRelational = "mongo_to_relational"

// MongoToRelationalTransformer 把 Mongo 文档拍平成关系行：
//   - 顶层标量（string / 数值 / bool / nil）→ 同名列原样保留
//   - 嵌套子文档（map）/ 数组（slice）→ 整团 json.Marshal 成字符串，存同名 JSON 列
//
// 期望 MongoSource 已把驱动特有类型归一成普通 Go 值（如 ObjectID→hex string），
// 故这里只按 reflect.Kind 区分 Map/Slice（转 JSON）与标量（原样）。
// 不就地修改输入 Cols；Cols 为空（如 delete）的 Mutation 原样透传。
type MongoToRelationalTransformer struct{}

func (MongoToRelationalTransformer) Transform(in sink.Mutation) (sink.Mutation, error) {
	if len(in.Cols) == 0 {
		return in, nil
	}
	flat := make(map[string]any, len(in.Cols))
	for k, v := range in.Cols {
		if v == nil {
			flat[k] = nil
			continue
		}
		switch reflect.TypeOf(v).Kind() {
		case reflect.Map, reflect.Slice:
			b, err := json.Marshal(v)
			if err != nil {
				return sink.Mutation{}, fmt.Errorf("mongo_to_relational: marshal nested field %q: %w", k, err)
			}
			flat[k] = string(b)
		default:
			flat[k] = v
		}
	}
	out := in
	out.Cols = flat
	return out, nil
}
