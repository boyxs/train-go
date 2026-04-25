package consts

import "testing"

func TestNormalizeCategory(t *testing.T) {
	testCases := []struct {
		name string
		in   string
		want string
	}{
		{name: "已知值tech原样返回", in: "tech", want: "tech"},
		{name: "已知值ai原样返回", in: "ai", want: "ai"},
		{name: "空串归到other", in: "", want: "other"},
		{name: "未知值归到other", in: "xxx", want: "other"},
		{name: "大小写不匹配归到other", in: "Tech", want: "other"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeCategory(tc.in); got != tc.want {
				t.Errorf("NormalizeCategory(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
