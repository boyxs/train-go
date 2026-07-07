// Package stringx 跨层通用字符串工具（无业务语义、无外部依赖，供任意层复用）。
package stringx

// Abbreviate 超过 maxRunes 个字符则按 rune 截断并追加 "..."，否则原样返回。
// 按 rune（而非 byte）计数，避免中文 / emoji 被截成半个字符。maxRunes<=0 返回空串。
func Abbreviate(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "..."
}
