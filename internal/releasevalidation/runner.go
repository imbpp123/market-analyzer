package releasevalidation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	marketanalyzerv1 "github.com/imbpp123/market-analyzer/api/go/marketanalyzer/v1"
	"github.com/shopspring/decimal"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const reportSchemaVersion = "market-analyzer-release-validation/v1"

type Report struct {
	SchemaVersion string       `json:"schema_version"`
	StartedAt     time.Time    `json:"started_at"`
	CompletedAt   time.Time    `json:"completed_at"`
	Configuration ReportConfig `json:"configuration"`
	Checks        []Check      `json:"checks"`
	Summary       Summary      `json:"summary"`
}

type ReportConfig struct {
	AnalyzerEndpoint        string          `json:"analyzer_endpoint"`
	Timeout                 string          `json:"timeout"`
	MaxResponseBytes        int             `json:"max_response_bytes"`
	BuildRevision           string          `json:"build_revision"`
	UpstreamContractVersion string          `json:"upstream_contract_version"`
	Selection               Selection       `json:"selection"`
	Settings                Settings        `json:"settings"`
	RuntimeSettings         RuntimeSettings `json:"runtime_settings"`
}

type Check struct {
	Name          string          `json:"name"`
	RPC           string          `json:"rpc"`
	PriceSource   string          `json:"price_source,omitempty"`
	ExtremaMethod string          `json:"extrema_method,omitempty"`
	StartedAt     time.Time       `json:"started_at"`
	Duration      string          `json:"duration"`
	Status        string          `json:"status"`
	ResponseBytes int             `json:"response_bytes,omitempty"`
	Request       json.RawMessage `json:"request"`
	Response      json.RawMessage `json:"response,omitempty"`
	Error         string          `json:"error,omitempty"`
}

type Summary struct {
	Total         int `json:"total"`
	Passed        int `json:"passed"`
	Failed        int `json:"failed"`
	ResponseBytes int `json:"response_bytes"`
}

type resultView struct {
	message  proto.Message
	metadata *marketanalyzerv1.Metadata
	candles  []*marketanalyzerv1.Candle
	extrema  *marketanalyzerv1.ExtremaResult
	zones    []*marketanalyzerv1.PriceZone
	method   string
	source   marketanalyzerv1.PriceSource
	minTouch uint32
}

type validationCase struct {
	name          string
	rpc           string
	priceSource   string
	extremaMethod string
	request       proto.Message
	call          func(context.Context) (resultView, error)
}

func Run(ctx context.Context, client marketanalyzerv1.MarketAnalyzerServiceClient, config Config, now func() time.Time) (Report, error) {
	if client == nil {
		return Report{}, errors.New("analyzer client is required")
	}
	if now == nil {
		return Report{}, errors.New("clock is required")
	}
	if err := config.Validate(); err != nil {
		return Report{}, fmt.Errorf("validate config: %w", err)
	}

	report := Report{
		SchemaVersion: reportSchemaVersion,
		StartedAt:     now().UTC(),
		Configuration: ReportConfig{
			AnalyzerEndpoint: config.AnalyzerEndpoint, Timeout: config.Timeout.String(), MaxResponseBytes: config.MaxResponseBytes,
			BuildRevision: config.BuildRevision, UpstreamContractVersion: config.UpstreamContractVersion,
			Selection: config.Selection, Settings: config.Settings, RuntimeSettings: config.RuntimeSettings,
		},
	}

	cases := validationCases(client, config)
	report.Checks = make([]Check, 0, len(cases))
	for _, testCase := range cases {
		check := Check{Name: testCase.name, RPC: testCase.rpc, PriceSource: testCase.priceSource,
			ExtremaMethod: testCase.extremaMethod, StartedAt: now().UTC()}
		requestJSON, err := marshalProto(testCase.request)
		if err != nil {
			return Report{}, fmt.Errorf("marshal %s request: %w", testCase.name, err)
		}
		check.Request = requestJSON

		callCtx, cancel := context.WithTimeout(ctx, config.Timeout)
		started := time.Now()
		view, callErr := testCase.call(callCtx)
		check.Duration = time.Since(started).String()
		cancel()
		if callErr == nil {
			callErr = validateResult(config.Selection, view)
		}

		if callErr != nil {
			check.Status = status.Code(callErr).String()
			check.Error = callErr.Error()
			report.Summary.Failed++
		} else {
			check.Status = "OK"
			check.ResponseBytes = proto.Size(view.message)
			check.Response, err = marshalProto(view.message)
			if err != nil {
				return Report{}, fmt.Errorf("marshal %s response: %w", testCase.name, err)
			}
			report.Summary.Passed++
			report.Summary.ResponseBytes += check.ResponseBytes
		}
		report.Checks = append(report.Checks, check)
	}

	report.CompletedAt = now().UTC()
	report.Summary.Total = len(report.Checks)
	if report.Summary.Failed > 0 {
		return report, fmt.Errorf("%d of %d validation checks failed", report.Summary.Failed, report.Summary.Total)
	}

	return report, nil
}

