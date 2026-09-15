# Phase 1: Prepare Market Data shared libraries

Status: planned. Execution repository: `imbpp123/market-data`.

## Task and goal

Execute this phase in the Market Data repository, not Market Analyzer. This file is a self-contained implementation brief that can be handed to an agent working in that repository. Read its current `AGENTS.md`, README, contribution guides, and Git status before changes. Preserve unrelated work.

Market Data owns the market models that it creates and stores. Other services need to use those same models and the same Protobuf conversions. Publish reusable model and conversion modules without changing Market Data behavior or its existing gRPC contract.

The expected local checkout is `/Users/nkornushkov/Projects/market-data`. Inspect the actual checkout rather than assume paths or versions are unchanged. This phase does not implement Analyzer or its indicators.

## Existing code to inspect

- `internal/domain/market.go`: exchange, market, instrument, ticker, market statistics, Kline, and related enums.
- `internal/domain/timeframe.go` and its tests: timeframe values and calendar operations.
- `internal/application/ticker`: the read model used for relative funding time.
- `internal/transport/grpc/conversion.go` and conversion tests: outbound messages, decimals, timestamps, and presence.
- `api/proto/marketdata/v1/market_data.proto`, `api/CLIENT_GUIDE.md`, and `api/go/go.mod`: the public contract and generated module.
- Root `go.mod`, Makefile, CI, and container build files: module replacements, test commands, and build context.

The inspected root module is named `market-data`, while generated clients already use a separate module. Use separate public modules below; do not rename the root module as part of this task.

## Public modules and dependency direction

Create two modules in the same Market Data repository:

| Directory | Module and package | Responsibility |
| --- | --- | --- |
| `pkg/market` | `github.com/imbpp123/market-data/pkg/market`, package `market` | Shared market models, common value validation, and candle calendar. |
| `pkg/marketgrpc` | `github.com/imbpp123/market-data/pkg/marketgrpc`, package `marketgrpc` | Conversions between shared models and the existing generated Market Data messages. |

The model module depends on standard Go packages and `shopspring/decimal v1.4.0`. It must not import generated messages, gRPC, exchange SDKs, or Market Data internal packages.

The conversion module depends on the model module and `github.com/imbpp123/market-data/api/go`. It may use Protobuf timestamp utilities. It must not import root `internal` packages or make network calls. The generated API module must not depend on either new module. This prevents a dependency cycle.

Both services can use the model module in domain code. Their infrastructure or transport uses the conversion module. Neither library imports Analyzer.

## Shared model scope

Move the existing domain definitions and behavior into the model module:

- Exchange and market identifiers and validation.
- Timeframe identifiers, parsing, calendar construction, alignment, boundary validation, slot shifting, and slot counting.
- Instrument, instrument status, and contract type.
- Kline, Ticker, and MarketStats, including decimal values, optional values, and timestamps.
- Common validation of individual market values needed by both server and client conversions.

Preserve field meanings and zero/absence distinctions. Retain server-side metadata that is part of existing market objects; do not add it to the wire contract just because the Go model is now public.

Keep request policies, retention, cache management, exchange normalization, retry rules, configuration, and service error mapping inside Market Data. `HistoryCutoff` is a retention helper: retain it internally and compose shared `Floor` and `Shift`. The calendar's optional slot-count bound is an explicit argument, not a library-wide request quota.

Calendar alignment and provider support are separate. Moving `NewCalendar` must not start accepting an interval that an exchange endpoint does not support. Preserve the existing distinction and test it.

Replace internal duplicate market definitions with shared types. Update consumers, or use simple aliases temporarily to reduce migration scope. Aliases must not keep a second implementation of calendar or model validation rules. New consumers use public types directly.

## Shared Protobuf conversions

Provide explicit typed conversions in both directions for Instrument, Ticker, MarketStats, and Kline, plus response collection helpers where identity appears at response level. Do not introduce reflection-based mapping or a generic conversion framework.

Required behavior:

1. Decode valid wire decimals to `decimal.Decimal` without float conversion or rounding. Bound numeric text and expansion before expensive allocation according to the existing contract.
2. Validate required and optional timestamps without silently substituting zero times. Preserve nanoseconds and optional presence.
3. Preserve optional decimals, counts, durations, and paired bid/ask fields. Signed funding rates and price changes remain valid where the contract allows them.
4. Supply response-level exchange, market, symbol, and interval when decoding Klines. Check collection identity consistently; do not create empty identity fields just because the row message omits them.
5. Return typed conversion errors with field and optional row context. Server and client adapters map these to their own application errors; the library does not return service-specific internal errors.
6. Reject malformed rows without a successful partial collection. Do not sort, fill gaps, or normalize symbol spelling.
7. Allocate independent output records and optional fields. Mutating an input message after conversion must not change the decoded result, and encoding must not expose mutable input pointers.

Raw data preservation is required for downstream analysis responses. Decimal parsing alone does not preserve original text. Provide plain source records alongside parsed models, including a `market.SourceKline` with original decimal strings, timestamps, and optional fields. Apply the same source-preservation approach to the other three message types. These records contain no Protobuf types. Conversion results provide both parsed values and original source records in the same order; consumers can retain the source records without reparsing.

