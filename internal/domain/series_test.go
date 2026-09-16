package domain

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seriesFixture() (CandleSelection, CandleRange, []Candle) {
	s := selection()
	s.CandleCount = 2
	r := CandleRange{timestamp("2026-09-15T14:00:00Z"), timestamp("2026-09-15T14:02:00Z")}
	candles := []Candle{
		{OpenTime: timestamp("2026-09-15T14:00:00Z"), CloseTime: timestamp("2026-09-15T14:01:00Z"), Open: dec("100"), High: dec("102"), Low: dec("99"), Close: dec("101"), FetchedAt: s.To},
		{OpenTime: timestamp("2026-09-15T14:01:00Z"), CloseTime: timestamp("2026-09-15T14:02:00Z"), Open: dec("101"), High: dec("102"), Low: dec("99"), Close: dec("100"), FetchedAt: s.To},
	}

	return s, r, candles
}

func TestSeriesOwnsValues(t *testing.T) {
	s, r, candles := seriesFixture()
	count := int64(0)
	candles[0].TradesCount = &count

	result, err := NewCandleSeries(t.Context(), s, s.Instrument, s.Interval, r, candles)

	require.NoError(t, err)

	count = 10
	candles[0].Open = dec("500")
	values := result.Candles()
	assert.Equal(t, "100", values[0].Open.String())
	require.NotNil(t, values[0].TradesCount)
	assert.Equal(t, int64(0), *values[0].TradesCount)
	assert.Nil(t, values[1].TradesCount)
	assert.True(t, values[0].Volume.IsZero())
	assert.True(t, values[0].Turnover.IsZero())
	values[0].Open = dec("200")
	*values[0].TradesCount = 20
	assert.Equal(t, "100", result.Candles()[0].Open.String())
	assert.Equal(t, int64(0), *result.Candles()[0].TradesCount)
	assert.Equal(t, s, result.Selection())
	assert.Equal(t, r, result.Range())
}

func TestSeriesRejectsInvalidCandles(t *testing.T) {
	cases := []struct {
		name   string
		change func([]Candle) []Candle
		field  string
	}{
		{"missing", func(c []Candle) []Candle { return c[:1] }, "candles"},
		{"empty", func(c []Candle) []Candle { return nil }, "candles"},
		{"extra", func(c []Candle) []Candle { return append(c, c[1]) }, "candles"},
		{"duplicate", func(c []Candle) []Candle { c[1] = c[0]; return c }, "candles[1].open_time"},
		{"order", func(c []Candle) []Candle { c[0], c[1] = c[1], c[0]; return c }, "candles[0].open_time"},
		{"gap", func(c []Candle) []Candle { c[1].OpenTime = c[1].CloseTime; return c }, "candles[1].open_time"},
		{"missing opening", func(c []Candle) []Candle { c[0].OpenTime = time.Time{}; return c }, "candles[0].open_time"},
		{"inclusive close", func(c []Candle) []Candle { c[0].CloseTime = c[0].CloseTime.Add(-time.Nanosecond); return c }, "candles[0].close_time"},
		{"missing close", func(c []Candle) []Candle { c[0].CloseTime = time.Time{}; return c }, "candles[0].close_time"},
		{"missing fetched time", func(c []Candle) []Candle { c[0].FetchedAt = time.Time{}; return c }, "candles[0].fetched_at"},
		{"invalid fetched time", func(c []Candle) []Candle { c[0].FetchedAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC); return c }, "candles[0].fetched_at"},
		{"zero open", func(c []Candle) []Candle { c[0].Open = dec("0"); return c }, "candles[0].open"},
		{"negative high", func(c []Candle) []Candle { c[0].High = dec("-1"); return c }, "candles[0].high"},
		{"zero low", func(c []Candle) []Candle { c[0].Low = dec("0"); return c }, "candles[0].low"},
		{"zero close", func(c []Candle) []Candle { c[0].Close = dec("0"); return c }, "candles[0].close"},
		{"open above high", func(c []Candle) []Candle { c[0].Open = dec("103"); return c }, "candles[0].open"},
		{"open below low", func(c []Candle) []Candle { c[0].Open = dec("98"); return c }, "candles[0].open"},
		{"close above high", func(c []Candle) []Candle { c[0].Close = dec("103"); return c }, "candles[0].close"},
		{"close below low", func(c []Candle) []Candle { c[0].Close = dec("98"); return c }, "candles[0].close"},
		{"negative volume", func(c []Candle) []Candle { c[0].Volume = dec("-1"); return c }, "candles[0].volume"},
		{"negative turnover", func(c []Candle) []Candle { c[0].Turnover = dec("-1"); return c }, "candles[0].turnover"},
		{"negative trades", func(c []Candle) []Candle { v := int64(-1); c[0].TradesCount = &v; return c }, "candles[0].trades_count"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, r, candles := seriesFixture()
			candles = tc.change(candles)
			original := make([]Candle, len(candles))
			for i, c := range candles {
				original[i] = cloneCandle(c)
			}

			_, err := NewCandleSeries(t.Context(), s, s.Instrument, s.Interval, r, candles)

			checkValidation(t, err, tc.field)
			assert.Equal(t, original, append([]Candle{}, candles...))
		})
	}
}

