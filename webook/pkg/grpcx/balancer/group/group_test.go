package group

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/resolver"
)

func TestGroup(t *testing.T) {
	require.Equal(t, "vip", Of(Set(resolver.Address{Addr: "x"}, "vip"))) // 往返
	require.Equal(t, "", Of(resolver.Address{Addr: "x"}))                // 没打 → 空串
	require.Equal(t, "", Of(Set(resolver.Address{}, "")))                // 空串 → 空串
}
