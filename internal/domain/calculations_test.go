package domain

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func calculationOperations() []struct {
	name string
	run  func(context.Context, CandleSeries) (any, error)
} {
	local := ExtremaSettings{Close, LocalExtremaSettings{1}}
	percent := ExtremaSettings{Close, PercentReversalSettings{dec("5")}}
	atr := ExtremaSettings{HighLow, ATRReversalSettings{2, dec("0.5")}}
	return []struct {
		name string
		run  func(context.Context, CandleSeries) (any, error)
	}{
		{"atr", func(ctx context.Context, s CandleSeries) (any, error) { return CalculateATR(ctx, s, ATRSettings{2}) }},
		{"natr", func(ctx context.Context, s CandleSeries) (any, error) { return CalculateNATR(ctx, s, ATRSettings{2}) }},
		{"local", func(ctx context.Context, s CandleSeries) (any, error) { return DetectExtrema(ctx, s, local) }},
		{"percent", func(ctx context.Context, s CandleSeries) (any, error) { return DetectExtrema(ctx, s, percent) }},
		{"atr reversal", func(ctx context.Context, s CandleSeries) (any, error) { return DetectExtrema(ctx, s, atr) }},
		{"trend", func(ctx context.Context, s CandleSeries) (any, error) {
			return CalculateTrend(ctx, s, TrendSettings{local, dec("0")})
		}},
		{"levels", func(ctx context.Context, s CandleSeries) (any, error) {
			return CalculateLevels(ctx, s, LevelSettings{atr, 2, dec("1"), 2, 1})
		}},
	}
}

func TestCalculationsRejectUnvalidatedSeries(t *testing.T) {
	for _, operation := range calculationOperations() {
		t.Run(operation.name, func(t *testing.T) {
			_, err := operation.run(t.Context(), CandleSeries{})

			var validation *ValidationError
			require.ErrorAs(t, err, &validation)
		})
	}
}

func TestCalculationsRejectInvalidSettings(t *testing.T) {
	series := closeSeries(t, "100", "110", "100", "110")
	cases := []struct {
		name string
		run  func(context.Context) (any, error)
	}{
		{"atr", func(ctx context.Context) (any, error) { return CalculateATR(ctx, series, ATRSettings{}) }},
		{"natr", func(ctx context.Context) (any, error) { return CalculateNATR(ctx, series, ATRSettings{4}) }},
		{"extrema", func(ctx context.Context) (any, error) { return DetectExtrema(ctx, series, ExtremaSettings{}) }},
		{"trend", func(ctx context.Context) (any, error) {
			return CalculateTrend(ctx, series, TrendSettings{ExtremaSettings{Close, LocalExtremaSettings{1}}, dec("-1")})
		}},
		{"levels", func(ctx context.Context) (any, error) {
			return CalculateLevels(ctx, series, LevelSettings{ExtremaSettings{Close, LocalExtremaSettings{1}}, 1, dec("1"), 1, 1})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.run(t.Context())

			var validation *ValidationError
			require.ErrorAs(t, err, &validation)
		})
	}
}

func TestCalculationsCancellationAndDeadline(t *testing.T) {
	for _, operation := range calculationOperations() {
		for _, deadline := range []bool{false, true} {
			name := operation.name + "/cancel"
			if deadline {
				name = operation.name + "/deadline"
			}
			t.Run(name, func(t *testing.T) {
				series := closeSeries(t, "100", "110", "100", "110", "100")
				ctx, cancel := context.WithCancel(t.Context())
				expected := context.Canceled
				if deadline {
					cancel()
					ctx, cancel = context.WithDeadline(t.Context(), time.Unix(0, 0))
					expected = context.DeadlineExceeded
				} else {
					cancel()
				}
				defer cancel()

				result, err := operation.run(ctx, series)

				require.ErrorIs(t, err, expected)
				assert.Empty(t, result)
			})
		}
	}
}

// Trigger cancellation at a deterministic checkpoint, without timers or scheduling.
type scanContext struct {
	context.Context
	remaining int
	cancel    context.CancelFunc
}

func (c *scanContext) Err() error {
	c.remaining--
	if c.remaining <= 0 {
		c.cancel()
	}
	return c.Context.Err()
}

func TestCalculationsCancelDuringWork(t *testing.T) {
	for _, operation := range calculationOperations() {
		t.Run(operation.name, func(t *testing.T) {
			prices := make([]string, 100)
			for i := range prices {
				if i%2 == 0 {
					prices[i] = "100"
				} else {
					prices[i] = "110"
				}
			}
			series := closeSeries(t, prices...)
			base, cancel := context.WithCancel(t.Context())
			defer cancel()
			ctx := &scanContext{base, 20, cancel}

			result, err := operation.run(ctx, series)

			require.ErrorIs(t, err, context.Canceled)
			assert.Empty(t, result)
		})
	}
}

func TestZoneCancellationDuringGrouping(t *testing.T) {
	cases := []struct {
		name  string
		limit int
	}{{"sort", 40}, {"later checkpoint", 240}, {"later sort checkpoint", 320}, {"final checkpoint", 400}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prices := make([]string, 20)
			points := make([]Extremum, 20)
			for i := range prices {
				prices[i] = "100"
				points[i] = Extremum{Kind: High, Price: dec("100"), CandleIndex: CandleIndex(i)}
			}
			series := closeSeries(t, prices...)
			base, cancel := context.WithCancel(t.Context())
			defer cancel()
			ctx := &scanContext{base, tc.limit, cancel}

			result, err := buildZones(ctx, series, points, dec("1"), dec("100"), 2, 1)

			require.ErrorIs(t, err, context.Canceled)
			assert.Nil(t, result)
		})
	}
}

