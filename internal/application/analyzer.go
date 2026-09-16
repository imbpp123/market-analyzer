package application

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/imbpp123/market-analyzer/internal/domain"
	"github.com/shopspring/decimal"
)

var plainDecimal = regexp.MustCompile(`^[+-]?[0-9]+(?:\.[0-9]+)?$`)

type Analyzer struct {
	reader   CandleReader
	clock    Clock
	timeout  time.Duration
	observer CalculationObserver
}

func NewAnalyzer(reader CandleReader, clock Clock, timeout time.Duration) (*Analyzer, error) {
	return NewAnalyzerWithObserver(reader, clock, timeout, nil)
}

func NewAnalyzerWithObserver(reader CandleReader, clock Clock, timeout time.Duration, observer CalculationObserver) (*Analyzer, error) {
	if reader == nil {
		return nil, &Error{Kind: InvalidParameter, Field: "reader", Err: errors.New("reader is required")}
	}

	if clock == nil {
		return nil, &Error{Kind: InvalidParameter, Field: "clock", Err: errors.New("clock is required")}
	}

	if timeout <= 0 {
		return nil, &Error{Kind: InvalidParameter, Field: "timeout", Err: errors.New("timeout must be positive")}
	}

	return &Analyzer{reader: reader, clock: clock, timeout: timeout, observer: observer}, nil
}

func (a *Analyzer) GetATR(ctx context.Context, request ATRRequest) (ATRResponse, error) {
	if request.Settings == nil {
		return ATRResponse{}, invalidRequest("settings", errors.New("settings are required"))
	}

	settings := domain.ATRSettings{Period: request.Settings.Period}
	prepared, err := a.prepare(ctx, request.Selection, func(count uint32) error { return settings.Validate(count) })
	if err != nil {
		return ATRResponse{}, err
	}
	defer prepared.cancel()

	started := time.Now()
	result, err := domain.CalculateATR(prepared.ctx, prepared.series, settings)
	a.observeCalculation("atr", time.Since(started))
	if err != nil {
		return ATRResponse{}, calculationError(err)
	}

	return ATRResponse{Metadata: prepared.metadata([]string{domain.WilderATRVersion}), Candles: prepared.source, Settings: *request.Settings, Result: result}, nil
}

func (a *Analyzer) GetNATR(ctx context.Context, request NATRRequest) (NATRResponse, error) {
	if request.Settings == nil {
		return NATRResponse{}, invalidRequest("settings", errors.New("settings are required"))
	}

	settings := domain.ATRSettings{Period: request.Settings.Period}
	prepared, err := a.prepare(ctx, request.Selection, func(count uint32) error { return settings.Validate(count) })
	if err != nil {
		return NATRResponse{}, err
	}
	defer prepared.cancel()

	started := time.Now()
	result, err := domain.CalculateNATR(prepared.ctx, prepared.series, settings)
	a.observeCalculation("natr", time.Since(started))
	if err != nil {
		return NATRResponse{}, calculationError(err)
	}

	return NATRResponse{Metadata: prepared.metadata([]string{domain.WilderATRVersion, domain.NATRVersion}), Candles: prepared.source, Settings: *request.Settings, Result: result}, nil
}

func (a *Analyzer) GetExtrema(ctx context.Context, request ExtremaRequest) (ExtremaResponse, error) {
	settings, err := parseExtremaSettings(request.Settings)
	if err != nil {
		return ExtremaResponse{}, err
	}

	prepared, err := a.prepare(ctx, request.Selection, func(count uint32) error { return settings.domain.Validate(count) })
	if err != nil {
		return ExtremaResponse{}, err
	}
	defer prepared.cancel()

	started := time.Now()
	result, err := domain.DetectExtrema(prepared.ctx, prepared.series, settings.domain)
	a.observeCalculation("extrema", time.Since(started))
	if err != nil {
		return ExtremaResponse{}, calculationError(err)
	}

	return ExtremaResponse{Metadata: prepared.metadata(extremaAlgorithms(settings.domain)), Candles: prepared.source, Settings: settings.application, Result: result}, nil
}

func (a *Analyzer) GetTrend(ctx context.Context, request TrendRequest) (TrendResponse, error) {
	settings, err := parseTrendSettings(request.Settings)
	if err != nil {
		return TrendResponse{}, err
	}

	prepared, err := a.prepare(ctx, request.Selection, func(count uint32) error { return settings.domain.Validate(count) })
	if err != nil {
		return TrendResponse{}, err
	}
	defer prepared.cancel()

	started := time.Now()
	result, err := domain.CalculateTrend(prepared.ctx, prepared.series, settings.domain)
	a.observeCalculation("trend", time.Since(started))
	if err != nil {
		return TrendResponse{}, calculationError(err)
	}

	algorithms := append(extremaAlgorithms(settings.domain.Extrema), domain.TrendVersion)
	return TrendResponse{Metadata: prepared.metadata(algorithms), Candles: prepared.source, Settings: settings.application, Result: result}, nil
}

