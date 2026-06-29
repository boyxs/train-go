package sensitive

import "testing"

func TestDFAFilter_Match(t *testing.T) {
	f := NewDFAFilter([]string{"敏感词", "fuck", "ab"})
	cases := []struct {
		name string
		text string
		want bool
	}{
		{"中文命中", "这是敏感词测试", true},
		{"未命中", "正常内容", false},
		{"空文本", "", false},
		{"英文命中", "say fuck you", true},
		{"中间命中", "xaby", true},
		{"不完整不命中", "a", false},
		{"区分大小写", "FUCK", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := f.Match(c.text); got != c.want {
				t.Fatalf("Match(%q) = %v, want %v", c.text, got, c.want)
			}
		})
	}
}

func TestDFAFilter_EmptyDict(t *testing.T) {
	f := NewDFAFilter(nil)
	if f.Match("fuck 敏感词") {
		t.Fatalf("空词库应不命中任何文本")
	}
}

func TestDFAFilter_OverlapWords(t *testing.T) {
	f := NewDFAFilter([]string{"ab", "abc"})
	if !f.Match("abc") {
		t.Fatalf(`Match("abc") = false, want true（短词 ab 应命中）`)
	}
}
