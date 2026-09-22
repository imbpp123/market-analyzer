package domain

import (
	"regexp"
	"time"

	"github.com/shopspring/decimal"
)

var plainMarketVolume = regexp.MustCompile(`^[+-]?[0-9]+(?:\.[0-9]+)?$`)

type MarketStats struct {
	Exchange   string
	Market     string
	Symbol     string
	Volume     string
	TradeCount *int64
	FetchedAt  time.Time
}

func (s MarketStats) Validate() error {
	if err := ValidateExchange(s.Exchange); err != nil {
		return err
	}

	if err := ValidateMarket(s.Market); err != nil {
		return err
	}

	if err := ValidateSymbol(s.Symbol); err != nil {
		return err
	}

	if len(s.Volume) == 0 || len(s.Volume) > 1024 || !plainMarketVolume.MatchString(s.Volume) {
		return invalid("volume", "must be plain base-10 text with at most 1024 characters")
	}

	volume, err := decimal.NewFromString(s.Volume)
	if err != nil || volume.IsNegative() {
		return invalid("volume", "must be a nonnegative decimal")
	}

	if s.TradeCount != nil && *s.TradeCount < 0 {
		return invalid("trade_count", "must be nonnegative")
	}

	if err := validateTime("fetched_at", s.FetchedAt); err != nil {
		return err
	}

	return nil
}
