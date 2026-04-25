package domain

// Dimension 榜单维度。
type Dimension string

const (
	DimensionHot      Dimension = "hot"      // 热度：综合行为 + 时间衰减
	DimensionNew      Dimension = "new"      // 最新：按发布时间
	DimensionBest     Dimension = "best"     // 最佳：Wilson 置信下界
	DimensionCategory Dimension = "category" // 分区：按 category 切片的热度
	DimensionUnknown  Dimension = "unknown"  // 前端未传或无效时的兜底
)

func (d Dimension) Valid() bool {
	switch d {
	case DimensionHot, DimensionNew, DimensionBest, DimensionCategory:
		return true
	}
	return false
}

// Trend 排名趋势。
type Trend string

const (
	TrendNew  Trend = "new"  // 新上榜
	TrendUp   Trend = "up"   // 上升
	TrendDown Trend = "down" // 下降
	TrendSame Trend = "same" // 持平
)

// ArticleRanking 文章榜单单条领域模型。与 dao.ArticleRanking 对应。
type ArticleRanking struct {
	Rank       int     `json:"rank"`
	ArticleId  int64   `json:"articleId"`
	Title      string  `json:"title"`
	Author     Author  `json:"author"`
	Category   string  `json:"category"`
	Clicks     int64   `json:"clicks"`
	Likes      int64   `json:"likes"`
	Collects   int64   `json:"collects"`
	Score      float64 `json:"score"`
	ScoreRatio float64 `json:"scoreRatio"` // 相对榜首 0~1，用于进度条
	Trend      Trend   `json:"trend"`
	TrendDelta int     `json:"trendDelta"` // 变化名次，eg +2 或 -1
}
