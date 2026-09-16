# Documentation

Market Analyzer exposes deterministic technical analysis calculations over a
gRPC API. Start with the quick start, then use the calculation guides for the
exact formulas and decision rules.

## Get started

- [Quick start](quick-start.md): run the local stack and make an ATR request.
- [API and data selection](api.md): RPCs, candle range rules, responses, and
  errors.
- [Operations](operations.md): configuration, probes, metrics, logs, and
  shutdown.

## Understand the system

- [Architecture](architecture.md): component boundaries, request flow, and
  dependency rules.
- [ATR and NATR](calculations/atr-natr.md): true range, Wilder smoothing, and
  normalization.
- [Price extrema](calculations/extrema.md): neighboring-candle, percentage
  reversal, and ATR reversal methods.
- [Trend classification](calculations/trend.md): tolerance and ordered trend
  decision rules.
- [Support and resistance zones](calculations/levels.md): extrema grouping,
  touch filtering, bounds, and zone roles.

## Contribute and validate

- [Development](development.md): toolchain, checks, code generation, and
  repository layout.
- [Release validation](validation/README.md): live correctness checks,
  controlled load, and benchmarks.
- [Development instructions](../AGENTS.md): repository contribution rules.
