package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestZoneGrouping(t *testing.T) {
	cases := []struct {
		name         string
		prices       []string
		indices      []CandleIndex
		width        string
		spacing, min uint32
		bounds       [][2]string
		accepted     [][]CandleIndex
		references   [][]ExtremumIndex
	}{
		{"fixed anchor", []string{"101.5", "100", "100.75", "101.5"}, []CandleIndex{1, 3, 5, 7}, "1", 1, 2, [][2]string{{"100", "100.75"}, {"101.5", "101.5"}}, [][]CandleIndex{{3, 5}, {1, 7}}, [][]ExtremumIndex{{1, 2}, {0, 3}}},
		{"width inclusive", []string{"100", "101"}, []CandleIndex{1, 3}, "1", 1, 2, [][2]string{{"100", "101"}}, [][]CandleIndex{{1, 3}}, [][]ExtremumIndex{{0, 1}}},
		{"spacing bounds and duplicate candle", []string{"100", "101", "100.5", "100.4", "100"}, []CandleIndex{10, 12, 16, 23, 10}, "1", 5, 3, [][2]string{{"100", "101"}}, [][]CandleIndex{{10, 16, 23}}, [][]ExtremumIndex{{0, 4, 3, 2, 1}}},
		{"discarded group keeps references", []string{"90", "100", "100"}, []CandleIndex{1, 2, 3}, "0", 1, 2, [][2]string{{"100", "100"}}, [][]CandleIndex{{2, 3}}, [][]ExtremumIndex{{1, 2}}},
		{"zero width separates prices", []string{"100", "100.1"}, []CandleIndex{1, 2}, "0", 1, 2, nil, nil, nil},
		{"insufficient touches", []string{"100", "100", "100"}, []CandleIndex{1, 2, 3}, "1", 5, 2, nil, nil, nil},
		{"no points", nil, nil, "1", 1, 2, nil, nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prices := make([]string, 25)
			for i := range prices {
				prices[i] = "100"
			}
			series := closeSeries(t, prices...)
			points := make([]Extremum, len(tc.prices))
			for i, price := range tc.prices {
				kind := High
				if i%2 == 1 {
					kind = Low
				}
				points[i] = Extremum{Kind: kind, CandleIndex: tc.indices[i], Price: dec(price)}
			}
			before := append([]Extremum{}, points...)

			zones, err := buildZones(t.Context(), series, points, dec(tc.width), dec("105"), tc.min, tc.spacing)

			require.NoError(t, err)
			require.Len(t, zones, len(tc.bounds))
			assert.Equal(t, before, points)
			for i, z := range zones {
				assert.Equal(t, tc.bounds[i][0], z.LowerBound.String())
				assert.Equal(t, tc.bounds[i][1], z.UpperBound.String())
				assert.Equal(t, tc.accepted[i], z.AcceptedCandleIndices)
				assert.Equal(t, tc.references[i], z.ExtremumIndices)
				assert.Equal(t, uint32(len(tc.accepted[i])), z.TouchCount)
				assert.Equal(t, series.candles[tc.accepted[i][0]].OpenTime, z.FirstTouchTime)
				assert.Equal(t, series.candles[tc.accepted[i][len(tc.accepted[i])-1]].OpenTime, z.LastTouchTime)
				assert.Equal(t, Support, z.Role)
			}
		})
	}
}

func TestZoneRolesAndMidpoint(t *testing.T) {
	cases := []struct {
		close string
		role  ZoneRole
	}{{"99", Resistance}, {"100", AtPrice}, {"100.5", AtPrice}, {"101", AtPrice}, {"102", Support}}
	for _, tc := range cases {
		t.Run(tc.close, func(t *testing.T) {
			series := closeSeries(t, "100", "101", tc.close)
			points := []Extremum{{Kind: Low, Price: dec("100"), CandleIndex: 0}, {Kind: High, Price: dec("101"), CandleIndex: 1}}

			zones, err := buildZones(t.Context(), series, points, dec("1"), dec(tc.close), 2, 1)

			require.NoError(t, err)
			require.Len(t, zones, 1)
			assert.Equal(t, tc.role, zones[0].Role)
			assert.Equal(t, "100.5", zones[0].RepresentativePrice.String())
		})
	}
}

