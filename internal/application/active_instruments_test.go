package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/imbpp123/market-analyzer/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type activeReader struct {
	instruments []domain.Instrument
	stats       []domain.MarketStats
	readError   error
	statsError  error
}

func (r activeReader) ReadInstruments(ctx context.Context, _, _ string) ([]domain.Instrument, error) {
	return r.instruments, r.readError
}

func (r activeReader) ReadMarketStats(ctx context.Context, _, _ string) ([]domain.MarketStats, error) {
	return r.stats, r.statsError
}

func activeFixture() activeReader {
	now := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	return activeReader{
		instruments: []domain.Instrument{
			{
				Exchange:   "binance",
				Market:     "spot",
				Symbol:     "AAAUSDT",
				BaseAsset:  "AAA",
				QuoteAsset: "USDT",
				Status:     domain.InstrumentStatusTrading,
			},
			{
				Exchange:   "binance",
				Market:     "spot",
				Symbol:     "BBBUSDT",
				BaseAsset:  "BBB",
				QuoteAsset: "USDT",
				Status:     domain.InstrumentStatusTrading,
			},
			{
				Exchange: "binance",
				Market:   "spot",
				Symbol:   "OLDUSDT",
				Status:   "halted",
			},
		},
		stats: []domain.MarketStats{
			{
				Exchange:   "binance",
				Market:     "spot",
				Symbol:     "AAAUSDT",
				Volume:     "100.00",
				TradeCount: activePointer(int64(10)),
				FetchedAt:  now,
			},
			{
				Exchange:  "binance",
				Market:    "spot",
				Symbol:    "BBBUSDT",
				Volume:    "99",
				FetchedAt: now,
			},
		},
	}
}

func activePointer[T any](value T) *T { return &value }

func activeAnalyzer(t *testing.T, source activeReader, candles *fakeReader) *Analyzer {
	t.Helper()
	analyzer, err := NewAnalyzerWithInstruments(candles, source, fixedClock{now: time.Date(2026, 1, 20, 12, 0, 0, 0, time.UTC)}, time.Second, nil)
	require.NoError(t, err)
	return analyzer
}

func TestFindActiveInstrumentsFiltersStatsAtInclusiveThreshold(t *testing.T) {
	reader := &fakeReader{read: activeCandles}
	source := activeFixture()
	analyzer := activeAnalyzer(t, source, reader)
	request := FindActiveInstrumentsRequest{Exchange: "binance", Market: "spot", MinVolume24h: activePointer("100"), MinTrades24h: activePointer(int64(10))}

	response, err := analyzer.FindActiveInstruments(t.Context(), request)

	require.NoError(t, err)
	require.Len(t, response.Instruments, 1)
	assert.Equal(t, "AAAUSDT", response.Instruments[0].Symbol)
	require.NotNil(t, response.Instruments[0].MarketStats)
	assert.Equal(t, "100.00", response.Instruments[0].MarketStats.Volume)
	assert.Equal(t, int64(10), *response.Instruments[0].MarketStats.TradeCount)
	assert.Equal(t, source.stats[0].FetchedAt, response.Instruments[0].MarketStats.FetchedAt)
	assert.Zero(t, reader.callCount())

	*source.stats[0].TradeCount = 20
	assert.Equal(t, int64(10), *response.Instruments[0].MarketStats.TradeCount)
}

func TestFindActiveInstrumentsSkipsMissingTradeCount(t *testing.T) {
	reader := &fakeReader{read: activeCandles}
	analyzer := activeAnalyzer(t, activeFixture(), reader)

	response, err := analyzer.FindActiveInstruments(t.Context(), FindActiveInstrumentsRequest{
		Exchange: "binance", Market: "spot", MinTrades24h: activePointer(int64(0)),
	})

	require.NoError(t, err)
	require.Len(t, response.Instruments, 1)
	assert.Equal(t, "AAAUSDT", response.Instruments[0].Symbol)
}

func TestFindActiveInstrumentsReturnsTradingRowsWithoutThresholds(t *testing.T) {
	reader := &fakeReader{read: activeCandles}
	analyzer := activeAnalyzer(t, activeFixture(), reader)

	response, err := analyzer.FindActiveInstruments(t.Context(), FindActiveInstrumentsRequest{Exchange: "binance", Market: "spot"})

	require.NoError(t, err)
	require.Len(t, response.Instruments, 2)
	assert.Equal(t, "AAAUSDT", response.Instruments[0].Symbol)
	assert.Equal(t, "BBBUSDT", response.Instruments[1].Symbol)
	assert.Nil(t, response.Instruments[0].MarketStats)
	assert.Zero(t, reader.callCount())
}

