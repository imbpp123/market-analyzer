# Market Analyzer

Status: draft for review. Date: 2026-09-14.

## Summary

Market Analyzer is a service for technical analysis of market data. It calculates indicators, identifies trends, and finds support and resistance levels. Its purpose is to turn price and trading activity data, such as candles, ticker data, and trading volumes, into analysis results that other applications can use.

## Context

Market Data is an existing service that provides exchange market data over gRPC. It supplies candles, ticker prices, bid and ask quotes, trading volumes, and other market statistics.

Market Analyzer uses these source data to calculate technical indicators, identify trends, and find support and resistance levels. Market Data handles data collection from exchanges; Market Analyzer handles the calculations requested by clients.

Integration references: [API libraries](https://github.com/imbpp123/market-data/tree/4e5ce32a4847738e99e786e342054cbbced632c5/api), [Protobuf schema](https://github.com/imbpp123/market-data/blob/4e5ce32a4847738e99e786e342054cbbced632c5/api/proto/marketdata/v1/market_data.proto), and [client guide](https://github.com/imbpp123/market-data/blob/4e5ce32a4847738e99e786e342054cbbced632c5/api/CLIENT_GUIDE.md).

Implementation work and phase-specific test cases are described in the [implementation phases](phases/README.md).

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

Pin the Market Data client, shared model and conversion modules, and code generators to reviewed compatible versions. The reviewed client module declares Go `1.27.1`; verify the build environment against that requirement before implementation.

### Boundaries

Keep four responsibilities separate:

| Responsibility | Owns |
| --- | --- |
| Domain | Shared market values and calendar, Analyzer selection rules, extrema, trend, ATR, NATR, and level calculations |
| Application | Input validation, one request time, range planning, a consumer-owned candle-reader interface, result assembly |
| Infrastructure | Market Data gRPC adapter, shared Protobuf conversions, and upstream error mapping |
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

#### 2. Percentage reversal — REVERSAL_PERCENT

This method confirms an extremum after an opposite price move of a specified percentage: a high after a sufficient decline, or a low after a sufficient increase.

Until confirmation, the point is a candidate. If the price reaches a new extreme, update the candidate.

This principle is used by the `Peak` and `Trough` functions in the [Wealth-Lab Pro reference published by Fidelity](https://www.fidelity.com/products/atp/content/wsFuncRef_US.pdf). The candle processing rules below define the behavior of this calculation.

**Input parameters**

Use the parameters from [Common input data and parameters](#common-input-data-and-parameters).

One additional parameter is required:

- `reversal_pct`: the opposite price move required to confirm an extremum, expressed as a percentage.

Require `0 < reversal_pct < 100`. A value of `2` means `2%`.

At least two source candles are required. This allows detection but does not guarantee a nonempty result.

**Price selection**

| Source | High candidate | High confirmation | Low candidate | Low confirmation |
| --- | --- | --- | --- | --- |
| `CLOSE` | Highest close | Decline in close | Lowest close | Increase in close |
| `HIGH_LOW` | Highest `high` | Decline in `low` | Lowest `low` | Increase in `high` |

This method does not use ATR or a neighboring candle count.

**Confirmation threshold**

Calculate the threshold from the current candidate price:

```text
threshold = candidate_price * reversal_pct / 100
```

Confirm a high when:

```text
candidate_price - confirmation_price >= threshold
```

Confirm a low when:

```text
confirmation_price - candidate_price >= threshold
```

Reaching the threshold exactly is sufficient. Apply the common [numerical rules](#numerical-rules) and compare values before rounding for the response.

**Initial direction**

At the start of the history, the search direction is unknown. Initialize both a high candidate and a low candidate from the first candle.

Process later candles in order. Update each candidate when a new extreme or an equal price appears, then check reversal conditions for candidates that were not updated on that candle:

- A sufficient decline confirms the first `HIGH`.
- A sufficient increase confirms the first `LOW`.

A candidate can only be confirmed on a later candle. If the same candle meets both eligible confirmation conditions, confirm neither point. Keep updating candidates normally and wait for a candle that gives an unambiguous first confirmation.

**Finding a high**

1. Keep the highest price as the high candidate.
2. If a higher price appears, update the candidate and recalculate the threshold.
3. If the candidate was not updated on the current candle, check for a sufficient decline.
4. When the threshold is reached, record a `HIGH`.
5. Start searching for a low, using the relevant price of the confirmation candle as the initial low candidate.

Do not confirm the new low candidate on that same candle.

**Finding a low**

1. Keep the lowest price as the low candidate.
2. If a lower price appears, update the candidate and recalculate the threshold.
3. If the candidate was not updated on the current candle, check for a sufficient increase.
4. When the threshold is reached, record a `LOW`.
5. Start searching for a high, using the relevant price of the confirmation candle as the initial high candidate.

Do not confirm the new high candidate on that same candle. After the first point, confirmed highs and lows alternate.

**Example**

With `price_source = CLOSE` and `reversal_pct = 5`:

```text
Closing prices: 100 → 104 → 110 → 108 → 106 → 104.5
```

The increase from `100` to `110` confirms the initial low at `100`. The method then searches for a high, with `110` as its candidate:

```text
threshold = 110 * 5 / 100 = 5.5
```

Closes of `108` and `106` do not provide a sufficient decline. The close at `104.5` reaches the threshold and confirms the high at `110`.

If a close at `112` appeared before confirmation, the candidate would update:

```text
threshold = 112 * 5 / 100 = 5.6
confirmation requires close <= 106.4
```

**Equal values**

When the candidate price repeats, keep the latest candle at that price. The percentage threshold stays the same, but this counts as a candidate update.

For example, while searching for a high:

```text
110 → 108 → 110 → 104.5
```

The confirmed high belongs to the second candle at `110`. Apply the same rule to lows.

**Ambiguous movement within a candle**

With `HIGH_LOW`, a candle may both update the candidate and show a sufficient opposite move. The order of its high and low is unknown.

Give candidate updates priority:

- Update the candidate first, including when its price repeats.
- If the candidate was updated, do not confirm it on that candle.
- Check for confirmation starting with the next candle.

Confirm at most one extremum per candle. This rule can miss a fast reversal within a candle, but does not assume an unknown order of price events.

**Confirmation and result**

Return only confirmed extrema. Do not include the final unconfirmed candidate.

Each point contains:

- Kind: `HIGH` or `LOW`.
- Source candle index, starting at zero, and opening time.
- Extremum price.
- Confirmation candle index.
- Confirmation time: the closing time of the confirmation candle.
- Reversal threshold in price units.
- Price that met the confirmation condition.

The result also includes the common input parameters, `reversal_pct`, actual data boundaries, and all source candles used.

Return points in chronological order. Keep the extremum time and confirmation time separate.

**Validation and limitations**

- Return a parameter error for an invalid `reversal_pct` or `candle_count < 2`.
- The received history must be complete and valid, with positive prices.
- No confirmed points is a successful result with an empty list.
- The percentage is measured from the candidate price. Equal percentage increases and declines can have different absolute sizes.
- The time needed for confirmation is not known in advance.
- The method does not adapt the threshold to current volatility.
- Changing the start of the selected history can change the point sequence.
- Adding later candles does not change confirmed extrema when the history start, earlier source data, and parameters stay unchanged.

#### 3. ATR-based reversal — REVERSAL_ATR

This method confirms an extremum after an opposite price move over a distance based on ATR:

```text
reversal_threshold = ATR * atr_multiplier
```

A high is confirmed after a sufficient decline, and a low after a sufficient increase. Unlike a fixed percentage threshold, the required movement depends on the instrument's volatility.

**Input parameters**

Use the parameters from [Common input data and parameters](#common-input-data-and-parameters).

Two additional parameters are required:

- `atr_period`: the ATR calculation period, an integer of at least 1.
- `atr_multiplier`: a positive factor that sets the required opposite movement in ATR units.

For example, `atr_multiplier = 1.5` means a distance of one and a half ATR values.

Require:

```text
candle_count >= atr_period + 2
```

This provides enough data for the first ATR and a possible candidate confirmation on the next candle. It does not guarantee a nonempty result.

**Calculation direction and initial history**

Select the candle range backward from `to`, but perform all calculations from the oldest candle to the newest.

The first `atr_period` candles of the selected history prepare the ATR calculation. The next candle provides the first ATR value and starts candidate detection.

For example, with `candle_count = 60` and `atr_period = 10`:

```text
From oldest to newest:

Candle 1       Previous close for the first TR.
Candles 2–11   First 10 TR values.
Candle 11      First ATR and initial candidates.
Candles 12–60  Extrema detection and confirmation.
```

Thus, detection excludes the 10 oldest candles, not the last 10 candles before `to`.

An unconfirmed candidate may remain at the end of the history. Confirmation depends on the size of a later price move, not on waiting for a fixed number of candles.

The `candle_count` includes all source history, including preparation candles. Do not request extra candles outside the selected range.

**Price selection**

| Source | High candidate | High confirmation | Low candidate | Low confirmation |
| --- | --- | --- | --- | --- |
| `CLOSE` | Highest close | Decline in close | Lowest close | Increase in close |
| `HIGH_LOW` | Highest `high` | Decline in `low` | Lowest `low` | Increase in `high` |

Always calculate ATR from the source `high`, `low`, and `close`, regardless of `price_source`.

**ATR calculation**

Use the calculation from [ATR and NATR](#atr-and-natr):

1. Calculate True Range starting with the second source candle.
2. Calculate the initial ATR as the mean of the first `atr_period` TR values.
3. Calculate ATR for the remaining candles in order using Wilder smoothing.

Extrema detection uses ATR values at the relevant candles, not the final ATR of the entire history. Each calculation uses only data available at that point in the sequence.

**Fixed candidate threshold**

For each candidate, keep:

- Its price.
- Its source candle.
- The ATR of that candle.
- The threshold `ATR * atr_multiplier`.

While the candidate stays unchanged, keep its ATR and threshold unchanged.

When the candidate updates, save the new candle's price and ATR, then recalculate the threshold. An equal extreme price also updates the candidate to the latest candle, including its ATR.

The threshold reflects volatility at the candidate candle. A change in ATR on later candles alone does not confirm a reversal.

**Confirmation conditions**

For a high:

```text
candidate_price - confirmation_price >= candidate_ATR * atr_multiplier
```

For a low:

```text
confirmation_price - candidate_price >= candidate_ATR * atr_multiplier
```

Reaching the threshold exactly is sufficient. Apply the [numerical rules](#numerical-rules). Use the intermediate ATR, not its rounded response value.

**Initial direction**

Create high and low candidates on the first candle with an available ATR.

Apply the initial direction rules of `REVERSAL_PERCENT`, using ATR thresholds:

- A sufficient decline confirms the first `HIGH`.
- A sufficient increase confirms the first `LOW`.
- Do not confirm a candidate on the candle that creates or updates it.
- If one candle can confirm both eligible candidates, confirm neither.
- Keep updating candidates until the first confirmation is unambiguous.

**Calculation steps**

When searching for a high:

1. Track the highest price.
2. If a higher or equal price appears, update the candidate, its candle, ATR, and threshold.
3. If the candidate did not update, check the decline from its price.
4. When the positive threshold is reached, confirm a `HIGH`.
5. Start searching for a low using the relevant price and ATR of the confirmation candle.

When searching for a low, reverse these steps: track the lowest price and wait for a sufficient increase.

After the first confirmed point, highs and lows alternate. Confirm at most one point per candle.

**Example**

The method is searching for a high:

```text
candidate_price: 100
candidate_ATR:   2
atr_multiplier:  1.5

threshold:       2 * 1.5 = 3
```

On later candles, the confirmation price declines:

```text
99 → 98 → 97
```

The moves to `99` and `98` are too small. At `97`, the high at `100` is confirmed if the candidate has not updated during that time.

If ATR changes to `1.5` during the decline, the existing candidate's threshold stays at `3`.

If a new peak appears before confirmation:

```text
new_candidate_price: 102
new_candidate_ATR:   2.4

new_threshold:       2.4 * 1.5 = 3.6
```

The high can now be confirmed on a later candle at a price of `98.4` or below:

```text
102 - 3.6 = 98.4
```

**Equal values and movement within a candle**

Apply the agreed `REVERSAL_PERCENT` rules:

- For equal extreme prices, select the latest candle.
- Candidate updates take priority over confirmation.
- An updated candidate can only be confirmed on a later candle.
- Do not confirm a new opposite-kind candidate on the candle that creates it.

For this method, repeating the price may change the threshold because the new candle's ATR may differ.

With `HIGH_LOW`, these rules avoid assuming the order in which a candle reached its high and low.

**Zero ATR**

Do not confirm a candidate with zero ATR. A zero threshold must not turn a lack of movement into a reversal automatically.

Keep tracking and updating the candidate using the normal rules. On an update, save the new candle's ATR. Confirmation becomes possible when the saved ATR is positive.

**Result**

Return only confirmed extrema. Do not include the final unconfirmed candidate.

Each point contains:

- Kind: `HIGH` or `LOW`.
- Source candle index, starting at zero, and opening time.
- Extremum price.
- ATR saved at the candidate candle.
- Reversal threshold in price units.
- Confirmation candle index.
- Confirmation time: the closing time of the confirmation candle.
- Price that met the confirmation condition.

The result also includes the common input parameters, `atr_period`, `atr_multiplier`, actual data boundaries, and all source candles used, including preparation history.

Return points in chronological order. Keep the extremum time and confirmation time separate.

**Validation and limitations**

- Return a parameter error for invalid parameters or `candle_count < atr_period + 2`.
- The received history must be complete and valid, with positive prices.
- No confirmed points is a successful result with an empty list.
- Do not detect extrema before the first ATR becomes available.
- The time needed for confirmation is not known in advance.
- The threshold reflects the candidate's ATR and does not follow later changes in volatility.
- Changing the start of the history can affect both ATR and the extremum sequence.
- Adding later candles does not change confirmed points when the history start, earlier source data, and parameters stay unchanged.

#### Common result

The indicator returns:

- The selected calculation method.
- Common input parameters and method parameters.
- Actual source data boundaries.
- All source candles used, including preparation history.
- A list of confirmed extrema.

Each point contains:

| Field | Content |
| --- | --- |
| Kind | `HIGH` or `LOW`. |
| Source candle index | Position of the extremum candle in the returned list, starting at zero. |
| Extremum time | Opening time of the source candle. |
| Extremum price | Value from the selected source: `close`, `high`, or `low`. |
| Confirmation candle index | Position of the candle that completed confirmation. |
| Confirmation time | Closing time of the confirmation candle. |

Keep the extremum time and confirmation time separate: a point occurs before enough data becomes available to confirm it.

The `REVERSAL_PERCENT` and `REVERSAL_ATR` methods also return:

- The reversal threshold in price units.
- The price that met the confirmation condition.

The `REVERSAL_ATR` method also returns the ATR saved at the candidate candle.

Sort points by source candle time. In `LOCAL_EXTREMA`, if one candle contains both kinds of extrema, return `HIGH` before `LOW`. This is the record order, not the order of price events within the candle.

Do not return unconfirmed candidates. No confirmed extrema is a successful result with an empty list.

#### Common validation

Before calling Market Data, check:

- All required parameters are present.
- The `price_source` value is supported.
- The selected method's parameters are valid.
- The requested `candle_count` meets the method's minimum candle requirement.

After receiving data, check:

- The data matches the requested exchange, market, instrument, timeframe, and selected range.
- All requested candles are present, with no gaps or duplicates.
- Candles are in chronological order.
- Candle time boundaries are valid.
- Prices are positive and OHLC values are valid: `low <= open <= high` and `low <= close <= high`.

Return an error for invalid parameters or data. Do not replace that error with a successful empty list of extrema.

Use only the selected history. Do not use candles that close after the requested `to` to confirm a point.

Preserve source values according to the [numerical rules](#numerical-rules). All source and confirmation candle indices in the result must refer to the returned candle list.

### Trend detection

Trend describes the direction of price movement through a sequence of peaks and troughs:

- **Uptrend**: highs and lows rise.
- **Downtrend**: highs and lows fall.
- **Sideways movement**: peaks and troughs stay within a horizontal range.

This definition follows [Fidelity's explanation of trend](https://www.fidelity.com/learning-center/trading-investing/technical-analysis/basic-concepts-trend). The classification rules below make it specific for this calculation.

The indicator determines one trend over a selected range. The caller chooses the range's scale and purpose.

Use the [Price extrema](#price-extrema) calculation with the supplied parameters to obtain confirmed extrema.

#### Input data and parameters

Selected range parameters:

- `exchange`, `market`, `symbol`: instrument identity.
- `to`, `candle_count`, `interval`: source candle selection according to [Candle selection](#candle-selection).

Extrema detection parameters:

- `price_source`: `CLOSE` or `HIGH_LOW`.
- Method: `LOCAL_EXTREMA`, `REVERSAL_PERCENT`, or `REVERSAL_ATR`.
- The selected method's parameters, as defined in [Price extrema](#price-extrema).

Price comparison parameter:

- `equality_tolerance_pct`: the allowed price difference, expressed as a percentage, within which values are treated as approximately equal.

All listed parameters are required. Require:

```text
equality_tolerance_pct >= 0
```

The source candle count must meet the selected extrema detection method's requirements.

#### Extrema preparation

Run Price extrema on the selected candles using the supplied method, parameters, and `price_source`.

Use only confirmed points. Unconfirmed candidates do not take part in classification.

Source candles and calculated extrema belong to the same instrument, timeframe, and range. Trend detection does not require an additional candle set.

If `REVERSAL_ATR` is selected, ATR is used inside extrema detection. Trend direction itself is determined by comparing the resulting point prices.

#### Comparison tolerance

The tolerance prevents small price differences from being treated as rising or falling structure.

Convert the percentage to price units:

```text
e = last_close * equality_tolerance_pct / 100
```

For example:

```text
last_close:             100
equality_tolerance_pct: 0.05
tolerance in price:     0.05
```

When comparing the next point with the previous one:

- A difference greater than `e` is an increase.
- A difference below `-e` is a decrease.
- A difference from `-e` through `e`, inclusive, is equality within tolerance.

Zero tolerance means exact value comparison.

The caller supplies the tolerance. It does not change detected extrema; it applies to their price comparisons after detection.

Apply [Numerical rules](#numerical-rules) before rounding results for the response.

#### Direction classification

Apply the checks in the order below. The first matching condition determines the result.

**1. Nearly constant price**

Calculate the full range of selected prices:

- For `CLOSE`: maximum close minus minimum close.
- For `HIGH_LOW`: maximum high minus minimum low.

If this range is at most `e`, return:

```text
SIDEWAYS
reason: flat_range
```

This case does not require extrema.

**2. Insufficient structure**

If fewer than two highs or fewer than two lows were detected, return:

```text
UNDETERMINED
reason: insufficient_structure
```

This is a valid result: source data is valid, but there are not enough confirmed points to compare direction.

**3. Rising structure**

Treat highs and lows as two separate sequences ordered by time.

Rising structure requires:

- Every next high is above the previous high by more than `e`.
- Every next low is above the previous low by more than `e`.

If the last close is below the last confirmed low by more than `e`, return:

```text
UNDETERMINED
reason: structure_broken
```

Otherwise, return:

```text
UP
reason: rising_structure
```

**4. Falling structure**

Falling structure requires:

- Every next high is below the previous high by more than `e`.
- Every next low is below the previous low by more than `e`.

If the last close is above the last confirmed high by more than `e`, return:

```text
UNDETERMINED
reason: structure_broken
```

Otherwise, return:

```text
DOWN
reason: falling_structure
```

Check for broken structure using the last source candle's close, regardless of `price_source`.

**5. Horizontal structure**

Sideways movement requires all of the following:

- The spread of confirmed highs is at most `e`.
- The spread of confirmed lows is at most `e`.
- The last close is inside this inclusive range:

```text
[lowest confirmed low - e, highest confirmed high + e]
```

Spread means the difference between the largest and smallest point prices of the same kind.

Return:

```text
SIDEWAYS
reason: horizontal_structure
```

**6. Mixed structure**

If none of the earlier conditions match, return:

```text
UNDETERMINED
reason: mixed_structure
```

An undetermined direction is not treated as sideways movement.

#### Examples

With zero tolerance:

```text
Highs: 100 → 110 → 120
Lows:   90 →  95 → 105
Last close: 115

Result: UP
Reason: rising_structure
```

```text
Highs: 120 → 110 → 100
Lows:  105 →  95 →  90
Last close: 95

Result: DOWN
Reason: falling_structure
```

```text
Highs: 100 → 110 → 105
Lows:   90 →  95 →  97
Last close: 102

Result: UNDETERMINED
Reason: mixed_structure
```

In the last example, highs do not keep one direction even though lows rise.

#### Result

Return one result for the selected range:

- State: `UP`, `DOWN`, `SIDEWAYS`, or `UNDETERMINED`.
- Classification reason.
- Range parameters and actual boundaries.
- Confirmed extrema used.
- Extrema detection method and parameters.
- Price source.
- Tolerance in percent and price units.
- Last close.
- All source candles used.

Do not calculate a confidence percentage.

#### Validation and limitations

- Validate required parameters, the tolerance, and the selected Price extrema method's settings.
- Received candles must match the selected range and pass common source data checks.
- Apply the selected method's validation rules during extrema detection.
- All source and confirmation candle references must match the returned data.
- Invalid parameters or data produce an error, not `UNDETERMINED`.
- Compare highs and lows as separate sequences. They do not have to alternate.
- Extrema confirmation is delayed, so the latest price moves may not yet appear in the structure.
- Changing the range, extrema method, its parameters, or tolerance can change the result.

This strict method evaluates the full extremum sequence in the selected range. A directional trend requires every consecutive pair of highs and every consecutive pair of lows to keep the direction. A correction can therefore produce `UNDETERMINED` even when the chart appears to mostly rise or fall.

### Support and resistance levels

**Support** is a price area where demand can stop or slow a fall. **Resistance** is a price area where supply can stop or slow a rise. Source: [Fidelity, Support and resistance](https://www.fidelity.com/learning-center/trading-investing/technical-analysis/support-and-resistance).

The service finds horizontal price zones from repeated confirmed extrema. A zone has a lower and an upper bound. It groups peaks and troughs with similar prices.

A detected zone is an analysis result for the selected data. It does not guarantee a future price reaction.

#### Input data and parameters

Common parameters:

| Parameter | Description |
| --- | --- |
| `exchange` | Exchange. |
| `market` | Market. |
| `symbol` | Trading instrument. |
| `to` | End time for candle selection. |
| `candle_count` | Total number of source candles, including calculation initialization. |
| `interval` | Candle timeframe. |

Select candles according to [Candle selection](#candle-selection).

Extrema detection parameters:

| Parameter | Description |
| --- | --- |
| `price_source` | `CLOSE` or `HIGH_LOW`, as defined in [Price extrema](#price-extrema). |
| Extrema detection method | `LOCAL_EXTREMA`, `REVERSAL_PERCENT`, or `REVERSAL_ATR`. |
| Selected method parameters | Values defined for that method in [Price extrema](#price-extrema). |

The client provides extrema detection settings. The service calculates the extrema from the selected candles.

Zone parameters:

| Parameter | Description and constraints |
| --- | --- |
| `atr_period` | ATR period for zone width. An integer of at least `1`. |
| `zone_width_atr` | ATR multiplier that defines the maximum zone width. A positive decimal value. |
| `min_touches` | Minimum number of accepted touches required to keep a zone. An integer of at least `2`. |
| `min_touch_separation_bars` | Minimum distance in candles between accepted touches in one zone. An integer of at least `1`. |

All parameters are required. The `candle_count` must meet the selected extrema method requirements and be at least `atr_period + 1`.

#### Data preparation

Use one set of selected candles:

1. Detect confirmed extrema according to [Price extrema](#price-extrema).
2. Calculate the latest ATR according to [ATR and NATR](#atr-and-natr).
3. Use the last source candle close as the reference price for zone roles.

The ATR for zone width always uses the source OHLC values, regardless of `price_source`.

With `REVERSAL_ATR`, the ATR period for extrema may differ from the ATR period for zone width. If both periods and source histories match, the ATR calculation can be reused.

Do not fetch extra candles outside the selected range. All calculations and comparisons follow [Numerical rules](#numerical-rules).

#### Maximum zone width

Calculate the maximum width in price units:

```text
maximum_zone_width = latest_ATR * zone_width_atr
```

For example:

```text
latest_ATR = 2
zone_width_atr = 0.5
maximum_zone_width = 1
```

In this example, the difference between the highest and lowest extrema prices in one zone cannot exceed `1`.

Use the same maximum width, based on the latest ATR, for all zones in one calculation.

#### Grouping extrema into zones

All confirmed peaks and troughs take part in grouping. They can belong to the same zone. Keep the kind of each extremum in the result.

Group them as follows:

1. Sort extrema by price in ascending order.
2. For equal prices, sort by source candle index. For equal prices and indices, put the peak before the trough.
3. Start a group with the lowest-priced remaining extremum. Its price `p` is the fixed group anchor.
4. Add following extrema while their price is at most `p + maximum_zone_width`.
5. The first extremum above this boundary starts the next group.
6. Repeat until all extrema are processed.

The group anchor does not change when points are added.

For example, with a maximum width of `1`:

```text
Extrema prices: 100, 100.75, 101.5

First group: 100, 100.75
Second group: 101.5
```

The points at `100.75` and `101.5` are close, but belong to different groups. Adding the last point to the first group would increase its width to `1.5`.

Each extremum belongs to only one group.

#### Counting touches

**A touch in this method is a confirmed extremum that passes the minimum spacing rule.** A candle crossing the zone does not count as a separate touch by itself.

For each group:

1. Collect the unique candle indices of its extrema.
2. Process the indices from oldest to newest.
3. Accept the first candle.
4. Accept the next candle only if its index minus the last accepted candle index is at least `min_touch_separation_bars`.

For example:

```text
Candle indices: 10, 12, 16, 23
min_touch_separation_bars = 5

Accepted touches: 10, 16, 23
Touch count: 3
```

If a peak and a trough from the same candle belong to one group, that candle counts as one touch.

A peak and a trough from different candles can count as two touches in one zone if they meet the spacing rule.

Extrema excluded from the touch count by the spacing rule remain in the group and affect its bounds. The spacing rule only affects the touch count.

Keep a group as a zone when its accepted touch count is at least `min_touches`. Discard other groups.

#### Bounds and representative price

For each retained group:

```text
lower_bound = minimum price of all extrema in the group
upper_bound = maximum price of all extrema in the group
representative_price = (lower_bound + upper_bound) / 2
```

The representative price is the zone midpoint. A client can use it to display the zone as one line.

All extrema in the group define its bounds, including those whose candles did not count as separate touches.

The actual width can be smaller than the maximum width. Do not expand the zone artificially. If all prices are equal, both bounds and the representative price are equal.

#### Zone role

Determine the role from the last source candle close:

| Condition | Role |
| --- | --- |
| Last close is above the upper bound | `SUPPORT`. |
| Last close is below the lower bound | `RESISTANCE`. |
| Last close is inside the zone, including its bounds | `AT_PRICE`. |

For a zone from `100` to `101`:

```text
Last close 105   → SUPPORT
Last close 98    → RESISTANCE
Last close 100.5 → AT_PRICE
```

The role describes the zone position relative to the last close. The method does not separately confirm a breakout, bounce, or retest.

A zone formed from historical peaks can therefore have the `SUPPORT` role when the last close is above it.

#### Result

Return all retained zones in ascending price order.

For each zone, return:

- Lower and upper bounds.
- Representative price.
- Role: `SUPPORT`, `RESISTANCE`, or `AT_PRICE`.
- Accepted touch count.
- Opening times of the first and last accepted touch candles.
- References to all extrema in the group, keeping their kinds.
- Candle indices accepted as touches, to distinguish them from other points in the group.

The common result contains:

- Instrument identity and candle selection parameters.
- Actual source time boundaries.
- Extrema method and parameters, including `price_source`.
- Confirmed extrema used in grouping.
- Zone parameters.
- Latest ATR value and its period.
- Calculated maximum zone width.
- Reference price: the last close.
- All source candles used in the calculations, including ATR initialization.

The touch count shows the number of confirmed interactions under this method's rules. It is not the probability that the zone will hold.

#### Validation and edge cases

Before fetching data, validate required parameters, their values, and the minimum candle count for the selected calculations.

After fetching data, validate the completeness and correctness of the source data according to the common document rules.

Additional rules:

- Group only confirmed extrema. Do not use unfinished candidates.
- Include an extremum exactly at `p + maximum_zone_width` in the current group.
- A distance exactly equal to `min_touch_separation_bars` allows a touch to count.
- Zero ATR gives zero maximum width: only extrema with equal prices are grouped together.
- No extrema, or no groups with enough touches, gives a successful result with an empty zone list.
- Insufficient or invalid source data causes an error.

New candles can change the latest ATR. A later calculation can therefore change the grouping of historical extrema, zone bounds, and zone roles. Zones are not guaranteed to remain unchanged across different source selections.

## Data Model

Market Data owns shared market models in `github.com/imbpp123/market-data/pkg/market`. Analyzer uses those models directly and owns its calculation inputs, results, and their relationships. Preparation and consumer handoff are defined in [phase 1](phases/01-market-data-prepare.md). It has no database entities or persistent identifiers. All objects exist within one analysis request.

### Domain values

| Value | Responsibility |
| --- | --- |
| Shared market values | `market.Exchange`, `market.Market`, `market.Timeframe`, `market.Instrument`, `market.Kline`, `market.Ticker`, and `market.MarketStats`; use the types needed by each calculation. |
| `CandleSelection` | Shared exchange, market, and timeframe values, exact symbol, requested `to`, and total `candle_count`. No catalog lookup is needed. |
| `CandleRange` | Calculated inclusive `from` and exclusive `to`, using the interval calendar. |
| `CandleSeries` | Instrument, interval, actual range, and a complete chronological list of shared `market.Kline` values. |
| `ATRSettings` | Smoothing period. |
| `ExtremaSettings` | Price source and exactly one of the three method settings. |
| `TrendSettings` | Extrema settings and equality tolerance percent. |
| `LevelSettings` | Extrema settings, ATR period for zone width, width multiplier, minimum touches, and touch spacing. |

Use `decimal.Decimal` for domain decimal values and the shared [numerical rules](#numerical-rules). Use the shared model module’s calendar operations in the domain; do not duplicate them in Analyzer. The application selects the required range and coordinates its loading. Pure shared models are allowed in inner layers; the Protobuf conversion module is not.

Values must satisfy their invariants before calculation. A validated `CandleSeries` has matching identity, correct slot boundaries, positive OHLC values, valid OHLC ordering, and no missing or duplicate slots. Calculations must not modify their input series or settings.

`ExtremaSettings` has three concrete alternatives:

- `LocalExtremaSettings`: `pivot_span`.
- `PercentReversalSettings`: `reversal_pct`.
- `ATRReversalSettings`: `atr_period` and `atr_multiplier`.

Use the alternatives directly. Do not represent a method with a set of unrelated optional fields that permits invalid combinations.

### Calculation results

| Result | Content |
| --- | --- |
| `ATRResult` | Latest ATR and its source candle index. |
| `NATRResult` | Latest NATR, the ATR used, reference close, and source candle index. |
| `Extremum` | Kind, source candle index, source price, confirmation candle index, and method-specific evidence. |
| `ExtremaResult` | Ordered confirmed extrema. |
| `TrendResult` | One state and reason, tolerance in price units, and reference close. |
| `PriceZone` | Bounds, midpoint, role, references to grouped extrema, and accepted touch candle indices. |
| `LevelsResult` | Ordered zones, latest ATR, maximum zone width, and reference close. |

An extremum's opening time and confirmation time are obtained from its source and confirmation candles. Reversal evidence contains the threshold and confirmation price; ATR reversal also contains the candidate ATR. Evidence that does not apply to the selected method is absent.

A zone references extrema by their zero-based positions in the calculation's `ExtremaResult`. Accepted touches reference zero-based positions in the source candle list. Keep these two index types distinct. A zone's touch count and first and last touch times are derived from its accepted candle indices.

These are value objects and calculation results. They do not need generated IDs, repositories, or an inheritance hierarchy.

### Domain calculations and composition

Implement the following responsibilities as concrete domain functions or small stateless services:

| Calculation | Inputs and responsibility |
| --- | --- |
| ATR calculation | Validated candles and period; calculate Wilder ATR values in chronological order. |
| NATR calculation | Intermediate ATR and matching close; calculate the normalized value. |
| Extrema detection | Validated candles and extrema settings; execute the selected method. ATR reversal uses the shared ATR calculation. |
| Trend classification | Validated candles, confirmed extrema, price source, and tolerance; apply the strict ordered classification rules. |
| Zone construction | Confirmed extrema, latest ATR, reference close, and zone settings; group points, count touches, and assign roles. |

The public ATR operation returns only the latest value. Internally, ATR reversal needs values at candidate candles. Use one ATR calculation that can provide the internal sequence, with the first value at source index `period`. Do not invent zero ATR values for earlier candles.

The application composes these calculations for each use case:

- ATR: calculate ATR and select its latest value.
- NATR: calculate ATR, then normalize its latest value.
- Extrema: detect extrema, using ATR when the selected method requires it.
- Trend: detect extrema, then classify one trend from the same candles and points.
- Levels: detect extrema, calculate the latest ATR for zone width, then construct zones.

For levels with ATR reversal, reuse an ATR sequence when the period and source history match. Different periods require separate sequences. Reuse lasts only within the current request; it does not create a cache shared by requests.

Internal trend classification and zone construction can accept calculated extrema. Clients provide extrema settings, not precomputed points. Domain calculations never call Analyzer RPCs or Market Data.

Use concrete calls for internal calculations. Interfaces are needed at external boundaries, not for every calculation. Long loops must honor request cancellation; a standard Go context may be passed for this purpose without adding I/O or transport dependencies.

## API / Interfaces

### Application and Market Data boundary

The application exposes five typed use cases matching the five analyses. Each accepts application-owned input types and a request context, and returns a typed analysis response or an application error. Transport code converts Protobuf messages to and from these types.

Define a consumer-owned `CandleReader` interface in the application. Its operation accepts an instrument, interval, and calculated `[from, to)` range, and returns the source series or a typed dependency error. The Market Data adapter implements this interface with `MarketDataService.GetKlines`.

For a valid request, plan one range and make one application-level `GetKlines` call. Do not fetch tickers, request separate histories for dependent indicators, retry in an application loop, or combine concurrent requests. Reuse the gRPC connection across requests.

Keep two representations of source data within the request:

- Shared `market.SourceKline` records preserve Market Data strings, timestamps, and optional values for the response.
- Shared `market.Kline` values contain parsed numbers for validation and calculation.

Both lists have the same length and index order. The infrastructure adapter calls `github.com/imbpp123/market-data/pkg/marketgrpc` to decode upstream messages once into shared models and source records. Market Data owns these conversions. Analyzer owns its analysis response mapping and RPC error mapping. The adapter removes upstream generated types at the boundary. Neither the domain nor the application imports Market Data Protobuf types.

Inject the candle reader and a clock. Capture `evaluated_at` once at request start. It records processing time and does not replace the caller's `to`. Reject a selected range whose end is after the current closed-candle boundary; do not silently clamp it. The calculated range must also satisfy Market Data timestamp and calendar constraints.

### gRPC service

Use Protobuf package `marketanalyzer.v1` and service `MarketAnalyzerService`. All methods are unary. Define typed request and response messages for each method.

| RPC | Request | Result |
| --- | --- | --- |
| `GetATR` | Selection and `ATRSettings` | Latest ATR. |
| `GetNATR` | Selection and `ATRSettings` | Latest NATR, ATR used, and reference close. |
| `GetExtrema` | Selection and `ExtremaSettings` | All confirmed extrema. |
| `GetTrend` | Selection and `TrendSettings` | One trend result and all extrema used. |
| `GetLevels` | Selection and `LevelSettings` | All retained zones, all extrema used in grouping, and volatility evidence. |

Each request contains a required `selection` with `exchange`, `market`, `symbol`, `to`, `candle_count`, and `interval`. One request selects one instrument and one interval. Field meanings follow [Candle selection](#candle-selection) and the pinned Market Data contract. Preserve exact symbol spelling and interval case.

`ExtremaSettings` contains required `price_source` and a Protobuf `oneof` with:

| Alternative | Fields |
| --- | --- |
| `local_extrema` | `pivot_span` |
| `reversal_percent` | `reversal_pct` |
| `reversal_atr` | `atr_period`, `atr_multiplier` |

The selected alternative identifies the method; do not add a second method selector that can disagree with it. A missing method is invalid.

`TrendSettings` contains required `extrema` and `equality_tolerance_pct`. `LevelSettings` contains required `extrema`, `atr_period`, `zone_width_atr`, `min_touches`, and `min_touch_separation_bars`. In level requests, `extrema.reversal_atr.atr_period` controls pivot detection, while `levels.atr_period` controls zone width.

Use presence for required scalar inputs, including values where explicit zero is valid. Use positive integer counts and periods represented as optional `uint32` fields; validate minimum values and arithmetic before conversion to platform integers. Use `google.protobuf.Timestamp` for times and strings for decimal parameters and outputs. There are no implicit algorithm defaults.

Decimal input parameters use plain base-10 notation with an optional sign and fractional part. Reject whitespace, exponent notation, nonnumeric values, and strings longer than 1,024 characters. Apply each parameter's sign and range rules after parsing. This is a numeric input bound, not a request rate limit. Return calculated decimal strings using [Numerical rules](#numerical-rules).

Enums reserve zero for `UNSPECIFIED`. Reject unspecified or unknown input enum values. Successful result enums never use zero:

- Price source: `CLOSE`, `HIGH_LOW`.
- Extremum kind: `HIGH`, `LOW`.
- Trend state: `UP`, `DOWN`, `SIDEWAYS`, `UNDETERMINED`.
- Zone role: `SUPPORT`, `RESISTANCE`, `AT_PRICE`.

Trend reasons are the stable values defined in [Trend detection](#trend-detection). Clients must handle unknown future reason values without treating them as a known state.

### Response structure and source fidelity

Each typed response contains common metadata, source candles, and its calculation result:

| Part | Content |
| --- | --- |
| Selection | Echoed instrument, interval, requested `to`, and `candle_count`. |
| Time | `evaluated_at`, actual `source_from`, and exclusive `source_to`. |
| Calculation | Effective settings, algorithm versions, and numeric policy version. |
| Source | One chronological `candles` list containing every selected source candle. |
| Result | The typed indicator result and the evidence required by its calculation section. |

Analyzer owns its `Candle` Protobuf message. It preserves the meanings and values of Market Data `Kline`: `open_time`, `close_time`, `open`, `high`, `low`, `close`, `volume`, `turnover`, optional `trades_count`, and `fetched_at`. Raw data means these Market Data values, not an exchange HTTP payload. Do not reconstruct source decimal strings from parsed values or replace missing trade counts with zero.

Source-derived prices in evidence, such as extrema prices, confirmation prices, and the reference close, retain the corresponding original source value. Derived results, including ATR, thresholds, tolerance, and zone values, follow response rounding. All decisions use intermediate values before response rounding. Displayed rounded values can therefore hide a small difference that affected a decision; source candles and effective settings allow recalculation.

Extrema appear once in each response that uses them. Zone `extremum_indices` refer to that list, including points from groups later discarded. Zone `accepted_candle_indices` refer to `candles`. Both lists use zero-based indices. Include point and confirmation times, touch count, and first and last accepted touch times as defined in the calculation sections; these must agree with their referenced records.

ATR and NATR include the final source candle index and its exclusive closing time as the value time. Extrema include method-specific evidence with presence, so an absent candidate ATR cannot be confused with zero. Do not include full ATR sequences or unfinished extrema candidates in public responses.

### Versioning

Keep API schema compatibility separate from calculation behavior. Use the following initial algorithm identifiers:

| Calculation | Identifier |
| --- | --- |
| Wilder ATR | `wilder_atr_v1` |
| NATR | `wilder_natr_v1` |
| Neighboring extrema | `local_extrema_v1` |
| Percentage reversal | `reversal_percent_v1` |
| ATR reversal | `reversal_atr_v1` |
| Strict trend classification | `swing_structure_v1` |
| Horizontal zones | `pivot_zones_v1` |

Return the identifiers of all calculations used, including dependencies. Use numeric policy `decimal_sig16_output8_pct6_v1` for the approved [Numerical rules](#numerical-rules).

Changing calculation or rounding rules requires a new identifier. Do not silently reinterpret existing identifiers. Version 1 accepts only the described methods; selecting historical algorithm versions is not a request feature. Generate the schema and Go client during implementation, and reserve removed Protobuf field numbers and enum values.

## Failure Modes / Edge Cases

### Invalid request or source data

Reject missing fields, invalid settings, unsupported selectors, and impossible calendar ranges before calling Market Data. Minimum candle checks validate the requested count, not the availability of actual data. Use checked arithmetic for counts, periods, and range planning.

After fetching, validate identity, exact slot count, chronological order, contiguous aligned intervals, and exclusive close times. Validate required timestamps and decimal fields, positive OHLC values, `low <= open <= high`, `low <= close <= high`, nonnegative volume and turnover, and nonnegative present trade counts. Preserve valid zero quantities and optional absence. Do not reorder, repair, or synthesize source records.

A failed load or invalid successful upstream response fails the whole analysis. An empty extrema or zone list is valid when the source data passes validation. `UNDETERMINED` is a valid trend result, never a replacement for a source or calculation error.

### Error mapping

The Market Data adapter converts upstream errors to application-owned errors. Only the gRPC transport maps these errors to public statuses and details.

| Condition | gRPC status | Analyzer reason |
| --- | --- | --- |
| Invalid caller parameters or locally invalid range | `INVALID_ARGUMENT` | `invalid_parameter` |
| Market Data rejects parameters, range, retention, or request size | `INVALID_ARGUMENT` | `market_data_rejected_request` |
| Unknown instrument | `NOT_FOUND` | `symbol_not_found` |
| Market Data reports incomplete data | `FAILED_PRECONDITION` | `incomplete_data` |
| Malformed successful response or upstream `DATA_LOSS` | `DATA_LOSS` | `invalid_market_data` |
| Market Data unavailable or not ready | `UNAVAILABLE` | `market_data_unavailable` |
| Upstream overload or response exceeds upstream receive limit | `RESOURCE_EXHAUSTED` | `market_data_resource_exhausted` |
| Analyzer request exceeds transport receive limit | `RESOURCE_EXHAUSTED` | Native transport error; details may be absent. |
| Analyzer response exceeds configured size | `RESOURCE_EXHAUSTED` | `response_too_large` |
| Caller or upstream cancellation | `CANCELLED` | `request_canceled` |
| Effective deadline or upstream deadline expires | `DEADLINE_EXCEEDED` | `request_timeout` |
| Upstream method is unavailable | `UNIMPLEMENTED` | `market_data_contract_mismatch` |
| Other upstream failures, including authentication and permission failures | `INTERNAL` | `market_data_failure` |
| Unexpected local calculation failure | `INTERNAL` | `internal_error` |

Application failures carry an Analyzer `ErrorDetail` with a stable `reason`, optional invalid `field`, and optional `upstream_code` and `upstream_reason`. Do not parse human-readable upstream error messages. Unknown or missing upstream details must not cause a panic. Preserve known status categories even when details are absent; map otherwise unclassified upstream failures to `market_data_failure`.

Native gRPC failures can occur before a handler or after the connection is lost, so custom details are not guaranteed. Clients must handle status-only errors.

### Deadlines and transport bounds

Use an overall request timeout of 30 seconds by default, with an earlier client deadline taking precedence. Apply it to loading, calculation, and response preparation. Pass cancellation through the reader and domain loops. Do not return a partial result after cancellation.

Initial configurable transport limits are 64 KiB for Analyzer requests, 16 MiB for Market Data responses, and 32 MiB for Analyzer responses, measured as uncompressed Protobuf sizes. Check the full Analyzer response size before sending it, including raw candles and evidence. Fail rather than truncate. Native request size rejection may occur before application validation.

These are initial operating settings to validate in load and serialization tests. They do not add request rate limits or promise that every possible result fits the configured transport size. Market Data retention and request constraints still apply independently.

## Operations and Observability

Run analysis gRPC and operational HTTP on separate listeners. Configure listener addresses, Market Data endpoint, request timeout, transport sizes, and shutdown timeout at startup. Pass validated settings into the application; domain calculations do not read environment variables.

Authentication, encryption, network access rules, and other environment security controls are deployment responsibilities. They are outside this service's calculation design.

### Operational endpoints

| Endpoint | Behavior |
| --- | --- |
| `GET /health` | `200` while the local process is live. No Market Data call. |
| `GET /ready` | `200` after valid configuration and listener initialization; `503` during startup and shutdown. |
| `GET /metrics` | Request and dependency metrics. |

Readiness means the Analyzer can accept requests. It does not guarantee that Market Data is reachable, a symbol is available, or source data is fresh. Dependency failures are reported through analysis responses and metrics. There are no HTTP analysis routes or REST gateway.

### Metrics and logs

Record request counts by RPC and status, total duration, Market Data duration, calculation duration, source candle count, serialized response size, and active requests. Keep metric labels bounded; do not use symbols, request IDs, or error message text as labels.

Structured request logs include the RPC, instrument, selected range, algorithm identifiers, duration, status, and stable error reason. Do not log full candle or extrema arrays by default. Observability belongs in transport and infrastructure wrappers, not domain calculations.

### Shutdown

On shutdown, set readiness false and stop accepting new analysis work. Let in-flight requests finish within a configurable shutdown timeout, initially 30 seconds. When it expires, cancel remaining work and stop the servers. Close the shared Market Data connection after request processing stops. The shutdown coordinator must not wait indefinitely for graceful gRPC shutdown.

## Testing / Validation

Use deterministic domain fixtures and the repository's testing rules. Expected values must be explicit and must not be calculated by copying the production algorithm into tests. Use `t.Context()` for tests and an injected clock for application time checks.

### Domain acceptance cases

| Area | Required behavior |
| --- | --- |
| Selection | Historical and current `to`; exact and partial boundaries; count includes initialization; UTC day, Monday week, month and leap-year behavior; Binance 3-day anchor; overflow and unavailable future range rejection. |
| Numerical rules | Exact addition, subtraction, and multiplication; 16 significant digits per division; ties away from zero; 8 significant output digits or 6 percentage decimal places; tiny and large values; comparisons before output rounding. |
| ATR and NATR | Explicit seed and recurrence examples; gaps; period 1; minimum count; zero volatility; changed initialization history; NATR from intermediate ATR. |
| Local extrema | Both price sources; strict neighbors; equal plateaus; both kinds on one candle; edge exclusions; confirmation indices and times; empty result. |
| Percentage reversal | Exact threshold; last equal candidate wins; update before confirmation; no same-candle confirmation; ambiguous initial reversal waits; alternating points; unfinished candidate excluded. |
| ATR reversal | Warmup at the oldest end; first candidate and earliest confirmation indices; candidate ATR frozen until update; equal-price update replaces ATR; zero candidate ATR; both price sources and shared reversal rules. |
| Trend | Every state and reason; flat range before insufficient structure; all adjacent pairs compared; mixed and broken structure; exact tolerance boundaries; independent high and low lists; all extrema methods. |
| Zones | Mixed high and low grouping; fixed lowest-price anchor; exact width boundary; unique candle touches; spacing equality; excluded touch points still affect bounds; zero ATR; empty result; role at both bounds. |
| Composition | Same source series throughout; equal ATR periods give consistent shared values; different periods remain independent; no rounding of intermediates before dependent calculations. |

Keep the approved worked examples as regression cases. In particular, extrema prices `100, 100.75, 101.5` with width `1` form two groups; indices `10, 12, 16, 23` with spacing `5` accept `10, 16, 23`. Build complete candle fixtures as well as direct point fixtures for trend and zone tests.

Check confirmation stability for reversal methods when candles are appended with earlier data and parameters unchanged. Do not apply this invariant to zone grouping: the latest ATR can change its historical groups.

### Application and integration checks

Use a small stateful candle-reader fake for application tests and local gRPC servers for adapter and transport tests. Unit tests must not depend on external exchanges or credentials.

Cover:

- Invalid parameters fail before the reader is called; valid requests read one complete range and return a correctly calculated result.
- Repeating a request after the fake source changes returns the new source and new result. Concurrent requests keep independent state.
- Wrong identity, gaps, duplicate or unordered slots, invalid OHLC, missing timestamps, and invalid quantities fail without a partial response.
- All raw decimal strings, timestamps, and optional trade count values survive adapter and response mapping, including absent versus zero.
- Every candle and extrema reference resolves correctly; returned times, touch counts, and evidence agree with those references.
- Every extrema settings alternative and every result variant survives actual Protobuf serialization. Missing settings, unknown enums, and invalid decimal inputs are rejected.
- Upstream status and optional details map as documented; missing and unknown details are handled.
- Cancellation during loading and calculation, effective deadlines, transport size boundaries, readiness, and bounded shutdown behave as specified.
- All five analyses are available over gRPC and no analysis is exposed over HTTP.

Once implementation exists, run formatting, focused tests, build, vet, unit tests, race tests, and the configured linter. Benchmark full responses with raw data and evidence, not only calculation functions. Documentation checks cover content consistency, section links, and whitespace; they do not prove runtime correctness or performance.

## Risks / Trade-offs

- Confirmed extrema have a delay. Local extrema skip equal plateaus; reversal methods depend on movement thresholds and initialization. A visible move may have too few confirmed points for a trend.
- Strict trend classification can return `UNDETERMINED` after one correction. This is the accepted initial behavior; a softer method would be a separate future design.
- Zones represent repeated confirmed extrema under the selected grouping rule. They do not count every price contact or confirm future reliability.
- The latest ATR sets all zone widths. New candles can regroup older extrema, and nearby points can lie on opposite sides of a group boundary.
- Decimal division and output formatting have defined rounding. Output values can conceal differences used internally near classification boundaries.
- Every request loads data and returns all source candles. Concurrent calculations, large decimal values, and evidence lists increase memory and response sizes; operating settings need measurements.
- Market Data owns collection, retention, and cached source values. Analyzer cannot recover unavailable history or guarantee later exchange corrections are reflected in a repeated request.

## Remaining Implementation Decisions

The calculation rules are defined. The remaining work is implementation and verification, not another choice of trend or extrema behavior.

Before release, record the deployed Market Data endpoint and contract version, pin the toolchain and generators, and validate the proposed timeouts and transport limits with the [release validation measurements](phases/06-release-validation.md). Environment security settings belong to deployment configuration.

## Sources

External sources explain indicator definitions and reference formulas. The calculation sections above define this service's exact behavior.

- [Fidelity: Basic concepts of trend](https://www.fidelity.com/learning-center/trading-investing/technical-analysis/basic-concepts-trend).
- [Fidelity: Average True Range](https://www.fidelity.com/learning-center/trading-investing/technical-analysis/technical-indicator-guide/atr).
- [TA-Lib: Normalized Average True Range](https://ta-lib.org/functions/natr.html).
- [Fidelity: Support and resistance](https://www.fidelity.com/learning-center/trading-investing/technical-analysis/support-and-resistance).
- [Fidelity: Wealth-Lab Pro function reference](https://www.fidelity.com/products/atp/content/wsFuncRef_US.pdf), for the percentage reversal principle.
- [shopspring/decimal v1.4.0](https://github.com/shopspring/decimal/tree/v1.4.0), the selected decimal library.
- [gRPC: Go basics tutorial](https://grpc.io/docs/languages/go/basics/).
- [Market Data API](https://github.com/imbpp123/market-data/tree/4e5ce32a4847738e99e786e342054cbbced632c5/api), including the pinned schema and client guide linked in Context.
