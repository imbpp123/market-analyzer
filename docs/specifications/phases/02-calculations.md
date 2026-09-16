# Phase 2: Calculations

Status: planned. Dependency: [Phase 1](01-domain-foundations.md).

## Summary

Implement every approved calculation as deterministic domain logic. This phase produces results from in-memory candles; it does not fetch market data.

## Context and goals

Use the calculation sections in the main specification as the source of truth: [ATR and NATR](../market-analyzer-v1.md#atr-and-natr), [Price extrema](../market-analyzer-v1.md#price-extrema), [Trend detection](../market-analyzer-v1.md#trend-detection), and [Support and resistance levels](../market-analyzer-v1.md#support-and-resistance-levels).

## Implementation work

1. Implement result values and evidence references from [Calculation results](../market-analyzer-v1.md#calculation-results). Keep candle indices and extrema indices distinct. Derive times from the referenced series.
2. Implement Wilder ATR once. Its internal sequence begins at candle index `period`; earlier entries are unavailable, not zero. Add latest-value access and NATR normalization from the intermediate ATR.
3. Implement strict neighboring extrema for both price sources. Preserve point order and confirmation references.
4. Implement percentage reversal as an explicit candidate state machine. Handle unknown initial direction, candidate updates, last equal candidate selection, later-candle confirmation, and alternating confirmed points.
5. Implement ATR reversal with the same agreed reversal lifecycle. Save ATR and threshold at each candidate update. Keep ATR warmup at the oldest end and never substitute the latest ATR for a saved candidate ATR.
6. Implement strict trend classification over confirmed points. Keep the six checks in their defined order. The classifier receives points internally; ATR is not a direct trend input.
7. Implement zone construction: fixed-anchor grouping, distinct spaced touches, bounds from all group points, midpoint, role, and evidence. Preserve the complete extrema list for response references.
8. Provide concrete composition helpers where needed for shared ATR calculation. Reuse a sequence only for matching period and history within a single calculation. Do not add a global cache or interfaces around every function.
9. Honor cancellation during long scans and grouping work. Preserve input slices and return results that do not expose mutable internal state shared across calls.

Keep calculation helpers in the domain. Application orchestration and public source serialization are phase 3 work.

## Test cases: ATR and extrema

| Case | Expected result |
| --- | --- |
| Period 3 with TR values 2, 4, 3, then 5 | Seed 3; next intermediate ATR `3.666666666666667`. |
| Same final ATR and close 100 | NATR uses intermediate ATR; approved response rounding matches the worked example. |
| Period 1, minimum history, price gaps, zero movement | Correct latest TR/ATR/NATR and no invented initialization values. |
| Changed history start with the same period | Each result matches its own explicit seed and recurrence. |
| Local extrema for CLOSE and HIGH_LOW | Strict comparisons use the selected source; correct candidate and confirmation indices. |
| Equal neighboring prices or missing edge neighbors | No extremum of the affected kind. |
| One candle is both high and low | Both records, HIGH before LOW, without an implied intrabar sequence. |
| Percentage reversal exactly at threshold | Eligible older candidate confirms. |
| New or equal candidate on the reversal candle | Update first; no same-candle confirmation. Last equal candle becomes the candidate. |
| Both initial reversal conditions eligible | Neither confirms until an unambiguous later candle. |
| Confirmed high followed by a low search | Initialize from the confirmation candle; no second confirmation on that candle. |
| ATR period 10 | First ATR/candidates at zero-based index 10; earliest confirmation at index 11. |
| ATR candidate unchanged while later volatility changes | Saved ATR and threshold stay unchanged. |
| New or equal ATR candidate | Save that candle's ATR and threshold; delay confirmation to a later candle. |
| Candidate ATR zero | Candidate cannot confirm until a valid later update gives positive ATR. |
| Append candles with fixed earlier data and settings | Previously confirmed reversal points remain unchanged. |
| Unfinished final candidate | Excluded from confirmed results. |

Apply shared reversal lifecycle cases to both reversal methods and both price sources. Use valid OHLC fixtures, not just isolated price sequences, for ATR-dependent cases.

## Test cases: trend, zones, and composition

| Case | Expected result |
| --- | --- |
| Flat source range without enough extrema | `SIDEWAYS / flat_range` before the insufficient-structure check. |
| Fewer than two highs or lows in a nonflat range | `UNDETERMINED / insufficient_structure`. |
| Highs 100, 110, 120; lows 90, 95, 105; close 115; zero tolerance | `UP / rising_structure`. |
| Falling sequences with an intact latest close | `DOWN / falling_structure`. |
| Highs 100, 110, 105 with rising lows | `UNDETERMINED / mixed_structure`; no last-pair shortcut. |
| Directional points with a last close breaking structure | `UNDETERMINED / structure_broken`. |
| Horizontal points and close at inclusive range boundaries | `SIDEWAYS / horizontal_structure`. |
| Difference exactly at tolerance | Equality; directional comparisons require strictly greater movement. |
| Nonalternating high and low lists | Compare each list separately. |
| Prices 100, 100.75, 101.5 with width 1 | Groups `[100, 100.75]` and `[101.5]`. |
| Point exactly at the width boundary | Included; no transitive group expansion. |
| Mixed highs and lows in one group | Both kinds can contribute touches; kinds remain in evidence. |
| Touch indices 10, 12, 16, 23 with spacing 5 | Accepted indices 10, 16, 23. |
| Two extrema from one candle in a group | One accepted touch, with both extrema retained. |
| Point excluded only by spacing defines a price bound | Bound still includes that point. |
| Latest close above, below, or exactly at either zone bound | SUPPORT, RESISTANCE, or AT_PRICE as specified. |
| Zero ATR or insufficient accepted touches | Equal-price groups only, or successful empty zones. |
| Different latest ATR on a later selection | Regrouping is allowed and matches explicit expected zones. |
| Equal versus different ATR periods in level composition | Consistent shared values for equal histories/periods; independent values otherwise. |
| Cancellation and concurrent repeated calculations | Cancellation propagates; no partial result or shared mutable state. |

Build direct point fixtures for classifier and grouping rules, plus full candle fixtures that run detection and dependent calculations together. Check indices, source prices, thresholds, and confirmation times, not only result counts.

## Completion criteria

All five analyses and all three extrema methods pass domain tests and shared checks. Every state and reason has coverage. No RPC, external reader, or runtime listener is required to run the calculations.

Next: [Phase 3](03-api-and-use-cases.md).