func TestSeriesRejectsMetadata(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Instrument, *Interval, *CandleRange)
		field  string
	}{
		{"identity", func(i *Instrument, _ *Interval, _ *CandleRange) { i.Symbol = "ETHUSDT" }, "instrument"},
		{"interval", func(_ *Instrument, i *Interval, _ *CandleRange) { *i = "3m" }, "interval"},
		{"start", func(_ *Instrument, _ *Interval, r *CandleRange) { r.From = r.From.Add(time.Minute) }, "range"},
		{"end", func(_ *Instrument, _ *Interval, r *CandleRange) { r.To = r.To.Add(time.Minute) }, "range"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, r, candles := seriesFixture()
			instrument, interval := s.Instrument, s.Interval
			tc.change(&instrument, &interval, &r)

			_, err := NewCandleSeries(t.Context(), s, instrument, interval, r, candles)

			checkValidation(t, err, tc.field)
		})
	}
}

func TestSeriesCancellation(t *testing.T) {
	s, r, candles := seriesFixture()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := NewCandleSeries(ctx, s, s.Instrument, s.Interval, r, candles)
	require.ErrorIs(t, err, context.Canceled)
}

func TestSeriesDeadline(t *testing.T) {
	s, r, candles := seriesFixture()
	ctx, cancel := context.WithDeadline(t.Context(), timestamp("2000-01-01T00:00:00Z"))
	defer cancel()

	_, err := NewCandleSeries(ctx, s, s.Instrument, s.Interval, r, candles)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestSeriesRejectsInvalidSelection(t *testing.T) {
	s, r, candles := seriesFixture()
	s.CandleCount = 0

	_, err := NewCandleSeries(t.Context(), s, s.Instrument, s.Interval, r, candles)

	checkValidation(t, err, "candle_count")
}

func TestSeriesUsesCalendarMonths(t *testing.T) {
	s, _, candles := seriesFixture()
	s.Interval = "1M"
	s.To = timestamp("2024-04-15T12:00:00Z")
	r := CandleRange{timestamp("2024-02-01T00:00:00Z"), timestamp("2024-04-01T00:00:00Z")}
	candles[0].OpenTime = r.From
	candles[0].CloseTime = timestamp("2024-03-01T00:00:00Z")
	candles[1].OpenTime = candles[0].CloseTime
	candles[1].CloseTime = r.To

	result, err := NewCandleSeries(t.Context(), s, s.Instrument, s.Interval, r, candles)

	require.NoError(t, err)
	assert.Equal(t, candles, result.Candles())
}