func validationCases(client marketanalyzerv1.MarketAnalyzerServiceClient, config Config) []validationCase {
	selection := func() *marketanalyzerv1.Selection {
		return &marketanalyzerv1.Selection{
			Exchange: pointer(config.Selection.Exchange), Market: pointer(config.Selection.Market), Symbol: pointer(config.Selection.Symbol),
			To: timestamppb.New(config.Selection.To), CandleCount: pointer(config.Selection.CandleCount), Interval: pointer(config.Selection.Interval),
		}
	}

	atrRequest := &marketanalyzerv1.GetATRRequest{Selection: selection(), Settings: &marketanalyzerv1.ATRSettings{Period: pointer(config.Settings.ATRPeriod)}}
	natrRequest := &marketanalyzerv1.GetNATRRequest{Selection: selection(), Settings: &marketanalyzerv1.ATRSettings{Period: pointer(config.Settings.ATRPeriod)}}
	cases := []validationCase{
		{name: "atr", rpc: "GetATR", request: atrRequest, call: func(ctx context.Context) (resultView, error) {
			response, err := client.GetATR(ctx, atrRequest)
			if err != nil {
				return resultView{}, err
			}
			if response.Result == nil {
				return resultView{}, errors.New("ATR result is missing")
			}
			return resultView{message: response, metadata: response.Metadata, candles: response.Candles}, validateResultReference(response.Result.CandleIndex, response.Result.ValueTime, response.Candles)
		}},
		{name: "natr", rpc: "GetNATR", request: natrRequest, call: func(ctx context.Context) (resultView, error) {
			response, err := client.GetNATR(ctx, natrRequest)
			if err != nil {
				return resultView{}, err
			}
			if response.Result == nil {
				return resultView{}, errors.New("NATR result is missing")
			}
			return resultView{message: response, metadata: response.Metadata, candles: response.Candles}, validateResultReference(response.Result.CandleIndex, response.Result.ValueTime, response.Candles)
		}},
	}

	for _, source := range []marketanalyzerv1.PriceSource{
		marketanalyzerv1.PriceSource_PRICE_SOURCE_CLOSE,
		marketanalyzerv1.PriceSource_PRICE_SOURCE_HIGH_LOW,
	} {
		for _, method := range []string{"local", "percent", "atr"} {
			extremaSettings := extremaSettings(config.Settings, source, method)
			for _, rpc := range []string{"GetExtrema", "GetTrend", "GetLevels"} {
				cases = append(cases, analysisCase(client, config, selection(), extremaSettings, source, method, rpc))
			}
		}
	}

	return cases
}

