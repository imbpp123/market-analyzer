package releasevalidation

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	marketanalyzerv1 "github.com/imbpp123/market-analyzer/api/go/marketanalyzer/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type fakeAnalyzerClient struct {
	calls  atomic.Int64
	atrErr error
}

func (f *fakeAnalyzerClient) GetATR(_ context.Context, request *marketanalyzerv1.GetATRRequest, _ ...grpcgo.CallOption) (*marketanalyzerv1.GetATRResponse, error) {
	f.calls.Add(1)
	if f.atrErr != nil {
		return nil, f.atrErr
	}
	metadata, candles := fakeSource(request.Selection)
	return &marketanalyzerv1.GetATRResponse{Metadata: metadata, Candles: candles, Settings: request.Settings,
		Result: &marketanalyzerv1.ATRResult{Value: "1", CandleIndex: 2, ValueTime: candles[2].CloseTime}}, nil
}

func (f *fakeAnalyzerClient) GetNATR(_ context.Context, request *marketanalyzerv1.GetNATRRequest, _ ...grpcgo.CallOption) (*marketanalyzerv1.GetNATRResponse, error) {
	f.calls.Add(1)
	metadata, candles := fakeSource(request.Selection)
	return &marketanalyzerv1.GetNATRResponse{Metadata: metadata, Candles: candles, Settings: request.Settings,
		Result: &marketanalyzerv1.NATRResult{Value: "1", Atr: "1", ReferenceClose: "101", CandleIndex: 2, ValueTime: candles[2].CloseTime}}, nil
}

func (f *fakeAnalyzerClient) GetExtrema(_ context.Context, request *marketanalyzerv1.GetExtremaRequest, _ ...grpcgo.CallOption) (*marketanalyzerv1.GetExtremaResponse, error) {
	f.calls.Add(1)
	metadata, candles := fakeSource(request.Selection)
	return &marketanalyzerv1.GetExtremaResponse{Metadata: metadata, Candles: candles, Settings: request.Settings,
		Result: &marketanalyzerv1.ExtremaResult{}}, nil
}

func (f *fakeAnalyzerClient) GetTrend(_ context.Context, request *marketanalyzerv1.GetTrendRequest, _ ...grpcgo.CallOption) (*marketanalyzerv1.GetTrendResponse, error) {
	f.calls.Add(1)
	metadata, candles := fakeSource(request.Selection)
	return &marketanalyzerv1.GetTrendResponse{Metadata: metadata, Candles: candles, Settings: request.Settings,
		Result: &marketanalyzerv1.TrendResult{State: marketanalyzerv1.TrendState_TREND_STATE_UNDETERMINED,
			Reason: "insufficient_structure", Tolerance: "0.1", ReferenceClose: "101", Extrema: &marketanalyzerv1.ExtremaResult{}}}, nil
}

func (f *fakeAnalyzerClient) GetLevels(_ context.Context, request *marketanalyzerv1.GetLevelsRequest, _ ...grpcgo.CallOption) (*marketanalyzerv1.GetLevelsResponse, error) {
	f.calls.Add(1)
	metadata, candles := fakeSource(request.Selection)
	return &marketanalyzerv1.GetLevelsResponse{Metadata: metadata, Candles: candles, Settings: request.Settings,
		Result: &marketanalyzerv1.LevelsResult{Extrema: &marketanalyzerv1.ExtremaResult{},
			Atr:              &marketanalyzerv1.ATRResult{Value: "1", CandleIndex: 2, ValueTime: candles[2].CloseTime},
			MaximumZoneWidth: "1", ReferenceClose: "101"}}, nil
}

func TestRunCoversRPCMethodAndPriceSourceMatrix(t *testing.T) {
	client := &fakeAnalyzerClient{}
	config := testConfig()
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)

	report, err := Run(t.Context(), client, config, func() time.Time { return now })

	require.NoError(t, err)
	assert.Equal(t, int64(20), client.calls.Load())
	assert.Equal(t, Summary{Total: 20, Passed: 20, ResponseBytes: report.Summary.ResponseBytes}, report.Summary)
	assert.Positive(t, report.Summary.ResponseBytes)
	assert.Len(t, report.Checks, 20)
	assert.JSONEq(t, `{"settings":{"period":1},"selection":{"exchange":"binance","market":"spot","symbol":"BTCUSDT","to":"2026-09-16T11:03:30Z","candle_count":3,"interval":"1m"}}`, string(report.Checks[0].Request))
	assert.Equal(t, now, report.StartedAt)
	assert.Equal(t, now, report.CompletedAt)
}

