# API and data selection

Market Analyzer exposes six unary RPCs in the `marketanalyzer.v1` package. The
canonical contract is
[`market_analyzer.proto`](../api/proto/marketanalyzer/v1/market_analyzer.proto),
and generated Go clients are in [`api/go`](../api/go/marketanalyzer/v1).

## RPCs

| RPC | Settings | Result |
| --- | --- | --- |
| `GetATR` | ATR period | Latest Wilder ATR |
| `GetNATR` | ATR period | Latest NATR, ATR, and reference close |
| `GetExtrema` | Price source and one extrema method | Confirmed extrema |
| `GetTrend` | Extrema settings and equality tolerance | Trend state, reason, and extrema |
| `GetLevels` | Extrema and zone settings | Price zones, extrema, and ATR evidence |
| `FindActiveInstruments` | Exchange, market, and optional minimum thresholds | Trading instruments that meet the thresholds |

## Find active instruments

`FindActiveInstruments` requires `exchange` (`binance` or `bybit`) and `market`
(`spot` or `linear`). It returns current instruments with `trading` status,
sorted by symbol. It accepts three optional inclusive minimum thresholds and
one optional NATR setting:

| Field | Meaning |
| --- | --- |
| `min_volume_24h` | Market Data rolling 24-hour `volume`, in base asset units |
| `min_trades_24h` | Market Data rolling 24-hour trade count |
| `min_natr` | Minimum NATR percentage from closed daily candles |
| `natr_period` | Optional NATR smoothing period; default 14, allowed range 1–999 |

For example, `min_natr: "4"` means at least 4%. Decimal thresholds are plain
base-10 strings and must be nonnegative. The trade count must be nonnegative.
An omitted threshold is not applied. A present zero threshold still requires
the corresponding data. An instrument with no trade count cannot meet a trade
count threshold; Market Data currently omits this count for Bybit.
`natr_period` requires `min_natr`. NATR uses `natr_period + 1` closed daily
candles. Market Data receives only the candle interval and time range; Analyzer
calculates NATR from those candles. The upper bound fits Market Data's default
1,000-slot request limit.

The service reads the instrument catalog first. It reads 24-hour statistics
only when a volume or trade count threshold is present. It requests daily
candles and calculates NATR only for instruments that pass those filters.
The NATR comparison uses the unrounded calculation. The response includes the
source volume and trade count when statistics were requested, and the NATR
value and candle time when NATR was requested. Each matched row has its
`exchange`, `market`, `symbol`, base asset, and quote asset.

Statistics and catalog snapshots can have different update times. The
response includes `stats_fetched_at` when statistics were read. A missing
statistics row excludes that instrument. If a required snapshot or candle
range is unavailable, the whole request fails. There is no partial result.
Every statistics row is validated before filtering. An invalid row also fails
the whole request, even when its symbol is not in the instrument catalog.
NATR requires one Market Data candle request per remaining instrument, so a
broad request can exceed the service timeout. Narrow the 24-hour thresholds
before adding a NATR threshold.

Example request for Binance spot:

```json
{"exchange":"binance","market":"spot","min_volume_24h":"1000","min_trades_24h":"100","min_natr":"2","natr_period":14}
```

All decimal parameters and calculated values use plain base-10 strings. Decimal
inputs may have a sign and fractional part. Whitespace, exponent notation,
invalid numbers, and values longer than 1,024 characters are rejected.

## Candle selection

Every request has one required `Selection`:

| Field | Meaning |
| --- | --- |
| `exchange` | `binance` or `bybit` |
| `market` | `spot` or `linear` |
| `symbol` | Exact upstream symbol; case is preserved |
| `to` | Analysis time |
| `candle_count` | Number of source candles |
| `interval` | Candle calendar interval |

Analyzer floors `to` to an interval boundary, excludes the candle that is still
open at `to`, and counts backward by `candle_count`. The resulting Market Data
request uses an inclusive start and exclusive end: `[from, to)`.

Example for 60 one-minute candles:

```text
requested to:  2026-09-15 14:02:30 UTC
source range:  [2026-09-15 13:02:00, 2026-09-15 14:02:00)
source opens:  13:02 through 14:01
```

The requested `to` remains in response metadata. The actual aligned boundaries
are returned as `source_from` and `source_to`. A range ending after the current
closed-candle boundary is rejected; it is not clamped.

Supported intervals are `1m`, `3m`, `5m`, `15m`, `30m`, `1h`, `2h`, `4h`,
`6h`, `12h`, `1d`, `1w`, and `1M`. Week and month boundaries use the
upstream UTC calendar rules.

## Response contract

Every successful calculation response (`GetATR` through `GetLevels`) includes:

- echoed selection and effective calculation settings;
- `evaluated_at`, `source_from`, and exclusive `source_to`;
- algorithm identifiers and numeric policy;
- every source candle used by the calculation;
- a typed result with indices and calculation evidence.

Candle and extremum indices are zero-based. `extremum_indices` in a zone refer
to positions in that response's extrema list. `accepted_candle_indices` refer
to positions in the response's candle list.

The service returns only confirmed extrema. It does not return unfinished
reversal candidates or a full ATR time series.

## Numeric policy

The policy identifier is `decimal_sig16_output8_pct6_v1`:

- decimal arithmetic is used instead of binary floating point;
- every division is rounded to 16 significant digits;
- intermediate values, not formatted response values, drive later decisions;
- general derived values are returned with up to 8 significant digits;
- percentage results are returned with up to 6 decimal places;
- rounding uses nearest value with ties away from zero.

Algorithm identifiers in metadata describe every calculation used:

| Identifier | Calculation |
| --- | --- |
| `wilder_atr_v1` | Wilder ATR |
| `wilder_natr_v1` | NATR |
| `local_extrema_v1` | Neighboring-candle extrema |
| `reversal_percent_v1` | Percentage reversal extrema |
| `reversal_atr_v1` | ATR reversal extrema |
| `swing_structure_v1` | Trend classification |
| `pivot_zones_v1` | Support and resistance zones |

## Source validation

A successful Market Data response must match the requested instrument,
interval, range, and candle count. Candles must be chronological, contiguous,
and aligned to the interval. OHLC prices must be positive and ordered so that
open and close are between low and high. Volume, turnover, and a present trade
count must be nonnegative.

Analyzer does not reorder, repair, or synthesize source records.

## Errors

Application errors include `ErrorDetail` with a stable `reason` and, when
available, an invalid field or upstream code and reason.

| gRPC status | Common reason |
| --- | --- |
| `INVALID_ARGUMENT` | `invalid_parameter`, `market_data_rejected_request` |
| `NOT_FOUND` | `symbol_not_found` |
| `FAILED_PRECONDITION` | `incomplete_data` |
| `DATA_LOSS` | `invalid_market_data` |
| `UNAVAILABLE` | `market_data_unavailable` |
| `RESOURCE_EXHAUSTED` | `market_data_resource_exhausted`, `response_too_large` |
| `CANCELLED` | `request_canceled` |
| `DEADLINE_EXCEEDED` | `request_timeout` |
| `UNIMPLEMENTED` | `market_data_contract_mismatch` |
| `INTERNAL` | `market_data_failure`, `internal_error` |

Native gRPC failures can happen outside the handler, so clients must also
handle status errors without custom details.
