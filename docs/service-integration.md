# Service integration

Phase 4 adds the Market Data gRPC adapter and the runnable Market Analyzer process. One Market Data connection is shared by all requests. Each analysis still makes one `GetKlines` call and does not retry it in an application loop.

## Build

Use Go `1.27.1` and run:

```sh
go build ./cmd/market-analyzer
make check
```

The Market Data Go module is pinned to commit `4e5ce32a4847738e99e786e342054cbbced632c5`.

## Configuration

Configuration uses environment variables. `MARKET_DATA_ENDPOINT` is required. Other settings have defaults.

| Variable | Default | Meaning |
| --- | --- | --- |
| `MARKET_DATA_ENDPOINT` | none | Market Data gRPC target, for example `localhost:9090`. |
| `MARKET_ANALYZER_GRPC_ADDRESS` | `:9091` | Analyzer gRPC listen address. |
| `MARKET_ANALYZER_HTTP_ADDRESS` | `:8081` | Operational HTTP listen address. |
| `MARKET_ANALYZER_REQUEST_TIMEOUT` | `30s` | Overall analysis timeout. An earlier client deadline wins. |
| `MARKET_ANALYZER_SHUTDOWN_TIMEOUT` | `30s` | Maximum graceful shutdown time. |
| `MARKET_ANALYZER_MAX_REQUEST_BYTES` | `65536` | Maximum uncompressed Analyzer request size. |
| `MARKET_DATA_MAX_RESPONSE_BYTES` | `16777216` | Maximum uncompressed Market Data response size. |
| `MARKET_ANALYZER_MAX_RESPONSE_BYTES` | `33554432` | Maximum uncompressed Analyzer response size. |

All durations and sizes must be positive. Invalid settings and listener failures stop startup with a clear error. TLS, authentication, credentials, and network access rules are deployment-owned. Do not put credentials in this configuration example.

Minimal local launch:

```sh
MARKET_DATA_ENDPOINT=localhost:9090 go run ./cmd/market-analyzer
```

## Operations

The HTTP listener has no analysis routes.

```sh
curl --fail http://localhost:8081/health
curl --fail http://localhost:8081/ready
curl --fail http://localhost:8081/metrics
```

`/health` reports that the local process is live. `/ready` reports that the Analyzer is accepting work. It does not call Market Data and stays ready when Market Data is unavailable. `/metrics` exposes request, dependency, calculation, source count, response size, and active request metrics. Labels contain only bounded RPC, status, and calculation values.

Request logs are JSON. They include the RPC, instrument, selected range, algorithm identifiers, duration, status, and stable error reason. Candle and extrema arrays are not logged.

## Shutdown

Send `SIGTERM` or `SIGINT`:

```sh
kill -TERM <pid>
```

The process first disables readiness and rejects new analysis work. Existing requests may finish before the shutdown timeout. At the deadline, remaining gRPC work is canceled and both servers are forced to stop. The shared Market Data connection closes after analysis processing stops.

## Verification

Adapter tests use the real generated Market Data messages and a local fake gRPC server. Process tests use ephemeral local ports and verify probes, dependency failure behavior, listener cleanup, metrics, graceful completion, forced cancellation, and rejection of new work during shutdown. Phase 5 still owns checks against deployed dependency data and load measurements.
