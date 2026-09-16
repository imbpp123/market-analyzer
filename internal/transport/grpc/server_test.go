package grpc

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	marketanalyzerv1 "github.com/imbpp123/market-analyzer/api/go/marketanalyzer/v1"
	"github.com/imbpp123/market-analyzer/internal/application"
	"github.com/imbpp123/market-analyzer/internal/domain"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type transportClock struct{ now time.Time }

func (c transportClock) Now() time.Time { return c.now }

type transportReader struct {
	mu    sync.Mutex
	calls int
	read  func(context.Context, domain.Instrument, domain.Interval, domain.CandleRange) (application.SourceSeries, error)
}

func (r *transportReader) ReadCandles(ctx context.Context, instrument domain.Instrument, interval domain.Interval, candleRange domain.CandleRange) (application.SourceSeries, error) {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
	return r.read(ctx, instrument, interval, candleRange)
}

func (r *transportReader) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func TestGRPCRoundTripAllMethods(t *testing.T) {
	reader := &transportReader{read: transportSource}
	client := startClient(t, reader, DefaultMaxResponseBytes, 64<<10)
	selection := protoSelection()

	atr, err := client.GetATR(t.Context(), &marketanalyzerv1.GetATRRequest{Selection: selection, Settings: &marketanalyzerv1.ATRSettings{Period: pointer(uint32(3))}})
	require.NoError(t, err)
	assert.Equal(t, "5.2962963", atr.Result.Value)
	assert.Equal(t, uint32(6), atr.Result.CandleIndex)
	assert.Equal(t, "10.000", atr.Candles[0].Close)
	assert.Nil(t, atr.Candles[0].TradesCount)
	require.NotNil(t, atr.Candles[1].TradesCount)
	assert.Zero(t, *atr.Candles[1].TradesCount)
	assert.Equal(t, []string{domain.WilderATRVersion}, atr.Metadata.AlgorithmIds)

	natr, err := client.GetNATR(t.Context(), &marketanalyzerv1.GetNATRRequest{Selection: selection, Settings: &marketanalyzerv1.ATRSettings{Period: pointer(uint32(3))}})
	require.NoError(t, err)
	assert.Equal(t, "52.962963", natr.Result.Value)
	assert.Equal(t, "10.000", natr.Result.ReferenceClose)

	methods := []*marketanalyzerv1.ExtremaSettings{
		localSettings(),
		{PriceSource: pointer(marketanalyzerv1.PriceSource_PRICE_SOURCE_CLOSE), Method: &marketanalyzerv1.ExtremaSettings_ReversalPercent{
			ReversalPercent: &marketanalyzerv1.PercentReversalSettings{ReversalPct: pointer("10")}}},
		{PriceSource: pointer(marketanalyzerv1.PriceSource_PRICE_SOURCE_HIGH_LOW), Method: &marketanalyzerv1.ExtremaSettings_ReversalAtr{
			ReversalAtr: &marketanalyzerv1.ATRReversalSettings{AtrPeriod: pointer(uint32(2)), AtrMultiplier: pointer("0.5")}}},
	}
	for _, settings := range methods {
		response, requestErr := client.GetExtrema(t.Context(), &marketanalyzerv1.GetExtremaRequest{Selection: selection, Settings: settings})
		require.NoError(t, requestErr)
		assert.NotNil(t, response.Result)
		assert.NotNil(t, response.Settings.GetMethod())
	}

	extrema, err := client.GetExtrema(t.Context(), &marketanalyzerv1.GetExtremaRequest{Selection: selection, Settings: localSettings()})
	require.NoError(t, err)
	require.NotEmpty(t, extrema.Result.Points)
	assert.Equal(t, "12.000", extrema.Result.Points[0].Price)
	assert.Nil(t, extrema.Result.Points[0].Reversal)
	assert.Equal(t, extrema.Candles[extrema.Result.Points[0].CandleIndex].OpenTime, extrema.Result.Points[0].Time)

	trend, err := client.GetTrend(t.Context(), &marketanalyzerv1.GetTrendRequest{Selection: selection, Settings: &marketanalyzerv1.TrendSettings{
		Extrema: localSettings(), EqualityTolerancePct: pointer("0")}})
	require.NoError(t, err)
	assert.Equal(t, marketanalyzerv1.TrendState_TREND_STATE_UNDETERMINED, trend.Result.State)
	assert.Equal(t, "mixed_structure", trend.Result.Reason)

	levelExtrema := &marketanalyzerv1.ExtremaSettings{PriceSource: pointer(marketanalyzerv1.PriceSource_PRICE_SOURCE_HIGH_LOW),
		Method: &marketanalyzerv1.ExtremaSettings_ReversalAtr{ReversalAtr: &marketanalyzerv1.ATRReversalSettings{
			AtrPeriod: pointer(uint32(2)), AtrMultiplier: pointer("0.5")}}}
	levels, err := client.GetLevels(t.Context(), &marketanalyzerv1.GetLevelsRequest{Selection: selection, Settings: &marketanalyzerv1.LevelSettings{
		Extrema: levelExtrema, AtrPeriod: pointer(uint32(3)), ZoneWidthAtr: pointer("1"), MinTouches: pointer(uint32(2)), MinTouchSeparationBars: pointer(uint32(1))}})
	require.NoError(t, err)
	assert.Equal(t, "10.000", levels.Result.ReferenceClose)
	assert.Equal(t, uint32(2), levels.Settings.Extrema.GetReversalAtr().GetAtrPeriod())
	assert.Equal(t, uint32(3), levels.Settings.GetAtrPeriod())
	assert.Equal(t, []string{domain.WilderATRVersion, domain.ATRReversalVersion, domain.ZonesVersion}, levels.Metadata.AlgorithmIds)
	for _, zone := range levels.Result.Zones {
		assert.Equal(t, uint32(len(zone.AcceptedCandleIndices)), zone.TouchCount)
		for _, index := range zone.ExtremumIndices {
			assert.Less(t, index, uint32(len(levels.Result.Extrema.Points)))
		}
	}
	assert.Equal(t, 8, reader.callCount())
}

