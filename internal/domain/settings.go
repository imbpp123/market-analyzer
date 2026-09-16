package domain

import "github.com/shopspring/decimal"

type ATRSettings struct{ Period uint32 }

func (s ATRSettings) Validate(count uint32) error {
	if s.Period == 0 {
		return invalid("period", "must be positive")
	}

	return minimumCount(count, uint64(s.Period)+1)
}

func minimumCount(count uint32, minimum uint64) error {
	if uint64(count) < minimum {
		return invalid("candle_count", "insufficient candles for settings")
	}

	return nil
}

type PriceSource string

const (
	Close   PriceSource = "CLOSE"
	HighLow PriceSource = "HIGH_LOW"
)

// ExtremaMethod is sealed: one concrete value selects exactly one method.
type ExtremaMethod interface{ extremaMethod() }

type LocalExtremaSettings struct{ PivotSpan uint32 }

func (LocalExtremaSettings) extremaMethod() {}

type PercentReversalSettings struct{ ReversalPct decimal.Decimal }

func (PercentReversalSettings) extremaMethod() {}

type ATRReversalSettings struct {
	ATRPeriod     uint32
	ATRMultiplier decimal.Decimal
}

func (ATRReversalSettings) extremaMethod() {}

type ExtremaSettings struct {
	PriceSource PriceSource
	Method      ExtremaMethod
}

func (s ExtremaSettings) Validate(count uint32) error {
	if s.PriceSource != Close && s.PriceSource != HighLow {
		return invalid("price_source", "unsupported price source")
	}

	var minimum uint64
	switch method := s.Method.(type) {
	case LocalExtremaSettings:
		if method.PivotSpan == 0 {
			return invalid("pivot_span", "must be positive")
		}

		minimum = 2*uint64(method.PivotSpan) + 1
	case PercentReversalSettings:
		if !method.ReversalPct.IsPositive() || method.ReversalPct.GreaterThanOrEqual(decimal.NewFromInt(100)) {
			return invalid("reversal_pct", "must be greater than zero and less than 100")
		}

		minimum = 2
	case ATRReversalSettings:
		if method.ATRPeriod == 0 {
			return invalid("atr_period", "must be positive")
		}

		if !method.ATRMultiplier.IsPositive() {
			return invalid("atr_multiplier", "must be positive")
		}

		minimum = uint64(method.ATRPeriod) + 2
	default:
		return invalid("extrema.method", "must select one concrete method value")
	}

	return minimumCount(count, minimum)
}

type TrendSettings struct {
	Extrema              ExtremaSettings
	EqualityTolerancePct decimal.Decimal
}

func (s TrendSettings) Validate(count uint32) error {
	if err := s.Extrema.Validate(count); err != nil {
		return err
	}

	if s.EqualityTolerancePct.IsNegative() {
		return invalid("equality_tolerance_pct", "must be nonnegative")
	}

	return nil
}

type LevelSettings struct {
	Extrema                ExtremaSettings
	ATRPeriod              uint32
	ZoneWidthATR           decimal.Decimal
	MinTouches             uint32
	MinTouchSeparationBars uint32
}

func (s LevelSettings) Validate(count uint32) error {
	if err := s.Extrema.Validate(count); err != nil {
		return err
	}

	if s.ATRPeriod == 0 {
		return invalid("atr_period", "must be positive")
	}

	if err := minimumCount(count, uint64(s.ATRPeriod)+1); err != nil {
		return err
	}

	if !s.ZoneWidthATR.IsPositive() {
		return invalid("zone_width_atr", "must be positive")
	}

	if s.MinTouches < 2 {
		return invalid("min_touches", "must be at least 2")
	}

	if s.MinTouchSeparationBars == 0 {
		return invalid("min_touch_separation_bars", "must be positive")
	}

	return nil
}
