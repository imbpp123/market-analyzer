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
- Local trend detection.
- Global trend detection.
- Support level detection.
- Resistance level detection.

The service must meet these requirements:

- Clients request analyses and receive results through gRPC.
- Clients can request ATR, NATR, trend analysis, or support and resistance levels separately and supply the required calculation parameters. Trend analysis returns both local and global trends.
- Each valid analysis request fetches the required exchange data from Market Data and calculates its result.
- Every successful response includes the result and all raw data used in the calculation.
- Analyzer does not cache data or results and does not apply rate limits.
- An HTTP server exposes operational endpoints, such as health and readiness. Analysis is available only through gRPC.

Calculations may reuse other calculations, such as ATR or NATR for level detection.

## Proposed Solution

### Language and boundaries

Use Go with Protobuf and `grpc-go`. The existing Market Data Go client removes the need to build another upstream client. The workload needs ordinary numerical calculations and request orchestration; Python adds no required capability here. gRPC supports generated Go clients and servers from a shared schema; see the [official Go tutorial](https://grpc.io/docs/languages/go/basics/).

Pin the Market Data client and code generators to reviewed versions. The reviewed client module declares Go `1.27.1`; verify the build environment against that requirement before implementation. Do not silently change the upstream module.

Keep four responsibilities separate:

| Responsibility | Owns |
| --- | --- |
| Domain | Candle value types, range arithmetic, trend, ATR, NATR, and level calculations |
| Application | Input validation, one request time, range planning, a consumer-owned candle-reader interface, result assembly |
| Infrastructure | Market Data gRPC adapter and exact mapping of values, presence, and upstream errors |
| Transport and entry point | Analyzer Protobuf mapping, gRPC handlers, operational HTTP, configuration, and dependency construction |

Domain and application code do not import generated Protobuf types, gRPC, or observability packages. Inject a clock and the candle reader. Reuse the upstream gRPC channel; a channel is not a data cache.

### Request flow and candle selection

1. Validate the request and capture UTC `evaluated_at` once.
2. Set `to` to the start of the interval containing `evaluated_at`.
3. Step backward by the required number of candle slots to obtain `from`.
4. Call `GetKlines` once for the complete `[from, to)` range.
5. Validate the returned identity, calendar, completeness, and candle values.
6. Calculate the selected result and return it with the source candles.

Proposed v1 behavior uses closed candles only. The current open candle is excluded even if it closes while the request is running. There is no caller-supplied historical end time in v1. For example, at `12:03:20 UTC`, a `1m` request ends at `12:03:00` and the last candle opens at `12:02:00`.

Use Market Data calendar rules: days start at UTC midnight, weeks on Monday, and months on the first day. A month is not 30 days. Binance `3d` slots use the `1970-01-02T00:00:00Z` anchor. Support exactly the interval and exchange/market combinations described in the pinned client guide; do not invent aliases or normalize symbols.

There is no preflight ticker or instrument call. The last selected candle close is the reference price. Market Data validates its enabled scopes and current instrument catalog.

The default upstream request and retention bounds are 1000 slots. Additional history counts toward them. Analyzer neither truncates depths nor splits requests to hide an upstream rejection. These are dependency restrictions, not a new Analyzer quota.

### Numerical rules

Preserve upstream decimal strings and optional fields unchanged in returned candles. Use exact decimal values for input comparisons and exact rational arithmetic for calculations in the proposed reference behavior. Go `math/big` can represent these values without binary floating-point conversion.

Round derived values only for serialization: 34 significant decimal digits, round half to even, plain decimal strings without unnecessary trailing zeros. Internal comparisons use unrounded values. Return `numeric_policy = exact_rational_output_34_v1`; this makes rounding part of the contract. No NaN, infinity, or implicit missing-to-zero conversion is allowed. Benchmark the arithmetic with the upstream numeric bounds before release.

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

A confirmed pivot high is a candle whose high is strictly greater than every high in the preceding and following `pivot_span` candles. A pivot low uses strictly lower lows. All neighbors must be inside that trend window. Equal-price plateaus do not create pivots. A candle may qualify as both a high and low; the method compares the two lists independently and does not infer the order of trades inside a candle.

No future candles are fetched. The final `pivot_span` candles cannot yet be confirmed pivots, but still provide confirmation evidence. Return each pivot's source index and the closing time of its final confirming candle.

For each window, apply these rules in order, using its tolerance value `e`:

1. If the full window's highest high minus lowest low is at most `e`, return `SIDEWAYS` with reason `flat_range`.
2. If fewer than two pivot highs or two pivot lows exist, return `UNDETERMINED` with reason `insufficient_structure`.
3. Compare every adjacent pair in each pivot list. Return `UP` if all high increases and all low increases are greater than `e`. If the latest close is below the latest pivot low minus `e`, return `UNDETERMINED` with reason `structure_broken` instead.
4. Return `DOWN` if all high decreases and all low decreases are greater than `e`. If the latest close is above the latest pivot high plus `e`, return `UNDETERMINED` with reason `structure_broken` instead.
5. Return `SIDEWAYS` if the spread of pivot highs and the spread of pivot lows are each at most `e`, and the latest close is inside `[minimum pivot low - e, maximum pivot high + e]`. The reason is `horizontal_structure`.
6. Otherwise return `UNDETERMINED` with reason `mixed_structure`.

`UP` and `DOWN` use reason `rising_structure` and `falling_structure`. `UNDETERMINED` is an analysis result, not an error or a synonym for sideways. This method is deliberately conservative: a reversal within a long window can make the global result undetermined while the local result is directional. A smooth rise with too few confirmed lows also remains undetermined.

Use all pivots inside each selected window. Comparing only the last pair would not describe the full requested depth. Return the window boundaries, all pivot evidence, tolerance, state, and reason. Do not invent confidence percentages.

### ATR and NATR

ATR measures price volatility and includes gaps from the previous close. It does not identify direction. Use Wilder smoothing, as described by [Fidelity's ATR guide](https://www.fidelity.com/learning-center/trading-investing/technical-analysis/technical-indicator-guide/atr).

Both requests require `period >= 1` and `history_bars >= period + 1`. `history_bars` is the total number of source candles, including the initial previous-close candle. It defines the initialization horizon, not a second smoothing period. Return the latest value only.

For chronological candles `C[0]` through `C[m-1]` and period `n`:

```text
TR[i] = max(high[i] - low[i], abs(high[i] - close[i-1]), abs(low[i] - close[i-1]))
ATR[n] = mean(TR[1], ..., TR[n])
ATR[i] = (ATR[i-1] * (n - 1) + TR[i]) / n, for i > n
NATR[i] = 100 * ATR[i] / close[i]
```

NATR is ATR as a percentage of the matching candle close. Source: [TA-Lib, Normalized Average True Range](https://ta-lib.org/functions/natr.html). ATR uses quote asset per base asset unit; NATR uses percent, so `2` means `2%`.

Return methods `wilder_atr_v1` and `wilder_natr_v1`. NATR calls the same internal ATR calculation and also returns the ATR and reference close used. It does not call the Analyzer ATR RPC.

For period `3`, TR values `2, 4, 3` seed ATR at `3`. A following TR of `5` gives ATR `11/3`. If its close is `100`, NATR is also `11/3` percent before output rounding. Zero price movement gives ATR and NATR `0` when the close is positive. Period `1` gives the latest TR before NATR normalization.

The same period with a different history length can produce a different latest ATR because the seed changes. Return the actual history boundaries; do not promise equality with a chart using another initialization horizon.

### Support and resistance levels

Support is a price area where demand can stop a fall; resistance is an area where supply can stop a rise. Their roles may change after a break. Source: [Fidelity, Support and resistance](https://www.fidelity.com/learning-center/trading-investing/technical-analysis/support-and-resistance).

Proposed method: `pivot_zones_v1`. It finds horizontal candidate zones from repeated confirmed extrema. This is our explicit rule, not a claim to reproduce Fidelity's proprietary chart levels. Trend lines, Fibonacci levels, daily pivot-point formulas, and breakout confirmation are outside this method.

Required parameters:

| Parameter | Meaning and validation |
| --- | --- |
| `lookback_bars` | Detection window; at least `2 * pivot_span + 1` |
| `pivot_span` | Strict pivot rule defined above; at least 1 |
| `atr_period` | Wilder period; at least 1 |
| `atr_history_bars` | Total ATR source window; at least `atr_period + 1` |
| `zone_width_atr` | Positive decimal multiplier for the latest ATR |
| `min_touches` | Minimum distinct accepted pivot candles; at least 2 |
| `min_touch_separation_bars` | Minimum source-index distance between accepted touches; at least 1 |

Fetch `max(lookback_bars, atr_history_bars)` closed candles once. Detection uses its `lookback_bars` suffix; ATR uses its `atr_history_bars` suffix. Return both ranges. Additional ATR candles are not level candidates.

1. Find confirmed pivot highs and lows in the detection window.
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

Proposed package: `marketanalyzer.v1`. All four methods are unary:

| RPC | Request beyond the common selector | Result |
| --- | --- | --- |
| `GetTrend` | Global/local bars, global/local pivot spans, equality tolerance percent | Global and local `TrendResult` |
| `GetATR` | Period, history bars | Latest ATR |
| `GetNATR` | Period, history bars | Latest NATR, ATR, reference close |
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
