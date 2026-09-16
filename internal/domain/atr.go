package domain

import (
	"context"

	"github.com/shopspring/decimal"
)

// atrSequence stores only available values, starting at the period's candle index.
type atrSequence struct {
	period uint32
	values []decimal.Decimal
}

func (s atrSequence) at(index int) decimal.Decimal {
	return s.values[index-int(s.period)]
}

func (s atrSequence) latest(series CandleSeries) ATRResult {
	index := len(series.candles) - 1
	return ATRResult{Value: s.at(index), CandleIndex: CandleIndex(index), ValueTime: series.candles[index].CloseTime}
}

func calculateATRSequence(ctx context.Context, series CandleSeries, settings ATRSettings) (atrSequence, error) {
	if err := ctx.Err(); err != nil {
		return atrSequence{}, err
	}

	if err := settings.Validate(uint32(len(series.candles))); err != nil {
		return atrSequence{}, err
	}

	period := int(settings.Period)
	values := make([]decimal.Decimal, 0, len(series.candles)-period)
	sum := decimal.Zero
	divisor := decimal.NewFromInt(int64(period))
	weight := decimal.NewFromInt(int64(period - 1))
	for i := 1; i < len(series.candles); i++ {
		if err := ctx.Err(); err != nil {
			return atrSequence{}, err
		}

		candle := series.candles[i]
		previousClose := series.candles[i-1].Close
		tr := decimal.Max(candle.High.Sub(candle.Low), candle.High.Sub(previousClose).Abs(), candle.Low.Sub(previousClose).Abs())
		if i <= period {
			sum = sum.Add(tr)
		}

		if i < period {
			continue
		}

		numerator := sum
		if i > period {
			numerator = values[len(values)-1].Mul(weight).Add(tr)
		}

		value, err := Divide(numerator, divisor)
		if err != nil {
			return atrSequence{}, err
		}

		values = append(values, value)
	}

	if err := ctx.Err(); err != nil {
		return atrSequence{}, err
	}

	return atrSequence{period: settings.Period, values: values}, nil
}

func CalculateATR(ctx context.Context, series CandleSeries, settings ATRSettings) (ATRResult, error) {
	sequence, err := calculateATRSequence(ctx, series, settings)
	if err != nil {
		return ATRResult{}, err
	}

	return sequence.latest(series), nil
}

func CalculateNATR(ctx context.Context, series CandleSeries, settings ATRSettings) (NATRResult, error) {
	atr, err := CalculateATR(ctx, series, settings)
	if err != nil {
		return NATRResult{}, err
	}

	close := series.candles[atr.CandleIndex].Close
	value, err := Divide(atr.Value.Mul(decimal.NewFromInt(100)), close)
	if err != nil {
		return NATRResult{}, err
	}

	if err := ctx.Err(); err != nil {
		return NATRResult{}, err
	}

	return NATRResult{Value: value, ATR: atr.Value, ReferenceClose: close, CandleIndex: atr.CandleIndex, ValueTime: atr.ValueTime}, nil
}
