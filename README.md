# Market Analyzer

Market Analyzer is a stateless gRPC service that calculates technical analysis
results from closed market candles. It loads one validated candle range from
[Market Data](https://github.com/imbpp123/market-data) for each request and
returns both the calculation result and the source candles used to produce it.

Supported analyses:

- Wilder ATR and normalized ATR (NATR);
- local, percentage-reversal, and ATR-reversal extrema;
- swing-structure trend classification;
- horizontal support and resistance zones.

The public API is defined in
[`market_analyzer.proto`](api/proto/marketanalyzer/v1/market_analyzer.proto).

## Quick start

Requirements: Docker with Compose support.

```sh
docker compose up --build
```

The local stack starts Market Data and Market Analyzer:

| Endpoint | Address |
| --- | --- |
| Analyzer gRPC API | `127.0.0.1:9091` |
| Analyzer health, readiness, and metrics | `127.0.0.1:8081` |
| Market Data gRPC API | `127.0.0.1:9090` |
| Market Data operations | `127.0.0.1:8080` |

Verify the Analyzer process:

```sh
curl --fail http://127.0.0.1:8081/health
curl --fail http://127.0.0.1:8081/ready
```

See the [quick start guide](docs/quick-start.md) for an example analysis request,
local Go execution, and shutdown instructions.

## Documentation

- [Documentation index](docs/README.md)
- [Quick start](docs/quick-start.md)
- [Architecture](docs/architecture.md)
- [API and data selection](docs/api.md)
- [ATR and NATR](docs/calculations/atr-natr.md)
- [Price extrema](docs/calculations/extrema.md)
- [Trend classification](docs/calculations/trend.md)
- [Support and resistance zones](docs/calculations/levels.md)
- [Operations](docs/operations.md)
- [Development](docs/development.md)
- [Release validation](docs/validation/README.md)

## Development

The project requires Go `1.27.1`.

```sh
go mod download
make check
make generate-check
```

`make check` runs build, vet, unit tests, race tests, lint, formatting checks,
and Git whitespace checks. See the [development guide](docs/development.md) for
the full command set.

## License

[MIT](LICENSE)
