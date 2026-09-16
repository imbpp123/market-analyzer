# Price extrema

An extremum is a confirmed price peak (`HIGH`) or trough (`LOW`). Market
Analyzer supports three methods. They intentionally produce different results:
one uses neighboring positions, one uses a percentage move, and one uses a
volatility-sized move.

All methods process candles chronologically and return only confirmed points.
Each point contains its source candle and time, price, confirmation candle and
time, and method-specific reversal evidence when applicable.

## Price source

`price_source` controls which candle values form candidates:

| Source | High value | Low value |
| --- | --- | --- |
| `CLOSE` | close | close |
| `HIGH_LOW` | high | low |

ATR is always calculated from high, low, and close, regardless of this setting.

## Neighboring candles

Select `local_extrema` and set `pivot_span >= 1`. The required history is:

```text
candle_count >= 2 * pivot_span + 1
```

For each candidate with `pivot_span` candles on both sides:

- it is a high when its selected high value is strictly greater than every
  selected high value in the window;
- it is a low when its selected low value is strictly less than every selected
  low value in the window.

Equal neighboring values reject that kind of extremum. The first and last
`pivot_span` candles cannot be candidates. A point is confirmed when the last
required candle on its right closes. With `HIGH_LOW`, one candle may be both a
high and a low; the response places the high first.

This method does not require highs and lows to alternate.

Algorithm identifier: `local_extrema_v1`.

## Percentage reversal

Select `reversal_percent` and set `0 < reversal_pct < 100`. At least two candles
are required. A value of `2` means a `2%` reversal.

For each current candidate:

```text
threshold = candidate_price * reversal_pct / 100
```

A high is confirmed when a later confirmation price satisfies:

```text
candidate_high - confirmation_price >= threshold
```

A low is confirmed when:

```text
confirmation_price - candidate_low >= threshold
```

The search starts with both high and low candidates because the initial
direction is unknown. A higher high or lower low updates the candidate before
confirmation is tested. An equal candidate price also moves the candidate to
the later candle. If the first candle movement could confirm both directions,
neither is confirmed; the method waits for an unambiguous move.

After the first point, highs and lows alternate. The confirmation candle starts
the candidate for the opposite kind, but cannot confirm that new candidate on
the same candle. The final unfinished candidate is omitted.

With `HIGH_LOW`, candidate updates take priority because the order of a candle's
high and low is unknown. A candle that updates a candidate cannot also confirm
that candidate.

Reversal evidence contains the threshold and the price that confirmed the move.
Algorithm identifier: `reversal_percent_v1`.

## ATR reversal

Select `reversal_atr`, set `atr_period >= 1`, and set a positive
`atr_multiplier`. The required history is:

```text
candle_count >= atr_period + 2
```

The oldest `atr_period` candles prepare the ATR calculation. Detection starts at
zero-based candle index `atr_period`, where the first ATR value is available.

When a candidate is created or updated, its threshold is fixed from the ATR at
that candidate candle:

```text
threshold = candidate_ATR * atr_multiplier
```

Later ATR changes do not change that candidate's threshold. Candidate updates,
confirmation, equal prices, initial ambiguity, alternation, and `HIGH_LOW`
handling follow the percentage-reversal state machine.

A zero threshold cannot confirm a reversal. The result evidence contains the
candidate ATR, threshold, and confirmation price.

Algorithm identifiers: `wilder_atr_v1` and `reversal_atr_v1`.

## Stability and limits

- No confirmed extrema is a valid empty result.
- Reversal methods depend on the start of the selected history.
- Adding later candles does not change an already confirmed reversal point when
  the earlier source data and settings remain unchanged.
- A point time is the source candle opening time. Its confirmation time is the
  confirmation candle closing time.
- All comparisons use intermediate decimal values before response formatting.
