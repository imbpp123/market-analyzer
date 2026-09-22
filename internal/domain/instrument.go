package domain

import (
	"time"
	"unicode"
	"unicode/utf8"
)

type InstrumentStatus string

const InstrumentStatusTrading InstrumentStatus = "trading"

type Instrument struct {
	Exchange   string
	Market     string
	Symbol     string
	BaseAsset  string
	QuoteAsset string
	Status     InstrumentStatus
}

// Validate checks the identity fields used by candle requests.
func (i Instrument) Validate() error {
	if err := ValidateExchange(i.Exchange); err != nil {
		return err
	}

	if err := ValidateMarket(i.Market); err != nil {
		return err
	}

	return ValidateSymbol(i.Symbol)
}

func (i Instrument) IsTrading() bool {
	return i.Status == InstrumentStatusTrading
}

type ActiveInstrument struct {
	Instrument
	MarketStats   *MarketStats
	NATR          *string
	NATRValueTime time.Time
}

const (
	ExchangeBinance = "binance"
	ExchangeBybit   = "bybit"
	MarketSpot      = "spot"
	MarketLinear    = "linear"
)

func ValidateExchange(exchange string) error {
	if exchange != ExchangeBinance && exchange != ExchangeBybit {
		return invalid("exchange", "unsupported exchange")
	}

	return nil
}

func ValidateMarket(market string) error {
	if market != MarketSpot && market != MarketLinear {
		return invalid("market", "unsupported market")
	}

	return nil
}

func ValidateSymbol(symbol string) error {
	if len(symbol) == 0 || len(symbol) > 128 || !utf8.ValidString(symbol) {
		return invalid("symbol", "must contain 1 to 128 UTF-8 bytes")
	}

	for _, r := range symbol {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return invalid("symbol", "must not contain whitespace or control characters")
		}
	}

	return nil
}
