package domain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateExchange(t *testing.T) {
	cases := []struct {
		name     string
		exchange string
		valid    bool
	}{
		{"binance", ExchangeBinance, true},
		{"bybit", ExchangeBybit, true},
		{"empty", "", false},
		{"unknown", "kraken", false},
		{"wrong case", "Binance", false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := ValidateExchange(testCase.exchange)

			if testCase.valid {
				require.NoError(t, err)
				return
			}
			checkValidation(t, err, "exchange")
		})
	}
}

func TestValidateMarket(t *testing.T) {
	cases := []struct {
		name   string
		market string
		valid  bool
	}{
		{"spot", MarketSpot, true},
		{"linear", MarketLinear, true},
		{"empty", "", false},
		{"unknown", "inverse", false},
		{"wrong case", "Spot", false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := ValidateMarket(testCase.market)

			if testCase.valid {
				require.NoError(t, err)
				return
			}
			checkValidation(t, err, "market")
		})
	}
}

func TestValidateSymbol(t *testing.T) {
	cases := []struct {
		name   string
		symbol string
		valid  bool
	}{
		{"ordinary", "BTCUSDT", true},
		{"mixed case", "bTc-USDT", true},
		{"maximum length", strings.Repeat("a", 128), true},
		{"empty", "", false},
		{"too long", strings.Repeat("a", 129), false},
		{"space", "BTC USDT", false},
		{"unicode space", "BTC\u00a0USDT", false},
		{"control", "BTC\x00USDT", false},
		{"invalid UTF8", "\xff", false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := ValidateSymbol(testCase.symbol)

			if testCase.valid {
				require.NoError(t, err)
				return
			}
			checkValidation(t, err, "symbol")
		})
	}
}

func TestInstrumentIsTrading(t *testing.T) {
	cases := []struct {
		name   string
		status InstrumentStatus
		want   bool
	}{
		{"trading", InstrumentStatusTrading, true},
		{"halted", "halted", false},
		{"unknown", "unknown", false},
		{"empty", "", false},
		{"wrong case", "Trading", false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			instrument := Instrument{Status: testCase.status}

			assert.Equal(t, testCase.want, instrument.IsTrading())
		})
	}
}

func TestInstrumentPreservesSymbol(t *testing.T) {
	symbol := "bTc" + strings.Repeat("a", 125)
	instrument := Instrument{
		Exchange: "bybit",
		Market:   "linear",
		Symbol:   symbol,
	}

	require.NoError(t, instrument.Validate())
	assert.Equal(t, symbol, instrument.Symbol)
}