func analysisCase(client marketanalyzerv1.MarketAnalyzerServiceClient, config Config, selection *marketanalyzerv1.Selection,
	extrema *marketanalyzerv1.ExtremaSettings, source marketanalyzerv1.PriceSource, method, rpc string,
) validationCase {
	name := fmt.Sprintf("%s/%s/%s", rpc, sourceName(source), method)
	testCase := validationCase{name: name, rpc: rpc, priceSource: sourceName(source), extremaMethod: method}

	switch rpc {
	case "GetExtrema":
		request := &marketanalyzerv1.GetExtremaRequest{Selection: selection, Settings: extrema}
		testCase.request = request
		testCase.call = func(ctx context.Context) (resultView, error) {
			response, err := client.GetExtrema(ctx, request)
			if err != nil {
				return resultView{}, err
			}
			if response.Result == nil {
				return resultView{}, errors.New("extrema result is missing")
			}
			return resultView{message: response, metadata: response.Metadata, candles: response.Candles,
				extrema: response.Result, method: method, source: source}, nil
		}
	case "GetTrend":
		request := &marketanalyzerv1.GetTrendRequest{Selection: selection, Settings: &marketanalyzerv1.TrendSettings{
			Extrema: extrema, EqualityTolerancePct: pointer(config.Settings.EqualityTolerancePct),
		}}
		testCase.request = request
		testCase.call = func(ctx context.Context) (resultView, error) {
			response, err := client.GetTrend(ctx, request)
			if err != nil {
				return resultView{}, err
			}
			if response.Result == nil || response.Result.Extrema == nil {
				return resultView{}, errors.New("trend result or extrema evidence is missing")
			}
			return resultView{message: response, metadata: response.Metadata, candles: response.Candles,
				extrema: response.Result.Extrema, method: method, source: source}, nil
		}
	case "GetLevels":
		request := &marketanalyzerv1.GetLevelsRequest{Selection: selection, Settings: &marketanalyzerv1.LevelSettings{
			Extrema: extrema, AtrPeriod: pointer(config.Settings.LevelATRPeriod), ZoneWidthAtr: pointer(config.Settings.ZoneWidthATR),
			MinTouches: pointer(config.Settings.MinTouches), MinTouchSeparationBars: pointer(config.Settings.MinTouchSeparationBars),
		}}
		testCase.request = request
		testCase.call = func(ctx context.Context) (resultView, error) {
			response, err := client.GetLevels(ctx, request)
			if err != nil {
				return resultView{}, err
			}
			if response.Result == nil || response.Result.Extrema == nil || response.Result.Atr == nil {
				return resultView{}, errors.New("levels result, extrema evidence, or ATR evidence is missing")
			}
			if err := validateResultReference(response.Result.Atr.CandleIndex, response.Result.Atr.ValueTime, response.Candles); err != nil {
				return resultView{}, err
			}
			return resultView{message: response, metadata: response.Metadata, candles: response.Candles,
				extrema: response.Result.Extrema, zones: response.Result.Zones, method: method, source: source,
				minTouch: config.Settings.MinTouches}, nil
		}
	}

	return testCase
}

func extremaSettings(settings Settings, source marketanalyzerv1.PriceSource, method string) *marketanalyzerv1.ExtremaSettings {
	result := &marketanalyzerv1.ExtremaSettings{PriceSource: pointer(source)}
	switch method {
	case "local":
		result.Method = &marketanalyzerv1.ExtremaSettings_LocalExtrema{LocalExtrema: &marketanalyzerv1.LocalExtremaSettings{PivotSpan: pointer(settings.PivotSpan)}}
	case "percent":
		result.Method = &marketanalyzerv1.ExtremaSettings_ReversalPercent{ReversalPercent: &marketanalyzerv1.PercentReversalSettings{ReversalPct: pointer(settings.ReversalPct)}}
	case "atr":
		result.Method = &marketanalyzerv1.ExtremaSettings_ReversalAtr{ReversalAtr: &marketanalyzerv1.ATRReversalSettings{
			AtrPeriod: pointer(settings.ReversalATRPeriod), AtrMultiplier: pointer(settings.ReversalATRMultiplier),
		}}
	}

	return result
}

func validateResult(selection Selection, view resultView) error {
	if view.message == nil {
		return errors.New("response is missing")
	}
	if view.metadata == nil || view.metadata.Selection == nil {
		return errors.New("metadata or echoed selection is missing")
	}
	if view.metadata.NumericPolicy == "" || len(view.metadata.AlgorithmIds) == 0 {
		return errors.New("numeric policy or algorithm identifiers are missing")
	}

	echo := view.metadata.Selection
	if echo.GetExchange() != selection.Exchange || echo.GetMarket() != selection.Market || echo.GetSymbol() != selection.Symbol ||
		echo.GetCandleCount() != selection.CandleCount || echo.GetInterval() != selection.Interval ||
		echo.To == nil || !echo.To.AsTime().Equal(selection.To) {
		return errors.New("echoed selection does not match the request")
	}
	if len(view.candles) != int(selection.CandleCount) {
		return fmt.Errorf("source candle count is %d, want %d", len(view.candles), selection.CandleCount)
	}
	if len(view.candles) == 0 || view.metadata.SourceFrom == nil || view.metadata.SourceTo == nil || view.metadata.EvaluatedAt == nil {
		return errors.New("source boundaries or evaluated_at are missing")
	}
	if err := view.metadata.SourceFrom.CheckValid(); err != nil {
		return fmt.Errorf("invalid source_from: %w", err)
	}
	if err := view.metadata.SourceTo.CheckValid(); err != nil {
		return fmt.Errorf("invalid source_to: %w", err)
	}
	if view.metadata.SourceTo.AsTime().After(selection.To) {
		return errors.New("source_to is after the requested to")
	}

	for index, candle := range view.candles {
		if err := validateCandle(index, candle); err != nil {
			return err
		}
		if index > 0 && !view.candles[index-1].CloseTime.AsTime().Equal(candle.OpenTime.AsTime()) {
			return fmt.Errorf("candles[%d] is not contiguous", index)
		}
	}
	if !view.candles[0].OpenTime.AsTime().Equal(view.metadata.SourceFrom.AsTime()) ||
		!view.candles[len(view.candles)-1].CloseTime.AsTime().Equal(view.metadata.SourceTo.AsTime()) {
		return errors.New("source candle boundaries do not match metadata")
	}

	if view.extrema != nil {
		if err := validateExtrema(view); err != nil {
			return err
		}
	}
	return validateZones(view)
}

