package weight

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/resolver"
)

func TestWeight(t *testing.T) {
	require.Equal(t, 40, Of(Set(resolver.Address{Addr: "x"}, 40))) // 往返
	require.Equal(t, 1, Of(resolver.Address{Addr: "x"}))           // 没打 → 1
	require.Equal(t, 1, Of(Set(resolver.Address{}, 0)))            // 0 → 1
	require.Equal(t, 1, Of(Set(resolver.Address{}, -3)))           // 负 → 1
}
