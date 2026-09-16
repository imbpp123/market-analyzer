package service

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	marketanalyzerv1 "github.com/imbpp123/market-analyzer/api/go/marketanalyzer/v1"
	marketdatav1 "github.com/imbpp123/market-data/api/go/marketdata/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type marketDataServer struct {
	marketdatav1.UnimplementedMarketDataServiceServer
	getKlines func(context.Context, *marketdatav1.GetKlinesRequest) (*marketdatav1.GetKlinesResponse, error)
}

func (s *marketDataServer) GetKlines(ctx context.Context, request *marketdatav1.GetKlinesRequest) (*marketdatav1.GetKlinesResponse, error) {
	return s.getKlines(ctx, request)
}

func TestAppServesAnalysisAndOperationalEndpoints(t *testing.T) {
	upstreamAddress := startMarketData(t, validKlines)
	app := startApp(t, upstreamAddress)
	client := analyzerClient(t, app.GRPCAddress())

	health, err := get(t, "http://"+app.HTTPAddress()+"/health")
	require.NoError(t, err)
	closeBody(t, health)
	assert.Equal(t, http.StatusOK, health.StatusCode)
	ready, err := get(t, "http://"+app.HTTPAddress()+"/ready")
	require.NoError(t, err)
	closeBody(t, ready)
	assert.Equal(t, http.StatusOK, ready.StatusCode)
	missing, err := get(t, "http://"+app.HTTPAddress()+"/analysis")
	require.NoError(t, err)
	closeBody(t, missing)
	assert.Equal(t, http.StatusNotFound, missing.StatusCode)

	response, err := client.GetATR(t.Context(), validATRRequest())
	require.NoError(t, err)
	assert.Len(t, response.Candles, 7)

	metrics, err := get(t, "http://"+app.HTTPAddress()+"/metrics")
	require.NoError(t, err)
	closeBody(t, metrics)
	body, err := io.ReadAll(metrics.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), `market_analyzer_requests_count{rpc="GetATR",status="OK"} 1`)
	assert.Contains(t, string(body), "market_analyzer_source_candles_sum 7")
	assert.Contains(t, string(body), `market_analyzer_calculation_count{calculation="atr"} 1`)
}

func TestAppReadinessDoesNotDependOnMarketData(t *testing.T) {
	upstreamAddress := startMarketData(t, func(context.Context, *marketdatav1.GetKlinesRequest) (*marketdatav1.GetKlinesResponse, error) {
		return nil, status.Error(codes.Unavailable, "offline")
	})
	app := startApp(t, upstreamAddress)
	client := analyzerClient(t, app.GRPCAddress())

	_, err := client.GetATR(t.Context(), validATRRequest())
	assert.Equal(t, codes.Unavailable, status.Code(err))
	ready, err := get(t, "http://"+app.HTTPAddress()+"/ready")
	require.NoError(t, err)
	closeBody(t, ready)
	assert.Equal(t, http.StatusOK, ready.StatusCode)
	metrics, metricsErr := get(t, "http://"+app.HTTPAddress()+"/metrics")
	require.NoError(t, metricsErr)
	closeBody(t, metrics)
	body, readErr := io.ReadAll(metrics.Body)
	require.NoError(t, readErr)
	assert.Contains(t, string(body), `market_analyzer_market_data_count{status="market_data_unavailable"} 1`)
}

func TestAppRejectsInvalidSuccessfulMarketDataResponse(t *testing.T) {
	upstreamAddress := startMarketData(t, func(ctx context.Context, request *marketdatav1.GetKlinesRequest) (*marketdatav1.GetKlinesResponse, error) {
		response, err := validKlines(ctx, request)
		response.Klines[0].Open = "not-a-decimal"
		return response, err
	})
	app := startApp(t, upstreamAddress)
	client := analyzerClient(t, app.GRPCAddress())

	_, err := client.GetATR(t.Context(), validATRRequest())

	assert.Equal(t, codes.DataLoss, status.Code(err))
}

func TestAppForcesBlockedRequestAtShutdownDeadline(t *testing.T) {
	started := make(chan struct{})
	canceled := make(chan struct{})
	upstreamAddress := startMarketData(t, func(ctx context.Context, _ *marketdatav1.GetKlinesRequest) (*marketdatav1.GetKlinesResponse, error) {
		close(started)
		<-ctx.Done()
		close(canceled)
		return nil, ctx.Err()
	})
	app := startApp(t, upstreamAddress)
	client := analyzerClient(t, app.GRPCAddress())
	requestDone := make(chan error, 1)
	go func() {
		_, err := client.GetATR(t.Context(), validATRRequest())
		requestDone <- err
	}()
	<-started

	shutdownContext, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	err := app.Shutdown(shutdownContext)

	assert.ErrorIs(t, err, context.DeadlineExceeded)
	guardContext, guardCancel := context.WithTimeout(t.Context(), time.Second)
	defer guardCancel()
	select {
	case <-canceled:
	case <-guardContext.Done():
		t.Fatal("upstream request was not canceled")
	}
	assert.Error(t, <-requestDone)
}

