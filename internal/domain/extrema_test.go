package domain

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalExtrema(t *testing.T) {
	cases := []struct {
		name    string
		source  PriceSource
		span    uint32
		rows    [][3]string
		kinds   []ExtremumKind
		indices []CandleIndex
		prices  []string
	}{
		{"close peak", Close, 2, [][3]string{{"100", "100", "100"}, {"103", "103", "103"}, {"108", "108", "108"}, {"104", "104", "104"}, {"102", "102", "102"}}, []ExtremumKind{High}, []CandleIndex{2}, []string{"108"}},
		{"close trough", Close, 2, [][3]string{{"100", "100", "100"}, {"97", "97", "97"}, {"92", "92", "92"}, {"96", "96", "96"}, {"99", "99", "99"}}, []ExtremumKind{Low}, []CandleIndex{2}, []string{"92"}},
		{"plateau", Close, 1, [][3]string{{"100", "100", "100"}, {"108", "108", "108"}, {"108", "108", "108"}, {"100", "100", "100"}}, nil, nil, nil},
		{"both wicks", HighLow, 1, [][3]string{{"102", "98", "100"}, {"110", "90", "100"}, {"102", "98", "100"}}, []ExtremumKind{High, Low}, []CandleIndex{1, 1}, []string{"110", "90"}},
		{"close ignores wicks", Close, 1, [][3]string{{"102", "98", "100"}, {"110", "90", "100"}, {"102", "98", "100"}}, nil, nil, nil},
		{"equal high only", HighLow, 1, [][3]string{{"110", "98", "100"}, {"110", "90", "100"}, {"102", "98", "100"}}, []ExtremumKind{Low}, []CandleIndex{1}, []string{"90"}},
		{"monotone edges", HighLow, 1, [][3]string{{"100", "100", "100"}, {"101", "101", "101"}, {"102", "102", "102"}}, nil, nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			series := calculationSeries(t, tc.rows)

			result, err := DetectExtrema(t.Context(), series, ExtremaSettings{tc.source, LocalExtremaSettings{tc.span}})

			require.NoError(t, err)
			require.Len(t, result.Points, len(tc.kinds))
			for i, p := range result.Points {
				assert.Equal(t, tc.kinds[i], p.Kind)
				assert.Equal(t, tc.indices[i], p.CandleIndex)
				assert.Equal(t, tc.prices[i], p.Price.String())
				assert.Equal(t, p.CandleIndex+CandleIndex(tc.span), p.ConfirmationCandleIndex)
				assert.Equal(t, series.candles[p.CandleIndex].OpenTime, p.Time)
				assert.Equal(t, series.candles[p.ConfirmationCandleIndex].CloseTime, p.ConfirmationTime)
				assert.Nil(t, p.Reversal)
			}
		})
	}
}

func TestPercentageWorkedExample(t *testing.T) {
	series := closeSeries(t, "100", "104", "110", "108", "106", "104.5")

	result, err := DetectExtrema(t.Context(), series, ExtremaSettings{Close, PercentReversalSettings{dec("5")}})

	require.NoError(t, err)
	require.Len(t, result.Points, 2)
	assert.Equal(t, Low, result.Points[0].Kind)
	assert.Equal(t, CandleIndex(0), result.Points[0].CandleIndex)
	assert.Equal(t, CandleIndex(2), result.Points[0].ConfirmationCandleIndex)
	high := result.Points[1]
	assert.Equal(t, High, high.Kind)
	assert.Equal(t, CandleIndex(2), high.CandleIndex)
	assert.Equal(t, CandleIndex(5), high.ConfirmationCandleIndex)
	assert.Equal(t, "110", high.Price.String())
	require.NotNil(t, high.Reversal)
	assert.Equal(t, "5.5", high.Reversal.Threshold.String())
	assert.Equal(t, "104.5", high.Reversal.ConfirmationPrice.String())
	assert.Nil(t, high.Reversal.CandidateATR)
}

