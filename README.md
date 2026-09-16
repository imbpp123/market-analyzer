# Market Analyzer

A planned stateless gRPC service for technical analysis of market data.
It calculates indicators, identifies trends, and finds support and resistance levels.
Version 1 uses candles from Market Data to calculate ATR, NATR, price extrema, global and local trends, and horizontal support and resistance levels.

Phases 1 through 3 implement the domain, typed application use cases, and the generated gRPC API. Calculations run on one validated in-memory candle set per request. The real Market Data adapter and runnable service are not implemented yet.

## Development

Use Go `1.27.1`, matching the Market Data client module at commit
`4e5ce32a4847738e99e786e342054cbbced632c5` (`api/go/go.mod`).
The module pins `shopspring/decimal v1.4.0` and `testify v1.11.1`.
The upstream client is not a dependency until service integration.

```sh
go mod download
make fmt
make lint
make check
```

`make lint` installs golangci-lint `v2.13.2` into `.bin` when needed and runs it.
`make check` runs build, vet, unit tests, race tests, golangci-lint, formatting,
and Git whitespace checks. If the default Go build cache is not writable, set
`GOCACHE` to a writable directory, for example
`GOCACHE=/tmp/market-analyzer-go-build make check`.

## Documentation

- [Version 1 specification](docs/specifications/market-analyzer-v1.md): requirements, proposed calculation rules, API, integration, and acceptance checks.
- [Implementation phases](docs/specifications/phases/README.md): ordered work, deliverables, test cases, and completion criteria.
- [Development instructions](AGENTS.md): project rules.
- [Domain foundation](docs/domain-foundations.md): package contracts, validation, and verification.
- [Domain calculations](docs/domain-calculations.md): calculation entry points, evidence, ownership, and verification.
- [API and use cases](docs/api-and-use-cases.md): application flow, gRPC boundary, generation, and verification.
