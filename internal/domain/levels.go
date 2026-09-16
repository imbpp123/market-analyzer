package domain

import (
	"context"

	"github.com/shopspring/decimal"
)

func CalculateLevels(ctx context.Context, series CandleSeries, settings LevelSettings) (LevelsResult, error) {
	if err := ctx.Err(); err != nil {
		return LevelsResult{}, err
	}

	if err := settings.Validate(uint32(len(series.candles))); err != nil {
		return LevelsResult{}, err
	}

	sequence, err := calculateATRSequence(ctx, series, ATRSettings{Period: settings.ATRPeriod})
	if err != nil {
		return LevelsResult{}, err
	}

	points, err := detectExtrema(ctx, series, settings.Extrema, &sequence)
	if err != nil {
		return LevelsResult{}, err
	}

	atr := sequence.latest(series)
	width := atr.Value.Mul(settings.ZoneWidthATR)
	close := series.candles[len(series.candles)-1].Close
	zones, err := buildZones(ctx, series, points.Points, width, close, settings.MinTouches, settings.MinTouchSeparationBars)
	if err != nil {
		return LevelsResult{}, err
	}

	return LevelsResult{Zones: zones, Extrema: points, ATR: atr, MaximumZoneWidth: width, ReferenceClose: close}, nil
}

// Sort references, keeping the complete chronological point list unchanged.
func buildZones(ctx context.Context, series CandleSeries, points []Extremum, width, close decimal.Decimal, minTouches, spacing uint32) ([]PriceZone, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	indices := make([]int, len(points))
	for i := range indices {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		indices[i] = i
	}

	err := sortIndices(ctx, indices, func(a, b int) bool {
		left, right := points[a], points[b]
		if comparison := left.Price.Cmp(right.Price); comparison != 0 {
			return comparison < 0
		}

		if left.CandleIndex != right.CandleIndex {
			return left.CandleIndex < right.CandleIndex
		}

		return left.Kind == High && right.Kind == Low
	})
	if err != nil {
		return nil, err
	}

	zones := make([]PriceZone, 0)
	for start := 0; start < len(indices); {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		lower := points[indices[start]].Price
		boundary := lower.Add(width)
		end := start + 1
		for end < len(indices) && points[indices[end]].Price.LessThanOrEqual(boundary) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}

			end++
		}

		group := indices[start:end]
		zone, err := zoneFromGroup(ctx, series, points, group, close, spacing)
		if err != nil {
			return nil, err
		}

		if zone.TouchCount >= minTouches {
			zones = append(zones, zone)
		}

		start = end
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return zones, nil
}

func zoneFromGroup(ctx context.Context, series CandleSeries, points []Extremum, group []int, close decimal.Decimal, spacing uint32) (PriceZone, error) {
	lower, upper := points[group[0]].Price, points[group[len(group)-1]].Price
	midpoint, err := Divide(lower.Add(upper), decimal.NewFromInt(2))
	if err != nil {
		return PriceZone{}, err
	}

	zone := PriceZone{LowerBound: lower, UpperBound: upper, RepresentativePrice: midpoint, Role: AtPrice,
		ExtremumIndices: make([]ExtremumIndex, len(group)), AcceptedCandleIndices: make([]CandleIndex, 0)}
	if close.GreaterThan(upper) {
		zone.Role = Support
	} else if close.LessThan(lower) {
		zone.Role = Resistance
	}

	candles := make([]int, len(group))
	for i, index := range group {
		if err := ctx.Err(); err != nil {
			return PriceZone{}, err
		}

		zone.ExtremumIndices[i] = ExtremumIndex(index)
		candles[i] = int(points[index].CandleIndex)
	}

	if err := sortIndices(ctx, candles, func(a, b int) bool { return a < b }); err != nil {
		return PriceZone{}, err
	}

	for _, index := range candles {
		if err := ctx.Err(); err != nil {
			return PriceZone{}, err
		}

		accepted := zone.AcceptedCandleIndices
		if len(accepted) == 0 || uint64(index)-uint64(accepted[len(accepted)-1]) >= uint64(spacing) {
			zone.AcceptedCandleIndices = append(accepted, CandleIndex(index))
		}
	}

	zone.TouchCount = uint32(len(zone.AcceptedCandleIndices))
	zone.FirstTouchTime = series.candles[zone.AcceptedCandleIndices[0]].OpenTime
	zone.LastTouchTime = series.candles[zone.AcceptedCandleIndices[len(zone.AcceptedCandleIndices)-1]].OpenTime
	return zone, nil
}

// Merge sorting allows cancellation inside sorting work without panics or workers.
func sortIndices(ctx context.Context, values []int, less func(int, int) bool) error {
	buffer := make([]int, len(values))
	var merge func(int, int) error
	merge = func(start, end int) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		if end-start < 2 {
			return nil
		}

		middle := start + (end-start)/2
		if err := merge(start, middle); err != nil {
			return err
		}

		if err := merge(middle, end); err != nil {
			return err
		}

		left, right := start, middle
		for i := start; i < end; i++ {
			if err := ctx.Err(); err != nil {
				return err
			}

			if left < middle && (right == end || !less(values[right], values[left])) {
				buffer[i] = values[left]
				left++
			} else {
				buffer[i] = values[right]
				right++
			}
		}

		for i := start; i < end; i++ {
			if err := ctx.Err(); err != nil {
				return err
			}

			values[i] = buffer[i]
		}

		return nil
	}

	return merge(0, len(values))
}