func TestValidateResultRejectsBrokenEvidence(t *testing.T) {
	config := testConfig()
	selection := protoSelection(config.Selection)
	metadata, candles := fakeSource(selection)
	view := resultView{message: &marketanalyzerv1.GetExtremaResponse{}, metadata: metadata, candles: candles,
		extrema: &marketanalyzerv1.ExtremaResult{Points: []*marketanalyzerv1.Extremum{{
			Kind: marketanalyzerv1.ExtremumKind_EXTREMUM_KIND_HIGH, CandleIndex: 1, ConfirmationCandleIndex: 2,
			Time: candles[1].OpenTime, ConfirmationTime: candles[2].CloseTime, Price: "wrong",
		}}}, method: "local", source: marketanalyzerv1.PriceSource_PRICE_SOURCE_HIGH_LOW}

	err := validateResult(config.Selection, view)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "price does not match")
}

func TestRunRecordsFailuresAndContinues(t *testing.T) {
	client := &fakeAnalyzerClient{atrErr: errors.New("dependency failed")}

	report, err := Run(t.Context(), client, testConfig(), time.Now)

	require.Error(t, err)
	assert.Equal(t, int64(20), client.calls.Load())
	assert.Equal(t, 1, report.Summary.Failed)
	assert.Equal(t, 19, report.Summary.Passed)
	assert.Equal(t, "Unknown", report.Checks[0].Status)
	assert.Contains(t, report.Checks[0].Error, "dependency failed")
}

func fakeSource(selection *marketanalyzerv1.Selection) (*marketanalyzerv1.Metadata, []*marketanalyzerv1.Candle) {
	start := time.Date(2026, 9, 16, 11, 0, 0, 0, time.UTC)
	candles := make([]*marketanalyzerv1.Candle, selection.GetCandleCount())
	for index := range candles {
		opening := start.Add(time.Duration(index) * time.Minute)
		candles[index] = &marketanalyzerv1.Candle{
			OpenTime: timestamppb.New(opening), CloseTime: timestamppb.New(opening.Add(time.Minute)),
			Open: "101", High: "102", Low: "100", Close: "101", Volume: "1", Turnover: "101", FetchedAt: timestamppb.New(opening.Add(time.Minute)),
		}
	}
	metadata := &marketanalyzerv1.Metadata{Selection: selection, EvaluatedAt: timestamppb.New(start.Add(time.Hour)),
		SourceFrom: timestamppb.New(start), SourceTo: timestamppb.New(start.Add(time.Duration(len(candles)) * time.Minute)),
		AlgorithmIds: []string{"test_v1"}, NumericPolicy: "test"}
	return metadata, candles
}

func testConfig() Config {
	return Config{
		AnalyzerEndpoint: "localhost:9091", Timeout: time.Second, MaxResponseBytes: 1 << 20,
		Selection: Selection{Exchange: "binance", Market: "spot", Symbol: "BTCUSDT",
			To: time.Date(2026, 9, 16, 11, 3, 30, 0, time.UTC), CandleCount: 3, Interval: "1m"},
		Settings: Settings{ATRPeriod: 1, PivotSpan: 1, ReversalPct: "2", ReversalATRPeriod: 1,
			ReversalATRMultiplier: "1", EqualityTolerancePct: "0.1", LevelATRPeriod: 1,
			ZoneWidthATR: "1", MinTouches: 2, MinTouchSeparationBars: 1},
		RuntimeSettings: RuntimeSettings{RequestTimeout: "30s", MaxRequestBytes: 64 << 10,
			MarketDataMaxResponseBytes: 16 << 20, MaxResponseBytes: 32 << 20},
	}
}

func protoSelection(input Selection) *marketanalyzerv1.Selection {
	return &marketanalyzerv1.Selection{Exchange: pointer(input.Exchange), Market: pointer(input.Market), Symbol: pointer(input.Symbol),
		To: timestamppb.New(input.To), CandleCount: pointer(input.CandleCount), Interval: pointer(input.Interval)}
}
