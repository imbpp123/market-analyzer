package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMarketStatsValidate(t *testing.T) {
	cases := []struct {
		name   string
		change func(*MarketStats)
		field  string
	}{
		{
			name: "valid",
		},
		{
			name:   "zero volume and missing trade count",
			change: func(stats *MarketStats) { stats.Volume = "0"; stats.TradeCount = nil },
		},
		{
			name:   "invalid exchange",
			change: func(stats *MarketStats) { stats.Exchange = "unknown" },
			field:  "exchange",
		},
		{
			name:   "invalid market",
			change: func(stats *MarketStats) { stats.Market = "unknown" },
			field:  "market",
		},
		{
			name:   "invalid symbol",
			change: func(stats *MarketStats) { stats.Symbol = "BAD SYMBOL" },
			field:  "symbol",
		},
		{
			name:   "empty volume",
			change: func(stats *MarketStats) { stats.Volume = "" },
			field:  "volume",
		},
		{
			name:   "exponent volume",
			change: func(stats *MarketStats) { stats.Volume = "1e2" },
			field:  "volume",
		},
		{
			name:   "long volume",
			change: func(stats *MarketStats) { stats.Volume = strings.Repeat("1", 1025) },
			field:  "volume",
		},
		{
			name:   "negative volume",
			change: func(stats *MarketStats) { stats.Volume = "-0.1" },
			field:  "volume",
		},
		{
			name: "negative trade count",
			change: func(stats *MarketStats) {
				count := int64(-1)
				stats.TradeCount = &count
			},
			field: "trade_count",
		},
		{
			name:   "missing fetched time",
			change: func(stats *MarketStats) { stats.FetchedAt = time.Time{} },
			field:  "fetched_at",
		},
		{
			name:   "invalid fetched time",
			change: func(stats *MarketStats) { stats.FetchedAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC) },
			field:  "fetched_at",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			count := int64(0)
			stats := MarketStats{
				Exchange:   ExchangeBinance,
				Market:     MarketSpot,
				Symbol:     "BTCUSDT",
				Volume:     "100.00",
				TradeCount: &count,
				FetchedAt:  time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC),
			}
			if testCase.change != nil {
				testCase.change(&stats)
			}

			err := stats.Validate()

			if testCase.field == "" {
				require.NoError(t, err)
				return
			}
			checkValidation(t, err, testCase.field)
		})
	}
}
