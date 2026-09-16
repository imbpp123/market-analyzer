package application

import (
	"context"
	"fmt"

	"github.com/imbpp123/market-analyzer/internal/domain"
	"github.com/shopspring/decimal"
)

func parseSource(ctx context.Context, source SourceSeries) ([]SourceCandle, []domain.Candle, error) {
	raw := make([]SourceCandle, len(source.Candles))
	parsed := make([]domain.Candle, len(source.Candles))
	for index, candle := range source.Candles {
		if err := ctx.Err(); err != nil {
			return nil, nil, calculationError(err)
		}

		values := make([]decimal.Decimal, 0, 6)
		for _, field := range []struct {
			name  string
			value string
		}{
			{"open", candle.Open}, {"high", candle.High}, {"low", candle.Low}, {"close", candle.Close},
			{"volume", candle.Volume}, {"turnover", candle.Turnover},
		} {
			value, err := parseDecimal(fmt.Sprintf("candles[%d].%s", index, field.name), field.value)
			if err != nil {
				applicationError := err.(*Error)
				applicationError.Kind = InvalidMarketData
				return nil, nil, applicationError
			}
			values = append(values, value)
		}

		raw[index] = cloneSourceCandle(candle)
		parsed[index] = domain.Candle{OpenTime: candle.OpenTime, CloseTime: candle.CloseTime, Open: values[0], High: values[1], Low: values[2],
			Close: values[3], Volume: values[4], Turnover: values[5], TradesCount: cloneInt64(candle.TradesCount), FetchedAt: candle.FetchedAt}
	}

	return raw, parsed, nil
}

func cloneSourceCandle(candle SourceCandle) SourceCandle {
	candle.TradesCount = cloneInt64(candle.TradesCount)
	return candle
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}

	clone := *value
	return &clone
}
