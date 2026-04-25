package service

import "math"

// 榜单公式参数。改动这些常量前先理解下面两个公式的数学含义，别盲改。
const (
	// 热度公式的三元权重：点击 < 点赞 < 收藏。
	// 用户行为的"意愿成本"递增 —— 点击几乎 0 成本，点赞要判断，收藏是最强意图。
	// 改权重就是在调"点击榜" vs "口碑榜"的天平。
	hotWeightClick   = 1.0
	hotWeightLike    = 3.0
	hotWeightCollect = 5.0

	// 重力系数 gravity：HN / Reddit 算法的经典参数。
	// 数学上：score ∝ 1/(h+2)^gravity；gravity 越大衰减越快。
	// 业界常见取值：HN=1.8（快速滚动） / Reddit=1.5（日榜节奏）。
	// 这里 1.5 → 半衰期约 ~7h，适合"日榜"这种 24h 窗口的业务。
	// 改大（2.0+）会变成"分钟级热搜"；改小（1.0-）会变成"周榜"。
	hotGravity = 1.5

	// Wilson 95% 置信区间的 z 值（标准正态分布 0.975 分位数）。
	// 要改置信度就改这个：90% → 1.645 / 99% → 2.576。
	// 越大越保守（小样本压得越狠），越小越激进。
	wilsonZ = 1.96
)

// HotScore 热度分 —— HN / Reddit 风格的"加权投票 / 时间衰减"公式。
//
// 公式：
//
//	            1·clicks + 3·likes + 5·collects
//	score = ─────────────────────────────────────
//	              (hoursSincePublish + 2)^1.5
//
// 分子：加权互动数，反映"有多少人真的买账"。
// 分母：时间衰减，让老文章自然从榜单掉下来。
//   - +2 是平滑项：防止刚发布（hours=0）的文章被 1^1.5=1 放大到极端值。
//     没有它，新文章只要 1 个点击就能冲上榜首；有它，新文章需要攒够票数才能超过老文章。
//   - ^1.5 是 gravity 指数（见 hotGravity 注释）。
//
// 直觉示例（clicks=1000 / likes=100 / collects=30，score ≈ 1450）:
//
//	hours=0:    1450 / (0+2)^1.5 = 1450 / 2.83  ≈ 512   （刚发布，平滑项压制）
//	hours=7:    1450 / (7+2)^1.5 = 1450 / 27    ≈ 53    （发布 7h 后，半衰期处）
//	hours=24:   1450 / (24+2)^1.5 = 1450 / 132  ≈ 11    （发布一天后，大幅衰减）
//	hours=168:  1450 / (168+2)^1.5 ≈ 1450 / 2215 ≈ 0.65 （一周前的文章几乎消失）
//
// 防御：负数小时（时钟漂移 / 数据脏）直接返 0，避免 Pow(负数, 1.5) 产生 NaN 干扰排序。
func HotScore(clicks, likes, collects int64, hoursSincePublish float64) float64 {
	if hoursSincePublish < 0 {
		return 0
	}
	numerator := hotWeightClick*float64(clicks) +
		hotWeightLike*float64(likes) +
		hotWeightCollect*float64(collects)
	denominator := math.Pow(hoursSincePublish+2, hotGravity)
	return numerator / denominator
}

// WilsonLowerBound 最佳榜（质量榜）的排序分，基于 Wilson score interval 的下界。
//
// 问题背景：如果直接用比率 p = positives/total 排序，小样本会失真 ——
//
//	A: 1 个点击 1 个赞 → 100%
//	B: 10000 个点击 9000 个赞 → 90%
//
// 按比率 A 排 B 前面，但显然 B 的 "真实好评率" 更可信。Wilson 下界就是解决这个：
// 基于二项分布的统计推断，小样本的下界被自然压低。
//
// 公式（Edwin Wilson, 1927）：
//
//	       p + z²/2n − z·√(p·(1−p)/n + z²/4n²)
//	L = ────────────────────────────────────────
//	                 1 + z²/n
//
// 其中 p=positives/total（观察到的比率），n=total（样本数），z=1.96（95% 置信）。
// 当 n → ∞，L → p（样本大到置信下界等于观察值）。
// 当 n 很小，分母 (1 + z²/n) 远大于 1，L 被强烈下拉。
//
// 直觉示例（positives=正向信号, total=总点击）:
//
//	1/1    (100%)       → Wilson ≈ 0.21   （小样本重压，1 条样本不信任）
//	9/10   (90%)        → Wilson ≈ 0.60
//	90/100 (90%)        → Wilson ≈ 0.83   （样本够了，接近观察值）
//	900/1000 (90%)      → Wilson ≈ 0.88
//	9000/10000 (90%)    → Wilson ≈ 0.896
//
// 防御：
//   - total<=0 / positives<0 / positives>total 都是不可能的输入（数据脏），直接返 0 不参与排名。
//   - n=0 时 p·(1−p)/n 会除零，必须在前面守护。
func WilsonLowerBound(positives, total int64) float64 {
	if total <= 0 || positives < 0 || positives > total {
		return 0
	}
	n := float64(total)
	p := float64(positives) / n
	z2 := wilsonZ * wilsonZ
	num := p + z2/(2*n) - wilsonZ*math.Sqrt(p*(1-p)/n+z2/(4*n*n))
	den := 1 + z2/n
	return num / den
}