func (a *Analyzer) GetLevels(ctx context.Context, request LevelsRequest) (LevelsResponse, error) {
	settings, err := parseLevelSettings(request.Settings)
	if err != nil {
		return LevelsResponse{}, err
	}

	prepared, err := a.prepare(ctx, request.Selection, func(count uint32) error { return settings.domain.Validate(count) })
	if err != nil {
		return LevelsResponse{}, err
	}
	defer prepared.cancel()

	started := time.Now()
	result, err := domain.CalculateLevels(prepared.ctx, prepared.series, settings.domain)
	a.observeCalculation("levels", time.Since(started))
	if err != nil {
		return LevelsResponse{}, calculationError(err)
	}

	algorithms := uniqueAlgorithms(append(extremaAlgorithms(settings.domain.Extrema), domain.WilderATRVersion, domain.ZonesVersion))
	return LevelsResponse{Metadata: prepared.metadata(algorithms), Candles: prepared.source, Settings: settings.application, Result: result}, nil
}

func (a *Analyzer) observeCalculation(name string, duration time.Duration) {
	if a.observer != nil {
		a.observer.ObserveCalculation(name, duration)
	}
}

type preparedAnalysis struct {
	ctx         context.Context
	cancel      context.CancelFunc
	selection   Selection
	evaluatedAt time.Time
	series      domain.CandleSeries
	source      []SourceCandle
}

func (p preparedAnalysis) metadata(algorithms []string) Metadata {
	rangeValue := p.series.Range()
	return Metadata{Selection: p.selection, EvaluatedAt: p.evaluatedAt, SourceFrom: rangeValue.From, SourceTo: rangeValue.To,
		Algorithms: append([]string(nil), algorithms...), NumericPolicy: domain.NumericPolicy}
}

func (a *Analyzer) prepare(ctx context.Context, input *Selection, validateSettings func(uint32) error) (preparedAnalysis, error) {
	evaluatedAt := a.clock.Now().UTC()
	requestCtx, cancel := context.WithTimeout(ctx, a.timeout)
	fail := func(err error) (preparedAnalysis, error) {
		cancel()
		return preparedAnalysis{}, err
	}

	selection, domainSelection, err := parseSelection(input)
	if err != nil {
		return fail(err)
	}

	if err := validateSettings(selection.CandleCount); err != nil {
		return fail(domainValidationError(err, InvalidParameter))
	}

	planned, err := domainSelection.Range()
	if err != nil {
		return fail(domainValidationError(err, InvalidParameter))
	}

	closedBoundary, err := domainSelection.Interval.Floor(evaluatedAt)
	if err != nil {
		return fail(domainValidationError(err, InvalidParameter))
	}

	if planned.To.After(closedBoundary) {
		return fail(invalidRequest("selection.to", errors.New("selected range ends after the current closed-candle boundary")))
	}

	loaded, err := a.reader.ReadCandles(requestCtx, domainSelection.Instrument, domainSelection.Interval, planned)
	if err != nil {
		return fail(normalizeError(err))
	}

	source, candles, err := parseSource(requestCtx, loaded)
	if err != nil {
		return fail(err)
	}

	series, err := domain.NewCandleSeries(requestCtx, domainSelection, loaded.Instrument, loaded.Interval, loaded.Range, candles)
	if err != nil {
		return fail(domainValidationError(err, InvalidMarketData))
	}

	return preparedAnalysis{ctx: requestCtx, cancel: cancel, selection: selection, evaluatedAt: evaluatedAt, series: series, source: source}, nil
}

func parseSelection(input *Selection) (Selection, domain.CandleSelection, error) {
	if input == nil {
		return Selection{}, domain.CandleSelection{}, invalidRequest("selection", errors.New("selection is required"))
	}

	selection := *input
	domainSelection := domain.CandleSelection{Instrument: domain.Instrument{Exchange: input.Exchange, Market: input.Market, Symbol: input.Symbol},
		Interval: domain.Interval(input.Interval), To: input.To, CandleCount: input.CandleCount}
	if _, err := domainSelection.Range(); err != nil {
		return Selection{}, domain.CandleSelection{}, domainValidationError(err, InvalidParameter)
	}

	return selection, domainSelection, nil
}

