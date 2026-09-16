package domain

import (
	"context"

	"github.com/shopspring/decimal"
)

func CalculateTrend(ctx context.Context, series CandleSeries, settings TrendSettings) (TrendResult, error) {
	if err := ctx.Err(); err != nil {
		return TrendResult{}, err
	}

	if err := settings.Validate(uint32(len(series.candles))); err != nil {
		return TrendResult{}, err
	}

	points, err := DetectExtrema(ctx, series, settings.Extrema)
	if err != nil {
		return TrendResult{}, err
	}

	return classifyTrend(ctx, series, points, settings.Extrema.PriceSource, settings.EqualityTolerancePct)
}

type priceStructure struct {
	count   int
	last    decimal.Decimal
	min     decimal.Decimal
	max     decimal.Decimal
	rising  bool
	falling bool
}

func (s *priceStructure) add(price, tolerance decimal.Decimal) {
	if s.count == 0 {
		s.min, s.max = price, price
		s.rising, s.falling = true, true
	} else {
		difference := price.Sub(s.last)
		s.rising = s.rising && difference.GreaterThan(tolerance)
		s.falling = s.falling && difference.LessThan(tolerance.Neg())
		s.min = decimal.Min(s.min, price)
		s.max = decimal.Max(s.max, price)
	}

	s.last = price
	s.count++
}

// points must be confirmed, ordered points from the same validated series.
func classifyTrend(ctx context.Context, series CandleSeries, points ExtremaResult, source PriceSource, tolerancePct decimal.Decimal) (TrendResult, error) {
	if err := ctx.Err(); err != nil {
		return TrendResult{}, err
	}

	close := series.candles[len(series.candles)-1].Close
	tolerance, err := Divide(close.Mul(tolerancePct), decimal.NewFromInt(100))
	if err != nil {
		return TrendResult{}, err
	}

	maximum, minimum := selectedPrices(series.candles[0], source)
	for _, candle := range series.candles {
		if err := ctx.Err(); err != nil {
			return TrendResult{}, err
		}

		high, low := selectedPrices(candle, source)
		maximum, minimum = decimal.Max(maximum, high), decimal.Min(minimum, low)
	}

	var highs, lows priceStructure
	for _, point := range points.Points {
		if err := ctx.Err(); err != nil {
			return TrendResult{}, err
		}

		if point.Kind == High {
			highs.add(point.Price, tolerance)
		} else {
			lows.add(point.Price, tolerance)
		}
	}

	state, reason := Undetermined, MixedStructure
	switch {
	case maximum.Sub(minimum).LessThanOrEqual(tolerance):
		state, reason = Sideways, FlatRange
	case highs.count < 2 || lows.count < 2:
		reason = InsufficientStructure
	case highs.rising && lows.rising:
		if close.LessThan(lows.last.Sub(tolerance)) {
			reason = StructureBroken
		} else {
			state, reason = Up, RisingStructure
		}
	case highs.falling && lows.falling:
		if close.GreaterThan(highs.last.Add(tolerance)) {
			reason = StructureBroken
		} else {
			state, reason = Down, FallingStructure
		}
	case highs.max.Sub(highs.min).LessThanOrEqual(tolerance) && lows.max.Sub(lows.min).LessThanOrEqual(tolerance) &&
		close.GreaterThanOrEqual(lows.min.Sub(tolerance)) && close.LessThanOrEqual(highs.max.Add(tolerance)):
		state, reason = Sideways, HorizontalStructure
	}

	if err := ctx.Err(); err != nil {
		return TrendResult{}, err
	}

	return TrendResult{State: state, Reason: reason, Tolerance: tolerance, ReferenceClose: close, Extrema: points}, nil
}
