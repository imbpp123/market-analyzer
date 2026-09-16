package domain

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

var benchmarkResult any

func BenchmarkCalculations(b *testing.B) {
	cases := []struct {
		name    string
		depth   int
		low     string
		high    string
		percent string
	}{
		{"typical/60", 60, "49000.12345678", "51000.87654321", "1"},
		{"typical/300", 300, "49000.12345678", "51000.87654321", "1"},
		{"typical/1000", 1000, "49000.12345678", "51000.87654321", "1"},
		{"tiny/300", 300, "0.00000001", "0.00000002", "1"},
		{"large/300", 300, strings.Repeat("9", 127), strings.Repeat("9", 128), "1"},
	}
	for _, testCase := range cases {
		b.Run(testCase.name, func(b *testing.B) {
			series := benchmarkCandleSeries(b, testCase.depth, testCase.low, testCase.high)
			local := ExtremaSettings{PriceSource: Close, Method: LocalExtremaSettings{PivotSpan: 1}}
			percent := ExtremaSettings{PriceSource: Close, Method: PercentReversalSettings{ReversalPct: decimal.RequireFromString(testCase.percent)}}
			atrReversal := ExtremaSettings{PriceSource: HighLow, Method: ATRReversalSettings{ATRPeriod: 14, ATRMultiplier: decimal.RequireFromString("0.5")}}

			benchmarks := []struct {
				name string
				run  func(context.Context) (any, error)
			}{
				{"atr", func(ctx context.Context) (any, error) { return CalculateATR(ctx, series, ATRSettings{Period: 14}) }},
				{"natr", func(ctx context.Context) (any, error) { return CalculateNATR(ctx, series, ATRSettings{Period: 14}) }},
				{"extrema_local", func(ctx context.Context) (any, error) { return DetectExtrema(ctx, series, local) }},
				{"extrema_percent", func(ctx context.Context) (any, error) { return DetectExtrema(ctx, series, percent) }},
				{"extrema_atr", func(ctx context.Context) (any, error) { return DetectExtrema(ctx, series, atrReversal) }},
				{"trend", func(ctx context.Context) (any, error) {
					return CalculateTrend(ctx, series, TrendSettings{Extrema: percent, EqualityTolerancePct: decimal.Zero})
				}},
				{"levels", func(ctx context.Context) (any, error) {
					return CalculateLevels(ctx, series, LevelSettings{Extrema: percent, ATRPeriod: 14,
						ZoneWidthATR: decimal.NewFromInt(2), MinTouches: 2, MinTouchSeparationBars: 1})
				}},
			}
			for _, benchmark := range benchmarks {
				b.Run(benchmark.name, func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						result, err := benchmark.run(b.Context())
						if err != nil {
							b.Fatal(err)
						}
						benchmarkResult = result
					}
				})
			}
		})
	}
}

func benchmarkCandleSeries(tb testing.TB, depth int, lowText, highText string) CandleSeries {
	tb.Helper()
	low := decimal.RequireFromString(lowText)
	high := decimal.RequireFromString(highText)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	instrument := Instrument{Exchange: "binance", Market: "spot", Symbol: "BTCUSDT"}
	selection := CandleSelection{Instrument: instrument, Interval: "1m", To: start.Add(time.Duration(depth) * time.Minute), CandleCount: uint32(depth)}
	candles := make([]Candle, depth)
	for index := range candles {
		price := low
		if index%2 == 1 {
			price = high
		}
		opening := start.Add(time.Duration(index) * time.Minute)
		candles[index] = Candle{OpenTime: opening, CloseTime: opening.Add(time.Minute), Open: price,
			High: price, Low: price, Close: price, Volume: decimal.NewFromInt(1), Turnover: price, FetchedAt: selection.To}
	}
	rangeValue, err := selection.Range()
	if err != nil {
		tb.Fatal(fmt.Errorf("build benchmark range: %w", err))
	}
	series, err := NewCandleSeries(tb.Context(), selection, instrument, selection.Interval, rangeValue, candles)
	if err != nil {
		tb.Fatal(fmt.Errorf("build benchmark series: %w", err))
	}
	return series
}
