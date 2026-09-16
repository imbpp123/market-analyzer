package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/imbpp123/market-analyzer/internal/domain"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type fakeReader struct {
	mu    sync.Mutex
	calls []domain.CandleRange
	read  func(context.Context, domain.Instrument, domain.Interval, domain.CandleRange) (SourceSeries, error)
}

func (f *fakeReader) ReadCandles(ctx context.Context, instrument domain.Instrument, interval domain.Interval, candleRange domain.CandleRange) (SourceSeries, error) {
	f.mu.Lock()
	f.calls = append(f.calls, candleRange)
	f.mu.Unlock()

	return f.read(ctx, instrument, interval, candleRange)
}

func (f *fakeReader) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func TestAnalyzerRunsFiveUseCasesWithOneReadEach(t *testing.T) {
	now := time.Date(2026, 1, 2, 12, 30, 0, 0, time.UTC)
	reader := &fakeReader{read: validSource}
	analyzer, err := NewAnalyzer(reader, fixedClock{now: now}, time.Second)
	require.NoError(t, err)
	selection := validSelection()

	atr, err := analyzer.GetATR(t.Context(), ATRRequest{Selection: &selection, Settings: &ATRSettings{Period: 3}})
	require.NoError(t, err)
	assert.Equal(t, domain.CandleIndex(6), atr.Result.CandleIndex)
	assert.Equal(t, "10.000", atr.Candles[0].Close)
	assert.Nil(t, atr.Candles[0].TradesCount)
	assert.Equal(t, now, atr.Metadata.EvaluatedAt)
	assert.Equal(t, selection.To, atr.Metadata.Selection.To)

	natr, err := analyzer.GetNATR(t.Context(), NATRRequest{Selection: &selection, Settings: &ATRSettings{Period: 3}})
	require.NoError(t, err)
	assert.Equal(t, domain.CandleIndex(6), natr.Result.CandleIndex)

	extremaSettings := ExtremaSettings{PriceSource: domain.Close, Method: LocalExtremaSettings{PivotSpan: 1}}
	extrema, err := analyzer.GetExtrema(t.Context(), ExtremaRequest{Selection: &selection, Settings: &extremaSettings})
	require.NoError(t, err)
	assert.NotEmpty(t, extrema.Result.Points)

	trend, err := analyzer.GetTrend(t.Context(), TrendRequest{Selection: &selection, Settings: &TrendSettings{
		Extrema: &extremaSettings, EqualityTolerancePct: "0"}})
	require.NoError(t, err)
	assert.NotEmpty(t, trend.Result.State)

	levels, err := analyzer.GetLevels(t.Context(), LevelsRequest{Selection: &selection, Settings: &LevelSettings{
		Extrema: &extremaSettings, ATRPeriod: 2, ZoneWidthATR: "1.0", MinTouches: 2, MinTouchSeparationBars: 1}})
	require.NoError(t, err)
	assert.Equal(t, domain.CandleIndex(6), levels.Result.ATR.CandleIndex)
	assert.Equal(t, 5, reader.callCount())
	assert.Equal(t, []string{domain.LocalExtremaVersion, domain.WilderATRVersion, domain.ZonesVersion}, levels.Metadata.Algorithms)
}

func TestAnalyzerRejectsInvalidInputBeforeReading(t *testing.T) {
	reader := &fakeReader{read: validSource}
	analyzer, err := NewAnalyzer(reader, fixedClock{now: time.Date(2026, 1, 2, 12, 30, 0, 0, time.UTC)}, time.Second)
	require.NoError(t, err)
	selection := validSelection()
	extrema := ExtremaSettings{PriceSource: domain.Close, Method: PercentReversalSettings{ReversalPct: "1e2"}}

	_, err = analyzer.GetExtrema(t.Context(), ExtremaRequest{Selection: &selection, Settings: &extrema})

	assertError(t, err, InvalidParameter, "settings.reversal_percent.reversal_pct")
	assert.Zero(t, reader.callCount())
}

func TestAnalyzerRejectsFutureRangeBeforeReading(t *testing.T) {
	now := time.Date(2026, 1, 2, 12, 30, 30, 0, time.UTC)
	reader := &fakeReader{read: validSource}
	analyzer, err := NewAnalyzer(reader, fixedClock{now: now}, time.Second)
	require.NoError(t, err)
	selection := validSelection()
	selection.To = now.Add(2 * time.Minute)

	_, err = analyzer.GetATR(t.Context(), ATRRequest{Selection: &selection, Settings: &ATRSettings{Period: 3}})

	assertError(t, err, InvalidParameter, "selection.to")
	assert.Zero(t, reader.callCount())
}

func TestAnalyzerRejectsInvalidSuccessfulSource(t *testing.T) {
	reader := &fakeReader{read: func(ctx context.Context, instrument domain.Instrument, interval domain.Interval, candleRange domain.CandleRange) (SourceSeries, error) {
		source, err := validSource(ctx, instrument, interval, candleRange)
		require.NoError(t, err)
		source.Candles[2].Open = "bad"
		return source, nil
	}}
	analyzer, err := NewAnalyzer(reader, fixedClock{now: time.Date(2026, 1, 2, 12, 30, 0, 0, time.UTC)}, time.Second)
	require.NoError(t, err)
	selection := validSelection()

	_, err = analyzer.GetATR(t.Context(), ATRRequest{Selection: &selection, Settings: &ATRSettings{Period: 3}})

	assertError(t, err, InvalidMarketData, "candles[2].open")
	assert.Equal(t, 1, reader.callCount())
}

