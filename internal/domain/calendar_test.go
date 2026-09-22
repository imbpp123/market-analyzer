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
	return CandleSelection{
		Instrument: Instrument{
			Exchange: "binance",
			Market:   "spot",
			Symbol:   "BTCUSDT",
		},
		Interval:    "1m",
		To:          timestamp("2026-09-15T14:02:30Z"),
		CandleCount: 60,
	}
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
		{"epoch", "1m", "1970-01-01T00:01:00Z", "1970-01-01T00:00:00Z", "1970-01-01T00:01:00Z", 1},
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
		{"one second", func(s *CandleSelection) { s.Interval = "1s" }, "interval"},
		{"eight hours", func(s *CandleSelection) { s.Interval = "8h" }, "interval"},
		{"three days", func(s *CandleSelection) { s.Interval = "3d" }, "interval"},
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
	intervals := []Interval{"1m", "3m", "5m", "15m", "30m", "1h", "2h", "4h", "6h", "12h", "1d", "1w", "1M"}

	for _, exchange := range []string{ExchangeBinance, ExchangeBybit} {
		for _, market := range []string{MarketSpot, MarketLinear} {
			for _, interval := range intervals {
				t.Run(exchange+"/"+market+"/"+string(interval), func(t *testing.T) {
					err := interval.Validate(Instrument{
						Exchange: exchange,
						Market:   market,
						Symbol:   "bTc-USDT",
					})

					require.NoError(t, err)
				})
			}
		}
	}
}

func TestRemovedIntervalsRejectFloor(t *testing.T) {
	for _, interval := range []Interval{"1s", "8h", "3d"} {
		t.Run(string(interval), func(t *testing.T) {
			_, err := interval.Floor(timestamp("2026-09-15T17:47:38Z"))

			checkValidation(t, err, "interval")
		})
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
		{"fixed overflow", "1m", timestamp("2026-01-01T00:00:00Z"), math.MaxInt64},
		{"fixed underflow", "1m", timestamp("2026-01-01T00:00:00Z"), math.MinInt64},
		{"months overflow", "1M", timestamp("2026-01-01T00:00:00Z"), math.MaxInt64},
		{"months underflow", "1M", timestamp("2026-01-01T00:00:00Z"), math.MinInt64},
		{"last timestamp", "1m", timestamp("9999-12-31T23:59:00Z"), 1},
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
		{"1m", "2026-09-15T17:47:00Z"},
		{"3m", "2026-09-15T17:45:00Z"},
		{"5m", "2026-09-15T17:45:00Z"},
		{"15m", "2026-09-15T17:45:00Z"},
		{"30m", "2026-09-15T17:30:00Z"},
		{"1h", "2026-09-15T17:00:00Z"},
		{"2h", "2026-09-15T16:00:00Z"},
		{"4h", "2026-09-15T16:00:00Z"},
		{"6h", "2026-09-15T12:00:00Z"},
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
	s.Interval = "1m"
	s.CandleCount = 1
	s.To = timestamp("1969-12-31T23:01:00-01:00")

	result, err := s.Range()

	require.NoError(t, err)
	assert.Equal(t, timestamp("1970-01-01T00:00:00Z"), result.From)
}
