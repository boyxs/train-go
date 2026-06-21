package etcd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWeightOf(t *testing.T) {
	tests := []struct {
		name string
		meta any
		want int
	}{
		{"nil", nil, 1},
		{"非 map", "oops", 1},
		{"无 weight 键", map[string]any{"meta": "x"}, 1},
		{"float64(etcd JSON 回读)", map[string]any{"weight": float64(40)}, 40},
		{"int(防御)", map[string]any{"weight": 50}, 50},
		{"零值按 1", map[string]any{"weight": float64(0)}, 1},
		{"负值按 1", map[string]any{"weight": float64(-5)}, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, weightOf(tc.meta))
		})
	}
}

func TestGroupOf(t *testing.T) {
	tests := []struct {
		name string
		meta any
		want string
	}{
		{"nil", nil, ""},
		{"非 map", "oops", ""},
		{"无 group 键", map[string]any{"weight": 1}, ""},
		{"正常", map[string]any{"group": "vip"}, "vip"},
		{"非 string 值(防御)", map[string]any{"group": 1}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, groupOf(tc.meta))
		})
	}
}