func TestAppLetsInflightRequestFinishAndRejectsNewWork(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	upstreamAddress := startMarketData(t, func(ctx context.Context, request *marketdatav1.GetKlinesRequest) (*marketdatav1.GetKlinesResponse, error) {
		close(started)
		select {
		case <-release:
			return validKlines(ctx, request)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	app := startApp(t, upstreamAddress)
	client := analyzerClient(t, app.GRPCAddress())
	response := make(chan *marketanalyzerv1.GetATRResponse, 1)
	requestError := make(chan error, 1)
	go func() {
		result, err := client.GetATR(t.Context(), validATRRequest())
		response <- result
		requestError <- err
	}()
	<-started
	app.admission.Close()

	_, err := client.GetATR(t.Context(), validATRRequest())
	assert.Equal(t, codes.Unavailable, status.Code(err))
	close(release)
	shutdownContext, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(shutdownContext))
	require.NoError(t, <-requestError)
	assert.Len(t, (<-response).Candles, 7)
}

func TestAppRejectsOccupiedHTTPListenerAndReleasesGRPCListener(t *testing.T) {
	grpcAddress := unusedAddress(t)
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, occupied.Close()) })
	config := testConfig("127.0.0.1:1")
	config.GRPCAddress = grpcAddress
	config.HTTPAddress = occupied.Addr().String()

	_, err = New(config, testLogger())

	require.Error(t, err)
	listener, listenErr := net.Listen("tcp", grpcAddress)
	require.NoError(t, listenErr)
	require.NoError(t, listener.Close())
}

func startApp(t *testing.T, upstreamAddress string) *App {
	t.Helper()
	app, err := New(testConfig(upstreamAddress), testLogger())
	require.NoError(t, err)
	app.Start()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), time.Second)
		defer cancel()
		_ = app.Shutdown(ctx)
	})
	return app
}

func testConfig(upstreamAddress string) Config {
	config := DefaultConfig()
	config.GRPCAddress = "127.0.0.1:0"
	config.HTTPAddress = "127.0.0.1:0"
	config.MarketDataEndpoint = upstreamAddress
	config.RequestTimeout = time.Second
	config.ShutdownTimeout = time.Second
	return config
}

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func startMarketData(t *testing.T, handler func(context.Context, *marketdatav1.GetKlinesRequest) (*marketdatav1.GetKlinesResponse, error)) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := grpcgo.NewServer()
	marketdatav1.RegisterMarketDataServiceServer(server, &marketDataServer{getKlines: handler})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})
	return listener.Addr().String()
}

func analyzerClient(t *testing.T, address string) marketanalyzerv1.MarketAnalyzerServiceClient {
	t.Helper()
	connection, err := grpcgo.NewClient(address, grpcgo.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, connection.Close()) })
	return marketanalyzerv1.NewMarketAnalyzerServiceClient(connection)
}

func get(t *testing.T, url string) (*http.Response, error) {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return http.DefaultClient.Do(request)
}

func closeBody(t *testing.T, response *http.Response) {
	t.Helper()
	t.Cleanup(func() { require.NoError(t, response.Body.Close()) })
}

func validATRRequest() *marketanalyzerv1.GetATRRequest {
	return &marketanalyzerv1.GetATRRequest{
		Selection: &marketanalyzerv1.Selection{Exchange: ptr("binance"), Market: ptr("spot"), Symbol: ptr("BTCUSDT"),
			To: timestamppb.New(time.Date(2026, 1, 2, 12, 7, 30, 0, time.UTC)), CandleCount: ptr(uint32(7)), Interval: ptr("1m")},
		Settings: &marketanalyzerv1.ATRSettings{Period: ptr(uint32(3))},
	}
}

func validKlines(_ context.Context, request *marketdatav1.GetKlinesRequest) (*marketdatav1.GetKlinesResponse, error) {
	prices := []string{"10", "12", "9", "13", "8", "14", "10"}
	opening := request.GetFrom().AsTime()
	klines := make([]*marketdatav1.Kline, len(prices))
	for index, price := range prices {
		closing := opening.Add(time.Minute)
		klines[index] = &marketdatav1.Kline{OpenTime: timestamppb.New(opening), CloseTime: timestamppb.New(closing),
			Open: price, High: "20", Low: "1", Close: price, Volume: "0", Turnover: "0", FetchedAt: timestamppb.New(closing)}
		opening = closing
	}
	return &marketdatav1.GetKlinesResponse{Exchange: request.GetExchange(), Market: request.GetMarket(), Symbol: request.GetSymbol(), Interval: request.GetInterval(), Klines: klines}, nil
}

func unusedAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())
	return address
}

func ptr[T any](value T) *T { return &value }