func TestReversalLifecycle(t *testing.T) {
	methods := []struct {
		name   string
		method ExtremaMethod
		warmup bool
	}{
		{"percent", PercentReversalSettings{dec("5")}, false},
		{"atr", ATRReversalSettings{1, dec("0.25")}, true},
	}
	cases := []struct {
		name                   string
		prices                 []string
		indices, confirmations []CandleIndex
		kinds                  []ExtremumKind
	}{
		{"equal high moves candidate", []string{"110", "108", "110", "104"}, []CandleIndex{2}, []CandleIndex{3}, []ExtremumKind{High}},
		{"equal low moves candidate", []string{"100", "102", "100", "106"}, []CandleIndex{2}, []CandleIndex{3}, []ExtremumKind{Low}},
		{"new high moves candidate", []string{"110", "108", "112", "106"}, []CandleIndex{2}, []CandleIndex{3}, []ExtremumKind{High}},
		{"alternating", []string{"100", "110", "100", "110"}, []CandleIndex{0, 1, 2}, []CandleIndex{1, 2, 3}, []ExtremumKind{Low, High, Low}},
		{"unfinished", []string{"100", "101", "102"}, nil, nil, nil},
	}
	for _, method := range methods {
		for _, tc := range cases {
			t.Run(method.name+"/"+tc.name, func(t *testing.T) {
				rows := make([][3]string, 0)
				offset := CandleIndex(0)
				if method.warmup {
					rows = append(rows, [3]string{tc.prices[0], tc.prices[0], tc.prices[0]})
					offset = 1
				}
				for _, price := range tc.prices {
					// Constant candle range makes ATR thresholds explicit for close prices.
					p := dec(price)
					rows = append(rows, [3]string{p.Add(dec("10")).String(), p.Sub(dec("10")).String(), price})
				}
				series := calculationSeries(t, rows)
				settings := ExtremaSettings{Close, method.method}

				result, err := DetectExtrema(t.Context(), series, settings)

				require.NoError(t, err)
				require.Len(t, result.Points, len(tc.indices))
				for i, p := range result.Points {
					assert.Equal(t, tc.indices[i]+offset, p.CandleIndex)
					assert.Equal(t, tc.confirmations[i]+offset, p.ConfirmationCandleIndex)
					assert.Equal(t, tc.kinds[i], p.Kind)
					assert.Equal(t, series.candles[p.CandleIndex].OpenTime, p.Time)
					assert.Equal(t, series.candles[p.ConfirmationCandleIndex].CloseTime, p.ConfirmationTime)
				}
			})
		}
	}
}

func TestReversalWickPriorityAndAmbiguity(t *testing.T) {
	methods := []struct {
		name   string
		method ExtremaMethod
		warmup bool
	}{
		{"percent", PercentReversalSettings{dec("5")}, false},
		{"atr", ATRReversalSettings{1, dec("0.25")}, true},
	}
	cases := []struct {
		name                string
		rows                [][3]string
		index, confirmation CandleIndex
		kind                ExtremumKind
	}{
		{"ambiguous waits", [][3]string{{"110", "90", "100"}, {"109", "91", "100"}, {"109", "91", "100"}, {"109", "106", "107"}}, 0, 3, Low},
		{"equal update first", [][3]string{{"110", "100", "105"}, {"110", "99", "105"}, {"101", "100", "100"}, {"104", "100", "102"}}, 1, 2, High},
		{"new update first", [][3]string{{"110", "100", "105"}, {"112", "99", "105"}, {"101", "100", "100"}, {"104", "100", "102"}}, 1, 2, High},
		{"equal low update first", [][3]string{{"110", "100", "105"}, {"111", "100", "105"}, {"110", "109", "110"}}, 1, 2, Low},
		{"new low update first", [][3]string{{"110", "100", "105"}, {"111", "99", "105"}, {"110", "109", "110"}}, 1, 2, Low},
	}
	for _, method := range methods {
		for _, tc := range cases {
			t.Run(method.name+"/"+tc.name, func(t *testing.T) {
				rows := append([][3]string(nil), tc.rows...)
				offset := CandleIndex(0)
				if method.warmup {
					rows = append([][3]string{tc.rows[0]}, rows...)
					offset = 1
				}
				series := calculationSeries(t, rows)

				result, err := DetectExtrema(t.Context(), series, ExtremaSettings{HighLow, method.method})

				require.NoError(t, err)
				require.NotEmpty(t, result.Points)
				p := result.Points[0]
				assert.Equal(t, tc.index+offset, p.CandleIndex)
				assert.Equal(t, tc.confirmation+offset, p.ConfirmationCandleIndex)
				assert.Equal(t, tc.kind, p.Kind)
				for _, p := range result.Points {
					assert.Greater(t, p.ConfirmationCandleIndex, p.CandleIndex)
				}
			})
		}
	}
}

