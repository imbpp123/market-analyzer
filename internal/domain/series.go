package domain

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

type Candle struct {
	OpenTime    time.Time
	CloseTime   time.Time
	Open        decimal.Decimal
	High        decimal.Decimal
	Low         decimal.Decimal
	Close       decimal.Decimal
	Volume      decimal.Decimal
	Turnover    decimal.Decimal
	TradesCount *int64
	FetchedAt   time.Time
}

// CandleSeries owns a validated copy of the source values.
type CandleSeries struct {
	selection   CandleSelection
	candleRange CandleRange
	candles     []Candle
}

func NewCandleSeries(ctx context.Context, selection CandleSelection, instrument Instrument, interval Interval, actual CandleRange, candles []Candle) (CandleSeries, error) {
	if err := ctx.Err(); err != nil {
		return CandleSeries{}, err
	}

	expected, err := selection.Range()
	if err != nil {
		return CandleSeries{}, err
	}

	if instrument != selection.Instrument {
		return CandleSeries{}, invalid("instrument", "source identity does not match selection")
	}

	if interval != selection.Interval {
		return CandleSeries{}, invalid("interval", "source interval does not match selection")
	}

	if !actual.From.Equal(expected.From) || !actual.To.Equal(expected.To) {
		return CandleSeries{}, invalid("range", "source range does not match selection")
	}

	if uint64(len(candles)) != uint64(selection.CandleCount) {
		return CandleSeries{}, invalid("candles", "source count does not match selection")
	}

	owned := make([]Candle, len(candles))
	opening := expected.From
	for index, candle := range candles {
		if err := ctx.Err(); err != nil {
			return CandleSeries{}, err
		}

		closing, err := interval.Shift(opening, 1)
		if err != nil {
			return CandleSeries{}, err
		}

		if err := candle.validate(opening, closing); err != nil {
			return CandleSeries{}, invalid(fmt.Sprintf("candles[%d].%s", index, err.Field), err.Rule)
		}

		owned[index] = cloneCandle(candle)
		opening = closing
	}

	if err := ctx.Err(); err != nil {
		return CandleSeries{}, err
	}

	return CandleSeries{selection: selection, candleRange: expected, candles: owned}, nil
}

func (c Candle) validate(opening, closing time.Time) *ValidationError {
	if !c.OpenTime.Equal(opening) {
		return invalid("open_time", "must match the next calendar slot")
	}

	if !c.CloseTime.Equal(closing) {
		return invalid("close_time", "must match the exclusive slot end")
	}

	if err := validateTime("fetched_at", c.FetchedAt); err != nil {
		return err
	}

	prices := []struct {
		field string
		value decimal.Decimal
	}{
		{"open", c.Open}, {"high", c.High}, {"low", c.Low}, {"close", c.Close},
	}

	for _, price := range prices {
		if !price.value.IsPositive() {
			return invalid(price.field, "must be positive")
		}
	}

	if c.Low.GreaterThan(c.Open) || c.Open.GreaterThan(c.High) {
		return invalid("open", "must be between low and high")
	}

	if c.Low.GreaterThan(c.Close) || c.Close.GreaterThan(c.High) {
		return invalid("close", "must be between low and high")
	}

	if c.Volume.IsNegative() {
		return invalid("volume", "must be nonnegative")
	}

	if c.Turnover.IsNegative() {
		return invalid("turnover", "must be nonnegative")
	}

	if c.TradesCount != nil && *c.TradesCount < 0 {
		return invalid("trades_count", "must be nonnegative when present")
	}

	return nil
}

func cloneCandle(c Candle) Candle {
	if c.TradesCount != nil {
		count := *c.TradesCount
		c.TradesCount = &count
	}

	return c
}

func (s CandleSeries) Selection() CandleSelection { return s.selection }

func (s CandleSeries) Range() CandleRange { return s.candleRange }

func (s CandleSeries) Candles() []Candle {
	result := make([]Candle, len(s.candles))
	for i, candle := range s.candles {
		result[i] = cloneCandle(candle)
	}

	return result
}
