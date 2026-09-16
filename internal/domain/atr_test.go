package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Each row contains high, low, and close. Open equals close.
func calculationSeries(t *testing.T, rows [][3]string) CandleSeries {
	t.Helper()
	selection := selection()
	selection.CandleCount = uint32(len(rows))
	start := timestamp("2026-09-01T00:00:00Z")
	selection.To = start.Add(time.Duration(len(rows)) * time.Minute)
	candles := make([]Candle, len(rows))
	for i, row := range rows {
		opening := start.Add(time.Duration(i) * time.Minute)
		candles[i] = Candle{OpenTime: opening, CloseTime: opening.Add(time.Minute), Open: dec(row[2]), High: dec(row[0]), Low: dec(row[1]), Close: dec(row[2]), FetchedAt: selection.To}
	}
	actual, err := selection.Range()
	require.NoError(t, err)
	series, err := NewCandleSeries(t.Context(), selection, selection.Instrument, selection.Interval, actual, candles)
	require.NoError(t, err)
	return series
}

func closeSeries(t *testing.T, prices ...string) CandleSeries {
	t.Helper()
	rows := make([][3]string, len(prices))
	for i, price := range prices {
		rows[i] = [3]string{price, price, price}
	}
	return calculationSeries(t, rows)
}

func TestATRWorkedExample(t *testing.T) {
	series := calculationSeries(t, [][3]string{{"100", "100", "100"}, {"101", "99", "100"}, {"102", "98", "100"}, {"102", "99", "100"}, {"103", "98", "100"}})

	sequence, err := calculateATRSequence(t.Context(), series, ATRSettings{3})
	require.NoError(t, err)
	require.Len(t, sequence.values, 2)
	assert.Equal(t, "3", sequence.at(3).String())
	assert.Equal(t, "3.666666666666667", sequence.at(4).String())
	atr, err := CalculateATR(t.Context(), series, ATRSettings{3})
	require.NoError(t, err)
	assert.Equal(t, CandleIndex(4), atr.CandleIndex)
	assert.Equal(t, series.candles[4].CloseTime, atr.ValueTime)
	formatted, err := FormatNumber(atr.Value)
	require.NoError(t, err)
	assert.Equal(t, "3.6666667", formatted)
	natr, err := CalculateNATR(t.Context(), series, ATRSettings{3})
	require.NoError(t, err)
	assert.Equal(t, "3.666666666666667", natr.Value.String())
	assert.Equal(t, "3.666667", FormatPercentage(natr.Value))
	assert.True(t, atr.Value.Equal(natr.ATR))
	assert.Equal(t, "100", natr.ReferenceClose.String())
	assert.Equal(t, atr.ValueTime, natr.ValueTime)
	assert.Equal(t, atr.CandleIndex, natr.CandleIndex)
}

func TestATRBoundaries(t *testing.T) {
	cases := []struct {
		name   string
		rows   [][3]string
		period uint32
		want   string
	}{
		{"up gap", [][3]string{{"100", "100", "100"}, {"112", "109", "110"}}, 1, "12"},
		{"down gap", [][3]string{{"100", "100", "100"}, {"91", "88", "90"}}, 1, "12"},
		{"zero", [][3]string{{"100", "100", "100"}, {"100", "100", "100"}}, 1, "0"},
		{"minimum seed", [][3]string{{"100", "100", "100"}, {"101", "99", "100"}, {"104", "100", "100"}}, 2, "3"},
		{"full history", [][3]string{{"100", "100", "100"}, {"102", "100", "100"}, {"104", "100", "100"}, {"106", "100", "100"}}, 2, "4.5"},
		{"later history", [][3]string{{"102", "100", "100"}, {"104", "100", "100"}, {"106", "100", "100"}}, 2, "5"},
		{"period one latest", [][3]string{{"100", "100", "100"}, {"102", "100", "100"}, {"104", "100", "100"}}, 1, "4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			series := calculationSeries(t, tc.rows)

			result, err := CalculateATR(t.Context(), series, ATRSettings{tc.period})

			require.NoError(t, err)
			assert.Equal(t, tc.want, result.Value.String())
		})
	}
}

func TestATRLongHistoryRounding(t *testing.T) {
	rows := make([][3]string, 1000)
	for i := range rows {
		rows[i] = [3]string{"103", "100", "100"}
	}
	rows[1] = [3]string{"101", "100", "100"}
	rows[2] = [3]string{"102", "100", "100"}
	series := calculationSeries(t, rows)

	result, err := CalculateATR(t.Context(), series, ATRSettings{3})

	require.NoError(t, err)
	assert.Equal(t, "2.999999999999999", result.Value.String())
}

func TestNATRZeroMovement(t *testing.T) {
	series := closeSeries(t, "100", "100")

	result, err := CalculateNATR(t.Context(), series, ATRSettings{1})

	require.NoError(t, err)
	assert.Equal(t, "0", result.Value.String())
	assert.Equal(t, "0", result.ATR.String())
}
