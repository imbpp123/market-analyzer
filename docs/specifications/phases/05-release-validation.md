# Phase 5: Release validation

Status: complete. Dependency: [Phase 4](04-service-integration.md).

## Summary

Verify the complete service with a test Market Data instance and measure the cost of calculations, source data, and evidence. Produce a release validation report rather than assume the proposed operating defaults are sufficient.

## Context and goals

Use the main specification's [Testing / Validation](../market-analyzer-v1.md#testing--validation), [Deadlines and transport bounds](../market-analyzer-v1.md#deadlines-and-transport-bounds), and [Risks / Trade-offs](../market-analyzer-v1.md#risks--trade-offs).

This phase needs a reachable test Market Data instance with complete candle history for selected instruments. Retention is time-dependent. Choose `to` and depth within the available range and record them for each run. Missing test data blocks the real dependency check, not deterministic tests or benchmarks.

## Implementation and validation work

1. Add a repeatable end-to-end test command using the generated Analyzer client. Accept the test endpoint and instrument selection through explicit test configuration. Keep live dependency tests separate from unit tests.
2. Run every RPC, every extrema method, and both price sources against test Market Data. Save the request settings, raw response data, build revision, upstream contract version, and effective runtime settings in the test report or associated fixtures.
3. Verify source identity, actual boundaries, raw fidelity, result metadata, and all evidence references. Use deterministic expected fixtures for numerical correctness; a live response that merely succeeds is not proof of the algorithm.
4. Add Go benchmarks for domain calculations and full response construction. Measure allocations and time for different depths and numeric sizes. Include extrema-rich data that produces large evidence lists and wide zones with many candidate references.
5. Run a controlled concurrent workload against the service. Measure end-to-end and upstream latency separately, CPU, memory, throughput, statuses, and response bytes. Observe cancellation and shutdown while work is active.
6. Verify transport limits using actual serialized response sizes. Test source responses and Analyzer evidence overhead separately. Adjust initial settings only when measurements justify the change, and update their documentation together.
7. Run all repository checks and generated-code reproducibility checks on the candidate revision. Add regression tests for discovered defects in the phase/layer that owns the behavior.
8. Write a validation report under `docs/validation/` with commands, environment, revisions, datasets, measurements, failures, and recommended operating settings. Link it from the implementation phase index when it exists.

Do not introduce caches, coalescing, rate limits, or a new algorithm to improve benchmark numbers. A behavior change requires a separate specification decision. This phase prepares a release candidate; deployment is a separate action.

## Test and measurement matrix

| Area | Cases and required evidence |
| --- | --- |
| RPC coverage | ATR, NATR, extrema, one trend, and levels succeed with complete valid source data; empty valid results remain valid. |
| Extrema combinations | Three methods with CLOSE and HIGH_LOW; trend and levels reuse each method correctly. |
| ATR composition | Levels with equal and different extrema/width ATR periods; returned settings and evidence agree. |
| History depth | Method minimum, representative depth such as 60 and 300, and the configured upstream maximum when available. Record actual bounds. |
| Numerical size | Typical cryptocurrency prices, very small and large prices, and large valid decimal strings using deterministic fixtures. |
| Evidence size | Many local extrema, both kinds on a candle, repeated equal-price groups, and all extrema preserved even when zones are discarded. |
| Latency | Report p50, p95, p99, throughput, and error counts with hardware, concurrency, and dataset context. |
| CPU and memory | Domain benchmark allocations, complete request allocations, peak process memory, and behavior after requests finish. |
| Cancellation | During upstream wait and expensive calculation; record how promptly work stops. |
| Timeouts | Earlier caller deadline and default service deadline; no late successful partial response. |
| Transport sizes | Exact boundary behavior from deterministic tests, plus measured real payload sizes and full Analyzer overhead. |
| Dependency failure | Unavailable Market Data and incomplete or expired history return the documented error category. |
| Shutdown under load | Requests finish or cancel within the configured deadline; process exits without blocked owned work. |
| Repeatability | Identical source fixtures/settings give identical calculation results; request-time metadata may differ. |

Live Market Data responses may change with source updates. Do not compare two independently fetched live snapshots as if they were a fixed oracle. Compare saved source data or a controlled upstream fixture when checking exact results.

## Report and completion criteria

The report must distinguish measured results from assumptions. No latency or throughput target has been approved, so report the measured operating range instead of inventing an SLA or declaring arbitrary numbers acceptable.

Completion requires:

- All deterministic unit and integration tests, build, vet, race, configured lint, and code-generation checks pass.
- End-to-end checks run against the test Market Data instance and their inputs are recorded.
- Benchmarks cover the matrix above, with failures explained and required fixes completed.
- Initial timeout and size settings have measurement evidence or are revised with documented reasons.
- No unresolved correctness, source fidelity, reference integrity, or lifecycle defect remains.
- The report clearly states any environment-dependent limits on the tested configuration.

If real dependency access is unavailable, record that check as blocked. Do not replace it with a fake run and mark the phase complete.

Completion evidence: the repeatable live RPC matrix, invariant checks, concurrent load tool, full JSON reports, domain benchmarks, full response benchmarks, transport checks, cancellation and shutdown checks, and generated-code reproducibility check passed. See [Release validation](../../validation/README.md) and the [final report](../../validation/2026-09-16-final.md).
