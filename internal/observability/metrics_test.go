package observability

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	marketanalyzerv1 "github.com/imbpp123/market-analyzer/api/go/marketanalyzer/v1"
	"github.com/imbpp123/market-analyzer/internal/application"
	"github.com/imbpp123/market-analyzer/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type readerFunc func(context.Context, domain.Instrument, domain.Interval, domain.CandleRange) (application.SourceSeries, error)

func (f readerFunc) ReadCandles(ctx context.Context, instrument domain.Instrument, interval domain.Interval, candleRange domain.CandleRange) (application.SourceSeries, error) {
	return f(ctx, instrument, interval, candleRange)
}

func TestRegistryRecordsBoundedMetrics(t *testing.T) {
	registry := NewRegistry()
	reader := NewReader(readerFunc(func(context.Context, domain.Instrument, domain.Interval, domain.CandleRange) (application.SourceSeries, error) {
		return application.SourceSeries{Candles: make([]application.SourceCandle, 3)}, nil
	}), registry)

	registry.RequestStarted()
	registry.RequestFinished("GetATR", "OK", time.Second, 42)
	registry.ObserveCalculation("atr", 2*time.Second)
	_, err := reader.ReadCandles(t.Context(), domain.Instrument{Symbol: "must-not-be-a-label"}, "1m", domain.CandleRange{})
	require.NoError(t, err)

	response := httptest.NewRecorder()
	registry.ServeHTTP(response, httptest.NewRequest("GET", "/metrics", nil).WithContext(t.Context()))
	body := response.Body.String()
	assert.Contains(t, body, `market_analyzer_requests_count{rpc="GetATR",status="OK"} 1`)
	assert.Contains(t, body, `market_analyzer_market_data_count{status="ok"} 1`)
	assert.Contains(t, body, "market_analyzer_source_candles_sum 3")
	assert.Contains(t, body, "market_analyzer_response_bytes_sum 42")
	assert.Contains(t, body, "market_analyzer_active_requests 0")
	assert.NotContains(t, body, "must-not-be-a-label")
}

func TestUnaryInterceptorWritesStructuredRequestLogWithoutArrays(t *testing.T) {
	var output bytes.Buffer
	registry := NewRegistry()
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	interceptor := UnaryServerInterceptor(registry, logger)
	request := &marketanalyzerv1.GetATRRequest{Selection: &marketanalyzerv1.Selection{
		Exchange: value("binance"), Market: value("spot"), Symbol: value("BTCUSDT"), Interval: value("1m"),
		CandleCount: value(uint32(7)), To: timestamppb.New(time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)),
	}}

	_, err := interceptor(t.Context(), request, &grpcgo.UnaryServerInfo{FullMethod: "/marketanalyzer.v1.MarketAnalyzerService/GetATR"},
		func(context.Context, any) (any, error) {
			return &marketanalyzerv1.GetATRResponse{Metadata: &marketanalyzerv1.Metadata{
				SourceFrom: timestamppb.New(time.Date(2026, 1, 2, 11, 53, 0, 0, time.UTC)),
				SourceTo:   timestamppb.New(time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)), AlgorithmIds: []string{"wilder_atr_v1"},
			}, Candles: []*marketanalyzerv1.Candle{{Open: "array-must-not-be-logged"}}}, nil
		})

	require.NoError(t, err)
	logLine := output.String()
	assert.Contains(t, logLine, `"rpc":"GetATR"`)
	assert.Contains(t, logLine, `"symbol":"BTCUSDT"`)
	assert.Contains(t, logLine, `"algorithm_ids":"wilder_atr_v1"`)
	assert.NotContains(t, logLine, "array-must-not-be-logged")
}

func value[T any](input T) *T { return &input }

func TestMeasuredReaderRecordsStableFailureKind(t *testing.T) {
	registry := NewRegistry()
	reader := NewReader(readerFunc(func(context.Context, domain.Instrument, domain.Interval, domain.CandleRange) (application.SourceSeries, error) {
		return application.SourceSeries{}, &application.Error{Kind: application.MarketDataUnavailable, Err: errors.New(strings.Repeat("unbounded", 10))}
	}), registry)

	_, err := reader.ReadCandles(t.Context(), domain.Instrument{}, "1m", domain.CandleRange{})
	require.Error(t, err)
	response := httptest.NewRecorder()
	registry.ServeHTTP(response, httptest.NewRequest("GET", "/metrics", nil).WithContext(t.Context()))
	assert.Contains(t, response.Body.String(), `status="market_data_unavailable"`)
	assert.NotContains(t, response.Body.String(), "unbounded")
}
