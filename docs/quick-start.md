# Quick start

The fastest way to run Market Analyzer is the checked-in Docker Compose stack.
It starts the compatible Market Data service and builds Analyzer from the
current source tree.

## Requirements

- Docker with Compose support.
- `curl` for operational checks.
- Optional: `grpcurl` and `protoc` for the sample gRPC request.

The pinned Market Data image is `linux/amd64`. Docker Desktop uses emulation on
ARM hosts.

## Start the services

```sh
docker compose up --build
```

Wait until both containers are healthy, then check Analyzer:

```sh
curl --fail http://127.0.0.1:8081/health
curl --fail http://127.0.0.1:8081/ready
```

Both calls return HTTP `200`. Readiness means Analyzer accepts requests. It does
not make a live call to Market Data.

## Request ATR

The server does not enable gRPC reflection, so `grpcurl` must load the checked-in
schema. The example selects 60 completed one-minute Binance spot candles and
calculates a 14-period ATR.

```sh
PROTO_INCLUDE="$(dirname "$(command -v protoc)")/../include"
TO="$(date -u +'%Y-%m-%dT%H:%M:00Z')"

grpcurl -plaintext \
  -import-path api/proto/marketanalyzer/v1 \
  -import-path "$PROTO_INCLUDE" \
  -proto market_analyzer.proto \
  -d "{\"selection\":{\"exchange\":\"binance\",\"market\":\"spot\",\"symbol\":\"BTCUSDT\",\"to\":\"$TO\",\"candleCount\":60,\"interval\":\"1m\"},\"settings\":{\"period\":14}}" \
  127.0.0.1:9091 \
  marketanalyzer.v1.MarketAnalyzerService/GetATR
```

The response contains:

- the latest ATR value and its source candle index;
- effective settings and algorithm identifiers;
- the exact source range;
- all source candles used by the calculation.

Market Data uses a moving in-memory history in this local stack. A historical
`to` value outside that retained range is rejected instead of being silently
changed.

## Run Analyzer without Docker

Start a compatible Market Data service first, then run:

```sh
MARKET_DATA_ENDPOINT=localhost:9090 go run ./cmd/market-analyzer
```

The default Analyzer listeners are `:9091` for gRPC and `:8081` for operations.
See [Operations](operations.md) for all environment variables.

## Stop the stack

```sh
docker compose down --timeout 40
```

Analyzer stops accepting new analysis calls, waits for active calls within the
configured shutdown timeout, and then closes its shared Market Data connection.
