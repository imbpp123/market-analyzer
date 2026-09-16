package grpc

import (
	"errors"
	"time"

	marketanalyzerv1 "github.com/imbpp123/market-analyzer/api/go/marketanalyzer/v1"
	"github.com/imbpp123/market-analyzer/internal/application"
	"github.com/imbpp123/market-analyzer/internal/domain"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func mapATRRequest(request *marketanalyzerv1.GetATRRequest) (application.ATRRequest, error) {
	if request == nil {
		return application.ATRRequest{}, invalid("request", "request is required")
	}

	selection, err := mapSelection(request.Selection)
	if err != nil {
		return application.ATRRequest{}, err
	}

	settings, err := mapATRSettings(request.Settings)
	if err != nil {
		return application.ATRRequest{}, err
	}

	return application.ATRRequest{Selection: selection, Settings: settings}, nil
}

func mapNATRRequest(request *marketanalyzerv1.GetNATRRequest) (application.NATRRequest, error) {
	if request == nil {
		return application.NATRRequest{}, invalid("request", "request is required")
	}

	selection, err := mapSelection(request.Selection)
	if err != nil {
		return application.NATRRequest{}, err
	}

	settings, err := mapATRSettings(request.Settings)
	if err != nil {
		return application.NATRRequest{}, err
	}

	return application.NATRRequest{Selection: selection, Settings: settings}, nil
}

func mapExtremaRequest(request *marketanalyzerv1.GetExtremaRequest) (application.ExtremaRequest, error) {
	if request == nil {
		return application.ExtremaRequest{}, invalid("request", "request is required")
	}

	selection, err := mapSelection(request.Selection)
	if err != nil {
		return application.ExtremaRequest{}, err
	}

	settings, err := mapExtremaSettings(request.Settings)
	if err != nil {
		return application.ExtremaRequest{}, err
	}

	return application.ExtremaRequest{Selection: selection, Settings: settings}, nil
}

func mapTrendRequest(request *marketanalyzerv1.GetTrendRequest) (application.TrendRequest, error) {
	if request == nil {
		return application.TrendRequest{}, invalid("request", "request is required")
	}

	selection, err := mapSelection(request.Selection)
	if err != nil {
		return application.TrendRequest{}, err
	}

	if request.Settings == nil {
		return application.TrendRequest{}, invalid("settings", "settings are required")
	}

	extrema, err := mapExtremaSettings(request.Settings.Extrema)
	if err != nil {
		return application.TrendRequest{}, err
	}

	if request.Settings.EqualityTolerancePct == nil {
		return application.TrendRequest{}, invalid("settings.equality_tolerance_pct", "field is required")
	}

	settings := &application.TrendSettings{Extrema: extrema, EqualityTolerancePct: *request.Settings.EqualityTolerancePct}
	return application.TrendRequest{Selection: selection, Settings: settings}, nil
}

func mapLevelsRequest(request *marketanalyzerv1.GetLevelsRequest) (application.LevelsRequest, error) {
	if request == nil {
		return application.LevelsRequest{}, invalid("request", "request is required")
	}

	selection, err := mapSelection(request.Selection)
	if err != nil {
		return application.LevelsRequest{}, err
	}

	if request.Settings == nil {
		return application.LevelsRequest{}, invalid("settings", "settings are required")
	}

	extrema, err := mapExtremaSettings(request.Settings.Extrema)
	if err != nil {
		return application.LevelsRequest{}, err
	}

	if request.Settings.AtrPeriod == nil || request.Settings.ZoneWidthAtr == nil || request.Settings.MinTouches == nil || request.Settings.MinTouchSeparationBars == nil {
		return application.LevelsRequest{}, invalid("settings", "all level scalar fields are required")
	}

	settings := &application.LevelSettings{Extrema: extrema, ATRPeriod: *request.Settings.AtrPeriod, ZoneWidthATR: *request.Settings.ZoneWidthAtr,
		MinTouches: *request.Settings.MinTouches, MinTouchSeparationBars: *request.Settings.MinTouchSeparationBars}
	return application.LevelsRequest{Selection: selection, Settings: settings}, nil
}

func mapSelection(input *marketanalyzerv1.Selection) (*application.Selection, error) {
	if input == nil {
		return nil, invalid("selection", "selection is required")
	}

	if input.Exchange == nil || input.Market == nil || input.Symbol == nil || input.CandleCount == nil || input.Interval == nil {
		return nil, invalid("selection", "all selection scalar fields are required")
	}

	to, err := mapTimestamp("selection.to", input.To)
	if err != nil {
		return nil, err
	}

	return &application.Selection{Exchange: *input.Exchange, Market: *input.Market, Symbol: *input.Symbol, To: to,
		CandleCount: *input.CandleCount, Interval: *input.Interval}, nil
}

func mapATRSettings(input *marketanalyzerv1.ATRSettings) (*application.ATRSettings, error) {
	if input == nil || input.Period == nil {
		return nil, invalid("settings.period", "field is required")
	}

	return &application.ATRSettings{Period: *input.Period}, nil
}

func mapExtremaSettings(input *marketanalyzerv1.ExtremaSettings) (*application.ExtremaSettings, error) {
	if input == nil {
		return nil, invalid("settings.extrema", "extrema settings are required")
	}

	if input.PriceSource == nil {
		return nil, invalid("settings.extrema.price_source", "field is required")
	}

	var source domain.PriceSource
	switch *input.PriceSource {
	case marketanalyzerv1.PriceSource_PRICE_SOURCE_CLOSE:
		source = domain.Close
	case marketanalyzerv1.PriceSource_PRICE_SOURCE_HIGH_LOW:
		source = domain.HighLow
	default:
		return nil, invalid("settings.extrema.price_source", "unsupported value")
	}

	settings := &application.ExtremaSettings{PriceSource: source}
	switch method := input.Method.(type) {
	case *marketanalyzerv1.ExtremaSettings_LocalExtrema:
		if method.LocalExtrema == nil || method.LocalExtrema.PivotSpan == nil {
			return nil, invalid("settings.extrema.local_extrema.pivot_span", "field is required")
		}
		settings.Method = application.LocalExtremaSettings{PivotSpan: *method.LocalExtrema.PivotSpan}
	case *marketanalyzerv1.ExtremaSettings_ReversalPercent:
		if method.ReversalPercent == nil || method.ReversalPercent.ReversalPct == nil {
			return nil, invalid("settings.extrema.reversal_percent.reversal_pct", "field is required")
		}
		settings.Method = application.PercentReversalSettings{ReversalPct: *method.ReversalPercent.ReversalPct}
	case *marketanalyzerv1.ExtremaSettings_ReversalAtr:
		if method.ReversalAtr == nil || method.ReversalAtr.AtrPeriod == nil || method.ReversalAtr.AtrMultiplier == nil {
			return nil, invalid("settings.extrema.reversal_atr", "all fields are required")
		}
		settings.Method = application.ATRReversalSettings{ATRPeriod: *method.ReversalAtr.AtrPeriod, ATRMultiplier: *method.ReversalAtr.AtrMultiplier}
	default:
		return nil, invalid("settings.extrema.method", "one method is required")
	}

	return settings, nil
}

func mapTimestamp(field string, value *timestamppb.Timestamp) (time.Time, error) {
	if value == nil {
		return time.Time{}, invalid(field, "field is required")
	}

	if err := value.CheckValid(); err != nil {
		return time.Time{}, invalid(field, "invalid timestamp")
	}

	return value.AsTime(), nil
}

func invalid(field, message string) *application.Error {
	return &application.Error{Kind: application.InvalidParameter, Field: field, Err: errors.New(message)}
}