func TestFindActiveInstrumentsAppliesExactNATR(t *testing.T) {
	reader := &fakeReader{read: activeCandles}
	analyzer := activeAnalyzer(t, activeFixture(), reader)

	response, err := analyzer.FindActiveInstruments(t.Context(), FindActiveInstrumentsRequest{
		Exchange: "binance", Market: "spot", MinVolume24h: activePointer("100"), MinNATR: activePointer("4"),
	})

	require.NoError(t, err)
	require.Len(t, response.Instruments, 1)
	assert.Equal(t, "AAAUSDT", response.Instruments[0].Symbol)
	assert.Equal(t, "4", *response.Instruments[0].NATR)
	assert.Equal(t, 1, reader.callCount())
	assert.Equal(t, 15*24*time.Hour, reader.calls[0].To.Sub(reader.calls[0].From))
}

func TestFindActiveInstrumentsExcludesBelowNATRThreshold(t *testing.T) {
	analyzer := activeAnalyzer(t, activeFixture(), &fakeReader{read: activeCandles})

	response, err := analyzer.FindActiveInstruments(t.Context(), FindActiveInstrumentsRequest{
		Exchange: "binance", Market: "spot", MinVolume24h: activePointer("100"), MinNATR: activePointer("4.000001"),
	})

	require.NoError(t, err)
	assert.Empty(t, response.Instruments)
}

func TestFindActiveInstrumentsUsesRequestedNATRPeriod(t *testing.T) {
	reader := &fakeReader{read: func(ctx context.Context, instrument domain.Instrument, interval domain.Interval, candleRange domain.CandleRange) (SourceSeries, error) {
		series, err := activeCandles(ctx, instrument, interval, candleRange)
		if err != nil {
			return SourceSeries{}, err
		}
		series.Candles[1].High = "110"
		series.Candles[1].Low = "90"
		return series, nil
	}}
	analyzer := activeAnalyzer(t, activeFixture(), reader)

	response, err := analyzer.FindActiveInstruments(t.Context(), FindActiveInstrumentsRequest{
		Exchange: "binance", Market: "spot", MinVolume24h: activePointer("100"), MinNATR: activePointer("10"), NATRPeriod: activePointer(uint32(2)),
	})

	require.NoError(t, err)
	require.Len(t, response.Instruments, 1)
	assert.Equal(t, "AAAUSDT", response.Instruments[0].Symbol)
	assert.Equal(t, "12", *response.Instruments[0].NATR)
	require.Len(t, reader.calls, 1)
	assert.Equal(t, 3*24*time.Hour, reader.calls[0].To.Sub(reader.calls[0].From))
}

func TestFindActiveInstrumentsAcceptsNATRPeriodOne(t *testing.T) {
	reader := &fakeReader{read: activeCandles}
	analyzer := activeAnalyzer(t, activeFixture(), reader)

	response, err := analyzer.FindActiveInstruments(t.Context(), FindActiveInstrumentsRequest{
		Exchange: "binance", Market: "spot", MinVolume24h: activePointer("100"), MinNATR: activePointer("4"), NATRPeriod: activePointer(uint32(1)),
	})

	require.NoError(t, err)
	require.Len(t, response.Instruments, 1)
	assert.Equal(t, "4", *response.Instruments[0].NATR)
	require.Len(t, reader.calls, 1)
	assert.Equal(t, 2*24*time.Hour, reader.calls[0].To.Sub(reader.calls[0].From))
}

func TestFindActiveInstrumentsRejectsInvalidFiltersBeforeReading(t *testing.T) {
	cases := []struct {
		name    string
		request FindActiveInstrumentsRequest
		field   string
	}{
		{"bad exchange", FindActiveInstrumentsRequest{Exchange: "bad", Market: "spot"}, "exchange"},
		{"bad market", FindActiveInstrumentsRequest{Exchange: "binance", Market: "bad"}, "market"},
		{"negative volume", FindActiveInstrumentsRequest{Exchange: "binance", Market: "spot", MinVolume24h: activePointer("-1")}, "min_volume_24h"},
		{"bad volume", FindActiveInstrumentsRequest{Exchange: "binance", Market: "spot", MinVolume24h: activePointer("1e3")}, "min_volume_24h"},
		{"negative count", FindActiveInstrumentsRequest{Exchange: "binance", Market: "spot", MinTrades24h: activePointer(int64(-1))}, "min_trades_24h"},
		{"bad NATR", FindActiveInstrumentsRequest{Exchange: "binance", Market: "spot", MinNATR: activePointer("")}, "min_natr"},
		{"zero NATR period", FindActiveInstrumentsRequest{Exchange: "binance", Market: "spot", MinNATR: activePointer("1"), NATRPeriod: activePointer(uint32(0))}, "natr_period"},
		{"large NATR period", FindActiveInstrumentsRequest{Exchange: "binance", Market: "spot", MinNATR: activePointer("1"), NATRPeriod: activePointer(uint32(1000))}, "natr_period"},
		{"unused NATR period", FindActiveInstrumentsRequest{Exchange: "binance", Market: "spot", NATRPeriod: activePointer(uint32(2))}, "natr_period"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			analyzer := activeAnalyzer(t, activeReader{readError: errors.New("should not read")}, &fakeReader{read: activeCandles})

			_, err := analyzer.FindActiveInstruments(t.Context(), testCase.request)

			assertError(t, err, InvalidParameter, testCase.field)
		})
	}
}

