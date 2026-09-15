# Market Analyzer

Status: draft for review. Date: 2026-09-14.

## Summary

Market Analyzer is a service for technical analysis of market data. It calculates indicators, identifies trends, and finds support and resistance levels. Its purpose is to turn price and trading activity data, such as candles, ticker data, and trading volumes, into analysis results that other applications can use.

## Context

Market Data is an existing service that provides exchange market data over gRPC. It supplies candles, ticker prices, bid and ask quotes, trading volumes, and other market statistics.

Market Analyzer uses these source data to calculate technical indicators, identify trends, and find support and resistance levels. Market Data handles data collection from exchanges; Market Analyzer handles the calculations requested by clients.

Integration references: [API libraries](https://github.com/imbpp123/market-data/tree/4e5ce32a4847738e99e786e342054cbbced632c5/api), [Protobuf schema](https://github.com/imbpp123/market-data/blob/4e5ce32a4847738e99e786e342054cbbced632c5/api/proto/marketdata/v1/market_data.proto), and [client guide](https://github.com/imbpp123/market-data/blob/4e5ce32a4847738e99e786e342054cbbced632c5/api/CLIENT_GUIDE.md).

## Version 1 Functional Requirements

Version 1 supports only the following calculations:

- ATR (Average True Range).
- NATR (Normalized Average True Range).
- Price extrema detection (pivot highs and lows).
- Local trend detection.
- Global trend detection.
- Support level detection.
- Resistance level detection.

The service must meet these requirements:

- Clients request analyses and receive results through gRPC.
- Clients can request ATR, NATR, price extrema, trend analysis, or support and resistance levels separately and supply the required calculation parameters. Trend analysis returns both local and global trends.
- Each valid analysis request fetches the required exchange data from Market Data and calculates its result.
- Every successful response includes the result and all raw data used in the calculation.
- Analyzer does not cache data or results and does not apply rate limits.
- An HTTP server exposes operational endpoints, such as health and readiness. Analysis is available only through gRPC.

Calculations may reuse other calculations, such as ATR or NATR for level detection.

## Proposed Solution

### Programming Language

Use Go with Protobuf and `grpc-go`. The existing Market Data Go client removes the need to build another upstream client. The workload needs ordinary numerical calculations and request orchestration; Python adds no required capability here. gRPC supports generated Go clients and servers from a shared schema; see the [official Go tutorial](https://grpc.io/docs/languages/go/basics/).

Pin the Market Data client and code generators to reviewed versions. The reviewed client module declares Go `1.27.1`; verify the build environment against that requirement before implementation.

### Boundaries

Keep four responsibilities separate:

| Responsibility | Owns |
| --- | --- |
| Domain | Candle value types, range arithmetic, extrema, trend, ATR, NATR, and level calculations |
| Application | Input validation, one request time, range planning, a consumer-owned candle-reader interface, result assembly |
| Infrastructure | Market Data gRPC adapter and exact mapping of values, presence, and upstream errors |
| Transport and entry point | Analyzer Protobuf mapping, gRPC handlers, operational HTTP, configuration, and dependency construction |

Domain and application code do not import generated Protobuf types, gRPC, or observability packages. Inject a clock and the candle reader. Reuse the upstream gRPC channel; a channel is not a data cache.

### Request flow

1. **Validate the request.** Check required parameters, types, allowed values, and parameter compatibility for the selected calculation. Return an error before calling Market Data if validation fails. Do not check actual data completeness or sufficiency at this stage.
2. **Get data from Market Data.** Request the data needed for the selected calculation over gRPC. Return an error if the full required data cannot be obtained.
3. **Validate the received data.** Check that the data matches the requested instrument, time range, timeframe, and other applicable parameters. Check that the data is valid, complete, and sufficient for the calculation. Return an error if any check fails.
4. **Run the calculation.** Calculate the result using the received data and request parameters.
5. **Return the response.** Return the analysis result and all source data from Market Data used in the calculation to the client over gRPC.

Analyzer does not cache data or results and does not apply rate limits. Request parameters and data selection rules are defined separately for each calculation.

### Candle selection

For candle-based calculations, the client provides these required parameters:

- `to`: the point in time for the analysis.
- `candle_count`: the number of source candles.
- `interval`: the candle timeframe.

Analyzer selects the last candle that closed at or before `to` and counts backward for `candle_count` candles, including that last candle. A candle that was still open at `to` is excluded.

For example:

```text
to:           2026-09-15 14:02:30 UTC
candle_count: 60
interval:     1m
```

This selects 60 candles with opening times from `13:02` through `14:01`, inclusive. The last candle closed at `14:02`. The request to Market Data uses the range `[13:02, 14:02)`.

The client can set `to` to the current time or a historical time. Keep the requested `to` separate from the calculated end boundary of the candle range.

Candle boundaries follow the Market Data calendar in UTC. Daily candles start at midnight, weekly candles on Monday, and monthly candles on the first day of each calendar month. Other intervals also follow Market Data rules.

The `candle_count` is the total number of input candles, including any additional history needed by the algorithm. Analyzer does not add candles beyond this count. The indicator period and other calculation parameters are provided separately.

If Market Data cannot provide the full requested range, Analyzer returns an error. It does not shorten the range, fill missing candles, or split the request to bypass Market Data limits. By default, Market Data limits both the request size and available history depth to 1000 candle slots.

Rules for using prices from the selected candles, including the choice of reference price, are defined separately for each calculation.

### Numerical rules

Use `decimal.Decimal` from `github.com/shopspring/decimal v1.4.0` for calculations, matching Market Data. Parse source decimal strings directly into this type without converting them to binary floating-point numbers.

Preserve the original Market Data values in the source data returned to clients. Keep missing optional values distinct from zero.

**Intermediate calculations**

Perform addition, subtraction, and multiplication without extra rounding. Round the result of each division to 16 significant digits. Count significant digits from the first nonzero digit, regardless of the decimal point position.

Use `DivRound` with an explicitly calculated number of decimal places to provide this precision. Do not change global library settings.

Further calculations and comparisons use intermediate values, not values rounded for the client response.

**Rounding rule**

Round to the nearest value. If the value is exactly halfway between two rounding choices, round away from zero.

For example, rounding to two decimal places gives:

- `1.234` → `1.23`.
- `1.236` → `1.24`.
- `1.235` → `1.24`.
- `-1.235` → `-1.24`.

Apply this rule both to intermediate division and to response formatting.

**Response precision**

Return calculated decimal values as strings:

- Use up to 8 significant digits for values other than percentages.
- Use up to 6 decimal places for percentage values.
- Remove unnecessary trailing zeros: `2.500000` becomes `2.5`.

For example, `11 / 3` is represented as `3.666666666666667` during calculation and returned as `3.6666667`. A small value such as `0.0000000012345678` keeps all 8 significant digits.

The same inputs and parameters must produce the same result. Tests for each algorithm must check numerical error over long histories and behavior near rounding boundaries.

### ATR and NATR

ATR measures price volatility, including gaps between candles. It does not identify direction. The calculation uses Wilder smoothing.

NATR expresses ATR as a percentage of the closing price. This allows clients to compare relative volatility across instruments with different prices.

**Request parameters**

- `exchange`, `market`, `symbol`: instrument identity.
- `to`, `candle_count`, `interval`: source candle selection, following the [candle selection rules](#candle-selection).
- `period`: the smoothing period, an integer of at least 1.

Require `candle_count >= period + 1`. The first candle provides the previous closing price for the next candle's True Range. All remaining candles take part in the sequential indicator calculation.

The `period` and `candle_count` have different purposes. For example, a period of 14 can be applied to a history of 300 candles.

**ATR calculation**

For each candle, starting with the second, calculate True Range:

```text
TR = max(
    high - low,
    abs(high - previous_close),
    abs(low - previous_close)
)
```

The first ATR value is the mean of the first `period` TR values:

```text
ATR = sum of the first period TR values / period
```

For each following candle, apply:

```text
ATR = (previous_ATR * (period - 1) + TR) / period
```

Calculate each value in order through the last selected candle. True Range and Wilder smoothing are described in [Fidelity's ATR guide](https://www.fidelity.com/learning-center/trading-investing/technical-analysis/technical-indicator-guide/atr).

**NATR calculation**

```text
NATR = (100 * ATR) / close
```

Use the ATR and closing price of the last selected candle. Multiply by 100 first, then divide.

NATR uses the shared internal ATR calculation. It does not call a separate gRPC method. The normalization formula is described in [TA-Lib's NATR guide](https://ta-lib.org/functions/natr.html).

**Precision**

Apply the common [numerical rules](#numerical-rules):

- Round each division to 16 significant digits.
- Use intermediate values in further calculations.
- Round to the nearest value, with ties rounded away from zero.
- Return ATR with up to 8 significant digits.
- Return NATR with up to 6 decimal places.

Calculate NATR from the intermediate ATR, not from the rounded response value.

**Example**

With a period of 3, the first TR values are `2`, `4`, and `3`:

```text
First ATR = (2 + 4 + 3) / 3 = 3
```

The next TR value is `5`:

```text
Next ATR = (3 * 2 + 5) / 3
         = 3.666666666666667
```

If the closing price of the last candle is `100`, the response contains:

```text
ATR  = "3.6666667"
NATR = "3.666667"
```

ATR uses the instrument's price units. NATR uses percent: `"2"` means `2%`.

**Response**

Return the latest calculated value, without the full indicator series:

- ATR returns the ATR value.
- NATR returns the NATR value, the ATR used, and the closing price.

Each response also includes the calculation parameters, requested `to`, actual data boundaries, and all source candles used.

**Validation and edge cases**

- If `period < 1` or `candle_count < period + 1`, reject the request before calling Market Data.
- After receiving the data, check its completeness and validity.
- Return an invalid source data error for nonpositive OHLC prices.
- With a period of 1, ATR equals the TR of the last candle.
- If all calculated TR values are zero, ATR and NATR are zero.
- Different source history lengths can produce different final values with the same period because the initial mean uses different data.

### Price extrema

Price extrema are local peaks and troughs in price movement:

- **High (`HIGH`)**: a peak followed by a price decline.
- **Low (`LOW`)**: a trough followed by a price increase.

An extremum is confirmed only after enough data about the following price movement becomes available. The confirmation conditions depend on the selected method.

The indicator finds a sequence of confirmed extrema over the selected period.

Three calculation methods are supported:

| Method | How an extremum is identified | Advantages | Limitations |
| --- | --- | --- | --- |
| **Neighboring candles** | Compare a candle's price with a specified number of candles on each side. A high must be strictly above the neighboring values; a low must be below them. | A simple rule that can detect small local price moves. | Does not account for movement size or volatility. Needs later candles for confirmation; equal peaks and troughs may be missed. |
| **Percentage reversal** | Confirm a high after a decline from it by a specified percentage, or a low after a corresponding increase. | Filters out small moves. The percentage threshold accounts for the instrument's price scale. | Does not adapt to changes in volatility. The time needed for confirmation is not known in advance. |
| **ATR-based reversal** | Confirm a point after an opposite price move equal to ATR multiplied by a specified factor. | Measures the size of a move relative to the instrument's volatility. | Requires ATR calculation and initial history. Results depend on the ATR period, multiplier, and the rule for choosing the ATR used in the threshold. |

The first method selects points by their position relative to neighboring candles. The second and third use the size of the opposite price movement. The methods can therefore return different sequences of extrema from the same data.

#### Common input data and parameters

All extrema detection methods use these parameters:

| Parameter | Purpose |
| --- | --- |
| `exchange` | Exchange. |
| `market` | Market type, such as spot or linear. |
| `symbol` | Instrument identifier on the selected exchange and market. |
| `to` | The point in time for the analysis. |
| `candle_count` | Number of source candles. |
| `interval` | Candle timeframe. |
| `price_source` | Prices used to identify extrema. |

All listed parameters are required. Select candles using the [candle selection rules](#candle-selection) and process them in chronological order.

The `price_source` accepts one of two values:

- **`CLOSE`**: identify highs and lows using candle closing prices.
- **`HIGH_LOW`**: identify highs using `high` and lows using `low`.

The `CLOSE` option describes the structure of closing prices. The `HIGH_LOW` option includes peaks and troughs within candles, including their wicks.

Confirmation rules and additional parameters are defined separately for each method. If a method uses ATR, calculate it from the source `high`, `low`, and `close` values, regardless of `price_source`.

#### 1. Neighboring candles — LOCAL_EXTREMA

This method identifies extrema by comparing each candle's price with a specified number of neighboring candles on both sides.

A high must be strictly above all compared values, and a low must be strictly below them. No minimum price movement is required.

**Input parameters**

Use the parameters from [Common input data and parameters](#common-input-data-and-parameters).

One additional parameter is required:

- `pivot_span`: the number of neighboring candles on each side of the candidate. An integer of at least 1.

The calculation requires:

```text
candle_count >= 2 * pivot_span + 1
```

For example, `pivot_span = 2` requires at least 5 candles: the candidate, two candles on the left, and two on the right.

**Price selection**

Depending on `price_source`:

- `CLOSE`: use closing prices to detect both highs and lows.
- `HIGH_LOW`: use `high` to detect highs and `low` to detect lows.

Compare source values before rounding for the response. This method does not use ATR or percentage tolerances.

**Calculation steps**

1. Process candles in chronological order.
2. For each candle, check that it has `pivot_span` neighbors on both sides within the selected history.
3. Compare the relevant candle price with all neighboring values.
4. If it is strictly above every compared value, record a `HIGH`.
5. If it is strictly below every compared value, record a `LOW`.

Candles without enough neighbors are not candidates, but they still provide comparison data for other candles.

**Example**

With `price_source = CLOSE` and `pivot_span = 2`:

```text
Closing prices: 100, 103, 108, 104, 102
                         ^
                        HIGH
```

The middle candle closes above both candles on the left and both candles on the right.

Similarly:

```text
Closing prices: 100, 97, 92, 96, 99
                        ^
                       LOW
```

**Equal values**

If any compared neighbor has the same price, the candidate is not a strict extremum of that kind.

For example:

```text
Closing prices: 100, 103, 108, 108, 102
```

The middle candle is not a strict high because a neighboring candle has the same closing price. The method does not merge equal peaks or troughs into one point.

**Confirmation**

An extremum is confirmed when the last required candle on its right closes.

With `pivot_span = 2`, a point on the one-minute candle `14:00–14:01` can be confirmed after the candle `14:02–14:03` closes, at `14:03`.

Keep the extremum time and confirmation time separate. If the required right-hand neighbors have not closed by the requested `to`, do not return the point.

The first and last `pivot_span` candles of the selected history cannot be extrema in this calculation. Do not request extra candles outside the selected history.

**Result**

Return all confirmed extrema. Each point contains:

- Kind: `HIGH` or `LOW`.
- Source candle index, starting at zero.
- Source candle opening time.
- Extremum price from the selected source.
- Confirmation candle index.
- Confirmation time: the closing time of the last required right-hand neighbor.

The result also includes the common input parameters, `pivot_span`, actual data boundaries, and all source candles used.

Sort points by source candle time. With `HIGH_LOW`, a candle can be both a high and a low. Return both points, with `HIGH` before `LOW`. This is the record order, not a claim about the order of price movements within the candle.

**Validation and limitations**

- Return a parameter error if `pivot_span < 1` or the requested `candle_count` is below the required minimum.
- The received history must be complete and valid.
- No detected extrema is a successful result with an empty list.
- A constant, strictly increasing, or strictly decreasing sequence of the selected prices has no strict extrema.
- Highs and lows do not have to alternate: the method checks each kind independently.
- Increasing `pivot_span` makes the comparison cover more neighboring candles. It does not measure a point's reliability.

### Trend definition and method

Fidelity defines trend through the direction of price peaks and troughs: rising peaks and troughs describe an uptrend; falling ones describe a downtrend; sideways movement remains in a horizontal range. This is a market definition, not a complete automated detector. Source: [Fidelity, Basic concepts of trend](https://www.fidelity.com/learning-center/trading-investing/technical-analysis/basic-concepts-trend).

Proposed method: `swing_structure_v1`. Both trends use the same interval and end time:

- `global_bars` selects the longer window.
- `local_bars` selects its most recent, shorter suffix.
- Require `global_bars > local_bars > 0`.
- Each window has its own required `pivot_span >= 1`. Require `bars >= 2 * pivot_span + 1`.
- A required `equality_tolerance_pct >= 0` defines small price differences. Its price value is `last_close * equality_tolerance_pct / 100`, shared by both windows.

Fetch exactly `global_bars` candles once. The local window uses the last `local_bars` candles from that response. Pivot confirmation requires no extra candles outside these windows.

Illustrative values are `global_bars = 300`, `local_bars = 60`, global span `5`, local span `2`, and tolerance `0.05` percent. These are examples, not defaults or calibrated values.

Use the [price extrema detector](#price-extrema) independently inside each trend window with its own `pivot_span`. Compare the resulting high and low lists separately. The trend tolerance below does not change the extrema detection rules.

For each window, apply these rules in order, using its tolerance value `e`:

1. If the full window's highest high minus lowest low is at most `e`, return `SIDEWAYS` with reason `flat_range`.
2. If fewer than two pivot highs or two pivot lows exist, return `UNDETERMINED` with reason `insufficient_structure`.
3. Compare every adjacent pair in each pivot list. Return `UP` if all high increases and all low increases are greater than `e`. If the latest close is below the latest pivot low minus `e`, return `UNDETERMINED` with reason `structure_broken` instead.
4. Return `DOWN` if all high decreases and all low decreases are greater than `e`. If the latest close is above the latest pivot high plus `e`, return `UNDETERMINED` with reason `structure_broken` instead.
5. Return `SIDEWAYS` if the spread of pivot highs and the spread of pivot lows are each at most `e`, and the latest close is inside `[minimum pivot low - e, maximum pivot high + e]`. The reason is `horizontal_structure`.
6. Otherwise return `UNDETERMINED` with reason `mixed_structure`.

`UP` and `DOWN` use reason `rising_structure` and `falling_structure`. `UNDETERMINED` is an analysis result, not an error or a synonym for sideways. This method is deliberately conservative: a reversal within a long window can make the global result undetermined while the local result is directional. A smooth rise with too few confirmed lows also remains undetermined.

Use all pivots inside each selected window. Comparing only the last pair would not describe the full requested depth. Return the window boundaries, all pivot evidence, tolerance, state, and reason. Do not invent confidence percentages.

### Support and resistance levels

Support is a price area where demand can stop a fall; resistance is an area where supply can stop a rise. Their roles may change after a break. Source: [Fidelity, Support and resistance](https://www.fidelity.com/learning-center/trading-investing/technical-analysis/support-and-resistance).

Proposed method: `pivot_zones_v1`. It finds horizontal candidate zones from repeated confirmed extrema. This is our explicit rule, not a claim to reproduce Fidelity's proprietary chart levels. Trend lines, Fibonacci levels, daily pivot-point formulas, and breakout confirmation are outside this method.

Required parameters:

| Parameter | Meaning and validation |
| --- | --- |
| `lookback_bars` | Detection window; at least `2 * pivot_span + 1` |
| `pivot_span` | [Price extrema](#price-extrema) comparison span; at least 1 |
| `atr_period` | Wilder period; at least 1 |
| `atr_history_bars` | Total ATR source window; at least `atr_period + 1` |
| `zone_width_atr` | Positive decimal multiplier for the latest ATR |
| `min_touches` | Minimum distinct accepted pivot candles; at least 2 |
| `min_touch_separation_bars` | Minimum source-index distance between accepted touches; at least 1 |

Fetch `max(lookback_bars, atr_history_bars)` closed candles once. Detection uses its `lookback_bars` suffix; ATR uses its `atr_history_bars` suffix. Return both ranges. Additional ATR candles are not level candidates.

1. Find confirmed pivot highs and lows in the detection window using the [price extrema detector](#price-extrema).
2. Set maximum zone width `w = zone_width_atr * latest_ATR` in price units.
3. Sort all candidate prices ascending, breaking ties by candle index, then high before low.
4. Start a group with the lowest remaining price `p`. Include consecutive prices at most `p + w`, then start the next group. Anchor each group at its first price; do not chain nearby prices into a zone wider than `w`.
5. Within each group, scan distinct candidate candle indices in time order. Accept the first and then only indices at least `min_touch_separation_bars` after the last accepted index. Two extrema on one candle count as one touch. Keep all candidate evidence and mark which candles counted.
6. Discard groups with fewer than `min_touches` accepted candles. A touch here means a qualifying pivot candle, not every candle whose wick intersects the zone.
7. Set zone bounds to the group's minimum and maximum candidate prices, and representative price to their midpoint. Classify it as `SUPPORT` when the latest close is above its upper bound, `RESISTANCE` when below its lower bound, or `AT_PRICE` when inside the inclusive bounds.
8. Return every retained zone in ascending price order, with bounds, midpoint, role, touch count, first and last accepted touch times, and candidate references.

Return the ATR, its input range, multiplier, and resulting maximum width. Using NATR would be equivalent after converting it back to price units: `close * NATR / 100 = ATR`; no second volatility calculation is needed.

Zero ATR gives zero zone width and groups only equal prices. No eligible zones is a successful empty result. A historical high below the latest close may now be a support candidate; the response role describes current position, not a separately confirmed breakout or retest. Pivot count is evidence, not a probability that the zone will hold.

## Data Model / API / Interfaces

Proposed package: `marketanalyzer.v1`. All five methods are unary:

| RPC | Request beyond the common selector | Result |
| --- | --- | --- |
| `GetTrend` | Global/local bars, global/local pivot spans, equality tolerance percent | Global and local `TrendResult` |
| `GetATR` | Period, history bars | Latest ATR |
| `GetNATR` | Period, history bars | Latest NATR, ATR, reference close |
| `GetExtrema` | `to`, `candle_count`, `pivot_span` | Confirmed pivot highs and lows |
| `GetLevels` | Parameters in the levels table | Zero or more zones and volatility evidence |

Common selector: required `exchange`, `market`, `symbol`, and `interval`, matching Market Data meanings. One request selects one series. Clients call again for another timeframe. Numeric counts use integer Protobuf fields; decimal parameters and values use strings; times use `google.protobuf.Timestamp`. Use field presence for required inputs, so omission is distinct from explicit zero where zero is valid. There are no implicit algorithm defaults in v1.

Every successful response includes:

| Field group | Content |
| --- | --- |
| Identity and time | Echoed series, `evaluated_at`, actual source `from` and `to` |
| Reproducibility | Method version, numeric policy, and effective parameters |
| Source | Chronological `candles`, returned once, including all initialization and confirmation candles |
| Result | Indicator values or analysis states, with source ranges and evidence references |

Analyzer owns its public `Candle` message with the same meanings as Market Data `Kline`: `open_time`, `close_time`, `open`, `high`, `low`, `close`, `volume`, `turnover`, optional `trades_count`, and `fetched_at`. Preserve all fields and their presence. Map at the adapter boundary; do not expose upstream generated types inside the domain. Raw means the Market Data candle values used, not an exchange HTTP payload or reconstructed OHLC data.

Evidence uses zero-based indices into `candles`. A range is `(start_index, count)`. Trend and level pivots contain kind, candle index, price, and confirmation time. Return latest-value time as the final source candle's exclusive `close_time`.

Enums reserve zero for `UNSPECIFIED`; successful results never use it. Trend states are `UP`, `DOWN`, `SIDEWAYS`, and `UNDETERMINED`. Zone roles are `SUPPORT`, `RESISTANCE`, and `AT_PRICE`.

No persistent data model is needed. Generate the concrete schema and client packages during implementation, after reviewing this draft. Changing an algorithm or its rounding rules requires a new method version; do not change meaning silently.

## Failure Modes / Edge Cases

Validate counts and arithmetic without integer overflow. Reject missing fields, unsupported selector values, invalid decimal text, and invalid parameter combinations before fetching data. Do not repair invalid requests with defaults.

Validate source identity, exact requested slot count, strict chronological order, aligned contiguous slots, and exclusive close times. Reject duplicates, gaps, wrong series, nonpositive OHLC prices, negative volume or turnover, negative present trade counts, and invalid OHLC ordering. Require `low <= open <= high` and `low <= close <= high`. Required timestamps and decimal fields must be valid. Preserve zero volume and missing trade counts; do not synthesize bars or reorder malformed responses.

| Condition | gRPC status and Analyzer reason |
| --- | --- |
| Bad caller parameters | `INVALID_ARGUMENT`, `invalid_parameter` |
| Upstream invalid range, retention, or size rejection | `INVALID_ARGUMENT`, `market_data_rejected_request`, with upstream reason |
| Unknown instrument | `NOT_FOUND`, `symbol_not_found` |
| Upstream incomplete candle history | `FAILED_PRECONDITION`, `incomplete_data` |
| Malformed successful upstream response | `DATA_LOSS`, `invalid_market_data` |
| Upstream unavailable or not ready | `UNAVAILABLE`, `market_data_unavailable` |
| Upstream overload or upstream response too large | `RESOURCE_EXHAUSTED`, `market_data_resource_exhausted` |
| Analyzer response exceeds configured transport size | `RESOURCE_EXHAUSTED`, `response_too_large` |
| Caller canceled or effective deadline expired | `CANCELLED`, `request_canceled`, or `DEADLINE_EXCEEDED`, `request_timeout` |
| Upstream authentication, permission, or unexpected internal failure | `INTERNAL`, `market_data_failure`; safe upstream status in details |
| Upstream method unavailable in the deployed contract | `UNIMPLEMENTED`, `market_data_contract_mismatch` |
| Internal calculation failure | `INTERNAL`, `internal_error` |

Preserve upstream `DATA_LOSS`, deadline, and cancellation status in the matching categories above. Attach an Analyzer error detail containing stable `reason` and, when relevant, `upstream_code` and optional `upstream_reason`. Do not parse upstream human-readable error messages. Handle missing and unknown upstream details without panic or raw credential disclosure.

No partial responses: malformed data or failed fetching fails the RPC. A valid trend window with too few pivots returns `UNDETERMINED`; too little actual candle history fails the RPC. Empty level results are valid.

Proposed operational defaults: a 30-second overall request deadline, with an earlier client deadline taking precedence; a 16 MiB upstream receive limit; and a separately configurable 32 MiB Analyzer response limit. The larger response allowance accounts for source candles plus evidence. These are transport and execution bounds, not rate limiting. Never silently truncate a result. Final values must be validated with serialized-response tests.

Propagate cancellation through the adapter and check it during long calculations. Do not add an application retry loop or retry policy. One upstream failure ends that analysis request. Market Data may continue its own shared fill after this caller cancels; Analyzer does not own that work.

## Security / Privacy

Analyzer needs a Market Data endpoint, not exchange credentials. The reviewed Market Data server has no native TLS or authentication. Use its plaintext endpoint only on a trusted connection, or an approved protected gRPC endpoint. Restrict operational HTTP to the deployment's internal network. Authentication and encryption at the Analyzer ingress remain deployment decisions.

## Observability

Proposed operational HTTP endpoints on a separate listener:

- `GET /health`: `200` while the local process is live; no upstream calls.
- `GET /ready`: `200` after valid configuration and listener initialization; `503` during startup and shutdown. It does not claim that all Market Data symbols are ready or fresh.
- `GET /metrics`: request counts by method and status, request and upstream duration, source candle counts, response bytes, and active requests. Do not label metrics with symbols or request IDs.

Log method, series, method version, elapsed time, and safe failure reason. Do not log full candle arrays by default. Set readiness false before graceful shutdown, stop accepting new requests, and finish or cancel in-flight work within the configured shutdown deadline.

## Migration / Rollout Plan

1. Review the algorithm and deployment decisions listed below; pin the integration contract and toolchain.
2. Define Analyzer Protobuf messages and implement pure calculations with the acceptance cases below.
3. Add the application flow, Market Data adapter, and operational listeners; verify real serialization boundaries.
4. Run the service against a test Market Data instance and measure latency, allocations, exact arithmetic cost, and response sizes at supported depths. Document tested bounds without adding caller quotas.

There is no data migration. This work item delivers documentation only.

## Testing / Validation

Unit tests use deterministic candles, an injected clock, and a small candle-reader fake. Use `t.Context()` and follow the repository test rules. Integration tests use a local gRPC server; external exchanges and credentials are not unit-test dependencies.

| Area | Required acceptance cases |
| --- | --- |
| Calendar and ranges | Current partial interval excluded; exact boundary; month length and leap year; Monday week; Binance 3-day anchor; total history includes initialization; UTC behavior |
| ATR/NATR | Hand-calculated seed and recurrence; gaps above/below previous close; period 1; exactly `period + 1` bars; longer history; zero range; tiny/large prices; output rounding and invalid close |
| Extrema | Strict highs and lows; span 1; minimum window; equal-price plateaus; flat and monotonic data; both kinds on one candle; missing edge neighbors; confirmation time; source indices; stable order; empty result; invalid span and count |
| Trend | Rising highs and lows; falling highs and lows; horizontal and flat ranges; mixed, broken, and insufficient structure; equality at tolerance; equal plateaus; one candle with both pivot kinds; independent local/global results; whole-window comparisons |
| Look-ahead | No pivot at the unconfirmed right edge; confirmation time matches its final neighbor; candles outside a local window cannot create local pivots |
| Levels | Separated repeated pivots; insufficient touches; clustered equal prices; width boundary; no transitive over-merging; touch spacing; one candle counted once; zero ATR; empty result; roles above/below/inside zone; deterministic order |
| Source fidelity | Exact decimal strings, timestamps, optional trade count absent versus zero, all source candles, valid source indices and initialization ranges |
| Invalid upstream data | Wrong series, gaps, duplicates, wrong order/calendar, missing timestamps, malformed prices, invalid OHLC or negative quantities |
| Request flow | Same request after upstream data changes returns a newly calculated result; no cache or coalescing; both trends and internal level ATR share the one fetched range |
| Failures and operations | Upstream status/details, missing details, cancellation during fetch and calculation, deadlines, response-size failure without truncation, readiness lifecycle, no HTTP analysis route |

Include concrete trend fixtures with pivot highs `100, 110, 120` and lows `90, 95, 105` for `UP`, and reversed sequences for `DOWN`, with zero tolerance and a latest close that does not break the structure. Mixed highs `100, 110, 105` must not pass the whole-window `UP` rule. With zone width `1`, candidates `100, 100.75, 101.5` form groups `[100, 100.75]` and `[101.5]` before touch filtering.

Once code exists, run formatting, focused tests, and the repository build, vet, unit, race, and configured lint checks. This documentation change needs content, links, and whitespace review; it cannot establish calculation accuracy or service performance.

## Risks / Trade-offs

- The proposed trend and level algorithms are deterministic heuristics. Parameter choices need chart examples before they become accepted rules.
- Strict pivots confirm with a delay, ignore equal-price plateaus, and can report insufficient structure in a visible directional move. The conservative global rule can remain undetermined after a reversal.
- Levels based only on pivots can miss meaningful price areas. A zone role and touch count do not establish its reliability.
- No Analyzer cache means repeated upstream calls and calculations. Upstream limits still apply, and returning every candle increases response size.
- Exact arithmetic avoids hidden input rounding but costs CPU and memory, especially with large decimal strings and long smoothing histories.
- Closed candles come from the Market Data snapshot. Its contract does not reconcile later exchange corrections to confirmed cached candles.

## Open Questions

1. Accept two nested depths on one timeframe as global/local, or use separate timeframes? This draft proposes nested depths.
2. Accept closed candles only in v1, or require the changing open candle? Open-candle analysis needs explicit provisional-result behavior.
3. Accept `swing_structure_v1`, including `UNDETERMINED`, whole-window comparison, strict pivots, and caller-supplied tolerance? Real chart examples should settle the intended behavior.
4. Accept horizontal `pivot_zones_v1` as the meaning of levels, or require another level type or significance rule?
5. Confirm deployment endpoint, ingress security, request deadline, and transport-size settings before release.

## Sources

Definitions were checked on 2026-09-14. Source definitions do not approve our proposed detection rules.

- [Fidelity: Basic concepts of trend](https://www.fidelity.com/learning-center/trading-investing/technical-analysis/basic-concepts-trend).
- [Fidelity: Average True Range](https://www.fidelity.com/learning-center/trading-investing/technical-analysis/technical-indicator-guide/atr).
- [TA-Lib: Normalized Average True Range](https://ta-lib.org/functions/natr.html).
- [Fidelity: Support and resistance](https://www.fidelity.com/learning-center/trading-investing/technical-analysis/support-and-resistance).
- [gRPC: Go basics tutorial](https://grpc.io/docs/languages/go/basics/).
- [Market Data API](https://github.com/imbpp123/market-data/tree/4e5ce32a4847738e99e786e342054cbbced632c5/api).
