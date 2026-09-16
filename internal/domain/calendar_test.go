package domain

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func timestamp(value string) time.Time {
	result, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		panic(err)
	}

	return result
}

func selection() CandleSelection {
	return CandleSelection{Instrument: Instrument{"binance", "spot", "BTCUSDT"}, Interval: "1m", To: timestamp("2026-09-15T14:02:30Z"), CandleCount: 60}
}

func TestSelectionRange(t *testing.T) {
	cases := []struct {
		name, interval, to, from, end string
		count                         uint32
	}{
		{"partial minute", "1m", "2026-09-15T14:02:30Z", "2026-09-15T13:02:00Z", "2026-09-15T14:02:00Z", 60},
		{"exact minute", "1m", "2026-09-15T14:02:00Z", "2026-09-15T14:01:00Z", "2026-09-15T14:02:00Z", 1},
		{"timezone", "1m", "2026-09-15T16:02:30+02:00", "2026-09-15T13:02:00Z", "2026-09-15T14:02:00Z", 60},
		{"week", "1w", "2026-09-16T12:00:00Z", "2026-09-07T00:00:00Z", "2026-09-14T00:00:00Z", 1},
		{"leap month", "1M", "2024-03-15T12:00:00Z", "2024-02-01T00:00:00Z", "2024-03-01T00:00:00Z", 1},
		{"year boundary", "1M", "2025-02-01T00:00:00Z", "2024-11-01T00:00:00Z", "2025-02-01T00:00:00Z", 3},
		{"leap day", "1d", "2024-03-01T01:00:00Z", "2024-02-29T00:00:00Z", "2024-03-01T00:00:00Z", 1},
		{"three day anchor", "3d", "1970-01-08T12:00:00Z", "1970-01-02T00:00:00Z", "1970-01-08T00:00:00Z", 2},
		{"epoch", "1s", "1970-01-01T00:00:01Z", "1970-01-01T00:00:00Z", "1970-01-01T00:00:01Z", 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := selection()
			s.Interval = Interval(tc.interval)
			s.To = timestamp(tc.to)
			s.CandleCount = tc.count

			result, err := s.Range()

			require.NoError(t, err)
			assert.Equal(t, timestamp(tc.from), result.From)
			assert.Equal(t, timestamp(tc.end), result.To)
		})
	}
}

func TestSelectionRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name   string
		change func(*CandleSelection)
		field  string
	}{
		{"exchange", func(s *CandleSelection) { s.Instrument.Exchange = "unknown" }, "exchange"},
		{"market", func(s *CandleSelection) { s.Instrument.Market = "inverse" }, "market"},
		{"empty symbol", func(s *CandleSelection) { s.Instrument.Symbol = "" }, "symbol"},
		{"symbol whitespace", func(s *CandleSelection) { s.Instrument.Symbol = "BTC USDT" }, "symbol"},
		{"symbol control", func(s *CandleSelection) { s.Instrument.Symbol = "BTC\x00" }, "symbol"},
		{"symbol unicode space", func(s *CandleSelection) { s.Instrument.Symbol = "BTC\u00a0" }, "symbol"},
		{"symbol length", func(s *CandleSelection) { s.Instrument.Symbol = strings.Repeat("a", 129) }, "symbol"},
		{"symbol invalid UTF8", func(s *CandleSelection) { s.Instrument.Symbol = "\xff" }, "symbol"},
		{"interval", func(s *CandleSelection) { s.Interval = "1H" }, "interval"},
		{"zero count", func(s *CandleSelection) { s.CandleCount = 0 }, "candle_count"},
		{"missing time", func(s *CandleSelection) { s.To = time.Time{} }, "to"},
		{"future timestamp overflow", func(s *CandleSelection) { s.To = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC) }, "to"},
		{"huge range", func(s *CandleSelection) { s.CandleCount = math.MaxUint32; s.Interval = "1M" }, "range"},
		{"before epoch", func(s *CandleSelection) { s.To = timestamp("1970-01-01T00:00:00Z") }, "range"},
		{"week before epoch", func(s *CandleSelection) { s.To = timestamp("1970-01-01T00:00:00Z"); s.Interval = "1w" }, "range"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := selection()
			tc.change(&s)

			_, err := s.Range()
			var detail *ValidationError
			require.ErrorAs(t, err, &detail)
			assert.Equal(t, tc.field, detail.Field)
			assert.Contains(t, err.Error(), tc.field)
		})
	}
}

