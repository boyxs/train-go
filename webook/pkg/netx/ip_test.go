package netx

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExternalIp(t *testing.T) {
	t.Log(ExternalIp())
	assert.True(t, len(ExternalIp()) > 0)
}
