# Phase 4: API and use cases

Status: planned. Dependency: [Phase 3](03-calculations.md).

## Summary

Expose the domain calculations through typed application use cases and a generated gRPC contract. Use a fake candle reader until the real adapter is added in phase 5.

## Context and goals

Implement [API / Interfaces](../market-analyzer-v1.md#api--interfaces), [Request flow](../market-analyzer-v1.md#request-flow), and the application parts of [Failure Modes / Edge Cases](../market-analyzer-v1.md#failure-modes--edge-cases). The result must preserve source data and calculation evidence through actual Protobuf serialization.

## Implementation work

1. Add application-owned request, response, and error types under `internal/application`. Define the consumer-owned `CandleReader` boundary and inject it with a clock. The reader accepts a planned range and returns source records or a typed dependency error.
2. Implement the five use cases. Validate settings, capture request time, plan one range, load it once, validate decoded source data, run domain calculations, and assemble the typed result. Do not fetch separate histories for nested calculations.
3. Use the shared source records and parsed `market.Kline` values delivered by the reader in matching order. The real reader will use the phase 1 converters; application code does not parse upstream values again. Fake readers return the same plain shared types. Preserve source timestamps and optional fields, and build evidence without duplicating arrays.
4. Apply the effective request deadline and propagate cancellation through reads and calculations. Check requested future ranges against the injected clock without replacing the client's `to`.
5. Define `marketanalyzer.v1.MarketAnalyzerService` under `api/proto/marketanalyzer/v1`, with the five RPCs specified in the main document. Generate Go messages and client/server bindings under `api/go/marketanalyzer/v1`. Pin generators and document regeneration commands.
6. Represent extrema settings with a `oneof`. Use required field presence and distinguish the ATR period for extrema from the ATR period for level width. Add typed results, method-specific evidence, and common metadata.
7. Add gRPC mapping and handlers under `internal/transport/grpc`. Keep generated messages and `marketgrpc` out of application and domain packages. Analyzer owns conversion of its analysis-specific results to its own Protobuf contract; upstream model conversions are not reimplemented here. Map validation and dependency errors to the specified statuses and details.
8. Assemble algorithm identifiers, numeric policy, source boundaries, requested selection, and `evaluated_at`. Format derived values only at the response boundary. Preserve source-derived prices from their original records.
9. Check the full serialized response against the configured limit before sending. Configure transport receive and send limits. Handlers must not truncate successful results.

Use the service's existing module structure if one is established in phase 2. Do not create an extra Go module only to hold generated code. A live Market Data adapter, process startup, and HTTP listeners belong to phase 5.

## Application test cases

| Case | Expected result |
| --- | --- |
| Each of five analyses with valid selection and settings | One reader call for the expected range and a fully checked result. |
| Missing settings, invalid decimal text, unsupported source or invalid count | Error before loading data. |
| Historical `to` versus current processing time | Selection remains historical; `evaluated_at` records the injected request time. |
| Selected end beyond current closed-candle boundary | Rejection without clamping or reading. |
| Reader returns wrong identity, missing/duplicate/unordered slots, invalid timestamps or OHLC | Whole request fails; no success payload. |
| Same request after the fake source changes | New source records and a result calculated from them. |
| Concurrent identical requests | Separate loads and independent results; no coalescing or shared state. |
| Levels using matching and different ATR periods | Correct composition from one source range. |
| Empty extrema or zones, or undetermined trend | Successful typed response with all source records. |
| Reader failure or calculation cancellation | Error propagated; no partial result. |
| Client deadline earlier than configured timeout | Earlier deadline wins and reaches the reader and calculation. |

## gRPC and serialization test cases

Run a real local gRPC server with the handlers and a fake reader. A direct handler call alone does not prove serialization behavior.

| Case | Expected result |
| --- | --- |
| All five RPCs and all extrema alternatives | Typed round-trip requests and explicit expected results. |
| Absent `oneof`, required scalar omission, zero tolerance, unknown enum | Invalid omissions rejected; explicit valid zero preserved; unknown enum rejected. |
| Different nested and level ATR periods | Both settings round-trip without being confused. |
| Raw decimal strings and optional trade count absent versus zero | Exact source values and presence preserved. |
| Source-derived prices versus derived rounded values | Original price text retained; derived values follow numeric policy. |
| Candle and extremum references | Every index resolves; point/confirmation times and zone touch counts agree. |
| Method-specific evidence | Required evidence present; inapplicable fields absent. |
| Algorithm identifiers and metadata | Correct dependency identifiers, numeric policy, requested `to`, and actual range. |
| Every application error category | Correct gRPC status and supported details; clients tolerate status-only failures. |
| Response at, below, and above configured size | Allowed sizes succeed; oversized result fails without truncation. |
| Request over transport limit | Native resource-exhausted error is acceptable without custom details. |

Test size boundaries with small test-specific limits and measured Protobuf sizes. Do not allocate huge fixtures merely to test a threshold. Check pre-canceled and in-progress cancellation using synchronization instead of sleeps.

## Completion criteria

A generated Go client can call all five RPCs on a test server. Application and transport tests pass with the real domain calculations. Regeneration produces no unexpected diff. No upstream generated type crosses the application boundary.

Next: [Phase 5](05-service-integration.md).
