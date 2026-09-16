package marketdata

import (
	"context"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/imbpp123/market-analyzer/internal/application"
	"github.com/imbpp123/market-analyzer/internal/domain"
	marketdatav1 "github.com/imbpp123/market-data/api/go/marketdata/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type fakeServer struct {
	marketdatav1.UnimplementedMarketDataServiceServer
	getKlines func(context.Context, *marketdatav1.GetKlinesRequest) (*marketdatav1.GetKlinesResponse, error)
}

func (s *fakeServer) GetKlines(ctx context.Context, request *marketdatav1.GetKlinesRequest) (*marketdatav1.GetKlinesResponse, error) {
	return s.getKlines(ctx, request)
}

func TestReaderMapsExactRequestAndResponse(t *testing.T) {
	from := time.Date(2026, 9, 15, 12, 0, 0, 123, time.UTC)
	to := from.Add(time.Minute)
	zero := int64(0)
	server := &fakeServer{getKlines: func(_ context.Context, request *marketdatav1.GetKlinesRequest) (*marketdatav1.GetKlinesResponse, error) {
		assert.Equal(t, "binance", request.GetExchange())
		assert.Equal(t, "spot", request.GetMarket())
		assert.Equal(t, "BTCUSDT", request.GetSymbol())
		assert.Equal(t, "1m", request.GetInterval())
		assert.Equal(t, from, request.GetFrom().AsTime())
		assert.Equal(t, to, request.GetTo().AsTime())

		return &marketdatav1.GetKlinesResponse{Exchange: "binance", Market: "spot", Symbol: "BTCUSDT", Interval: "1m", Klines: []*marketdatav1.Kline{
			{OpenTime: timestamppb.New(from), CloseTime: timestamppb.New(to), Open: "1.00", High: "2.00", Low: "0.50", Close: "1.50", Volume: "0", Turnover: "0.000", FetchedAt: timestamppb.New(to)},
			{OpenTime: timestamppb.New(to), CloseTime: timestamppb.New(to.Add(time.Minute)), Open: "1.50", High: "2.50", Low: "1.25", Close: "2.00", Volume: "3", Turnover: "6", TradesCount: &zero, FetchedAt: timestamppb.New(to.Add(time.Nanosecond))},
		}}, nil
	}}
	reader := startReader(t, server, DefaultMaxResponseBytes)

	result, err := reader.ReadCandles(t.Context(), domain.Instrument{Exchange: "binance", Market: "spot", Symbol: "BTCUSDT"}, "1m", domain.CandleRange{From: from, To: to})

	require.NoError(t, err)
	require.Len(t, result.Candles, 2)
	assert.Equal(t, "1.00", result.Candles[0].Open)
	assert.Equal(t, "0.000", result.Candles[0].Turnover)
	assert.Nil(t, result.Candles[0].TradesCount)
	require.NotNil(t, result.Candles[1].TradesCount)
	assert.Zero(t, *result.Candles[1].TradesCount)
	assert.Equal(t, to.Add(time.Nanosecond), result.Candles[1].FetchedAt)
}

func TestReaderMapsStatusAndStructuredReason(t *testing.T) {
	cases := []struct {
		name string
		code codes.Code
		kind application.ErrorKind
	}{
		{"invalid", codes.InvalidArgument, application.MarketDataRejectedRequest},
		{"not_found", codes.NotFound, application.SymbolNotFound},
		{"incomplete", codes.FailedPrecondition, application.IncompleteData},
		{"data_loss", codes.DataLoss, application.InvalidMarketData},
		{"unavailable", codes.Unavailable, application.MarketDataUnavailable},
		{"exhausted", codes.ResourceExhausted, application.MarketDataResourceExhausted},
		{"canceled", codes.Canceled, application.RequestCanceled},
		{"deadline", codes.DeadlineExceeded, application.RequestTimeout},
		{"unimplemented", codes.Unimplemented, application.MarketDataContractMismatch},
		{"permission", codes.PermissionDenied, application.MarketDataFailure},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			base := status.New(testCase.code, "message that must not be parsed")
			withDetail, err := base.WithDetails(&marketdatav1.ErrorDetail{Reason: "upstream_reason"})
			require.NoError(t, err)
			reader := startReader(t, &fakeServer{getKlines: func(context.Context, *marketdatav1.GetKlinesRequest) (*marketdatav1.GetKlinesResponse, error) {
				return nil, withDetail.Err()
			}}, DefaultMaxResponseBytes)

			_, err = reader.ReadCandles(t.Context(), domain.Instrument{}, "1m", domain.CandleRange{})

			var applicationError *application.Error
			require.ErrorAs(t, err, &applicationError)
			assert.Equal(t, testCase.kind, applicationError.Kind)
			assert.Equal(t, testCase.code.String(), applicationError.UpstreamCode)
			assert.Equal(t, "upstream_reason", applicationError.UpstreamReason)
		})
	}
}

