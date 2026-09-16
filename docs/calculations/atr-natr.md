# ATR and NATR

Market Analyzer implements Wilder's Average True Range (ATR) and Normalized ATR
(NATR). ATR measures movement in price units. NATR expresses the same movement
as a percentage of the latest close.

## Input

Both RPCs use a closed candle selection and one setting:

- `period`: positive ATR smoothing period;
- `candle_count`: at least `period + 1`.

The oldest candle provides the previous close for the first True Range. The
first defined ATR is therefore available at zero-based candle index `period`.

## True Range

For each candle after the first source candle:

```text
TR = max(
  high - low,
  abs(high - previous_close),
  abs(low - previous_close)
)
```

This includes price gaps between candles. Calculations run from the oldest
candle to the newest candle.

## Wilder ATR

The first ATR is the arithmetic mean of the first `period` True Range values:

```text
ATR_period = sum(TR_1 ... TR_period) / period
```

Every later value uses Wilder smoothing:

```text
ATR_current = (ATR_previous * (period - 1) + TR_current) / period
```

The public `GetATR` result contains only the value for the last selected candle,
its candle index, and its exclusive closing time. Internal users such as ATR
reversal extrema use the sequence without exposing it through the API.

Example with `period = 3`, initial True Range values `2`, `4`, and `3`:

```text
first ATR = (2 + 4 + 3) / 3 = 3
next TR   = 5
next ATR  = (3 * 2 + 5) / 3 = 3.666666666666667
```

The formatted ATR value is `3.6666667`.

## NATR

NATR uses the intermediate ATR and the close of the same last candle:

```text
NATR = (100 * ATR) / close
```

NATR is a percentage. A returned value of `2` means `2%`. The response also
contains the ATR used and the reference close. NATR is calculated before ATR is
formatted, so output rounding does not feed back into the formula.

## Precision and edge cases

Each division follows the shared [numeric policy](../api.md#numeric-policy).
ATR is formatted with up to 8 significant digits. NATR is formatted with up to
6 decimal places.

- `period = 1` makes the latest ATR equal to the latest True Range.
- If every True Range is zero, ATR and NATR are zero.
- The initial mean depends on the selected history. The same period and ending
  candle can produce a different result when the history starts elsewhere.
- Nonpositive or malformed OHLC input fails source validation.

Algorithm identifiers: `wilder_atr_v1` and `wilder_natr_v1`.
