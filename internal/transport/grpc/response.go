package grpc

import (
	"fmt"

	marketanalyzerv1 "github.com/imbpp123/market-analyzer/api/go/marketanalyzer/v1"
	"github.com/imbpp123/market-analyzer/internal/application"
	"github.com/imbpp123/market-analyzer/internal/domain"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func mapATRResponse(input application.ATRResponse) (*marketanalyzerv1.GetATRResponse, error) {
	value, err := domain.FormatNumber(input.Result.Value)
	if err != nil {
		return nil, responseError(err)
	}

	return &marketanalyzerv1.GetATRResponse{Metadata: mapMetadata(input.Metadata), Candles: mapCandles(input.Candles),
		Settings: mapATRSettingsResponse(input.Settings), Result: &marketanalyzerv1.ATRResult{Value: value,
			CandleIndex: uint32(input.Result.CandleIndex), ValueTime: timestamppb.New(input.Result.ValueTime)}}, nil
}

func mapNATRResponse(input application.NATRResponse) (*marketanalyzerv1.GetNATRResponse, error) {
	atr, err := domain.FormatNumber(input.Result.ATR)
	if err != nil {
		return nil, responseError(err)
	}

	return &marketanalyzerv1.GetNATRResponse{Metadata: mapMetadata(input.Metadata), Candles: mapCandles(input.Candles),
		Settings: mapATRSettingsResponse(input.Settings), Result: &marketanalyzerv1.NATRResult{Value: domain.FormatPercentage(input.Result.Value),
			Atr: atr, ReferenceClose: input.Candles[input.Result.CandleIndex].Close, CandleIndex: uint32(input.Result.CandleIndex),
			ValueTime: timestamppb.New(input.Result.ValueTime)}}, nil
}

func mapExtremaResponse(input application.ExtremaResponse) (*marketanalyzerv1.GetExtremaResponse, error) {
	result, err := mapExtremaResult(input.Result, input.Settings, input.Candles)
	if err != nil {
		return nil, err
	}

	return &marketanalyzerv1.GetExtremaResponse{Metadata: mapMetadata(input.Metadata), Candles: mapCandles(input.Candles),
		Settings: mapExtremaSettingsResponse(input.Settings), Result: result}, nil
}

func mapTrendResponse(input application.TrendResponse) (*marketanalyzerv1.GetTrendResponse, error) {
	extrema, err := mapExtremaResult(input.Result.Extrema, *input.Settings.Extrema, input.Candles)
	if err != nil {
		return nil, err
	}

	tolerance, err := domain.FormatNumber(input.Result.Tolerance)
	if err != nil {
		return nil, responseError(err)
	}

	state, err := mapTrendState(input.Result.State)
	if err != nil {
		return nil, err
	}

	return &marketanalyzerv1.GetTrendResponse{Metadata: mapMetadata(input.Metadata), Candles: mapCandles(input.Candles),
		Settings: mapTrendSettingsResponse(input.Settings), Result: &marketanalyzerv1.TrendResult{State: state, Reason: string(input.Result.Reason),
			Tolerance: tolerance, ReferenceClose: input.Candles[len(input.Candles)-1].Close, Extrema: extrema}}, nil
}

func mapLevelsResponse(input application.LevelsResponse) (*marketanalyzerv1.GetLevelsResponse, error) {
	extrema, err := mapExtremaResult(input.Result.Extrema, *input.Settings.Extrema, input.Candles)
	if err != nil {
		return nil, err
	}

	atrValue, err := domain.FormatNumber(input.Result.ATR.Value)
	if err != nil {
		return nil, responseError(err)
	}

	width, err := domain.FormatNumber(input.Result.MaximumZoneWidth)
	if err != nil {
		return nil, responseError(err)
	}

	zones := make([]*marketanalyzerv1.PriceZone, len(input.Result.Zones))
	for index, zone := range input.Result.Zones {
		zones[index], err = mapZone(zone)
		if err != nil {
			return nil, err
		}
	}

	result := &marketanalyzerv1.LevelsResult{Zones: zones, Extrema: extrema,
		Atr:              &marketanalyzerv1.ATRResult{Value: atrValue, CandleIndex: uint32(input.Result.ATR.CandleIndex), ValueTime: timestamppb.New(input.Result.ATR.ValueTime)},
		MaximumZoneWidth: width, ReferenceClose: input.Candles[len(input.Candles)-1].Close}
	return &marketanalyzerv1.GetLevelsResponse{Metadata: mapMetadata(input.Metadata), Candles: mapCandles(input.Candles),
		Settings: mapLevelSettingsResponse(input.Settings), Result: result}, nil
}

func mapMetadata(input application.Metadata) *marketanalyzerv1.Metadata {
	return &marketanalyzerv1.Metadata{Selection: mapSelectionResponse(input.Selection), EvaluatedAt: timestamppb.New(input.EvaluatedAt),
		SourceFrom: timestamppb.New(input.SourceFrom), SourceTo: timestamppb.New(input.SourceTo),
		AlgorithmIds: append([]string(nil), input.Algorithms...), NumericPolicy: input.NumericPolicy}
}

func mapSelectionResponse(input application.Selection) *marketanalyzerv1.Selection {
	return &marketanalyzerv1.Selection{Exchange: pointer(input.Exchange), Market: pointer(input.Market), Symbol: pointer(input.Symbol),
		To: timestamppb.New(input.To), CandleCount: pointer(input.CandleCount), Interval: pointer(input.Interval)}
}

func mapCandles(input []application.SourceCandle) []*marketanalyzerv1.Candle {
	result := make([]*marketanalyzerv1.Candle, len(input))
	for index, candle := range input {
		result[index] = &marketanalyzerv1.Candle{OpenTime: timestamppb.New(candle.OpenTime), CloseTime: timestamppb.New(candle.CloseTime),
			Open: candle.Open, High: candle.High, Low: candle.Low, Close: candle.Close, Volume: candle.Volume, Turnover: candle.Turnover,
			TradesCount: clonePointer(candle.TradesCount), FetchedAt: timestamppb.New(candle.FetchedAt)}
	}

	return result
}

func mapExtremaResult(input domain.ExtremaResult, settings application.ExtremaSettings, candles []application.SourceCandle) (*marketanalyzerv1.ExtremaResult, error) {
	points := make([]*marketanalyzerv1.Extremum, len(input.Points))
	for index, point := range input.Points {
		if int(point.CandleIndex) >= len(candles) || int(point.ConfirmationCandleIndex) >= len(candles) {
			return nil, responseError(fmt.Errorf("extremum %d has an invalid source reference", index))
		}

		kind, err := mapExtremumKind(point.Kind)
		if err != nil {
			return nil, err
		}

		mapped := &marketanalyzerv1.Extremum{Kind: kind, CandleIndex: uint32(point.CandleIndex), Time: timestamppb.New(point.Time),
			Price: sourcePrice(candles[point.CandleIndex], settings.PriceSource, point.Kind), ConfirmationCandleIndex: uint32(point.ConfirmationCandleIndex),
			ConfirmationTime: timestamppb.New(point.ConfirmationTime)}
		if point.Reversal != nil {
			threshold, err := domain.FormatNumber(point.Reversal.Threshold)
			if err != nil {
				return nil, responseError(err)
			}
			mapped.Reversal = &marketanalyzerv1.ReversalEvidence{Threshold: threshold,
				ConfirmationPrice: confirmationPrice(candles[point.ConfirmationCandleIndex], settings.PriceSource, point.Kind)}
			if point.Reversal.CandidateATR != nil {
				candidate, err := domain.FormatNumber(*point.Reversal.CandidateATR)
				if err != nil {
					return nil, responseError(err)
				}
				mapped.Reversal.CandidateAtr = &candidate
			}
		}
		points[index] = mapped
	}

	return &marketanalyzerv1.ExtremaResult{Points: points}, nil
}

func mapZone(input domain.PriceZone) (*marketanalyzerv1.PriceZone, error) {
	lower, err := domain.FormatNumber(input.LowerBound)
	if err != nil {
		return nil, responseError(err)
	}
	upper, err := domain.FormatNumber(input.UpperBound)
	if err != nil {
		return nil, responseError(err)
	}
	representative, err := domain.FormatNumber(input.RepresentativePrice)
	if err != nil {
		return nil, responseError(err)
	}
	role, err := mapZoneRole(input.Role)
	if err != nil {
		return nil, err
	}

	extremumIndices := make([]uint32, len(input.ExtremumIndices))
	for index, value := range input.ExtremumIndices {
		extremumIndices[index] = uint32(value)
	}
	acceptedIndices := make([]uint32, len(input.AcceptedCandleIndices))
	for index, value := range input.AcceptedCandleIndices {
		acceptedIndices[index] = uint32(value)
	}

	return &marketanalyzerv1.PriceZone{LowerBound: lower, UpperBound: upper, RepresentativePrice: representative, Role: role,
		ExtremumIndices: extremumIndices, AcceptedCandleIndices: acceptedIndices, TouchCount: input.TouchCount,
		FirstTouchTime: timestamppb.New(input.FirstTouchTime), LastTouchTime: timestamppb.New(input.LastTouchTime)}, nil
}

func sourcePrice(candle application.SourceCandle, source domain.PriceSource, kind domain.ExtremumKind) string {
	if source == domain.Close {
		return candle.Close
	}
	if kind == domain.High {
		return candle.High
	}
	return candle.Low
}

func confirmationPrice(candle application.SourceCandle, source domain.PriceSource, kind domain.ExtremumKind) string {
	if source == domain.Close {
		return candle.Close
	}
	if kind == domain.High {
		return candle.Low
	}
	return candle.High
}

func mapATRSettingsResponse(input application.ATRSettings) *marketanalyzerv1.ATRSettings {
	return &marketanalyzerv1.ATRSettings{Period: pointer(input.Period)}
}

func mapExtremaSettingsResponse(input application.ExtremaSettings) *marketanalyzerv1.ExtremaSettings {
	result := &marketanalyzerv1.ExtremaSettings{}
	source := marketanalyzerv1.PriceSource_PRICE_SOURCE_CLOSE
	if input.PriceSource == domain.HighLow {
		source = marketanalyzerv1.PriceSource_PRICE_SOURCE_HIGH_LOW
	}
	result.PriceSource = &source
	switch method := input.Method.(type) {
	case application.LocalExtremaSettings:
		result.Method = &marketanalyzerv1.ExtremaSettings_LocalExtrema{LocalExtrema: &marketanalyzerv1.LocalExtremaSettings{PivotSpan: pointer(method.PivotSpan)}}
	case application.PercentReversalSettings:
		result.Method = &marketanalyzerv1.ExtremaSettings_ReversalPercent{ReversalPercent: &marketanalyzerv1.PercentReversalSettings{ReversalPct: pointer(method.ReversalPct)}}
	case application.ATRReversalSettings:
		result.Method = &marketanalyzerv1.ExtremaSettings_ReversalAtr{ReversalAtr: &marketanalyzerv1.ATRReversalSettings{
			AtrPeriod: pointer(method.ATRPeriod), AtrMultiplier: pointer(method.ATRMultiplier)}}
	}
	return result
}

func mapTrendSettingsResponse(input application.TrendSettings) *marketanalyzerv1.TrendSettings {
	return &marketanalyzerv1.TrendSettings{Extrema: mapExtremaSettingsResponse(*input.Extrema), EqualityTolerancePct: pointer(input.EqualityTolerancePct)}
}

func mapLevelSettingsResponse(input application.LevelSettings) *marketanalyzerv1.LevelSettings {
	return &marketanalyzerv1.LevelSettings{Extrema: mapExtremaSettingsResponse(*input.Extrema), AtrPeriod: pointer(input.ATRPeriod),
		ZoneWidthAtr: pointer(input.ZoneWidthATR), MinTouches: pointer(input.MinTouches), MinTouchSeparationBars: pointer(input.MinTouchSeparationBars)}
}

func mapExtremumKind(input domain.ExtremumKind) (marketanalyzerv1.ExtremumKind, error) {
	switch input {
	case domain.High:
		return marketanalyzerv1.ExtremumKind_EXTREMUM_KIND_HIGH, nil
	case domain.Low:
		return marketanalyzerv1.ExtremumKind_EXTREMUM_KIND_LOW, nil
	default:
		return 0, responseError(fmt.Errorf("unknown extremum kind %q", input))
	}
}

func mapTrendState(input domain.TrendState) (marketanalyzerv1.TrendState, error) {
	switch input {
	case domain.Up:
		return marketanalyzerv1.TrendState_TREND_STATE_UP, nil
	case domain.Down:
		return marketanalyzerv1.TrendState_TREND_STATE_DOWN, nil
	case domain.Sideways:
		return marketanalyzerv1.TrendState_TREND_STATE_SIDEWAYS, nil
	case domain.Undetermined:
		return marketanalyzerv1.TrendState_TREND_STATE_UNDETERMINED, nil
	default:
		return 0, responseError(fmt.Errorf("unknown trend state %q", input))
	}
}

func mapZoneRole(input domain.ZoneRole) (marketanalyzerv1.ZoneRole, error) {
	switch input {
	case domain.Support:
		return marketanalyzerv1.ZoneRole_ZONE_ROLE_SUPPORT, nil
	case domain.Resistance:
		return marketanalyzerv1.ZoneRole_ZONE_ROLE_RESISTANCE, nil
	case domain.AtPrice:
		return marketanalyzerv1.ZoneRole_ZONE_ROLE_AT_PRICE, nil
	default:
		return 0, responseError(fmt.Errorf("unknown zone role %q", input))
	}
}

func responseError(err error) *application.Error {
	return &application.Error{Kind: application.InternalError, Err: err}
}

func pointer[T any](value T) *T { return &value }

func clonePointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	return pointer(*value)
}