func TestFindActiveInstrumentsRejectsInvalidStats(t *testing.T) {
	source := activeFixture()
	source.stats[0].Volume = "bad"
	analyzer := activeAnalyzer(t, source, &fakeReader{read: activeCandles})

	_, err := analyzer.FindActiveInstruments(t.Context(), FindActiveInstrumentsRequest{
		Exchange: "binance", Market: "spot", MinVolume24h: activePointer("1"),
	})

	assertError(t, err, InvalidMarketData, "")
}

func TestFindActiveInstrumentsRejectsInvalidUnusedStats(t *testing.T) {
	source := activeFixture()
	source.stats = append(source.stats, domain.MarketStats{
		Exchange:  "binance",
		Market:    "spot",
		Symbol:    "OTHERUSDT",
		Volume:    "bad",
		FetchedAt: time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC),
	})
	analyzer := activeAnalyzer(t, source, &fakeReader{read: activeCandles})

	response, err := analyzer.FindActiveInstruments(t.Context(), FindActiveInstrumentsRequest{
		Exchange:     "binance",
		Market:       "spot",
		MinVolume24h: activePointer("1"),
	})

	assertError(t, err, InvalidMarketData, "")
	assert.Empty(t, response.Instruments)
}

func TestFindActiveInstrumentsRejectsInvalidSourceSymbol(t *testing.T) {
	source := activeFixture()
	source.instruments[0].Symbol = "BAD SYMBOL"
	analyzer := activeAnalyzer(t, source, &fakeReader{read: activeCandles})

	response, err := analyzer.FindActiveInstruments(t.Context(), FindActiveInstrumentsRequest{
		Exchange: "binance",
		Market:   "spot",
	})

	assertError(t, err, InvalidMarketData, "")
	assert.Empty(t, response.Instruments)
}

func TestFindActiveInstrumentsRejectsInvalidStatsSymbol(t *testing.T) {
	source := activeFixture()
	source.stats[0].Symbol = "BAD SYMBOL"
	analyzer := activeAnalyzer(t, source, &fakeReader{read: activeCandles})

	response, err := analyzer.FindActiveInstruments(t.Context(), FindActiveInstrumentsRequest{
		Exchange:     "binance",
		Market:       "spot",
		MinVolume24h: activePointer("1"),
	})

	assertError(t, err, InvalidMarketData, "")
	assert.Empty(t, response.Instruments)
}

func TestFindActiveInstrumentsPreservesSourceError(t *testing.T) {
	source := activeFixture()
	source.statsError = &Error{Kind: MarketDataUnavailable, Err: errors.New("source down")}
	analyzer := activeAnalyzer(t, source, &fakeReader{read: activeCandles})

	_, err := analyzer.FindActiveInstruments(t.Context(), FindActiveInstrumentsRequest{
		Exchange: "binance", Market: "spot", MinVolume24h: activePointer("1"),
	})

	assertError(t, err, MarketDataUnavailable, "")
}

func TestFindActiveInstrumentsDoesNotReturnPartialListOnNATRFailure(t *testing.T) {
	reader := &fakeReader{read: func(ctx context.Context, instrument domain.Instrument, interval domain.Interval, candleRange domain.CandleRange) (SourceSeries, error) {
		if instrument.Symbol == "BBBUSDT" {
			return SourceSeries{}, &Error{Kind: IncompleteData, Err: errors.New("missing candle")}
		}
		return activeCandles(ctx, instrument, interval, candleRange)
	}}
	analyzer := activeAnalyzer(t, activeFixture(), reader)

	response, err := analyzer.FindActiveInstruments(t.Context(), FindActiveInstrumentsRequest{
		Exchange: "binance", Market: "spot", MinNATR: activePointer("1"),
	})

	assertError(t, err, IncompleteData, "")
	assert.Empty(t, response.Instruments)
}

func TestFindActiveInstrumentsHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	analyzer := activeAnalyzer(t, activeFixture(), &fakeReader{read: activeCandles})

	response, err := analyzer.FindActiveInstruments(ctx, FindActiveInstrumentsRequest{Exchange: "binance", Market: "spot"})

	assertError(t, err, RequestCanceled, "")
	assert.Empty(t, response.Instruments)
}

func activeCandles(ctx context.Context, instrument domain.Instrument, interval domain.Interval, candleRange domain.CandleRange) (SourceSeries, error) {
	candles := make([]SourceCandle, int(candleRange.To.Sub(candleRange.From)/(24*time.Hour)))
	opening := candleRange.From
	for index := range candles {
		closing, err := interval.Shift(opening, 1)
		if err != nil {
			return SourceSeries{}, err
		}
		candles[index] = SourceCandle{OpenTime: opening, CloseTime: closing, Open: "100", High: "102", Low: "98", Close: "100",
			Volume: "1", Turnover: "100", FetchedAt: closing}
		opening = closing
	}
	return SourceSeries{Instrument: instrument, Interval: interval, Range: candleRange, Candles: candles}, ctx.Err()
}
