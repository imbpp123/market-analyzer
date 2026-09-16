# Architecture

Market Analyzer is a stateless calculation service. Market Data owns exchange
collection and candle storage. Analyzer owns input validation, candle selection,
technical calculations, and result evidence.

## Components

```text
gRPC client
    |
    v
transport/grpc  ->  application  ->  CandleReader interface
                         |                  |
                         v                  v
                       domain       infrastructure/marketdata
                                                |
                                                v
                                      Market Data gRPC service

HTTP client  ->  transport/httpops  ->  health, readiness, metrics
```

The code follows inward dependencies:

| Layer | Responsibility |
| --- | --- |
| `internal/domain` | Candle values, validation, decimal policy, calendar rules, ATR, NATR, extrema, trend, and zones. |
| `internal/application` | Use-case validation, range planning, deadlines, one candle read, and response assembly. |
| `internal/infrastructure/marketdata` | Market Data gRPC adapter and upstream error mapping. |
| `internal/transport` | Analyzer Protobuf mapping, public gRPC errors, response limits, and operational HTTP. |
| `internal/service` | Configuration, dependency construction, listeners, admission control, and shutdown. |
| `cmd/market-analyzer` | Process entry point and signal handling. |

Domain and application packages do not depend on generated Protobuf messages,
gRPC, HTTP, or observability packages. The application defines the `CandleReader`
interface because it consumes candle data. The Market Data adapter implements
that interface.

## Request flow

Each analysis request follows the same path:

1. The gRPC transport checks required Protobuf fields and maps them to
   application-owned values.
2. The application captures `evaluated_at`, applies the earlier of the client
   deadline and the configured request timeout, and validates settings.
3. The application calculates one closed candle range and calls Market Data
   once with `GetKlines`.
4. Source identity, range, count, timestamps, continuity, prices, volume, and
   turnover are validated. Invalid source data fails the whole request.
5. The domain runs the selected calculation in memory and checks cancellation
   during long loops.
6. The transport formats derived decimals, checks the full uncompressed
   Protobuf response size, and returns the result with every source candle.

There is no data cache, result cache, request coalescing, retry loop, or rate
limiter. The Market Data gRPC connection is shared, but all request data and
calculation state are local to one request.

## Source fidelity

The application keeps two candle representations in the same order:

- source records preserve Market Data decimal strings, timestamps, and optional
  trade counts for the public response;
- domain candles contain parsed decimal values for validation and calculation.

This avoids changing upstream values when a response is built. Derived values
use the documented [numeric policy](api.md#numeric-policy), while source values
remain byte-for-byte decimal text from Market Data.

## Composition

Public operations compose the domain functions directly:

| Operation | Calculation chain |
| --- | --- |
| ATR | Wilder ATR sequence, latest value |
| NATR | Wilder ATR sequence, latest ATR normalized by the matching close |
| Extrema | One selected extrema method; ATR reversal also calculates ATR |
| Trend | Extrema detection, then ordered swing-structure classification |
| Levels | Extrema detection, latest ATR for zone width, then zone construction |

When levels use ATR-reversal extrema and both ATR periods match, the ATR sequence
is reused inside that request. Different periods use separate sequences. This is
calculation reuse, not cross-request caching.

## Failure boundaries

Invalid caller fields fail before Market Data is called. Dependency errors are
converted to application error categories by the adapter. Only the gRPC
transport maps those categories to public status codes and `ErrorDetail`.

Cancellation never returns a partial result. An empty extrema list, an empty
zone list, or an `UNDETERMINED` trend is a valid successful result when the
source data itself is valid.