func TestGRPCRejectsPresenceAndUnknownEnumBeforeReading(t *testing.T) {
	reader := &transportReader{read: transportSource}
	client := startClient(t, reader, DefaultMaxResponseBytes, 64<<10)

	_, err := client.GetTrend(t.Context(), &marketanalyzerv1.GetTrendRequest{Selection: protoSelection(), Settings: &marketanalyzerv1.TrendSettings{Extrema: localSettings()}})
	assertStatusDetail(t, err, codes.InvalidArgument, application.InvalidParameter, "settings.equality_tolerance_pct")

	unknown := marketanalyzerv1.PriceSource(99)
	_, err = client.GetExtrema(t.Context(), &marketanalyzerv1.GetExtremaRequest{Selection: protoSelection(), Settings: &marketanalyzerv1.ExtremaSettings{
		PriceSource: &unknown, Method: &marketanalyzerv1.ExtremaSettings_LocalExtrema{LocalExtrema: &marketanalyzerv1.LocalExtremaSettings{PivotSpan: pointer(uint32(1))}}}})
	assertStatusDetail(t, err, codes.InvalidArgument, application.InvalidParameter, "settings.extrema.price_source")
	assert.Zero(t, reader.callCount())
}

func TestGRPCMapsDependencyError(t *testing.T) {
	reader := &transportReader{read: func(context.Context, domain.Instrument, domain.Interval, domain.CandleRange) (application.SourceSeries, error) {
		return application.SourceSeries{}, &application.Error{Kind: application.MarketDataUnavailable, UpstreamCode: "UNAVAILABLE", UpstreamReason: "not_ready"}
	}}
	client := startClient(t, reader, DefaultMaxResponseBytes, 64<<10)

	_, err := client.GetATR(t.Context(), &marketanalyzerv1.GetATRRequest{Selection: protoSelection(), Settings: &marketanalyzerv1.ATRSettings{Period: pointer(uint32(3))}})

	assertStatusDetail(t, err, codes.Unavailable, application.MarketDataUnavailable, "")
}

func TestGRPCResponseSizeBoundary(t *testing.T) {
	reader := &transportReader{read: transportSource}
	client := startClient(t, reader, DefaultMaxResponseBytes, 64<<10)
	request := &marketanalyzerv1.GetATRRequest{Selection: protoSelection(), Settings: &marketanalyzerv1.ATRSettings{Period: pointer(uint32(3))}}
	response, err := client.GetATR(t.Context(), request)
	require.NoError(t, err)
	size := proto.Size(response)

	exactClient := startClient(t, &transportReader{read: transportSource}, size, 64<<10)
	_, err = exactClient.GetATR(t.Context(), request)
	require.NoError(t, err)

	smallClient := startClient(t, &transportReader{read: transportSource}, size-1, 64<<10)
	_, err = smallClient.GetATR(t.Context(), request)
	assertStatusDetail(t, err, codes.ResourceExhausted, application.ResponseTooLarge, "")
}

func TestGRPCRequestTransportLimit(t *testing.T) {
	reader := &transportReader{read: transportSource}
	client := startClient(t, reader, DefaultMaxResponseBytes, 64)
	request := &marketanalyzerv1.GetATRRequest{Selection: protoSelection(), Settings: &marketanalyzerv1.ATRSettings{Period: pointer(uint32(3))}}
	request.Selection.Symbol = pointer(strings.Repeat("A", 1024))

	_, err := client.GetATR(t.Context(), request)

	assert.Equal(t, codes.ResourceExhausted, status.Code(err))
	assert.Zero(t, reader.callCount())
}

func TestGRPCInProgressCancellation(t *testing.T) {
	started := make(chan struct{})
	reader := &transportReader{read: func(ctx context.Context, _ domain.Instrument, _ domain.Interval, _ domain.CandleRange) (application.SourceSeries, error) {
		close(started)
		<-ctx.Done()
		return application.SourceSeries{}, ctx.Err()
	}}
	client := startClient(t, reader, DefaultMaxResponseBytes, 64<<10)
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		_, err := client.GetATR(ctx, &marketanalyzerv1.GetATRRequest{Selection: protoSelection(), Settings: &marketanalyzerv1.ATRSettings{Period: pointer(uint32(3))}})
		result <- err
	}()
	<-started
	cancel()

	assert.Equal(t, codes.Canceled, status.Code(<-result))
}