func TestATRCandidateEvidence(t *testing.T) {
	cases := []struct {
		name                string
		rows                [][3]string
		source              PriceSource
		index, confirmation CandleIndex
		atr, threshold      string
	}{
		{"saved despite volatility", [][3]string{{"100", "100", "100"}, {"101", "99", "100"}, {"99", "80", "99"}, {"98", "97", "97"}}, Close, 1, 3, "2", "3"},
		{"equal replaces atr", [][3]string{{"100", "100", "100"}, {"101", "99", "100"}, {"99", "98", "99"}, {"100", "99", "100"}, {"99", "98", "98"}}, Close, 3, 4, "1", "1.5"},
		{"zero candidate cannot confirm", [][3]string{{"100", "100", "100"}, {"100", "100", "100"}, {"101", "99", "100"}, {"97", "97", "97"}}, Close, 2, 3, "2", "3"},
		{"wick saved atr", [][3]string{{"100", "100", "100"}, {"101", "99", "100"}, {"100", "99.5", "100"}, {"100", "97", "99"}}, HighLow, 1, 3, "2", "3"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			series := calculationSeries(t, tc.rows)

			result, err := DetectExtrema(t.Context(), series, ExtremaSettings{tc.source, ATRReversalSettings{1, dec("1.5")}})

			require.NoError(t, err)
			require.NotEmpty(t, result.Points)
			p := result.Points[0]
			assert.Equal(t, High, p.Kind)
			assert.Equal(t, tc.index, p.CandleIndex)
			assert.Equal(t, tc.confirmation, p.ConfirmationCandleIndex)
			require.NotNil(t, p.Reversal)
			require.NotNil(t, p.Reversal.CandidateATR)
			assert.Equal(t, tc.atr, p.Reversal.CandidateATR.String())
			assert.Equal(t, tc.threshold, p.Reversal.Threshold.String())
		})
	}
}

func TestATRWarmup(t *testing.T) {
	for _, source := range []PriceSource{Close, HighLow} {
		t.Run(string(source), func(t *testing.T) {
			rows := make([][3]string, 12)
			for i := range rows {
				rows[i] = [3]string{"101", "99", "100"}
			}
			rows[11] = [3]string{"105", "104", "105"}
			series := calculationSeries(t, rows)

			result, err := DetectExtrema(t.Context(), series, ExtremaSettings{source, ATRReversalSettings{10, dec("1")}})

			require.NoError(t, err)
			require.Len(t, result.Points, 1)
			p := result.Points[0]
			assert.Equal(t, Low, p.Kind)
			assert.Equal(t, CandleIndex(10), p.CandleIndex)
			assert.Equal(t, CandleIndex(11), p.ConfirmationCandleIndex)
			assert.Equal(t, "2", p.Reversal.CandidateATR.String())
		})
	}
}

