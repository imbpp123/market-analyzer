package grpc

import (
	"context"
	"strings"
	"testing"
	"time"

	marketanalyzerv1 "github.com/imbpp123/market-analyzer/api/go/marketanalyzer/v1"
	"github.com/imbpp123/market-analyzer/internal/application"
	"github.com/imbpp123/market-analyzer/internal/domain"
	"github.com/shopspring/decimal"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var benchmarkResponse *marketanalyzerv1.GetLevelsResponse

type responseBenchmarkReader struct {
	low  decimal.Decimal
	high decimal.Decimal
}

func (r responseBenchmarkReader) ReadCandles(ctx context.Context, instrument domain.Instrument, interval domain.Interval, candleRange domain.CandleRange) (application.SourceSeries, error) {
	count := int(candleRange.To.Sub(candleRange.From) / time.Minute)
	candles := make([]application.SourceCandle, count)
	opening := candleRange.From
	for index := range candles {
		if err := ctx.Err(); err != nil {
			return application.SourceSeries{}, err
		}
		price := r.low
		if index%2 == 1 {
			price = r.high
		}
		closing, err := interval.Shift(opening, 1)
		if err != nil {
			return application.SourceSeries{}, err
		}
		text := price.String()
		candles[index] = application.SourceCandle{OpenTime: opening, CloseTime: closing, Open: text, High: text, Low: text,
			Close: text, Volume: "1", Turnover: text, FetchedAt: candleRange.To}
		opening = closing
	}

	return application.SourceSeries{Instrument: instrument, Interval: interval, Range: candleRange, Candles: candles}, nil
}

func BenchmarkFullResponseConstruction(b *testing.B) {
	cases := []struct {
		name  string
		depth uint32
		low   string
		high  string
	}{
		{"typical/60", 60, "49000.12345678", "51000.87654321"},
		{"typical/300", 300, "49000.12345678", "51000.87654321"},
		{"typical/1000", 1000, "49000.12345678", "51000.87654321"},
		{"tiny/300", 300, "0.00000001", "0.00000002"},
		{"large/300", 300, strings.Repeat("9", 127), strings.Repeat("9", 128)},
	}
	for _, testCase := range cases {
		b.Run(testCase.name, func(b *testing.B) {
			reader := responseBenchmarkReader{low: decimal.RequireFromString(testCase.low), high: decimal.RequireFromString(testCase.high)}
			analyzer, err := application.NewAnalyzer(reader, transportClock{now: time.Date(2026, 1, 2, 1, 0, 0, 0, time.UTC)}, time.Minute)
			if err != nil {
				b.Fatal(err)
			}
			server, err := NewServer(analyzer, DefaultMaxResponseBytes)
			if err != nil {
				b.Fatal(err)
			}
			request := benchmarkLevelsRequest(testCase.depth)
			initial, err := server.GetLevels(b.Context(), request)
			if err != nil {
				b.Fatal(err)
			}
			responseBytes := proto.Size(initial)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				response, requestErr := server.GetLevels(b.Context(), request)
				if requestErr != nil {
					b.Fatal(requestErr)
				}
				benchmarkResponse = response
			}
			b.ReportMetric(float64(responseBytes), "response_bytes")
		})
	}
}

func benchmarkLevelsRequest(depth uint32) *marketanalyzerv1.GetLevelsRequest {
	selection := &marketanalyzerv1.Selection{Exchange: pointer("binance"), Market: pointer("spot"), Symbol: pointer("BTCUSDT"),
		To: timestamppb.New(time.Date(2026, 1, 2, 0, 0, 30, 0, time.UTC)), CandleCount: pointer(depth), Interval: pointer("1m")}
	extrema := &marketanalyzerv1.ExtremaSettings{PriceSource: pointer(marketanalyzerv1.PriceSource_PRICE_SOURCE_CLOSE),
		Method: &marketanalyzerv1.ExtremaSettings_ReversalPercent{ReversalPercent: &marketanalyzerv1.PercentReversalSettings{ReversalPct: pointer("1")}}}
	return &marketanalyzerv1.GetLevelsRequest{Selection: selection, Settings: &marketanalyzerv1.LevelSettings{
		Extrema: extrema, AtrPeriod: pointer(uint32(14)), ZoneWidthAtr: pointer("2"), MinTouches: pointer(uint32(2)), MinTouchSeparationBars: pointer(uint32(1)),
	}}
}
