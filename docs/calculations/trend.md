# Trend classification

Market Analyzer classifies one trend for the selected candle range from
confirmed extrema. It does not use moving averages, regression, or ATR. The
chosen [extrema method](extrema.md) defines the swing points supplied to the
classifier.

## Input

`TrendSettings` contains:

- complete extrema settings;
- `equality_tolerance_pct`, a nonnegative percentage.

The tolerance converts the percentage to price units using the last source
candle close:

```text
tolerance = last_close * equality_tolerance_pct / 100
```

For example, a last close of `100` and a tolerance percentage of `0.5` gives a
price tolerance of `0.5`.

## Swing structures

High and low extrema are evaluated as two independent chronological sequences.

- Highs are rising only when every next high is more than `tolerance` above the
  previous high.
- Highs are falling only when every next high is more than `tolerance` below the
  previous high.
- Lows use the same rules.

Equality and changes inside the tolerance are not directional. The classifier
also records the minimum and maximum of each extremum sequence and the minimum
and maximum selected price over all source candles.

## Ordered decision rules

Rules are evaluated in this exact order. The first matching rule wins.

1. If the complete selected price range is no wider than `tolerance`, return
   `SIDEWAYS` with reason `flat_range`.
2. If there are fewer than two confirmed highs or fewer than two confirmed lows,
   return `UNDETERMINED` with reason `insufficient_structure`.
3. If highs and lows are both rising, return `UP` with reason
   `rising_structure`, unless the last close is below the latest low minus the
   tolerance. That break returns `UNDETERMINED` with reason `structure_broken`.
4. If highs and lows are both falling, return `DOWN` with reason
   `falling_structure`, unless the last close is above the latest high plus the
   tolerance. That break returns `UNDETERMINED` with reason `structure_broken`.
5. If the complete high spread and low spread are each no wider than the
   tolerance, and the last close remains between the low and high structures
   with tolerance, return `SIDEWAYS` with reason `horizontal_structure`.
6. Otherwise return `UNDETERMINED` with reason `mixed_structure`.

The final range check in rule 5 is:

```text
last_close >= minimum_low - tolerance
last_close <= maximum_high + tolerance
```

The break checks prevent an old rising or falling structure from being reported
as active after the latest close has already invalidated it.

## Result

The result contains:

- `UP`, `DOWN`, `SIDEWAYS`, or `UNDETERMINED`;
- the stable reason string;
- tolerance in price units;
- the last close used as reference;
- all confirmed extrema used for classification.

`UNDETERMINED` is a valid analysis result, not an error. It means the confirmed
structure does not justify a directional label under these strict rules.

Algorithm metadata includes the selected extrema algorithm and
`swing_structure_v1`.
