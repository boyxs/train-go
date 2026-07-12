package domain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeSlug(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"英文小写", "Golang", "golang"},
		{"trim+空白折叠为单-", "  Hello   World  ", "hello-world"},
		{"CJK 原样保留", "职场", "职场"},
		{"中英混合空白转-", "Go 并发编程", "go-并发编程"},
		{"剥 path 不安全字符", "a/b?c#d", "abcd"},
		{"首尾-裁剪", "  spring boot  ", "spring-boot"},
		{"全空白→空", "   ", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, NormalizeSlug(c.in))
		})
	}
}

func TestIsValidTagName(t *testing.T) {
	assert.True(t, IsValidTagName("Golang"), "普通英文合法")
	assert.True(t, IsValidTagName("并发"), "CJK 合法")
	assert.True(t, IsValidTagName("  Go  "), "trim 后非空合法")
	assert.False(t, IsValidTagName(""), "空非法")
	assert.False(t, IsValidTagName("   "), "纯空白非法")
	assert.False(t, IsValidTagName(strings.Repeat("x", TagNameMaxLen+1)), "超长非法")
	assert.True(t, IsValidTagName(strings.Repeat("x", TagNameMaxLen)), "边界长度合法")
}