func TestCalculationsConcurrentOwnership(t *testing.T) {
	series := closeSeries(t, "100", "110", "100", "120", "100", "110", "100")
	before := series.Candles()
	for _, operation := range calculationOperations() {
		t.Run(operation.name, func(t *testing.T) {
			expected, err := operation.run(t.Context(), series)
			require.NoError(t, err)
			var workers sync.WaitGroup
			for range 8 {
				workers.Go(func() {
					result, err := operation.run(t.Context(), series)
					assert.NoError(t, err)
					assert.Equal(t, expected, result)
					// Changes to one result must not affect another calculation or the source.
					switch value := result.(type) {
					case ExtremaResult:
						if len(value.Points) > 0 {
							value.Points[0].Price = dec("1")
							if value.Points[0].Reversal != nil {
								value.Points[0].Reversal.Threshold = dec("1")
								if value.Points[0].Reversal.CandidateATR != nil {
									*value.Points[0].Reversal.CandidateATR = dec("1")
								}
							}
						}
					case TrendResult:
						if len(value.Extrema.Points) > 0 {
							value.Extrema.Points[0].Price = dec("1")
						}
					case LevelsResult:
						if len(value.Zones) > 0 {
							value.Zones[0].ExtremumIndices[0] = 999
							value.Zones[0].AcceptedCandleIndices[0] = 999
						}
						if len(value.Extrema.Points) > 0 {
							value.Extrema.Points[0].Price = dec("1")
						}
					}
				})
			}
			workers.Wait()
			again, err := operation.run(t.Context(), series)
			require.NoError(t, err)
			assert.Equal(t, expected, again)
			assert.Equal(t, before, series.Candles())
		})
	}
}

func TestDependentScansCancellation(t *testing.T) {
	cases := []struct {
		name string
		run  func(context.Context, CandleSeries, ExtremaResult, atrSequence) (any, error)
	}{
		{"reversal", func(ctx context.Context, s CandleSeries, p ExtremaResult, a atrSequence) (any, error) {
			result, err := reversalExtrema(ctx, s, ExtremaSettings{Close, ATRReversalSettings{1, dec("0.5")}}, &a)
			return result, err
		}},
		{"trend", func(ctx context.Context, s CandleSeries, p ExtremaResult, a atrSequence) (any, error) {
			result, err := classifyTrend(ctx, s, p, Close, dec("0"))
			return result, err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prices := make([]string, 100)
			for i := range prices {
				if i%2 == 0 {
					prices[i] = "100"
				} else {
					prices[i] = "110"
				}
			}
			series := closeSeries(t, prices...)
			points, err := DetectExtrema(t.Context(), series, ExtremaSettings{Close, LocalExtremaSettings{1}})
			require.NoError(t, err)
			sequence, err := calculateATRSequence(t.Context(), series, ATRSettings{1})
			require.NoError(t, err)
			base, cancel := context.WithCancel(t.Context())
			defer cancel()
			ctx := &scanContext{base, 20, cancel}

			result, err := tc.run(ctx, series, points, sequence)
			assert.Empty(t, result)

			require.ErrorIs(t, err, context.Canceled)
		})
	}
}

func TestLongHistoryComposition(t *testing.T) {
	cases := []struct {
		name    string
		method  ExtremaMethod
		points  int
		first   CandleIndex
		touches uint32
	}{
		{"local", LocalExtremaSettings{1}, 998, 1, 499},
		{"percent", PercentReversalSettings{dec("5")}, 999, 0, 500},
		{"atr", ATRReversalSettings{3, dec("0.5")}, 996, 3, 498},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prices := make([]string, 1000)
			for i := range prices {
				if i%2 == 0 {
					prices[i] = "100"
				} else {
					prices[i] = "110"
				}
			}
			series := closeSeries(t, prices...)
			extrema := ExtremaSettings{Close, tc.method}

			trend, err := CalculateTrend(t.Context(), series, TrendSettings{extrema, dec("0")})
			require.NoError(t, err)
			levels, err := CalculateLevels(t.Context(), series, LevelSettings{extrema, 3, dec("0.1"), 2, 2})

			require.NoError(t, err)
			assert.Equal(t, Sideways, trend.State)
			assert.Equal(t, HorizontalStructure, trend.Reason)
			require.Len(t, levels.Extrema.Points, tc.points)
			assert.Equal(t, tc.first, levels.Extrema.Points[0].CandleIndex)
			assert.Equal(t, CandleIndex(998), levels.Extrema.Points[tc.points-1].CandleIndex)
			assert.Equal(t, CandleIndex(999), levels.Extrema.Points[tc.points-1].ConfirmationCandleIndex)
			assert.Equal(t, "10", levels.ATR.Value.String())
			assert.Equal(t, "1", levels.MaximumZoneWidth.String())
			require.Len(t, levels.Zones, 2)
			assert.Equal(t, "100", levels.Zones[0].RepresentativePrice.String())
			assert.Equal(t, "110", levels.Zones[1].RepresentativePrice.String())
			assert.Equal(t, tc.touches, levels.Zones[0].TouchCount)
			assert.Equal(t, trend.Extrema, levels.Extrema)
		})
	}
}