func TestReversalAppendStability(t *testing.T) {
	methods := []ExtremaMethod{PercentReversalSettings{dec("5")}, ATRReversalSettings{1, dec("0.5")}}
	for _, method := range methods {
		for _, source := range []PriceSource{Close, HighLow} {
			t.Run(string(source)+"/"+extremaMethodName(method), func(t *testing.T) {
				prices := []string{"100", "105", "115", "100", "110", "120", "105", "100", "115"}
				settings := ExtremaSettings{source, method}
				prefix := closeSeries(t, prices[:6]...)
				full := closeSeries(t, prices...)

				before, err := DetectExtrema(t.Context(), prefix, settings)
				require.NoError(t, err)
				after, err := DetectExtrema(t.Context(), full, settings)

				require.NoError(t, err)
				require.NotEmpty(t, before.Points)
				require.GreaterOrEqual(t, len(after.Points), len(before.Points))
				assert.Equal(t, before.Points, after.Points[:len(before.Points)])
			})
		}
	}
}

func extremaMethodName(method ExtremaMethod) string {
	switch method.(type) {
	case PercentReversalSettings:
		return "percent"
	case ATRReversalSettings:
		return "atr"
	default:
		return "local"
	}
}

func TestReversalThresholdBeforeOutputRounding(t *testing.T) {
	series := closeSeries(t, "100", "95.00000001", "95")

	result, err := DetectExtrema(t.Context(), series, ExtremaSettings{Close, PercentReversalSettings{decimal.NewFromInt(5)}})

	require.NoError(t, err)
	require.Len(t, result.Points, 1)
	assert.Equal(t, CandleIndex(2), result.Points[0].ConfirmationCandleIndex)
}

func TestReversalHighLowAlternation(t *testing.T) {
	methods := []struct {
		name   string
		method ExtremaMethod
		warmup bool
	}{
		{"percent", PercentReversalSettings{dec("5")}, false},
		{"atr", ATRReversalSettings{1, dec("0.25")}, true},
	}
	for _, method := range methods {
		t.Run(method.name, func(t *testing.T) {
			rows := [][3]string{{"110", "90", "100"}, {"120", "100", "110"}, {"119", "116", "118"}, {"120", "90", "100"}, {"119", "100", "110"}, {"110", "101", "105"}}
			offset := CandleIndex(0)
			if method.warmup {
				rows = append([][3]string{rows[0]}, rows...)
				offset = 1
			}
			series := calculationSeries(t, rows)

			result, err := DetectExtrema(t.Context(), series, ExtremaSettings{HighLow, method.method})

			require.NoError(t, err)
			require.Len(t, result.Points, 3)
			expected := []struct {
				kind                     ExtremumKind
				index, confirmation      CandleIndex
				price, confirmationPrice string
			}{
				{Low, 0, 1, "90", "120"}, {High, 3, 4, "120", "100"}, {Low, 4, 5, "100", "110"},
			}
			for i, want := range expected {
				point := result.Points[i]
				assert.Equal(t, want.kind, point.Kind)
				assert.Equal(t, want.index+offset, point.CandleIndex)
				assert.Equal(t, want.confirmation+offset, point.ConfirmationCandleIndex)
				assert.Equal(t, want.price, point.Price.String())
				assert.Equal(t, want.confirmationPrice, point.Reversal.ConfirmationPrice.String())
			}
		})
	}
}

func TestATRZeroCandidateStaysUnavailable(t *testing.T) {
	cases := []struct {
		name   string
		prices []string
	}{
		{"zero movement", []string{"100", "100", "100", "100"}},
		{"low stays zero while high moves", []string{"100", "100", "101", "102"}},
		{"high stays zero while low moves", []string{"100", "100", "99", "98"}},
	}
	for _, tc := range cases {
		for _, source := range []PriceSource{Close, HighLow} {
			t.Run(tc.name+"/"+string(source), func(t *testing.T) {
				series := closeSeries(t, tc.prices...)

				result, err := DetectExtrema(t.Context(), series, ExtremaSettings{source, ATRReversalSettings{1, dec("1")}})

				require.NoError(t, err)
				assert.Empty(t, result.Points)
			})
		}
	}
}
