package domain

import "testing"

func TestDimension_Valid(t *testing.T) {
	testCases := []struct {
		dim  Dimension
		want bool
	}{
		{DimensionHot, true},
		{DimensionNew, true},
		{DimensionBest, true},
		{DimensionCategory, true},
		{"", false},
		{"xxx", false},
		{"HOT", false},
	}
	for _, tc := range testCases {
		t.Run(string(tc.dim), func(t *testing.T) {
			if got := tc.dim.Valid(); got != tc.want {
				t.Errorf("Dimension(%q).Valid() = %v, want %v", tc.dim, got, tc.want)
			}
		})
	}
}
