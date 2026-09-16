# Support and resistance zones

Market Analyzer builds horizontal price zones from confirmed extrema. A zone is
a cluster of nearby extremum prices with enough time-separated touches. ATR
sets the maximum width; the latest close assigns the current role.

## Input

`LevelSettings` contains:

- complete extrema settings;
- `atr_period >= 1` for zone width;
- positive `zone_width_atr`;
- `min_touches >= 2`;
- positive `min_touch_separation_bars`.

The extrema ATR period and the zone-width ATR period are independent. If both
periods match, the internal sequence can be reused within the request.

## Maximum width

Analyzer calculates the latest Wilder ATR from the selected candles:

```text
maximum_zone_width = latest_ATR * zone_width_atr
```

The width is a maximum grouping distance, not a radius around a center.

## Grouping

Confirmed extrema are sorted by price. Starting with the lowest ungrouped price
`p`, Analyzer puts every following point with this condition into the group:

```text
point_price <= p + maximum_zone_width
```

The anchor stays fixed at the group's first price. Grouping is therefore not
transitive: a chain of individually close prices cannot extend the group beyond
`p + maximum_zone_width`. A point exactly on the boundary is included.

After one group is complete, the next ungrouped point starts another group.
Zones are returned in ascending price order.

## Touches

All extrema remain members of the price group. To count independent touches,
their source candle indices are sorted chronologically. The first candle counts.
A later candle counts only when:

```text
current_index - last_accepted_index >= min_touch_separation_bars
```

Distance equal to the setting is accepted. A group is retained only when its
accepted touch count is at least `min_touches`.

The touch filter affects the count and first and last touch times. It does not
remove extrema from the group or change its bounds.

## Bounds and role

For a retained group:

```text
lower_bound          = minimum group price
upper_bound          = maximum group price
representative_price = (lower_bound + upper_bound) / 2
```

The latest source candle close assigns the role:

| Condition | Role |
| --- | --- |
| close above upper bound | `SUPPORT` |
| close below lower bound | `RESISTANCE` |
| close inside or on the bounds | `AT_PRICE` |

The role only describes position relative to the latest close. It does not
claim that a breakout, bounce, or retest occurred. A zone formed by old highs
can become support when price moves above it.

## Result and edge cases

Each zone contains bounds, midpoint, role, references to all grouped extrema,
accepted candle indices, touch count, and first and last accepted touch times.
The full result also contains all confirmed extrema, latest ATR, maximum width,
and reference close.

- No extrema or no group with enough touches returns an empty zone list.
- Zero ATR gives zero maximum width, so only equal extremum prices group
  together.
- New candles can change the latest ATR, confirmed extrema, grouping, bounds,
  and role. Zones are not stable across different source selections.
- Every decision uses unformatted decimal values.

Algorithm metadata includes the extrema algorithm, `wilder_atr_v1`, and
`pivot_zones_v1`.
