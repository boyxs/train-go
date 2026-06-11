package transform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/webook/migrator/pipeline/sink"
)

func TestIdentityTransformer(t *testing.T) {
	in := sink.Mutation{Op: sink.OpInsert, Table: "article", PK: "1", Cols: map[string]any{"title": "a"}}
	out, err := IdentityTransformer{}.Transform(in)
	require.NoError(t, err)
	assert.Equal(t, in, out)
}

func TestRegistry_Get(t *testing.T) {
	reg := NewRegistry()

	t.Run("空名 → Identity（未指定 transform 的表原样透传）", func(t *testing.T) {
		tf, err := reg.Get("")
		require.NoError(t, err)
		_, ok := tf.(IdentityTransformer)
		assert.True(t, ok)
	})

	t.Run("已注册名 → 对应 transformer", func(t *testing.T) {
		reg.Register("marker", markerTransformer{})
		tf, err := reg.Get("marker")
		require.NoError(t, err)
		_, ok := tf.(markerTransformer)
		assert.True(t, ok)
	})

	t.Run("未注册名 → error（暴露配置错误，不静默退化）", func(t *testing.T) {
		_, err := reg.Get("nope")
		assert.ErrorContains(t, err, "nope")
	})
}

// markerTransformer 仅用于校验注册表能挂自定义 transformer，行为同 Identity。
type markerTransformer struct{}

func (markerTransformer) Transform(in sink.Mutation) (sink.Mutation, error) { return in, nil }
