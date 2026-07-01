package ginx

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOk_DataOnly(t *testing.T) {
	assert.Equal(t, Result{Code: 200, Msg: "OK", Data: 1}, Success(1))
}

// 命名构造器：状态码由函数名决定，写进 Result.Code。
func TestNamedResults_CodeFromFuncName(t *testing.T) {
	cases := []struct {
		name string
		got  Result
		code int
	}{
		{"BadRequest", BadRequest("x"), 400},
		{"Unauthorized", Unauthorized("x"), 401},
		{"Forbidden", Forbidden("x"), 403},
		{"NotFound", NotFound("x"), 404},
		{"Conflict", Conflict("x"), 409},
		{"TooManyRequests", TooManyRequests("x"), 429},
		{"Internal", Internal("x"), 500},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, Result{Code: c.code, Msg: "x"}, c.got)
		})
	}
}