func TestIntervalCombinations(t *testing.T) {
	intervals := []Interval{"1s", "1m", "3m", "5m", "15m", "30m", "1h", "2h", "4h", "6h", "8h", "12h", "1d", "3d", "1w", "1M"}
	scopes := []struct {
		exchange, market string
		extras           bool
		seconds          bool
	}{
		{"binance", "spot", true, true}, {"binance", "linear", true, false}, {"bybit", "spot", false, false}, {"bybit", "linear", false, false},
	}

	for _, scope := range scopes {
		for _, interval := range intervals {
			t.Run(scope.exchange+"/"+scope.market+"/"+string(interval), func(t *testing.T) {
				err := interval.Validate(Instrument{scope.exchange, scope.market, "bTc-USDT"})
				allowed := true
				if interval == "1s" {
					allowed = scope.seconds
				}

				if interval == "8h" || interval == "3d" {
					allowed = scope.extras
				}

				if allowed {

					require.NoError(t, err)
				} else {
					require.Error(t, err)
				}
			})
		}
	}
}

func TestShiftRejectsInvalidBoundaries(t *testing.T) {
	cases := []struct {
		name     string
		interval Interval
		boundary time.Time
		slots    int64
	}{
		{"unknown", "bad", timestamp("2026-01-01T00:00:00Z"), 1},
		{"unaligned", "1m", timestamp("2026-01-01T00:00:01Z"), 1},
		{"seconds overflow", "1s", timestamp("2026-01-01T00:00:00Z"), math.MaxInt64},
		{"seconds underflow", "1s", timestamp("2026-01-01T00:00:00Z"), math.MinInt64},
		{"months overflow", "1M", timestamp("2026-01-01T00:00:00Z"), math.MaxInt64},
		{"months underflow", "1M", timestamp("2026-01-01T00:00:00Z"), math.MinInt64},
		{"last timestamp", "1s", timestamp("9999-12-31T23:59:59Z"), 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {

			_, err := tc.interval.Shift(tc.boundary, tc.slots)
			require.Error(t, err)
		})
	}
}

func TestFloorFixedIntervals(t *testing.T) {
	cases := []struct {
		interval Interval
		want     string
	}{
		{"1s", "2026-09-15T17:47:38Z"},
		{"1m", "2026-09-15T17:47:00Z"},
		{"3m", "2026-09-15T17:45:00Z"},
		{"5m", "2026-09-15T17:45:00Z"},
		{"15m", "2026-09-15T17:45:00Z"},
		{"30m", "2026-09-15T17:30:00Z"},
		{"1h", "2026-09-15T17:00:00Z"},
		{"2h", "2026-09-15T16:00:00Z"},
		{"4h", "2026-09-15T16:00:00Z"},
		{"6h", "2026-09-15T12:00:00Z"},
		{"8h", "2026-09-15T16:00:00Z"},
		{"12h", "2026-09-15T12:00:00Z"},
		{"1d", "2026-09-15T00:00:00Z"},
	}

	for _, tc := range cases {
		t.Run(string(tc.interval), func(t *testing.T) {

			result, err := tc.interval.Floor(timestamp("2026-09-15T17:47:38.123456789Z"))

			require.NoError(t, err)
			assert.Equal(t, timestamp(tc.want), result)
		})
	}
}

func TestEpochUsesUTCYear(t *testing.T) {
	s := selection()
	s.Interval = "1s"
	s.CandleCount = 1
	s.To = timestamp("1969-12-31T23:00:01-01:00")

	result, err := s.Range()

	require.NoError(t, err)
	assert.Equal(t, timestamp("1970-01-01T00:00:00Z"), result.From)
}

func TestInstrumentPreservesSymbol(t *testing.T) {
	symbol := "bTc" + strings.Repeat("a", 125)
	instrument := Instrument{"bybit", "linear", symbol}

	require.NoError(t, instrument.Validate())
	assert.Equal(t, symbol, instrument.Symbol)
}
