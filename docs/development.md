# Development

## Toolchain

The module requires Go `1.27.1`. Protobuf generation also requires `protoc`.
The Makefile pins:

- `protoc-gen-go v1.36.10`;
- `protoc-gen-go-grpc v1.5.1`;
- `golangci-lint v2.13.2`.

The Market Data Go client is pinned in `go.mod` to the reviewed upstream
contract revision.

## Repository layout

| Path | Content |
| --- | --- |
| `api/proto` | Public Protobuf schema |
| `api/go` | Checked-in generated Go API |
| `cmd/market-analyzer` | Service process |
| `cmd/market-analyzer-validate` | Live correctness runner |
| `cmd/market-analyzer-load` | Controlled live workload runner |
| `internal/domain` | Values, validation, calendars, and calculations |
| `internal/application` | Analysis use cases and external contracts |
| `internal/infrastructure` | Market Data adapter |
| `internal/transport` | gRPC and operational HTTP transports |
| `internal/service` | Configuration and process lifecycle |
| `docs/validation` | Release validation guide and recorded evidence |

## Checks

Install dependencies and run the complete repository checks:

```sh
go mod download
make check
make generate-check
```

`make check` runs:

- `go build ./...`;
- `go vet ./...`;
- `go test ./...`;
- `go test -race ./...`;
- `golangci-lint`;
- `gofmt` verification;
- `git diff --check`.

If the default Go build cache is not writable, use a task-local cache:

```sh
GOCACHE=/tmp/market-analyzer-go-build make check
```

## Generate the API

Install the pinned generators and regenerate checked-in Go files:

```sh
make generate-tools
make generate
```

Verify that generated output is reproducible without changing files:

```sh
make generate-check
```

`PROTO_INCLUDE` defaults to the include directory next to `protoc`. Set it when
well-known Protobuf files are installed elsewhere.

## Benchmarks and live validation

Run deterministic domain and response-construction benchmarks:

```sh
make benchmark
```

Live checks require a running Analyzer backed by Market Data:

```sh
make validate-live \
  VALIDATION_CONFIG=docs/validation/config.local.json \
  VALIDATION_REPORT=validation-report.json

make load-live \
  VALIDATION_CONFIG=docs/validation/config.local.json \
  LOAD_REPORT=load-report.json \
  LOAD_CONCURRENCY=8 \
  LOAD_REQUESTS=200
```

See [Release validation](validation/README.md) for configuration, evidence, and
interpretation rules.

## Contribution rules

Read [`AGENTS.md`](../AGENTS.md) before changing the project. Keep domain
calculations independent from I/O and generated types. Non-trivial behavior
changes require tests for normal, boundary, and failure cases. Do not edit
generated Go files by hand.
