# Market Analyzer

A planned stateless gRPC service for technical analysis of market data.
It calculates indicators, identifies trends, and finds support and resistance levels.
Version 1 uses candles from Market Data to calculate ATR, NATR, price extrema, global and local trends, and horizontal support and resistance levels.

The repository currently contains a draft specification. The service is not implemented.
Go is the recommended implementation language.

## Documentation

- [Version 1 specification](docs/specifications/market-analyzer-v1.md): requirements, proposed calculation rules, API, integration, and acceptance checks.
- [Development instructions](AGENTS.md): project rules.
