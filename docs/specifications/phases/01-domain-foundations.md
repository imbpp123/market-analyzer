# Phase 1: Domain foundations

Status: planned. Dependencies: none.

## Summary

Create the Go foundation and the validated values needed by every calculation. The result is a tested domain package without network access.

## Context and goals

The repository currently contains documentation. This phase establishes a buildable module and implements the common rules in [Data Model](../market-analyzer-v1.md#data-model), [Candle selection](../market-analyzer-v1.md#candle-selection), and [Numerical rules](../market-analyzer-v1.md#numerical-rules).

## Implementation work

1. Inspect the repository and current upstream client module. Choose and document a Go toolchain compatible with the pinned client requirement. Create the module and reproducible build and test commands. Pin `shopspring/decimal v1.4.0`; use the repository-required assertion library for tests.
2. Add domain values under `internal/domain`: instrument, interval, selection, range, candle series, and validated algorithm settings. Keep constructors and helpers private where possible; export only values needed by the application.
3. Represent extrema method settings as exclusive alternatives. Check method parameters and minimum candle counts with overflow-safe arithmetic. Do not add unrequested defaults.
4. Implement UTC interval alignment and backward slot traversal. Calendar months must use calendar arithmetic. Follow the pinned Market Data interval combinations and Binance 3-day anchor. Return errors for ranges before the supported epoch or arithmetic overflow.
5. Implement complete series validation: identity, count, order, contiguous calendar slots, timestamps, positive OHLC, OHLC ordering, and nonnegative quantities. Do not sort or repair invalid inputs.
6. Implement common decimal division and formatting. Division must provide 16 significant digits using explicit `DivRound` scale, including when the required scale is negative. Determine magnitude using decimal/integer operations, not floating-point logarithms. Handle zero explicitly and never change global library precision.
7. Keep output formatting separate from intermediate calculations. Provide the 8-significant-digit and 6-decimal-place percentage formats, with ties away from zero and trimmed trailing zeros.

Raw source records and Protobuf conversion belong to phase 3. This phase handles parsed domain values. Request-time future checks belong to the application; range arithmetic accepts explicit inputs and does not read the clock.

## Interfaces and deliverables

Deliver validated domain values, typed validation errors, calendar helpers, and decimal helpers. Validation errors identify the invalid field or invariant without gRPC status types. Do not add reader interfaces, persistence, or service listeners yet.

Keep one source of validation rules. Later application and transport code must call these rules rather than independently implement algorithm constraints.

## Test cases

| Case | Expected result |
| --- | --- |
| `to = 2026-09-15 14:02:30 UTC`, count 60, interval 1m | Range `[13:02, 14:02)` with 60 slots. |
| `to` exactly on a slot boundary | Last selected candle ends at that boundary. |
| Non-UTC representation of the same timestamp | Same selected range. |
| Weekly, monthly, leap-year, and Binance 3-day ranges | Correct pinned calendar boundaries; no fixed 30-day month assumption. |
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

The module builds and all phase tests and shared checks pass. Domain packages have no Protobuf, gRPC, configuration, or observability imports. No indicator implementation is claimed in this phase.

Next: [Phase 2](02-calculations.md).
