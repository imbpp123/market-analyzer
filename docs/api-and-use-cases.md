# API and use cases

Phase 3 adds application use cases and the `marketanalyzer.v1` gRPC API. The application owns the candle reader contract, source records, request types, response types, and dependency error categories. Generated Protobuf messages stay in the transport package.

## Request flow

Each use case captures one processing time and applies the earlier of the client deadline and the configured request timeout. It validates settings and the selected calendar range before I/O. A valid request performs one candle read for one planned range.

The application preserves source records and parses each decimal field once for domain validation and calculation. It does not cache or combine requests. A source failure or invalid source record fails the full request.

The gRPC transport validates required Protobuf presence and enums. It formats derived decimals at the response boundary. Source-derived decimal text and optional trade count presence are copied from the source record. The transport measures the full Protobuf response before returning it and does not truncate results.

## API generation

The schema is in `api/proto/marketanalyzer/v1/market_analyzer.proto`. Generated Go messages and gRPC bindings are in `api/go/marketanalyzer/v1`.

Install the pinned generators and regenerate the API:

```sh
make generate-tools
make generate
```

Generator versions are pinned in the `Makefile`: `protoc-gen-go v1.36.10` and `protoc-gen-go-grpc v1.5.1`. `PROTO_INCLUDE` defaults to the include directory next to `protoc`. Set it explicitly when well-known Protobuf files are installed elsewhere.

## Transport construction

Create the application analyzer with a candle reader, clock, and request timeout. Create the gRPC handler with the analyzer and maximum uncompressed response size. Configure the owning `grpc.Server` with matching receive and send limits. Process startup and the real Market Data adapter belong to phase 4.

## Validation

Application tests use a stateful fake reader. Transport tests run a local gRPC server over an in-memory connection. They cover all five RPCs, all extrema alternatives, required field presence, unknown enums, source fidelity, error details, cancellation, and request and response size limits.