func TestAnalyzerPreservesDependencyError(t *testing.T) {
	reader := &fakeReader{read: func(context.Context, domain.Instrument, domain.Interval, domain.CandleRange) (SourceSeries, error) {
		return SourceSeries{}, &Error{Kind: SymbolNotFound, UpstreamCode: "NOT_FOUND", UpstreamReason: "symbol_not_found", Err: errors.New("missing symbol")}
	}}
	analyzer, err := NewAnalyzer(reader, fixedClock{now: time.Date(2026, 1, 2, 12, 30, 0, 0, time.UTC)}, time.Second)
	require.NoError(t, err)
	selection := validSelection()

	_, err = analyzer.GetATR(t.Context(), ATRRequest{Selection: &selection, Settings: &ATRSettings{Period: 3}})

	assertError(t, err, SymbolNotFound, "")
}

func TestAnalyzerLoadsFreshDataForConcurrentRequests(t *testing.T) {
	var mu sync.Mutex
	call := 0
	reader := &fakeReader{read: func(ctx context.Context, instrument domain.Instrument, interval domain.Interval, candleRange domain.CandleRange) (SourceSeries, error) {
		source, err := validSource(ctx, instrument, interval, candleRange)
		if err != nil {
			return SourceSeries{}, err
		}
		mu.Lock()
		call++
		current := call
		mu.Unlock()
		if current%2 == 0 {
			source.Candles[6].High = "30.000"
		}
		return source, nil
	}}
	analyzer, err := NewAnalyzer(reader, fixedClock{now: time.Date(2026, 1, 2, 12, 30, 0, 0, time.UTC)}, time.Second)
	require.NoError(t, err)
	selection := validSelection()

	results := make(chan string, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			response, requestErr := analyzer.GetATR(t.Context(), ATRRequest{Selection: &selection, Settings: &ATRSettings{Period: 3}})
			require.NoError(t, requestErr)
			results <- response.Result.Value.String()
		}()
	}
	wait.Wait()
	close(results)

	values := make(map[string]bool)
	for value := range results {
		values[value] = true
	}
	assert.Len(t, values, 2)
	assert.Equal(t, 2, reader.callCount())
}

func TestAnalyzerUsesEarlierClientDeadline(t *testing.T) {
	deadlineSeen := make(chan time.Time, 1)
	reader := &fakeReader{read: func(ctx context.Context, _ domain.Instrument, _ domain.Interval, _ domain.CandleRange) (SourceSeries, error) {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		deadlineSeen <- deadline
		return SourceSeries{}, context.DeadlineExceeded
	}}
	analyzer, err := NewAnalyzer(reader, fixedClock{now: time.Date(2026, 1, 2, 12, 30, 0, 0, time.UTC)}, time.Minute)
	require.NoError(t, err)
	selection := validSelection()
	clientDeadline := time.Now().Add(10 * time.Second)
	ctx, cancel := context.WithDeadline(t.Context(), clientDeadline)
	defer cancel()

	_, err = analyzer.GetATR(ctx, ATRRequest{Selection: &selection, Settings: &ATRSettings{Period: 3}})

	assertError(t, err, RequestTimeout, "")
	assert.WithinDuration(t, clientDeadline, <-deadlineSeen, time.Millisecond)
}

func TestAnalyzerUsesDefaultDeadlineWithoutClientDeadline(t *testing.T) {
	deadlineSeen := make(chan time.Time, 1)
	reader := &fakeReader{read: func(ctx context.Context, _ domain.Instrument, _ domain.Interval, _ domain.CandleRange) (SourceSeries, error) {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		deadlineSeen <- deadline
		return SourceSeries{}, context.DeadlineExceeded
	}}
	timeout := 20 * time.Second
	analyzer, err := NewAnalyzer(reader, fixedClock{now: time.Date(2026, 1, 2, 12, 30, 0, 0, time.UTC)}, timeout)
	require.NoError(t, err)
	selection := validSelection()
	expectedDeadline := time.Now().Add(timeout)

	_, err = analyzer.GetATR(t.Context(), ATRRequest{Selection: &selection, Settings: &ATRSettings{Period: 3}})

	assertError(t, err, RequestTimeout, "")
	assert.WithinDuration(t, expectedDeadline, <-deadlineSeen, 100*time.Millisecond)
}

func validSelection() Selection {
	return Selection{Exchange: "binance", Market: "spot", Symbol: "BTCUSDT", To: time.Date(2026, 1, 2, 12, 7, 30, 0, time.UTC), CandleCount: 7, Interval: "1m"}
}

func validSource(ctx context.Context, instrument domain.Instrument, interval domain.Interval, candleRange domain.CandleRange) (SourceSeries, error) {
	prices := []string{"10.000", "12.000", "9.000", "13.000", "8.000", "14.000", "10.000"}
	candles := make([]SourceCandle, len(prices))
	opening := candleRange.From
	for index, price := range prices {
		closing, err := interval.Shift(opening, 1)
		if err != nil {
			return SourceSeries{}, err
		}
		count := int64(0)
		candles[index] = SourceCandle{OpenTime: opening, CloseTime: closing, Open: price, High: addPrice(price, 1), Low: addPrice(price, -1),
			Close: price, Volume: "0.000", Turnover: "0", FetchedAt: closing, TradesCount: &count}
		if index == 0 {
			candles[index].TradesCount = nil
		}
		opening = closing
	}

	return SourceSeries{Instrument: instrument, Interval: interval, Range: candleRange, Candles: candles}, ctx.Err()
}

func addPrice(value string, delta int64) string {
	parsed, _ := decimal.NewFromString(value)
	return parsed.Add(decimal.NewFromInt(delta)).StringFixed(3)
}

func assertError(t *testing.T, err error, kind ErrorKind, field string) {
	t.Helper()
	var applicationError *Error
	require.ErrorAs(t, err, &applicationError)
	assert.Equal(t, kind, applicationError.Kind)
	assert.Equal(t, field, applicationError.Field)
}
