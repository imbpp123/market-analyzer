# Domain foundation

Phase 1 adds `internal/domain`. It has no network, Protobuf, gRPC, configuration, or observability code. Calculation rules remain in the [main specification](specifications/market-analyzer-v1.md).

## Contracts

- `Instrument.Validate` and `Interval.Validate` check exact identifiers and supported combinations. Symbols are preserved, including case. The calendar and selector rules follow the Market Data [client guide](https://github.com/imbpp123/market-data/blob/4e5ce32a4847738e99e786e342054cbbced632c5/api/CLIENT_GUIDE.md) at the pinned commit.
- `CandleSelection.Range` validates the selection and returns the closed `[from, to)` range. The requested `To` stays unchanged. `Interval.Floor` and `Interval.Shift` use UTC calendar boundaries. Timestamp limits are the Unix epoch through year 9999. Range arithmetic does not read a clock or enforce upstream retention limits.
- Algorithm settings expose `Validate(candleCount)`. Call it before calculation. Counts and periods use `uint32`; minimum counts use `uint64` arithmetic. `ExtremaSettings.Method` holds one concrete value: `LocalExtremaSettings`, `PercentReversalSettings`, or `ATRReversalSettings`. Missing methods and pointer values are rejected. Multiple methods cannot be represented in this field.
- `NewCandleSeries` checks the expected selection against source identity, interval, range, and candles. It returns a complete validated copy or an error. It checks context cancellation while processing candles. `Candles()` returns another copy, including optional trade counts. Use the constructor before passing a series to calculations; the zero value is not a validated series.
- `ValidationError` contains the invalid field and rule. Candle errors include the source index. Transport status mapping belongs outside the domain.
- `Divide` uses 16 significant digits with an explicit `DivRound` scale. `FormatNumber` uses 8 significant digits, and `FormatPercentage` uses 6 decimal places. Formatting does not change intermediate values. Addition, subtraction, and multiplication use decimal operations directly. Global decimal settings are never changed.

Settings and candles in this phase contain parsed values. Required scalar presence, plain decimal text validation, the 1,024-character input bound, raw source preservation, and Protobuf conversion belong to phase 3. The application will reject unavailable future ranges using an explicit request time. These checks are not inferred from zero-valued parsed fields.

## Validation

Tests beside the code cover calendar alignment and traversal, interval combinations, timestamp and count overflow, settings boundaries, malformed series, ownership, cancellation, and decimal rounding. Numerical fixtures include 1,024-character source values, very small fractions, negative division scales, rounding carries, and concurrent operations.

Run the [development checks](../README.md#development) before extending this package. Phase 2 adds indicator calculations; no indicator result is available in phase 1.