func validateCandle(index int, candle *marketanalyzerv1.Candle) error {
	if candle == nil || candle.OpenTime == nil || candle.CloseTime == nil || candle.FetchedAt == nil {
		return fmt.Errorf("candles[%d] or its timestamps are missing", index)
	}
	for name, value := range map[string]*timestamppb.Timestamp{"open_time": candle.OpenTime, "close_time": candle.CloseTime, "fetched_at": candle.FetchedAt} {
		if err := value.CheckValid(); err != nil {
			return fmt.Errorf("candles[%d].%s is invalid: %w", index, name, err)
		}
	}
	if !candle.OpenTime.AsTime().Before(candle.CloseTime.AsTime()) {
		return fmt.Errorf("candles[%d] has an invalid time range", index)
	}

	values := make(map[string]decimal.Decimal, 6)
	for name, text := range map[string]string{
		"open": candle.Open, "high": candle.High, "low": candle.Low, "close": candle.Close,
		"volume": candle.Volume, "turnover": candle.Turnover,
	} {
		value, err := decimal.NewFromString(text)
		if err != nil {
			return fmt.Errorf("candles[%d].%s is not a decimal", index, name)
		}
		values[name] = value
	}
	if !values["open"].IsPositive() || !values["high"].IsPositive() || !values["low"].IsPositive() || !values["close"].IsPositive() {
		return fmt.Errorf("candles[%d] contains a nonpositive price", index)
	}
	if values["low"].GreaterThan(values["open"]) || values["open"].GreaterThan(values["high"]) ||
		values["low"].GreaterThan(values["close"]) || values["close"].GreaterThan(values["high"]) {
		return fmt.Errorf("candles[%d] violates OHLC ordering", index)
	}
	if values["volume"].IsNegative() || values["turnover"].IsNegative() || candle.TradesCount != nil && *candle.TradesCount < 0 {
		return fmt.Errorf("candles[%d] contains a negative quantity", index)
	}

	return nil
}

func validateExtrema(view resultView) error {
	for index, point := range view.extrema.Points {
		if point == nil || int(point.CandleIndex) >= len(view.candles) || int(point.ConfirmationCandleIndex) >= len(view.candles) {
			return fmt.Errorf("extrema[%d] has an invalid source reference", index)
		}
		candle := view.candles[point.CandleIndex]
		confirmation := view.candles[point.ConfirmationCandleIndex]
		if point.Time == nil || !point.Time.AsTime().Equal(candle.OpenTime.AsTime()) || point.ConfirmationTime == nil ||
			!point.ConfirmationTime.AsTime().Equal(confirmation.CloseTime.AsTime()) {
			return fmt.Errorf("extrema[%d] has inconsistent evidence time", index)
		}
		if point.ConfirmationCandleIndex < point.CandleIndex {
			return fmt.Errorf("extrema[%d] is confirmed before its candidate candle", index)
		}
		switch point.Kind {
		case marketanalyzerv1.ExtremumKind_EXTREMUM_KIND_HIGH, marketanalyzerv1.ExtremumKind_EXTREMUM_KIND_LOW:
		default:
			return fmt.Errorf("extrema[%d] has an unknown kind", index)
		}
		expectedPrice := candle.Close
		if view.source == marketanalyzerv1.PriceSource_PRICE_SOURCE_HIGH_LOW {
			switch point.Kind {
			case marketanalyzerv1.ExtremumKind_EXTREMUM_KIND_HIGH:
				expectedPrice = candle.High
			case marketanalyzerv1.ExtremumKind_EXTREMUM_KIND_LOW:
				expectedPrice = candle.Low
			default:
				return fmt.Errorf("extrema[%d] has an unknown kind", index)
			}
		}
		if point.Price != expectedPrice {
			return fmt.Errorf("extrema[%d] price does not match its source candle", index)
		}
		if view.method == "local" && point.Reversal != nil {
			return fmt.Errorf("extrema[%d] has unexpected reversal evidence", index)
		}
		if view.method != "local" && point.Reversal == nil {
			return fmt.Errorf("extrema[%d] is missing reversal evidence", index)
		}
		if view.method == "atr" && point.Reversal != nil && point.Reversal.CandidateAtr == nil {
			return fmt.Errorf("extrema[%d] is missing candidate ATR evidence", index)
		}
		if point.Reversal != nil {
			if _, err := decimal.NewFromString(point.Reversal.Threshold); err != nil {
				return fmt.Errorf("extrema[%d] has an invalid reversal threshold", index)
			}
			confirmationPrice := confirmation.Close
			if view.source == marketanalyzerv1.PriceSource_PRICE_SOURCE_HIGH_LOW {
				if point.Kind == marketanalyzerv1.ExtremumKind_EXTREMUM_KIND_HIGH {
					confirmationPrice = confirmation.Low
				} else {
					confirmationPrice = confirmation.High
				}
			}
			if point.Reversal.ConfirmationPrice != confirmationPrice {
				return fmt.Errorf("extrema[%d] confirmation price does not match its source candle", index)
			}
			if point.Reversal.CandidateAtr != nil {
				if _, err := decimal.NewFromString(*point.Reversal.CandidateAtr); err != nil {
					return fmt.Errorf("extrema[%d] has an invalid candidate ATR", index)
				}
			}
		}
	}

	return nil
}