func TestZoneEqualPriceTieOrder(t *testing.T) {
	series := closeSeries(t, "100", "100")
	points := []Extremum{{Kind: Low, Price: dec("100"), CandleIndex: 1}, {Kind: High, Price: dec("100"), CandleIndex: 1}, {Kind: High, Price: dec("100"), CandleIndex: 0}}

	zones, err := buildZones(t.Context(), series, points, dec("0"), dec("100"), 2, 1)

	require.NoError(t, err)
	require.Len(t, zones, 1)
	assert.Equal(t, []ExtremumIndex{2, 1, 0}, zones[0].ExtremumIndices)
	assert.Equal(t, []CandleIndex{0, 1}, zones[0].AcceptedCandleIndices)
}

func TestLevelsComposition(t *testing.T) {
	cases := []struct {
		name    string
		period  uint32
		wantATR string
	}{
		{"matching period", 1, "10"},
		{"different period", 2, "11.875"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			series := closeSeries(t, "100", "110", "100", "120", "100", "110", "100")
			settings := LevelSettings{Extrema: ExtremaSettings{Close, ATRReversalSettings{1, dec("0.5")}}, ATRPeriod: tc.period, ZoneWidthATR: dec("0.1"), MinTouches: 2, MinTouchSeparationBars: 2}

			result, err := CalculateLevels(t.Context(), series, settings)

			require.NoError(t, err)
			assert.Equal(t, tc.wantATR, result.ATR.Value.String())
			require.Len(t, result.Extrema.Points, 5)
			require.Len(t, result.Zones, 2)
			assert.Equal(t, "100", result.Zones[0].LowerBound.String())
			assert.Equal(t, "110", result.Zones[1].UpperBound.String())
			assert.Equal(t, []ExtremumIndex{1, 3}, result.Zones[0].ExtremumIndices)
			assert.Equal(t, []ExtremumIndex{0, 4}, result.Zones[1].ExtremumIndices)
			assert.Equal(t, []CandleIndex{2, 4}, result.Zones[0].AcceptedCandleIndices)
			assert.Equal(t, AtPrice, result.Zones[0].Role)
			assert.Equal(t, Resistance, result.Zones[1].Role)
			independent, err := DetectExtrema(t.Context(), series, settings.Extrema)
			require.NoError(t, err)
			assert.Equal(t, independent, result.Extrema)
		})
	}
}

func TestLevelsRegroupWithLatestATR(t *testing.T) {
	cases := []struct {
		name, lastHigh, width string
		zones                 int
	}{
		{"narrow", "100", "1", 2}, {"wide", "220", "12", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows := [][3]string{{"100", "100", "100"}, {"110", "110", "110"}, {"100", "100", "100"}, {"110", "110", "110"}, {"100", "100", "100"}, {"110", "110", "110"}, {tc.lastHigh, "100", "100"}}
			series := calculationSeries(t, rows)
			settings := LevelSettings{ExtremaSettings{Close, LocalExtremaSettings{1}}, 1, dec("0.1"), 2, 1}

			result, err := CalculateLevels(t.Context(), series, settings)

			require.NoError(t, err)
			require.Len(t, result.Zones, tc.zones)
			require.Len(t, result.Extrema.Points, 5)
			assert.Equal(t, tc.width, result.MaximumZoneWidth.String())
			assert.Equal(t, "100", result.Zones[0].LowerBound.String())
			if tc.name == "wide" {
				assert.Equal(t, "110", result.Zones[0].UpperBound.String())
			}
		})
	}
}

func TestLevelsZeroVolatility(t *testing.T) {
	series := closeSeries(t, "100", "100", "100")
	settings := LevelSettings{ExtremaSettings{Close, LocalExtremaSettings{1}}, 1, dec("1"), 2, 1}

	result, err := CalculateLevels(t.Context(), series, settings)

	require.NoError(t, err)
	assert.Empty(t, result.Extrema.Points)
	assert.Empty(t, result.Zones)
	assert.True(t, result.MaximumZoneWidth.IsZero())
}

func TestZoneBoundaryBeforeOutputRounding(t *testing.T) {
	series := closeSeries(t, "100", "101.00000001", "101.00000001")
	points := []Extremum{{Kind: Low, Price: dec("100"), CandleIndex: 0}, {Kind: High, Price: dec("101.00000001"), CandleIndex: 1}, {Kind: Low, Price: dec("101.00000001"), CandleIndex: 2}}

	zones, err := buildZones(t.Context(), series, points, dec("1"), dec("101.00000001"), 2, 1)

	require.NoError(t, err)
	require.Len(t, zones, 1)
	assert.Equal(t, []ExtremumIndex{1, 2}, zones[0].ExtremumIndices)
	assert.Equal(t, "101.00000001", zones[0].LowerBound.String())
}
