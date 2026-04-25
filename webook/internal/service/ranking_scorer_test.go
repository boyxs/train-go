package service

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHotScore(t *testing.T) {
	testCases := []struct {
		name              string
		clicks            int64
		likes             int64
		collects          int64
		hoursSincePublish float64
		want              float64
	}{
		{name: "零互动零小时", clicks: 0, likes: 0, collects: 0, hoursSincePublish: 0, want: 0},
		{name: "刚发布有互动", clicks: 100, likes: 10, collects: 2, hoursSincePublish: 0, want: 140.0 / math.Pow(2, 1.5)},
		{name: "发布5小时后", clicks: 100, likes: 10, collects: 2, hoursSincePublish: 5, want: 140.0 / math.Pow(7, 1.5)},
		{name: "权重验证collect大于like大于click", clicks: 1, likes: 1, collects: 1, hoursSincePublish: 0, want: 9.0 / math.Pow(2, 1.5)},
		{name: "负小时防御返0", clicks: 100, likes: 0, collects: 0, hoursSincePublish: -1, want: 0},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := HotScore(tc.clicks, tc.likes, tc.collects, tc.hoursSincePublish)
			assert.InDelta(t, tc.want, got, 0.01)
		})
	}

	t.Run("大数不溢出", func(t *testing.T) {
		got := HotScore(1_000_000, 100_000, 10_000, 12)
		assert.False(t, math.IsNaN(got), "不应为 NaN")
		assert.False(t, math.IsInf(got, 0), "不应为 Inf")
		assert.Greater(t, got, 0.0)
	})
}

func TestWilsonLowerBound(t *testing.T) {
	testCases := []struct {
		name      string
		positives int64
		total     int64
		want      float64
	}{
		{name: "total为0幂等返0", positives: 0, total: 0, want: 0},
		{name: "完美样本100分之100", positives: 100, total: 100, want: 0.9639},
		{name: "中性样本50分之100", positives: 50, total: 100, want: 0.4037},
		{name: "小样本1分之1被压制", positives: 1, total: 1, want: 0.2065},
		{name: "大样本1000分之1000趋近1", positives: 1000, total: 1000, want: 0.9962},
		{name: "positives大于total防御返0", positives: 10, total: 5, want: 0},
		{name: "负positives防御返0", positives: -1, total: 100, want: 0},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := WilsonLowerBound(tc.positives, tc.total)
			assert.InDelta(t, tc.want, got, 0.01)
		})
	}
}