func TestApplicationErrorCodeMapping(t *testing.T) {
	cases := []struct {
		kind application.ErrorKind
		code codes.Code
	}{
		{application.InvalidParameter, codes.InvalidArgument}, {application.MarketDataRejectedRequest, codes.InvalidArgument},
		{application.SymbolNotFound, codes.NotFound}, {application.IncompleteData, codes.FailedPrecondition},
		{application.InvalidMarketData, codes.DataLoss}, {application.MarketDataUnavailable, codes.Unavailable},
		{application.MarketDataResourceExhausted, codes.ResourceExhausted}, {application.ResponseTooLarge, codes.ResourceExhausted},
		{application.RequestCanceled, codes.Canceled}, {application.RequestTimeout, codes.DeadlineExceeded},
		{application.MarketDataContractMismatch, codes.Unimplemented}, {application.MarketDataFailure, codes.Internal},
		{application.InternalError, codes.Internal},
	}
	for _, testCase := range cases {
		t.Run(string(testCase.kind), func(t *testing.T) {
			err := publicError(&application.Error{Kind: testCase.kind})
			assert.Equal(t, testCase.code, status.Code(err))
		})
	}
}

func startClient(t *testing.T, reader application.CandleReader, maxResponseBytes, maxRequestBytes int) marketanalyzerv1.MarketAnalyzerServiceClient {
	t.Helper()
	analyzer, err := application.NewAnalyzer(reader, transportClock{now: time.Date(2026, 1, 2, 12, 30, 0, 0, time.UTC)}, time.Second)
	require.NoError(t, err)
	service, err := NewServer(analyzer, maxResponseBytes)
	require.NoError(t, err)

	listener := bufconn.Listen(1 << 20)
	server := grpcgo.NewServer(grpcgo.MaxRecvMsgSize(maxRequestBytes), grpcgo.MaxSendMsgSize(maxResponseBytes))
	marketanalyzerv1.RegisterMarketAnalyzerServiceServer(server, service)
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})

	connection, err := grpcgo.NewClient("passthrough:///bufnet", grpcgo.WithTransportCredentials(insecure.NewCredentials()),
		grpcgo.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, connection.Close()) })
	return marketanalyzerv1.NewMarketAnalyzerServiceClient(connection)
}

func protoSelection() *marketanalyzerv1.Selection {
	return &marketanalyzerv1.Selection{Exchange: pointer("binance"), Market: pointer("spot"), Symbol: pointer("BTCUSDT"),
		To: timestamppb.New(time.Date(2026, 1, 2, 12, 7, 30, 0, time.UTC)), CandleCount: pointer(uint32(7)), Interval: pointer("1m")}
}

func localSettings() *marketanalyzerv1.ExtremaSettings {
	return &marketanalyzerv1.ExtremaSettings{PriceSource: pointer(marketanalyzerv1.PriceSource_PRICE_SOURCE_CLOSE),
		Method: &marketanalyzerv1.ExtremaSettings_LocalExtrema{LocalExtrema: &marketanalyzerv1.LocalExtremaSettings{PivotSpan: pointer(uint32(1))}}}
}

func transportSource(ctx context.Context, instrument domain.Instrument, interval domain.Interval, candleRange domain.CandleRange) (application.SourceSeries, error) {
	prices := []string{"10.000", "12.000", "9.000", "13.000", "8.000", "14.000", "10.000"}
	candles := make([]application.SourceCandle, len(prices))
	opening := candleRange.From
	for index, price := range prices {
		closing, err := interval.Shift(opening, 1)
		if err != nil {
			return application.SourceSeries{}, err
		}
		parsed, _ := decimal.NewFromString(price)
		count := int64(0)
		candles[index] = application.SourceCandle{OpenTime: opening, CloseTime: closing, Open: price,
			High: parsed.Add(decimal.NewFromInt(1)).StringFixed(3), Low: parsed.Sub(decimal.NewFromInt(1)).StringFixed(3), Close: price,
			Volume: "0.000", Turnover: "0", TradesCount: &count, FetchedAt: closing}
		if index == 0 {
			candles[index].TradesCount = nil
		}
		opening = closing
	}
	return application.SourceSeries{Instrument: instrument, Interval: interval, Range: candleRange, Candles: candles}, ctx.Err()
}

func assertStatusDetail(t *testing.T, err error, code codes.Code, reason application.ErrorKind, field string) {
	t.Helper()
	statusValue, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, code, statusValue.Code())
	require.NotEmpty(t, statusValue.Details())
	detail, ok := statusValue.Details()[0].(*marketanalyzerv1.ErrorDetail)
	require.True(t, ok)
	assert.Equal(t, string(reason), detail.Reason)
	assert.Equal(t, field, detail.GetField())
}
