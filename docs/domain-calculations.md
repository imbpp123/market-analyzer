# Domain calculations

Phase 2 adds deterministic calculations to `internal/domain`. The [main specification](specifications/market-analyzer-v1.md#atr-and-natr) owns the formulas and decision rules. Calculations use in-memory candles and have no external I/O.

## Entry points

Create a validated `CandleSeries` with `NewCandleSeries` first. Each entry point takes a context and validates its settings against the actual candle count. The zero series is rejected.

| Function | Result |
| --- | --- |
| `CalculateATR` | Latest intermediate Wilder ATR, candle index, and exclusive closing time. |
| `CalculateNATR` | Latest NATR, intermediate ATR, reference close, candle index, and exclusive closing time. |
| `DetectExtrema` | Confirmed points for neighboring candles, percentage reversal, or ATR reversal. |
| `CalculateTrend` | One state and reason for the whole range, tolerance in price units, reference close, and all confirmed points. |
| `CalculateLevels` | Retained zones, all confirmed points, latest ATR, maximum zone width, and reference close. |

Results keep intermediate decimal values. Use `FormatNumber` and `FormatPercentage` only when preparing a response. Raw source strings and public response metadata remain phase 3 work.

## Calculation boundaries

The private ATR sequence stores only available values, beginning at candle index `period`. NATR and ATR reversal use this same implementation. Level composition reuses its sequence for extrema only when both periods match. This sequence is local to the current call and its source history. There is no cache.

Both reversal methods share a candidate state machine. Candidate updates happen before confirmation. Equal prices replace the candidate, and ATR candidates keep their saved ATR until the next update. An ambiguous first reversal confirms neither point. The final unfinished candidate is excluded.

The trend classifier receives confirmed points internally. It applies the six checks in specification order and compares high and low sequences independently. ATR is not a classifier input.

Zone construction sorts references to points, preserving the full chronological point list. It uses a fixed price anchor for each group. Touch spacing affects the accepted candle list, while all group points still define the bounds. Sorting and scans check cancellation. Cancellation returns an error without a partial result.

## Evidence and ownership

`CandleIndex` and `ExtremumIndex` are separate types. Point and confirmation times come from their referenced candles. Zone touch counts and first and last touch times come from accepted candle indices. Zone extrema references include points excluded by touch spacing; the complete point list also retains points from discarded groups.

Local points have no reversal evidence. Percentage reversal adds a threshold and confirmation price. ATR reversal also has a present candidate ATR. No unavailable ATR warmup values are represented as zero.

Functions do not modify their inputs. Each call owns its result slices and evidence pointers, so results from concurrent calls can be changed independently. Result values themselves are mutable; callers must synchronize access if they share one result.

## Validation

Tests beside the implementation cover the approved examples, OHLC fixtures for both price sources, candidate updates, ambiguity, zero ATR, confirmation stability, all trend states and reasons, fixed-anchor zones, spacing, reference integrity, and independent ATR periods. Tests also check long histories, boundaries before output rounding, cancellation, deadlines, and concurrent result ownership.

`GOCACHE=/tmp/market-analyzer-go-build make check` passes build, vet, unit tests, race tests, formatting, and Git whitespace checks. No separate linter is configured. Runtime integration, response serialization, and release measurements are later phases.
