# go-openholidays — Design

## Overview

go-openholidays is a single-package Go SDK that exposes five OpenHolidays endpoints (`/PublicHolidays`, `/SchoolHolidays`, `/Countries`, `/Languages`, `/Subdivisions`) through a `Client` with a composable RoundTripper chain. This doc describes the architecture shipped in v0.1.0; [pkg.go.dev](https://pkg.go.dev/github.com/egeek-tech/go-openholidays) hosts the per-symbol reference.

## Client Lifecycle

`NewClient` applies functional options to a fresh internal config, composes the per-request `*http.Client` (via the RoundTripper chain below), seeds a per-Client `*math/rand/v2.Rand` from `crypto/rand`, and returns an immutable `*Client`. `Close` is idempotent (`sync.Once`-guarded) and stops the cache sweeper if one was wired.

```
   +-----------------+    +-----------------+    +-----------------+
   |  NewClient(opts)| -> |  composeHTTP    | -> |  *http.Client   |
   +-----------------+    |  Client(cfg)    |    |  (chain wrapped)|
                          +-----------------+    +--------+--------+
                                                          |
                          +-----------------+              v
   defer c.Close() ---->  |  c.PublicHolidays/SchoolHolidays/...    |
                          +-----------------+
                                  |
                                  v
                          +-----------------+
                          |  cache.Close()  |  (once, via sync.Once)
                          +-----------------+
```

## RoundTripper Chain

The chain composition order, from caller toward the network, is:

```
  request -> hook -> cache -> logging -> header -> base -> network
                                                      |
                                                      v
  response <-------------------------------------- network
```

Decorator responsibilities:

- `hook` invokes the user `RequestHookFunc` callback after every round trip (including cache-hit synthetic responses; D-88 / D-89). A nil hook is a no-op.
- `cache` short-circuits the GET when an unexpired cached entry is present (RESIL-06..09). Cache-hit responses carry the `CacheHitContextKey` so the hook decorator can distinguish them from network responses.
- `logging` emits an `slog` Debug event per round trip with method/URL/status/duration only — response bodies are never logged at any level (PROJECT.md HTTP semantics).
- `header` injects `Accept: application/json` and `User-Agent: go-openholidays/<Version>` (or whatever `WithUserAgent` set) on every request.
- `base` is the user-supplied `*http.Client` (default `&http.Client{}`) — this is where any custom Transport, TLS config, or proxy settings live.

Invariant (RESIL-05 / STATE.md Key Decisions): **retry lives in the endpoint layer, NOT as a RoundTripper.** Placing retry inside the chain — e.g. an order like `retry → cache → hook → logging → header → base` — would double-retry when callers supply their own retrying `*http.Client` via `WithHTTPClient`. The shipped chain therefore lifts retry out of the RoundTripper layer entirely and runs it inside `request.go::doJSONGet` against the composed `*http.Client`.

## Cache Architecture

Cache wiring is opt-in. `WithCache(ttl)` installs the built-in `MemoryCache` (a `map[string][]byte` under `sync.RWMutex` with a janitor goroutine that sweeps expired entries every TTL/2). `WithCacheBackend(c Cache)` accepts a user-supplied implementation of the public `Cache` interface for callers that need Redis, memcached, etc.

Key invariants:

- **Only metadata endpoints are cached by default** (RESIL-07): `/Countries`, `/Languages`, `/Subdivisions`. `/PublicHolidays` and `/SchoolHolidays` are NEVER cached — their cardinality (every CountryIsoCode × ValidFrom × ValidTo combination) defeats a TTL cache and the cached entries would mask real changes around year boundaries.
- **Sweeper is started lazily on first `Set`** (RESIL-08) so a Client constructed with `WithCache` but never actually used pays no goroutine cost.
- **Sweeper is stopped by `Client.Close`** (D-85) via `sync.Once` so concurrent `Close` calls are race-free under the race detector.
- The cache transport caches only the raw bytes of the response body. Decoding (including strict-mode unknown-field detection) runs on every read.

## Retry Architecture

Retry wiring is opt-in via `WithRetry(maxAttempts, baseDelay)`. The retry loop lives in the endpoint layer (see `request.go::doJSONGet`), not as a RoundTripper, per the RESIL-05 invariant above.

Mechanics (RESIL-01..05):

- **Backoff**: exponential with full jitter — `delay = rand.Int64N(baseDelay << attempt)` (`math/rand/v2`, per-Client ChaCha8-seeded for fleet diversity; D-78).
- **Cap**: every computed delay is clamped to `WithMaxRetryWait` (default 30 s) so an unbounded `Retry-After` cannot pin the request.
- **Retry-After parsing**: RFC 7231 — accepts integer seconds and HTTP-date forms, picks the larger of `Retry-After` and the computed jittered backoff.
- **Retryable conditions**: network errors (`net.Error.Timeout()`, connection reset, EOF), 408, 429, 502, 503, 504. 4xx (except 408/429) and 2xx are never retried.
- **Cancellation**: sleeps are ctx-aware via `Client.sleepFunc` (default `ctxSleep`) — never bare `time.Sleep`. Cancellation propagates within ≤ 100 ms (CLIENT-09).
- **Idempotency**: all five endpoints are GETs and safely retryable.

## Error Model

The package exposes seven sentinel errors plus one leaf type:

| Symbol | When returned |
|--------|----------------|
| `ErrInvalidCountry` | Client-side: malformed ISO 3166-1 alpha-2 country code (D-22 / VALID-01). |
| `ErrInvalidLanguage` | Client-side: malformed ISO 639-1 language code. |
| `ErrInvalidDateRange` | Client-side: `ValidFrom` is strictly after `ValidTo`. |
| `ErrDateRangeTooLarge` | Client-side: window spans more than 3 calendar years (VALID-03). |
| `ErrEmptyResponse` | 2xx with an empty body where a payload was required. |
| `ErrResponseTooLarge` | Upstream response exceeded the 10 MiB cap. |
| `ErrMalformedResponse` | Post-decode invariant failure on Holiday (zero StartDate/EndDate, or EndDate before StartDate; D-65 / CL-12). |
| `*APIError` | Non-2xx upstream response. Carries `StatusCode`, `Path`, capped `Body` (4 KiB), and best-effort `Message` parsed from RFC 7807 ProblemDetails. |

All sentinels are detectable via `errors.Is`. `*APIError` is also detectable via `errors.As` to recover the populated value. Per ERR-04, neither error nor log output ever includes the full response body above `slog.LevelDebug` — `APIError.Body` is excluded from `Error()` output by design and the response-body cap is enforced before logging.

## Strict Decoding

`WithStrictDecoding(bool)` is OFF by default and immutable after `NewClient` (CL-16). When strict mode is on, the JSON decoder calls `DisallowUnknownFields` so any field not declared on the package's typed structs surfaces a decode error. Because the cache transport stores raw bytes (see above), strict mode applies to cached entries on every read — a cache-hit decode failure surfaces exactly as a cache-miss decode failure would.

Strict mode is recommended for CI test environments where upstream schema drift should fail loudly; production deployments typically keep it off so a newly-added upstream field does not break the consumer.
