# Market Analyzer implementation phases and Market Data preparation

These files describe planned implementation work. No phase is complete yet.

The [main specification](../market-analyzer-v1.md) owns calculation rules, domain meanings, API behavior, and operational requirements. Phase files define work order, deliverables, and tests. They do not replace the specification or introduce alternative algorithms.

## Phase order

| Phase | Work | Depends on | Completion result |
| --- | --- | --- | --- |
| [1. Prepare Market Data](01-market-data-prepare.md) | Shared market models and Protobuf conversions; execute in Market Data. | None. | Published, tested modules and a consumer handoff. |
| [2. Domain foundations](02-domain-foundations.md) | Go setup, shared model adoption, decimal rules, selection and source validation. | Phase 1. | Tested inputs and numerical operations. |
| [3. Calculations](03-calculations.md) | ATR, NATR, three extrema methods, trend, and zones. | Phase 2. | Tested calculations on in-memory candles. |
| [4. API and use cases](04-api-and-use-cases.md) | Protobuf, application composition, source preservation, gRPC handlers. | Phase 3. | Five RPCs tested with a fake reader. |
| [5. Service integration](05-service-integration.md) | Market Data adapter, startup, HTTP operations, metrics, and shutdown. | Phase 4. | Runnable service with tested external boundaries. |
| [6. Release validation](06-release-validation.md) | Real dependency checks, benchmarks, response sizes, and release evidence. | Phase 5. | Measured and documented release candidate. |

Phase 1 is a self-contained task for the Market Data repository and follows that repository’s instructions. Phases 2–6 run in Market Analyzer and consume the versions delivered by phase 1.

Implement each phase in this order. Keep its tests beside the code they cover. Later phases extend earlier tests only when they add behavior or expose a failure.

## Shared implementation rules

Follow [development instructions](../../../AGENTS.md) and the specification's [boundaries](../market-analyzer-v1.md#boundaries). Keep domain calculations independent of generated messages and external I/O. Add packages when their code is needed, not as empty placeholders.

Use English for code, test names, fixtures, logs, and documentation. Use `t.Context()`, explicit expected results, deterministic time, and stateful fakes at I/O boundaries. Do not use live exchanges in unit tests or add mock interfaces around every calculation.

After each code phase, run formatting, focused tests, build, vet, unit tests, race tests, and the configured linter. Use repository targets when present. Before a Go module exists, code checks are not available; creating the module is phase 2 work.

A phase completion report must list implemented work, changed files, checks and results, and unresolved issues. Do not mark a phase complete when required checks cannot run. Record the blocker and completed independent work instead.

These documents plan work only. They do not authorize deployment or claim that the service exists.