func parseExtremaSettings(input *ExtremaSettings) (parsedExtremaSettings, error) {
	if input == nil {
		return parsedExtremaSettings{}, invalidRequest("settings", errors.New("settings are required"))
	}

	result := parsedExtremaSettings{application: *input}
	result.domain.PriceSource = input.PriceSource
	switch method := input.Method.(type) {
	case LocalExtremaSettings:
		result.domain.Method = domain.LocalExtremaSettings{PivotSpan: method.PivotSpan}
	case PercentReversalSettings:
		value, err := parseDecimal("settings.reversal_percent.reversal_pct", method.ReversalPct)
		if err != nil {
			return parsedExtremaSettings{}, err
		}
		result.domain.Method = domain.PercentReversalSettings{ReversalPct: value}
	case ATRReversalSettings:
		value, err := parseDecimal("settings.reversal_atr.atr_multiplier", method.ATRMultiplier)
		if err != nil {
			return parsedExtremaSettings{}, err
		}
		result.domain.Method = domain.ATRReversalSettings{ATRPeriod: method.ATRPeriod, ATRMultiplier: value}
	default:
		return parsedExtremaSettings{}, invalidRequest("settings.method", errors.New("one extrema method is required"))
	}

	return result, nil
}

func parseTrendSettings(input *TrendSettings) (parsedTrendSettings, error) {
	if input == nil {
		return parsedTrendSettings{}, invalidRequest("settings", errors.New("settings are required"))
	}

	extrema, err := parseExtremaSettings(input.Extrema)
	if err != nil {
		return parsedTrendSettings{}, err
	}

	tolerance, err := parseDecimal("settings.equality_tolerance_pct", input.EqualityTolerancePct)
	if err != nil {
		return parsedTrendSettings{}, err
	}

	applicationSettings := TrendSettings{Extrema: &extrema.application, EqualityTolerancePct: input.EqualityTolerancePct}
	return parsedTrendSettings{application: applicationSettings, domain: domain.TrendSettings{Extrema: extrema.domain, EqualityTolerancePct: tolerance}}, nil
}

func parseLevelSettings(input *LevelSettings) (parsedLevelSettings, error) {
	if input == nil {
		return parsedLevelSettings{}, invalidRequest("settings", errors.New("settings are required"))
	}

	extrema, err := parseExtremaSettings(input.Extrema)
	if err != nil {
		return parsedLevelSettings{}, err
	}

	width, err := parseDecimal("settings.zone_width_atr", input.ZoneWidthATR)
	if err != nil {
		return parsedLevelSettings{}, err
	}

	settings := domain.LevelSettings{Extrema: extrema.domain, ATRPeriod: input.ATRPeriod, ZoneWidthATR: width,
		MinTouches: input.MinTouches, MinTouchSeparationBars: input.MinTouchSeparationBars}
	applicationSettings := LevelSettings{Extrema: &extrema.application, ATRPeriod: input.ATRPeriod, ZoneWidthATR: input.ZoneWidthATR,
		MinTouches: input.MinTouches, MinTouchSeparationBars: input.MinTouchSeparationBars}
	return parsedLevelSettings{application: applicationSettings, domain: settings}, nil
}

func parseDecimal(field, value string) (decimal.Decimal, error) {
	if len(value) == 0 || len(value) > 1024 || !plainDecimal.MatchString(value) {
		return decimal.Zero, invalidRequest(field, errors.New("must be plain base-10 text with at most 1024 characters"))
	}

	parsed, err := decimal.NewFromString(value)
	if err != nil {
		return decimal.Zero, invalidRequest(field, errors.New("must be a valid decimal"))
	}

	return parsed, nil
}

func extremaAlgorithms(settings domain.ExtremaSettings) []string {
	switch settings.Method.(type) {
	case domain.LocalExtremaSettings:
		return []string{domain.LocalExtremaVersion}
	case domain.PercentReversalSettings:
		return []string{domain.PercentReversalVersion}
	case domain.ATRReversalSettings:
		return []string{domain.WilderATRVersion, domain.ATRReversalVersion}
	default:
		return nil
	}
}

func uniqueAlgorithms(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}

	return result
}

func invalidRequest(field string, err error) *Error {
	return &Error{Kind: InvalidParameter, Field: field, Err: err}
}

func domainValidationError(err error, kind ErrorKind) error {
	var validation *domain.ValidationError
	if errors.As(err, &validation) {
		return &Error{Kind: kind, Field: validation.Field, Err: err}
	}

	return calculationError(err)
}

func normalizeError(err error) error {
	var applicationError *Error
	if errors.As(err, &applicationError) {
		return applicationError
	}

	return calculationError(err)
}

func calculationError(err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return &Error{Kind: RequestCanceled, Err: err}
	case errors.Is(err, context.DeadlineExceeded):
		return &Error{Kind: RequestTimeout, Err: err}
	default:
		return &Error{Kind: InternalError, Err: err}
	}
}
