# Phase 2: Domain foundations

Status: planned. Dependency: [Phase 1: Prepare Market Data](01-market-data-prepare.md), including resolvable module versions.

## Summary

Create the Go foundation and Analyzer-specific values using the shared Market Data models prepared in phase 1. The result is a tested domain package without network access.

## Context and goals

The repository currently contains documentation. This phase establishes a buildable module and implements the common rules in [Data Model](../market-analyzer-v1.md#data-model), [Candle selection](../market-analyzer-v1.md#candle-selection), and [Numerical rules](../market-analyzer-v1.md#numerical-rules).

## Implementation work

1. Read the phase 1 handoff and pin the delivered `github.com/imbpp123/market-data/pkg/market` version. Inspect the repository and current upstream client module. Choose and document a Go toolchain compatible with the pinned client requirement. Create the module and reproducible build and test commands. Pin `shopspring/decimal v1.4.0`; use the repository-required assertion library for tests.
2. Use shared `market.Exchange`, `market.Market`, `market.Timeframe`, `market.Kline`, and calendar values directly. Add only Analyzer-owned selection, range, validated series, and algorithm settings under `internal/domain`. A series contains shared Klines. A selection needs exchange, market, and symbol, not a fully populated instrument catalog record. Do not recreate Instrument, Ticker, MarketStats, or Kline models. Keep helpers private where possible.
3. Represent extrema method settings as exclusive alternatives. Check method parameters and minimum candle counts with overflow-safe arithmetic. Do not add unrequested defaults.
4. Compose shared calendar `Floor` and `Shift` operations to plan the requested range. Do not implement a second calendar. Keep Analyzer request constraints, supported selector checks, and pre-epoch rejection local. Use shared boundary validation for series slots and propagate calendar errors.
5. Reuse shared individual-value validation and implement Analyzer-specific complete series validation: identity, count, order, contiguous calendar slots, timestamps, positive OHLC, OHLC ordering, and nonnegative quantities. Do not sort or repair invalid inputs.
6. Implement common decimal division and formatting. Division must provide 16 significant digits using explicit `DivRound` scale, including when the required scale is negative. Determine magnitude using decimal/integer operations, not floating-point logarithms. Handle zero explicitly and never change global library precision.
7. Keep output formatting separate from intermediate calculations. Provide the 8-significant-digit and 6-decimal-place percentage formats, with ties away from zero and trimmed trailing zeros.

Shared source records and upstream Protobuf conversions are delivered by phase 1. Response composition uses them in phase 4; infrastructure invokes the converters in phase 5. This phase handles shared parsed market values and Analyzer-specific rules. Request-time future checks belong to the application; range arithmetic accepts explicit inputs and does not read the clock.

## Interfaces and deliverables

Deliver validated domain values, typed validation errors, selection helpers built on the shared calendar, and decimal helpers. Validation errors identify the invalid field or invariant without gRPC status types. Do not add reader interfaces, persistence, or service listeners yet.

Keep one source of validation rules. Later application and transport code must call these rules rather than independently implement algorithm constraints.

## Test cases

| Case | Expected result |
| --- | --- |
| `to = 2026-09-15 14:02:30 UTC`, count 60, interval 1m | Range `[13:02, 14:02)` with 60 slots. |
| `to` exactly on a slot boundary | Last selected candle ends at that boundary. |
| Non-UTC representation of the same timestamp | Same selected range. |
| Representative weekly, monthly, and Binance 3-day selections | Analyzer passes the right interval and count to the shared calendar; expected range is correct. Exhaustive calendar tests belong to the shared module. |
| Unknown interval or unsupported exchange/market combination | Validation error. |
| Zero count, huge span or period, range before epoch | Validation error without overflow or panic. |
| Missing or multiple extrema alternatives; invalid source or method settings | Validation error; no inferred defaults. |
| Missing slot, duplicate, wrong order, identity or close time | Invalid series; original input remains unchanged. |
| Nonpositive OHLC or invalid OHLC ordering | Invalid series. |
| Zero volume and turnover | Valid; negative quantities fail. |
| Exact addition, subtraction, multiplication | No extra rounding. |
| `11 / 3` | Intermediate `3.666666666666667`; numeric output `3.6666667`; percentage output `3.666667`. |
| Positive and negative halfway cases | Round away from zero, including `1.235 → 1.24` and `-1.235 → -1.24` at two places. |
| Tiny values and division near a power-of-ten boundary | Correct significant-digit count; no float conversion or lost leading fractional zeros. |
| Zero and division by zero | Zero formats as `0`; division by zero returns an error. |
| Values equal only after output rounding | Internal comparison still distinguishes them. |
| Concurrent numerical operations | Independent results; global decimal settings remain unchanged. |

Use explicit decimal strings for expected values. Include numbers large enough to exercise negative division scale and the largest supported source text sizes.

## Completion criteria

The module builds and all phase tests and shared checks pass. Domain packages have no Protobuf, gRPC, configuration, or observability imports. The domain may import the pure shared model module, but not `marketgrpc`. The pinned model builds without a local Market Data checkout. No indicator implementation is claimed in this phase.

Next: [Phase 3](03-calculations.md).