func TestReaderHandlesMissingDetailsAndReceiveLimit(t *testing.T) {
	large := strings.Repeat("x", 2048)
	reader := startReader(t, &fakeServer{getKlines: func(context.Context, *marketdatav1.GetKlinesRequest) (*marketdatav1.GetKlinesResponse, error) {
		return &marketdatav1.GetKlinesResponse{Exchange: large}, nil
	}}, 128)

	_, err := reader.ReadCandles(t.Context(), domain.Instrument{}, "1m", domain.CandleRange{})

	var applicationError *application.Error
	require.ErrorAs(t, err, &applicationError)
	assert.Equal(t, application.MarketDataResourceExhausted, applicationError.Kind)
	assert.Empty(t, applicationError.UpstreamReason)
}

func TestReaderPropagatesCallerCancellation(t *testing.T) {
	started := make(chan struct{})
	reader := startReader(t, &fakeServer{getKlines: func(ctx context.Context, _ *marketdatav1.GetKlinesRequest) (*marketdatav1.GetKlinesResponse, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}}, DefaultMaxResponseBytes)
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		_, err := reader.ReadCandles(ctx, domain.Instrument{}, "1m", domain.CandleRange{})
		result <- err
	}()
	<-started
	cancel()

	var applicationError *application.Error
	require.ErrorAs(t, <-result, &applicationError)
	assert.Equal(t, application.RequestCanceled, applicationError.Kind)
}

func TestReaderIgnoresUnrelatedErrorDetails(t *testing.T) {
	base := status.New(codes.Unavailable, "opaque message")
	withDetail, err := base.WithDetails(durationpb.New(time.Second))
	require.NoError(t, err)

	mapped := mapError(withDetail.Err())

	var applicationError *application.Error
	require.ErrorAs(t, mapped, &applicationError)
	assert.Equal(t, application.MarketDataUnavailable, applicationError.Kind)
	assert.Empty(t, applicationError.UpstreamReason)
}

func TestReaderRunsConcurrentCallsIndependently(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})
	reader := startReader(t, &fakeServer{getKlines: func(_ context.Context, request *marketdatav1.GetKlinesRequest) (*marketdatav1.GetKlinesResponse, error) {
		started <- request.GetSymbol()
		<-release
		return &marketdatav1.GetKlinesResponse{Exchange: request.GetExchange(), Market: request.GetMarket(), Symbol: request.GetSymbol(), Interval: request.GetInterval()}, nil
	}}, DefaultMaxResponseBytes)
	results := make(chan application.SourceSeries, 2)
	errors := make(chan error, 2)
	for _, symbol := range []string{"BTCUSDT", "ETHUSDT"} {
		go func() {
			series, err := reader.ReadCandles(t.Context(), domain.Instrument{Exchange: "binance", Market: "spot", Symbol: symbol}, "1m", domain.CandleRange{})
			results <- series
			errors <- err
		}()
	}
	first := <-started
	second := <-started
	close(release)

	require.NoError(t, <-errors)
	require.NoError(t, <-errors)
	assert.ElementsMatch(t, []string{"BTCUSDT", "ETHUSDT"}, []string{first, second})
	assert.NotEqual(t, (<-results).Instrument.Symbol, (<-results).Instrument.Symbol)
}

func TestReaderDoesNotRetryFailedCall(t *testing.T) {
	var calls atomic.Int32
	reader := startReader(t, &fakeServer{getKlines: func(context.Context, *marketdatav1.GetKlinesRequest) (*marketdatav1.GetKlinesResponse, error) {
		calls.Add(1)
		return nil, status.Error(codes.Unavailable, "offline")
	}}, DefaultMaxResponseBytes)

	_, err := reader.ReadCandles(t.Context(), domain.Instrument{}, "1m", domain.CandleRange{})

	require.Error(t, err)
	assert.Equal(t, int32(1), calls.Load())
}

func startReader(t *testing.T, server marketdatav1.MarketDataServiceServer, maxResponseBytes int) *Reader {
	t.Helper()
	listener := bufconn.Listen(1 << 20)
	grpcServer := grpcgo.NewServer()
	marketdatav1.RegisterMarketDataServiceServer(grpcServer, server)
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(func() {
		grpcServer.Stop()
		_ = listener.Close()
	})

	connection, err := grpcgo.NewClient("passthrough:///market-data", grpcgo.WithTransportCredentials(insecure.NewCredentials()),
		grpcgo.WithDefaultCallOptions(grpcgo.MaxCallRecvMsgSize(maxResponseBytes)),
		grpcgo.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, connection.Close()) })
	reader, err := NewReader(marketdatav1.NewMarketDataServiceClient(connection))
	require.NoError(t, err)
	return reader
}
