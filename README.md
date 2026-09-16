# Market Analyzer

A planned stateless gRPC service for technical analysis of market data.
It calculates indicators, identifies trends, and finds support and resistance levels.
Version 1 uses candles from Market Data to calculate ATR, NATR, price extrema, global and local trends, and horizontal support and resistance levels.

Phases 1 through 4 implement the domain, typed application use cases, generated gRPC API, Market Data adapter, and runnable service. Calculations run on one validated in-memory candle set per request.

## Development

Use Go `1.27.1`, matching the Market Data client module at commit
`4e5ce32a4847738e99e786e342054cbbced632c5` (`api/go/go.mod`).
The module pins `shopspring/decimal v1.4.0` and `testify v1.11.1`.
The Market Data client is pinned to that reviewed commit.

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

Run Market Data and Market Analyzer locally with Docker Compose:

```sh
docker compose up --build
```

Market Data listens on `127.0.0.1:9090` and `127.0.0.1:8080`. Market Analyzer
listens on `127.0.0.1:9091` and `127.0.0.1:8081`. The pinned Market Data image
is published only for `linux/amd64`; Docker Desktop uses emulation on ARM hosts.

## Documentation

- [Version 1 specification](docs/specifications/market-analyzer-v1.md): requirements, proposed calculation rules, API, integration, and acceptance checks.
- [Implementation phases](docs/specifications/phases/README.md): ordered work, deliverables, test cases, and completion criteria.
- [Development instructions](AGENTS.md): project rules.
- [Domain foundation](docs/domain-foundations.md): package contracts, validation, and verification.
- [Domain calculations](docs/domain-calculations.md): calculation entry points, evidence, ownership, and verification.
- [API and use cases](docs/api-and-use-cases.md): application flow, gRPC boundary, generation, and verification.
- [Service integration](docs/service-integration.md): configuration, launch, operational endpoints, metrics, and shutdown.
- [Release validation](docs/validation/README.md): live RPC matrix, evidence report, benchmarks, and generated-code checks.
