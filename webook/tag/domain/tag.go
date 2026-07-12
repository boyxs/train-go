package domain

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// Tag 通用标签领域模型（与被标注对象解耦）。
type Tag struct {
	Id          int64
	Name        string
	Slug        string
	Type        string
	Description string
	RefCount    int64 // 内容引用数（多少对象打了此标签）
	FollowCount int64 // 关注数（多少用户关注此标签）
	// WeeklyNewCount 近 7 天新增关联数（仅 Detail 填充；其余读路径为 0）。
	WeeklyNewCount int64
}

const (
	// TagTypeTopic 内容主题标签命名空间（默认）。intent/… 后续扩展。
	TagTypeTopic = "topic"
	// 标注来源（通用，不绑单一来源）。
	TagSourceAuthor = "author"
	TagSourceAI     = "ai"
	// MaxTagsPerBiz 单个对象标签数上限。
	MaxTagsPerBiz = 5
	// TagNameMaxLen 标签名长度上限（rune 计）。
	TagNameMaxLen = 30
)

var (
	slugUnsafeRe     = regexp.MustCompile(`[/?#%&]+`) // path 不安全字符，剥掉
	slugWhitespaceRe = regexp.MustCompile(`\s+`)      // 连续空白 → 单个 -
	slugDashRe       = regexp.MustCompile(`-+`)       // 连续 - 折叠
)

// NormalizeSlug 生成 URL 友好 slug：小写、trim、空白折叠为单个 '-'、剥 path 不安全字符（/?#%& 空白）；
// CJK 原样保留（路由 UTF-8 百分号编码），免拼音库。
func NormalizeSlug(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = slugUnsafeRe.ReplaceAllString(s, "")
	s = slugWhitespaceRe.ReplaceAllString(s, "-")
	s = slugDashRe.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// IsValidTagName 校验标签名：trim 后非空且 rune 长度 ≤ TagNameMaxLen。
func IsValidTagName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	return utf8.RuneCountInString(name) <= TagNameMaxLen
}
