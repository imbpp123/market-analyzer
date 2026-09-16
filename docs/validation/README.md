# Release validation

Release validation uses a separate live command and benchmark suite. Unit tests
do not call external services.

The completed local run is recorded in the
[release validation report](2026-09-16-final.md). The
[initial report](2026-09-16-initial.md) records the earlier blocked state before
a live dependency was available.

## Live correctness matrix

Copy `config.example.json` to an ignored local file. Set a historical `to` value that is available in the test Market Data instance. Record the actual service runtime settings and pinned Market Data contract version in the same file.

Start Market Analyzer against the test Market Data endpoint, then run:

```sh
make validate-live \
  VALIDATION_CONFIG=docs/validation/config.local.json \
  VALIDATION_REPORT=validation-report.json
```

The command makes 20 independent Analyzer calls:

- ATR and NATR;
- extrema, trend, and levels;
- local, percentage reversal, and ATR reversal extrema;
- `CLOSE` and `HIGH_LOW` price sources.

The report contains the exact requests, full responses, response sizes, durations, build revision, upstream contract version, and effective service settings. It checks source boundaries, candle count and continuity, OHLC validity, result references, evidence references, and zone touch references. It exits with a failure when any call or invariant check fails. The output file is created with mode `0600` and is never overwritten.

The command uses plaintext gRPC because the current service has no TLS listener. Network access and credentials remain deployment-owned.

## Controlled live load

Run a mixed workload after the correctness matrix passes:

```sh
make load-live \
  VALIDATION_CONFIG=docs/validation/config.local.json \
  LOAD_REPORT=load-report.json \
  LOAD_CONCURRENCY=8 \
  LOAD_REQUESTS=200
```

The workload cycles through the same 20 RPC, method, and price-source cases used by the correctness matrix. It validates every response while calls run concurrently. The report records each call, status counts, response bytes, throughput, and end-to-end p50, p95, p99, and maximum latency. Analyzer metric deltas provide separate average request, Market Data, and calculation durations. The metrics endpoint defaults to `http://127.0.0.1:8081/metrics` and can be changed with `LOAD_METRICS_URL`.

Use an external process monitor for peak CPU and memory. The Analyzer metrics endpoint does not expose process resource statistics. Run shutdown-under-load separately because connection errors are expected when the process stops.

## Benchmarks

Run the release benchmark suite with:

```sh
make benchmark
```

The domain benchmarks cover 60, 300, and 1000 candles, small and large decimal values, all calculation families, and extrema-rich alternating data. Full response benchmarks include source candles, calculation, evidence mapping, and Protobuf size checks.

Benchmark output is environment-specific. Save the full output with the release validation report. Do not use a short smoke run as release evidence.

## Generated code

Install the pinned generators once and verify reproducibility:

```sh
make generate-tools
make generate-check
```

`generate-check` writes generated files to a temporary directory and compares them with the checked-in files.
