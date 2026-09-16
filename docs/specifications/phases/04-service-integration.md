# Phase 4: Service integration

Status: complete. Dependency: [Phase 3](03-api-and-use-cases.md).

## Summary

Connect the use cases to Market Data and add a runnable process with operational endpoints, metrics, configuration, deadlines, and bounded shutdown.

## Context and goals

Implement the adapter from [Application and Market Data boundary](../market-analyzer-v1.md#application-and-market-data-boundary) and the behavior in [Operations and Observability](../market-analyzer-v1.md#operations-and-observability). Follow the pinned schema and client guide linked in the specification's Context.

## Implementation work

1. Pin the reviewed Market Data Go client version. Implement `CandleReader` under `internal/infrastructure/marketdata` using `GetKlines`. Map exact selectors and planned boundaries; make no ticker preflight call or application retry loop.
2. Convert all upstream fields to application-owned source records, preserving decimal strings, timestamps, and optional presence. Convert upstream errors by status and structured details without parsing message text.
3. Reuse one upstream gRPC connection across independent requests. Configure the receive limit and propagate each caller's context. Let the process own connection cleanup.
4. Add `cmd/market-analyzer` as the entry point and construct domain/application, adapter, and transport dependencies there. Keep configuration loading and environment access outside calculation code.
5. Add validated startup settings for gRPC and HTTP addresses, Market Data endpoint, request and shutdown timeouts, and transport sizes. Use the initial defaults in the specification. Reject invalid settings and listener failures with clear startup errors.
6. Add HTTP `/health`, `/ready`, and `/metrics`. Readiness reflects local startup and shutdown state. Do not block it on Market Data availability or add HTTP analysis routes.
7. Add bounded-label metrics and structured logs in wrappers. Record request, dependency, and calculation durations, active requests, source counts, response sizes, and statuses. Do not add observability imports to the domain.
8. Implement shutdown ownership: readiness false, stop accepting analysis work, wait for in-flight requests within the shutdown deadline, then cancel and force stop if needed. Close the upstream connection after processing stops. Ensure the HTTP server also exits.
9. Document build, configuration, launch, operational checks, and shutdown commands. Supply a minimal local configuration example without environment credentials. Environment security remains deployment-owned.

## Adapter test cases

Use a local fake Market Data gRPC server with the real generated upstream messages and the real adapter.

| Case | Expected result |
| --- | --- |
| Valid planned range | Exact exchange, market, symbol, interval, and `[from, to)` reach `GetKlines`. |
| Valid Kline response | Every source field preserved, including missing versus zero trades and timestamp precision. |
| Invalid successful response | Application validation rejects it; adapter does not repair fields or order. |
| Each documented upstream status and reason | Expected application error and final Analyzer status/details. |
| Missing, unknown, or unrelated status details | Stable category handling without panic or message parsing. |
| Upstream response above receive limit | Resource-exhausted category, no partial source list. |
| Upstream deadline, caller cancellation, unavailable server | Context/error propagation as specified. |
| Two simultaneous valid calls | Both reach the upstream server independently and return their own data. |
| Failed upstream call | Analysis fails; no application retry or alternate RPC. |

## Process and operations test cases

| Case | Expected result |
| --- | --- |
| Missing endpoint, invalid duration/size, occupied listener | Startup fails clearly and cleans up already created resources. |
| Initialized listeners | Health and readiness return 200. |
| Upstream unavailable after startup | Local readiness remains 200; analysis fails and dependency metrics record it. |
| Startup and shutdown readiness states | 503 until initialized and after shutdown starts. |
| HTTP request to an analysis route | No analysis handler exists. |
| Successful and failed analysis | Metrics and logs contain the expected bounded fields; arrays are not logged. |
| In-flight request completes before shutdown deadline | Successful response, then clean server and connection closure. |
| In-flight request exceeds shutdown deadline | Work canceled and shutdown returns within the configured bound. |
| New work arrives during shutdown | Not admitted as new analysis work. |
| Cancellation under concurrent load | Active request counts return to zero; no owned worker remains blocked. |

Use ephemeral local ports, synchronization, and bounded contexts in tests. Isolate lifecycle logic for deterministic unit tests, then verify actual listeners in integration tests. Do not infer shutdown correctness from a mocked method call alone.

## Completion criteria

Implementation and verification: [Service integration](../../service-integration.md). The pinned Market Data adapter, process configuration, operational HTTP server, bounded metrics and logs, listener lifecycle, and bounded shutdown are covered by local integration tests.

The service builds and runs with a configured Market Data endpoint. Local integration and lifecycle tests pass, including race checks. Run instructions and all settings are documented. Actual dependency data and load measurements are covered in phase 5; local fakes alone do not complete that phase.

Next: [Phase 5](05-release-validation.md).
