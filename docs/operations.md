# Operations

Market Analyzer runs one gRPC listener for analysis and one HTTP listener for
operations. It keeps no persistent state and shares one plaintext gRPC
connection to Market Data.

## Configuration

Configuration uses environment variables. `MARKET_DATA_ENDPOINT` is required;
all other values have defaults.

| Variable | Default | Meaning |
| --- | --- | --- |
| `MARKET_DATA_ENDPOINT` | none | Market Data gRPC target, for example `localhost:9090` |
| `MARKET_ANALYZER_GRPC_ADDRESS` | `:9091` | Analyzer gRPC listen address |
| `MARKET_ANALYZER_HTTP_ADDRESS` | `:8081` | Operational HTTP listen address |
| `MARKET_ANALYZER_REQUEST_TIMEOUT` | `30s` | Maximum analysis duration; an earlier client deadline wins |
| `MARKET_ANALYZER_SHUTDOWN_TIMEOUT` | `30s` | Graceful shutdown limit |
| `MARKET_ANALYZER_MAX_REQUEST_BYTES` | `65536` | Maximum uncompressed Analyzer request size |
| `MARKET_DATA_MAX_RESPONSE_BYTES` | `16777216` | Maximum uncompressed Market Data response size |
| `MARKET_ANALYZER_MAX_RESPONSE_BYTES` | `33554432` | Maximum uncompressed Analyzer response size |

Durations and sizes must be positive. Empty listener addresses, a missing
Market Data endpoint, invalid values, or listener failures stop startup.

TLS, authentication, authorization, credentials, and network policy are not
implemented by this process. They must be provided by the deployment.

## Operational endpoints

The HTTP listener has no analysis routes.

```sh
curl --fail http://127.0.0.1:8081/health
curl --fail http://127.0.0.1:8081/ready
curl --fail http://127.0.0.1:8081/metrics
```

- `/health` reports that the local process is live.
- `/ready` reports whether Analyzer accepts new analysis calls. It does not call
  Market Data, so dependency failure does not make this endpoint unready.
- `/metrics` exposes Prometheus text metrics for requests, dependency calls,
  calculations, source candle counts, response sizes, and active requests.

Metric labels use bounded RPC, status, and calculation names. They do not use
symbols or arbitrary request values.

## Logs

The process writes JSON logs to standard output. Request logs include RPC,
instrument, selected range, algorithm identifiers, duration, status, and stable
error reason. Full candle and extremum arrays are not logged.

## Shutdown

`SIGINT` and `SIGTERM` start graceful shutdown:

1. readiness closes and new analysis calls are rejected;
2. active gRPC calls may finish within the shutdown timeout;
3. at the deadline, remaining gRPC work is canceled and servers are forced to
   stop;
4. the shared Market Data connection closes after analysis work stops.

The Compose stack uses a 40-second grace period, which is longer than the
default 30-second application timeout.

## Container security

The checked-in image runs as user `65532:65532` from a scratch base. The Compose
services use read-only filesystems, drop all Linux capabilities, enable
`no-new-privileges`, and bind host ports only to `127.0.0.1`.

The local Market Data service uses memory storage. Its retained candle history
is lost when the container stops.
