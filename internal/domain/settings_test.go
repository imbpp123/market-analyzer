package domain

import (
	"math"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dec(value string) decimal.Decimal { return decimal.RequireFromString(value) }

func TestExtremaSettings(t *testing.T) {
	cases := []struct {
		name   string
		source PriceSource
		method ExtremaMethod
		count  uint32
		field  string
	}{
		{"local", Close, LocalExtremaSettings{2}, 5, ""},
		{"high low", HighLow, LocalExtremaSettings{1}, 3, ""},
		{"local too short", Close, LocalExtremaSettings{2}, 4, "candle_count"},
		{"local zero", Close, LocalExtremaSettings{}, 5, "pivot_span"},
		{"local overflow", Close, LocalExtremaSettings{math.MaxUint32}, math.MaxUint32, "candle_count"},
		{"percent", Close, PercentReversalSettings{dec("0.01")}, 2, ""},
		{"percent too short", Close, PercentReversalSettings{dec("2")}, 1, "candle_count"},
		{"percent zero", Close, PercentReversalSettings{}, 2, "reversal_pct"},
		{"percent negative", Close, PercentReversalSettings{dec("-1")}, 2, "reversal_pct"},
		{"percent hundred", Close, PercentReversalSettings{dec("100")}, 2, "reversal_pct"},
		{"percent above hundred", Close, PercentReversalSettings{dec("101")}, 2, "reversal_pct"},
		{"atr", HighLow, ATRReversalSettings{1, dec("1.5")}, 3, ""},
		{"atr too short", Close, ATRReversalSettings{1, dec("1")}, 2, "candle_count"},
		{"atr zero period", Close, ATRReversalSettings{0, dec("1")}, 3, "atr_period"},
		{"atr zero multiplier", Close, ATRReversalSettings{1, decimal.Zero}, 3, "atr_multiplier"},
		{"atr negative multiplier", Close, ATRReversalSettings{1, dec("-1")}, 3, "atr_multiplier"},
		{"atr overflow", Close, ATRReversalSettings{math.MaxUint32, dec("1")}, math.MaxUint32, "candle_count"},
		{"missing method", Close, nil, 3, "extrema.method"},
		{"nil method pointer", Close, (*LocalExtremaSettings)(nil), 3, "extrema.method"},
		{"missing source", "", LocalExtremaSettings{1}, 3, "price_source"},
		{"unknown source", "OPEN", LocalExtremaSettings{1}, 3, "price_source"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {

			err := (ExtremaSettings{tc.source, tc.method}).Validate(tc.count)

			checkValidation(t, err, tc.field)
		})
	}
}

func checkValidation(t *testing.T, err error, field string) {
	t.Helper()
	if field == "" {

		require.NoError(t, err)
		return
	}

	var detail *ValidationError
	require.ErrorAs(t, err, &detail)
	assert.Equal(t, field, detail.Field)
}

func TestATRSettings(t *testing.T) {
	cases := []struct {
		name          string
		period, count uint32
		field         string
	}{
		{"minimum", 1, 2, ""}, {"history", 14, 300, ""}, {"zero", 0, 2, "period"}, {"short", 14, 14, "candle_count"}, {"overflow", math.MaxUint32, math.MaxUint32, "candle_count"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {

			checkValidation(t, (ATRSettings{tc.period}).Validate(tc.count), tc.field)
		})
	}
}

func TestTrendSettings(t *testing.T) {
	cases := []struct {
		name, tolerance string
		count           uint32
		field           string
	}{
		{"explicit zero", "0", 3, ""}, {"positive", "1", 3, ""}, {"negative", "-0.01", 3, "equality_tolerance_pct"}, {"short history", "0", 2, "candle_count"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			settings := TrendSettings{ExtremaSettings{Close, LocalExtremaSettings{1}}, dec(tc.tolerance)}

			checkValidation(t, settings.Validate(tc.count), tc.field)
		})
	}
}

func TestLevelSettings(t *testing.T) {
	cases := []struct {
		name   string
		change func(*LevelSettings)
		count  uint32
		field  string
	}{
		{"valid", func(s *LevelSettings) {}, 3, ""},
		{"extrema short", func(s *LevelSettings) { s.Extrema.Method = LocalExtremaSettings{2} }, 3, "candle_count"},
		{"atr short", func(s *LevelSettings) { s.ATRPeriod = 3 }, 3, "candle_count"},
		{"atr zero", func(s *LevelSettings) { s.ATRPeriod = 0 }, 3, "atr_period"},
		{"atr overflow", func(s *LevelSettings) { s.ATRPeriod = math.MaxUint32 }, math.MaxUint32, "candle_count"},
		{"zero width", func(s *LevelSettings) { s.ZoneWidthATR = decimal.Zero }, 3, "zone_width_atr"},
		{"negative width", func(s *LevelSettings) { s.ZoneWidthATR = dec("-1") }, 3, "zone_width_atr"},
		{"few touches", func(s *LevelSettings) { s.MinTouches = 1 }, 3, "min_touches"},
		{"zero spacing", func(s *LevelSettings) { s.MinTouchSeparationBars = 0 }, 3, "min_touch_separation_bars"},
		{"independent atr periods", func(s *LevelSettings) { s.Extrema.Method = ATRReversalSettings{5, dec("2")}; s.ATRPeriod = 14 }, 15, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			settings := LevelSettings{ExtremaSettings{Close, LocalExtremaSettings{1}}, 1, dec("1"), 2, 1}
			tc.change(&settings)

			checkValidation(t, settings.Validate(tc.count), tc.field)
		})
	}
}
