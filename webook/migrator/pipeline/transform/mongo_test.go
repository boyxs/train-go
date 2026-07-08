package transform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/boyxs/train-go/webook/migrator/pipeline/sink"
)

func TestMongoToRelationalTransformer(t *testing.T) {
	tf := MongoToRelationalTransformer{}

	t.Run("顶层标量同名保留 / 嵌套文档 + 数组整团转 JSON 列", func(t *testing.T) {
		in := sink.Mutation{Op: sink.OpInsert, Table: "user", PK: "65abc", Cols: map[string]any{
			"_id":     "65abc",
			"name":    "bob",
			"views":   int64(5),
			"vip":     true,
			"profile": map[string]any{"city": "SG", "age": int64(30)}, // 嵌套子文档
			"tags":    []any{"go", "db"},                              // 数组
		}}
		out, err := tf.Transform(in)
		require.NoError(t, err)

		// 顶层标量原样
		assert.Equal(t, "65abc", out.Cols["_id"])
		assert.Equal(t, "bob", out.Cols["name"])
		assert.Equal(t, int64(5), out.Cols["views"])
		assert.Equal(t, true, out.Cols["vip"])
		// 嵌套 → JSON 字符串列（json.Marshal 对 map key 排序）
		assert.JSONEq(t, `{"city":"SG","age":30}`, out.Cols["profile"].(string))
		assert.JSONEq(t, `["go","db"]`, out.Cols["tags"].(string))
		// Op/Table/PK 透传
		assert.Equal(t, sink.OpInsert, out.Op)
		assert.Equal(t, "user", out.Table)
		assert.Equal(t, "65abc", out.PK)
	})

	t.Run("delete（Cols 空）→ 原样透传", func(t *testing.T) {
		in := sink.Mutation{Op: sink.OpDelete, Table: "user", PK: "65abc"}
		out, err := tf.Transform(in)
		require.NoError(t, err)
		assert.Equal(t, in, out)
	})

	t.Run("不改原 Cols（不就地改输入）", func(t *testing.T) {
		orig := map[string]any{"profile": map[string]any{"city": "SG"}}
		in := sink.Mutation{Op: sink.OpInsert, Table: "user", PK: "1", Cols: orig}
		_, err := tf.Transform(in)
		require.NoError(t, err)
		// 输入的嵌套字段仍是 map，没被替换成 string
		_, stillMap := orig["profile"].(map[string]any)
		assert.True(t, stillMap, "Transform 不应就地修改输入 Cols")
	})
}
