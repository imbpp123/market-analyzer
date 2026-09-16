# Market Analyzer implementation phases

These files describe implementation work. Phases 1 through 5 are complete.

The [main specification](../market-analyzer-v1.md) owns calculation rules, domain meanings, API behavior, and operational requirements. Phase files define work order, deliverables, and tests. They do not replace the specification or introduce alternative algorithms.

## Phase order

| Phase | Work | Depends on | Completion result |
| --- | --- | --- | --- |
| [1. Domain foundations](01-domain-foundations.md) | Go setup, domain values, decimal rules, calendar and source validation. | None. | Tested inputs and numerical operations. |
| [2. Calculations](02-calculations.md) | ATR, NATR, three extrema methods, trend, and zones. | Phase 1. | Tested calculations on in-memory candles. |
| [3. API and use cases](03-api-and-use-cases.md) | Protobuf, application composition, source preservation, gRPC handlers. | Phase 2. | Five RPCs tested with a fake reader. |
| [4. Service integration](04-service-integration.md) | Market Data adapter, startup, HTTP operations, metrics, and shutdown. | Phase 3. | Runnable service with tested external boundaries. |
| [5. Release validation](05-release-validation.md) | Real dependency checks, benchmarks, response sizes, and release evidence. | Phase 4. | Measured and documented release candidate. |

All phases run in Market Analyzer. They use the existing Market Data gRPC API and do not require changes to Market Data or shared domain libraries.

Implement each phase in this order. Keep its tests beside the code they cover. Later phases extend earlier tests only when they add behavior or expose a failure.

## Shared implementation rules

Follow [development instructions](../../../AGENTS.md) and the specification's [boundaries](../market-analyzer-v1.md#boundaries). Keep domain calculations independent of generated messages and external I/O. Add packages when their code is needed, not as empty placeholders.

Use English for code, test names, fixtures, logs, and documentation. Use `t.Context()`, explicit expected results, deterministic time, and stateful fakes at I/O boundaries. Do not use live exchanges in unit tests or add mock interfaces around every calculation.

After each code phase, run formatting, focused tests, build, vet, unit tests, race tests, and the configured linter. Use repository targets when present. Before a Go module exists, code checks are not available; creating the module is phase 1 work.

A phase completion report must list implemented work, changed files, checks and results, and unresolved issues. Do not mark a phase complete when required checks cannot run. Record the blocker and completed independent work instead.

Phase status records completed work and planned work. It does not authorize deployment or claim that the service exists.
