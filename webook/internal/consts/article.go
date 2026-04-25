package consts

// 文章分类枚举。空串视为"其他"。
const (
	CategoryTech   = "tech"
	CategoryCareer = "career"
	CategoryLife   = "life"
	CategoryAI     = "ai"
	CategoryOther  = "other"
)

// AllCategories 全部分类白名单，分区榜枚举时用。
var AllCategories = []string{
	CategoryTech,
	CategoryCareer,
	CategoryLife,
	CategoryAI,
	CategoryOther,
}

// NormalizeCategory 把空串/未知值归一化到 "other"，保证分区榜稳定。
func NormalizeCategory(c string) string {
	for _, v := range AllCategories {
		if v == c {
			return c
		}
	}
	return CategoryOther
}