func validateZones(view resultView) error {
	for index, zone := range view.zones {
		if zone == nil || int(zone.TouchCount) != len(zone.AcceptedCandleIndices) || zone.TouchCount < view.minTouch {
			return fmt.Errorf("zones[%d] has an invalid touch count", index)
		}
		lower, lowerErr := decimal.NewFromString(zone.LowerBound)
		upper, upperErr := decimal.NewFromString(zone.UpperBound)
		representative, representativeErr := decimal.NewFromString(zone.RepresentativePrice)
		if lowerErr != nil || upperErr != nil || representativeErr != nil || lower.GreaterThan(upper) ||
			representative.LessThan(lower) || representative.GreaterThan(upper) {
			return fmt.Errorf("zones[%d] has invalid price bounds", index)
		}
		switch zone.Role {
		case marketanalyzerv1.ZoneRole_ZONE_ROLE_SUPPORT, marketanalyzerv1.ZoneRole_ZONE_ROLE_RESISTANCE,
			marketanalyzerv1.ZoneRole_ZONE_ROLE_AT_PRICE:
		default:
			return fmt.Errorf("zones[%d] has an unknown role", index)
		}
		for _, extremumIndex := range zone.ExtremumIndices {
			if view.extrema == nil || int(extremumIndex) >= len(view.extrema.Points) {
				return fmt.Errorf("zones[%d] has an invalid extremum reference", index)
			}
		}
		for _, candleIndex := range zone.AcceptedCandleIndices {
			if int(candleIndex) >= len(view.candles) {
				return fmt.Errorf("zones[%d] has an invalid candle reference", index)
			}
		}
		if len(zone.AcceptedCandleIndices) > 0 {
			first := view.candles[zone.AcceptedCandleIndices[0]].OpenTime.AsTime()
			last := view.candles[zone.AcceptedCandleIndices[len(zone.AcceptedCandleIndices)-1]].OpenTime.AsTime()
			if zone.FirstTouchTime == nil || zone.LastTouchTime == nil || !zone.FirstTouchTime.AsTime().Equal(first) || !zone.LastTouchTime.AsTime().Equal(last) {
				return fmt.Errorf("zones[%d] has inconsistent touch times", index)
			}
		}
	}

	return nil
}

func validateResultReference(index uint32, valueTime *timestamppb.Timestamp, candles []*marketanalyzerv1.Candle) error {
	if int(index) >= len(candles) || valueTime == nil || candles[index] == nil || candles[index].CloseTime == nil ||
		!valueTime.AsTime().Equal(candles[index].CloseTime.AsTime()) {
		return errors.New("result has an invalid source reference")
	}
	return nil
}

func marshalProto(message proto.Message) (json.RawMessage, error) {
	data, err := (protojson.MarshalOptions{UseProtoNames: true}).Marshal(message)
	return json.RawMessage(data), err
}

func sourceName(source marketanalyzerv1.PriceSource) string {
	if source == marketanalyzerv1.PriceSource_PRICE_SOURCE_HIGH_LOW {
		return "HIGH_LOW"
	}
	return "CLOSE"
}

func pointer[T any](value T) *T { return &value }