Distinguish two encoding paths: a domain model is encoded using the existing canonical decimal formatting; an unmodified source record can reproduce its original known wire fields. Do not claim that decoding and re-encoding only a decimal model preserves the original spelling. Do not silently pair changed domain values with stale source text.

### Fields that do not map one-to-one

Inspect and explicitly document these differences before implementing converters:

- Domain `Ticker.NextFundingAt` is an absolute time; the API provides remaining seconds at read time. Share a transport-independent ticker snapshot/read model for the wire meaning. Compute remaining time in the existing application logic, or with an explicitly supplied reference time where required. Never read the wall clock inside a converter or invent an absolute funding time while decoding.
- Instrument contract metadata is not fully exposed by the schema. Decoding must leave unavailable values absent or at a documented unknown value; encoding must preserve the current field selection.
- Market statistics use the supported rolling `24h` window. Preserve the exact mapping to the domain duration.
- Kline close times are exclusive. `fetched_at` is a receipt timestamp; it does not by itself prove that the exchange response was final.

Round-trip guarantees apply to fields present in the public contract. Untransmitted internal metadata and unknown future Protobuf fields are not reconstructed by the shared domain models.

## Integrate Market Data itself

Move reusable outbound conversion logic from the current gRPC transport into `marketgrpc` and make handlers use it. Add the inbound conversions for library consumers. Keep collection response size enforcement, cancellation of response assembly, and gRPC error mapping in the server wrappers. Preserve early size checks; do not allocate a whole unbounded response merely to use a helper.

Make root domain/application/infrastructure code use the shared model. Existing HTTP and gRPC responses, exchange behavior, cache behavior, limits, and error status meanings must remain unchanged. Update transport-specific error mapping for the new conversion errors.

Update root module dependencies and local relative replacements as needed for repository development. Update Docker and CI build contexts so both nested modules are available before dependency resolution. A root `go test ./...` does not test nested modules; add explicit module checks.

## Test cases

| Area | Required cases and result |
| --- | --- |
| Calendar | Move existing tests; fixed intervals, UTC, Monday weeks, variable months, leap years, Binance 3-day anchor, invalid boundaries, overflow, negative shifts, and slot bounds keep current behavior. |
| Scope | Calendar availability does not bypass exchange endpoint support. |
| Shared values | Existing enums and validation preserve valid and invalid values; model consumers compile without internal imports. |
| Four model conversions | Explicit fixtures cover every published field in both directions; compare actual values, not only object counts. |
| Decimal values | Zero, tiny and large values, signed fields where allowed, exact strings, malformed input, and excessive expansion. No float conversion. |
| Presence | Absent versus zero optional decimals/counts; absent versus present timestamps; invalid bid/ask pair presence. |
| Timestamps | Required missing timestamps, invalid nanos/ranges, valid subsecond precision, and exclusive candle close times. |
| Funding | Relative seconds remain relative; no fabricated absolute timestamp. Existing countdown behavior is tested with an explicit clock in its owner. |
| Identity | Kline response identity reaches each model; malformed collection inputs fail without partial success. |
| Source fidelity | Decode preserves original text and known wire fields; source-record encoding reproduces them, including optional absence. |
| Ownership | Mutation of input messages, output messages, slices, or optional values does not corrupt independent results. |
| Server regression | Existing conversion, size-boundary, snapshot, status mapping, HTTP, gRPC, and retention tests pass unchanged in behavior. |
| External consumer | A small separate Go module imports both public modules, performs a calendar operation and a Kline conversion, and builds without importing Market Data internals. |

Use deterministic fixtures, repository test conventions, and `t.Context()`. Move relevant existing tests rather than keeping two copies of the same implementation tests. Run formatting, build, vet, unit tests, race tests, and configured lint for the root and each new module; also run the existing API module checks.

## Versioning and handoff to Analyzer

Give each nested module its own `go.mod`, required sums, README, and exported API documentation. Pin compatible Go, decimal, and generated API versions. Document installation and basic use for a consumer outside this checkout.

Provide a reproducible external-consumer check with workspace mode disabled. Local absolute replacements and an unpublished sibling module must not be required by Analyzer. Repository-relative replacements may support Market Data development, but consumer builds must resolve pinned public versions of all dependencies.

Use actual available module versions or commit-derived pseudo-versions. Do not invent a version. If release tags are used, their paths must match nested modules, for example `pkg/market/v0.1.0` and `pkg/marketgrpc/v0.1.0`. Commit, push, tagging, and publication follow the executing session's authorization; document any publication step still required.

Deliver a handoff report containing:

- Source commit and resolvable versions of both modules and the generated API dependency.
- Go requirement, public type/function names, and minimal import examples.
- Source-preservation API and the documented non-one-to-one mappings.
- Changes to Market Data consumers, CI/build commands, and test results.
- External-consumer check result with no checkout-specific replacements.
- Remaining issues, if any.

The phase is ready for Analyzer only when shared modules are resolvable and the external-consumer check passes. If code is implemented locally but publication is pending, report that state explicitly; do not claim the dependency is ready.
