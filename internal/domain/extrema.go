package domain

import (
	"context"

	"github.com/shopspring/decimal"
)

func DetectExtrema(ctx context.Context, series CandleSeries, settings ExtremaSettings) (ExtremaResult, error) {
	return detectExtrema(ctx, series, settings, nil)
}

// A supplied sequence belongs to this series and is reused only for a matching period.
func detectExtrema(ctx context.Context, series CandleSeries, settings ExtremaSettings, sequence *atrSequence) (ExtremaResult, error) {
	if err := ctx.Err(); err != nil {
		return ExtremaResult{}, err
	}

	if err := settings.Validate(uint32(len(series.candles))); err != nil {
		return ExtremaResult{}, err
	}

	switch method := settings.Method.(type) {
	case LocalExtremaSettings:
		return localExtrema(ctx, series, settings.PriceSource, int(method.PivotSpan))
	case ATRReversalSettings:
		if sequence == nil || sequence.period != method.ATRPeriod {
			values, err := calculateATRSequence(ctx, series, ATRSettings{Period: method.ATRPeriod})
			if err != nil {
				return ExtremaResult{}, err
			}

			sequence = &values
		}
	}

	return reversalExtrema(ctx, series, settings, sequence)
}

func selectedPrices(c Candle, source PriceSource) (decimal.Decimal, decimal.Decimal) {
	if source == Close {
		return c.Close, c.Close
	}

	return c.High, c.Low
}

func confirmedPoint(series CandleSeries, kind ExtremumKind, index, confirmation int, price decimal.Decimal) Extremum {
	return Extremum{Kind: kind, CandleIndex: CandleIndex(index), Time: series.candles[index].OpenTime, Price: price,
		ConfirmationCandleIndex: CandleIndex(confirmation), ConfirmationTime: series.candles[confirmation].CloseTime}
}

func localExtrema(ctx context.Context, series CandleSeries, source PriceSource, span int) (ExtremaResult, error) {
	points := make([]Extremum, 0)
	for i := span; i < len(series.candles)-span; i++ {
		high, low := selectedPrices(series.candles[i], source)
		isHigh, isLow := true, true
		for j := i - span; j <= i+span; j++ {
			if err := ctx.Err(); err != nil {
				return ExtremaResult{}, err
			}

			if j == i {
				continue
			}

			neighborHigh, neighborLow := selectedPrices(series.candles[j], source)
			isHigh = isHigh && high.GreaterThan(neighborHigh)
			isLow = isLow && low.LessThan(neighborLow)
			if !isHigh && !isLow {
				break
			}
		}

		if isHigh {
			points = append(points, confirmedPoint(series, High, i, i+span, high))
		}

		if isLow {
			points = append(points, confirmedPoint(series, Low, i, i+span, low))
		}
	}

	if err := ctx.Err(); err != nil {
		return ExtremaResult{}, err
	}

	return ExtremaResult{Points: points}, nil
}

type reversalCandidate struct {
	index    int
	price    decimal.Decimal
	evidence ReversalEvidence
}

func newCandidate(index int, price decimal.Decimal, method ExtremaMethod, sequence *atrSequence) (reversalCandidate, error) {
	evidence := ReversalEvidence{}
	switch settings := method.(type) {
	case PercentReversalSettings:
		threshold, err := Divide(price.Mul(settings.ReversalPct), decimal.NewFromInt(100))
		if err != nil {
			return reversalCandidate{}, err
		}

		evidence.Threshold = threshold
	case ATRReversalSettings:
		atr := sequence.at(index)
		evidence.CandidateATR = &atr
		evidence.Threshold = atr.Mul(settings.ATRMultiplier)
	}

	return reversalCandidate{index: index, price: price, evidence: evidence}, nil
}

func reversalExtrema(ctx context.Context, series CandleSeries, settings ExtremaSettings, sequence *atrSequence) (ExtremaResult, error) {
	start := 0
	if method, ok := settings.Method.(ATRReversalSettings); ok {
		start = int(method.ATRPeriod)
	}

	highPrice, lowPrice := selectedPrices(series.candles[start], settings.PriceSource)
	high, err := newCandidate(start, highPrice, settings.Method, sequence)
	if err != nil {
		return ExtremaResult{}, err
	}

	low, err := newCandidate(start, lowPrice, settings.Method, sequence)
	if err != nil {
		return ExtremaResult{}, err
	}

	var searching ExtremumKind // Empty means the initial direction is unknown.
	points := make([]Extremum, 0)
	for i := start + 1; i < len(series.candles); i++ {
		if err := ctx.Err(); err != nil {
			return ExtremaResult{}, err
		}

		highPrice, lowPrice = selectedPrices(series.candles[i], settings.PriceSource)
		if searching != Low && highPrice.GreaterThanOrEqual(high.price) {
			high, err = newCandidate(i, highPrice, settings.Method, sequence)
			if err != nil {
				return ExtremaResult{}, err
			}
		}

		if searching != High && lowPrice.LessThanOrEqual(low.price) {
			low, err = newCandidate(i, lowPrice, settings.Method, sequence)
			if err != nil {
				return ExtremaResult{}, err
			}
		}

		confirmHigh := searching != Low && high.index < i && high.evidence.Threshold.IsPositive() && high.price.Sub(lowPrice).GreaterThanOrEqual(high.evidence.Threshold)
		confirmLow := searching != High && low.index < i && low.evidence.Threshold.IsPositive() && highPrice.Sub(low.price).GreaterThanOrEqual(low.evidence.Threshold)
		if confirmHigh == confirmLow {
			continue
		}

		candidate, kind, confirmationPrice := high, High, lowPrice
		if confirmLow {
			candidate, kind, confirmationPrice = low, Low, highPrice
		}

		point := confirmedPoint(series, kind, candidate.index, i, candidate.price)
		evidence := candidate.evidence
		evidence.ConfirmationPrice = confirmationPrice
		point.Reversal = &evidence
		points = append(points, point)

		if confirmHigh {
			searching = Low
			low, err = newCandidate(i, lowPrice, settings.Method, sequence)
		} else {
			searching = High
			high, err = newCandidate(i, highPrice, settings.Method, sequence)
		}

		if err != nil {
			return ExtremaResult{}, err
		}
	}

	if err := ctx.Err(); err != nil {
		return ExtremaResult{}, err
	}

	return ExtremaResult{Points: points}, nil
}
