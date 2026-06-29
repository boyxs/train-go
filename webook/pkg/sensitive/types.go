package sensitive

//go:generate mockgen -source=./types.go -package=sensitivemocks -destination=mocks/filter.mock.go Filter

// Filter 敏感词过滤器。
type Filter interface {
	// Match 返回 text 是否命中任意敏感词。
	Match(text string) bool
}
