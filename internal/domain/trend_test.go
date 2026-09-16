package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrendClassification(t *testing.T) {
	cases := []struct {
		name             string
		highs, lows      []string
		close, tolerance string
		state            TrendState
		reason           TrendReason
	}{
		{"up", []string{"100", "110", "120"}, []string{"90", "95", "105"}, "115", "0", Up, RisingStructure},
		{"down", []string{"120", "110", "100"}, []string{"105", "95", "90"}, "95", "0", Down, FallingStructure},
		{"mixed full sequence", []string{"100", "110", "105"}, []string{"90", "95", "97"}, "102", "0", Undetermined, MixedStructure},
		{"up broken", []string{"100", "110"}, []string{"90", "95"}, "94", "0", Undetermined, StructureBroken},
		{"down broken", []string{"110", "100"}, []string{"95", "90"}, "101", "0", Undetermined, StructureBroken},
		{"horizontal lower", []string{"110", "110"}, []string{"90", "90"}, "90", "0", Sideways, HorizontalStructure},
		{"horizontal upper", []string{"110", "110"}, []string{"90", "90"}, "110", "0", Sideways, HorizontalStructure},
		{"horizontal outside", []string{"110", "110"}, []string{"90", "90"}, "111", "0", Undetermined, MixedStructure},
		{"insufficient high", []string{"110"}, []string{"90", "95"}, "100", "0", Undetermined, InsufficientStructure},
		{"insufficient low", []string{"110", "120"}, []string{"90"}, "100", "0", Undetermined, InsufficientStructure},
		{"exact tolerance equality", []string{"110", "111"}, []string{"90", "91"}, "100", "1", Sideways, HorizontalStructure},
		{"adjacent equal but spread larger", []string{"110", "111", "112"}, []string{"90", "91", "92"}, "100", "1", Undetermined, MixedStructure},
		{"up close at tolerance", []string{"110", "120"}, []string{"90", "101"}, "100", "1", Up, RisingStructure},
		{"down close at tolerance", []string{"110", "99"}, []string{"90", "80"}, "100", "1", Down, FallingStructure},
		{"horizontal lower tolerance", []string{"110", "110"}, []string{"101", "101"}, "100", "1", Sideways, HorizontalStructure},
		{"horizontal upper tolerance", []string{"99", "99"}, []string{"90", "90"}, "100", "1", Sideways, HorizontalStructure},
		{"strict below tolerance", []string{"110", "111.00000001"}, []string{"90", "91.00000001"}, "100", "1", Up, RisingStructure},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			series := closeSeries(t, "80", "130", tc.close)
			points := ExtremaResult{}
			// Nonalternating lists are valid. Classification compares each kind separately.
			for _, price := range tc.highs {
				points.Points = append(points.Points, Extremum{Kind: High, Price: dec(price)})
			}
			for _, price := range tc.lows {
				points.Points = append(points.Points, Extremum{Kind: Low, Price: dec(price)})
			}

			result, err := classifyTrend(t.Context(), series, points, Close, dec(tc.tolerance))

			require.NoError(t, err)
			assert.Equal(t, tc.state, result.State)
			assert.Equal(t, tc.reason, result.Reason)
			assert.Equal(t, tc.close, result.ReferenceClose.String())
			assert.Equal(t, points, result.Extrema)
		})
	}
}

func TestTrendFlatRangePriorityAndSource(t *testing.T) {
	cases := []struct {
		name      string
		source    PriceSource
		tolerance string
		reason    TrendReason
	}{
		{"flat closes", Close, "0", FlatRange},
		{"wicks not flat", HighLow, "0", InsufficientStructure},
		{"range exactly tolerance", HighLow, "2", FlatRange},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			series := calculationSeries(t, [][3]string{{"101", "99", "100"}, {"101", "99", "100"}, {"101", "99", "100"}})

			result, err := CalculateTrend(t.Context(), series, TrendSettings{ExtremaSettings{tc.source, LocalExtremaSettings{1}}, dec(tc.tolerance)})

			require.NoError(t, err)
			assert.Equal(t, tc.reason, result.Reason)
			if tc.reason == FlatRange {
				assert.Equal(t, Sideways, result.State)
			}
			assert.Empty(t, result.Extrema.Points)
		})
	}
}

func TestTrendCompositionAllMethods(t *testing.T) {
	methods := []ExtremaMethod{LocalExtremaSettings{1}, PercentReversalSettings{dec("2")}, ATRReversalSettings{1, dec("0.5")}}
	for _, method := range methods {
		for _, source := range []PriceSource{Close, HighLow} {
			t.Run(extremaMethodName(method)+"/"+string(source), func(t *testing.T) {
				series := closeSeries(t, "80", "85", "100", "90", "110", "95", "120", "105", "115")
				settings := ExtremaSettings{source, method}

				result, err := CalculateTrend(t.Context(), series, TrendSettings{settings, dec("0")})

				require.NoError(t, err)
				assert.Equal(t, Up, result.State)
				assert.Equal(t, RisingStructure, result.Reason)
				points, err := DetectExtrema(t.Context(), series, settings)
				require.NoError(t, err)
				assert.Equal(t, points, result.Extrema)
			})
		}
	}
}
