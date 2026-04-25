package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/webook/internal/domain"
)

func TestComputeTrend(t *testing.T) {
	testCases := []struct {
		name      string
		prev      map[int64]int
		articleId int64
		currRank  int
		wantTrend domain.Trend
		wantDelta int
	}{
		{name: "新上榜", prev: map[int64]int{}, articleId: 100, currRank: 5, wantTrend: domain.TrendNew, wantDelta: 0},
		{name: "上升2位", prev: map[int64]int{100: 5}, articleId: 100, currRank: 3, wantTrend: domain.TrendUp, wantDelta: 2},
		{name: "下降1位", prev: map[int64]int{100: 3}, articleId: 100, currRank: 4, wantTrend: domain.TrendDown, wantDelta: 1},
		{name: "持平", prev: map[int64]int{100: 3}, articleId: 100, currRank: 3, wantTrend: domain.TrendSame, wantDelta: 0},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			trend, delta := computeTrend(tc.prev, tc.articleId, tc.currRank)
			assert.Equal(t, tc.wantTrend, trend)
			assert.Equal(t, tc.wantDelta, delta)
		})
	}
}
