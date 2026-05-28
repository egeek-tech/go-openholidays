---
phase: 04-resilience
plan: 04
subsystem: cache
tags:
  - resil-06
  - resil-07
  - resil-08
  - resil-09
  - test-06
  - client-08
  - cache
  - roundtripper
dependency-graph:
  requires:
    - 04-01-SUMMARY.md   # fakeClock test seam (newFakeClock, fc.Now)
    - 04-02-SUMMARY.md   # Cache interface declared in config.go; client.Close calls c.cache.Close
    - 04-03-SUMMARY.md   # retryConfig wired; doJSONGet has retry loop (orthogonal to cache here)
  provides:
    - cache.go::MemoryCache + NewMemoryCache + newMemoryCacheWithClock
    - transport_cache.go::cacheTransport + CacheHitContextKey + isCacheablePath + cacheKey
    - options.go::WithCache + WithCacheBackend
    - config.go::buildTransport chain insertion for cacheTransport
    - internal_test.go::allowedVars["CacheHitContextKey"]
  affects:
    - client.go (Close path now invokes a real MemoryCache backend via the interface)
    - Plan 05 (hookTransport will read CacheHitContextKey from req.Context())
    - Plan 06 (PROJECT.md docs append — CL-15 cache surface, CL-16 strict-decoding)
tech-stack:
  added: []
  patterns:
    - "RoundTripper decorator (cacheTransport between hookTransport and loggingTransport — D-89)"
    - "Lazy goroutine start via sync.Once on first Put (D-84)"
    - "Idempotent shutdown via sync.Once on Close (D-85)"
    - "Lazy expiration on read in Get + active sweeper for memory bound (D-81)"
    - "Context-key idiom: unexported type, exported zero-value var (CacheHitContextKey)"
    - "Per-Client cache isolation — cache lives on *Client, key excludes Host (Pitfall CACHE-2 / D-82)"
key-files:
  created:
    - cache.go
    - cache_test.go
    - transport_cache.go
    - transport_cache_test.go
  modified:
    - options.go
    - options_test.go
    - config.go
    - client_test.go
    - internal_test.go
decisions:
  - "Plan 04 honors D-79..D-86 verbatim for the cache layer + D-93 strict/cache composition."
  - "DEVIATION (planner-authorized): CacheHitContextKey is allowlisted in internal_test.go::allowedVars, overriding CONTEXT.md D-97 step 6 (\"allowlist needs NO updates\"). Pattern Mapper finding clarified the AST audit gates ALL exported package-level vars."
  - "Compile-time conformance var _ Cache = (*MemoryCache)(nil) lives in cache_test.go (test file — excluded from AST audit) rather than cache.go, avoiding a blank-identifier var in production code."
  - "WithCache(ttl) uses time.Now literally (NOT c.nowFunc) because options run BEFORE Client construction; fake-clock tests route through WithCacheBackend(newMemoryCacheWithClock(ttl, fc.Now)) per D-86 documented compromise."
  - "TestClient_CloseStopsSweeper extended (Rule 1 auto-fix) with explicit c.http.CloseIdleConnections() + srv.Close() BEFORE the goroutine-delta assertion. D-96's verbatim pattern omits server bookkeeping and would be flaky with the connection-pool + httptest.Server worker goroutine churn."
  - "CL-14 was taken by Phase 3 (school_holidays_test.go Gold-Rule-3 exception). Plan 06 (PROJECT.md docs append) will use CL-15 (cache surface) and CL-16 (strict-decoding) instead of the CONTEXT.md-noted CL-14/CL-15."
metrics:
  duration_sec: 686
  tasks_completed: 4
  files_changed: 9
  commits: 7
  completed: 2026-05-28
---

# Phase 4 Plan 4: Cache layer Summary

In-memory TTL cache (MemoryCache) + cacheTransport RoundTripper wired via WithCache / WithCacheBackend; per-Client isolation, lazy sweeper start, idempotent Close, and strict-decoding composition all locked by tests.

## What was built

- **`cache.go`** — `MemoryCache` struct + `NewMemoryCache(ttl)` public constructor + `newMemoryCacheWithClock(ttl, nowFn)` unexported test seam + sweeper goroutine. Backed by `map[string]entry{value, expiresAt}` under `sync.RWMutex` (Pitfall CACHE-4). Lazy sweeper start via `sync.Once` on first Put (D-84); tick interval is `max(ttl/4, 30s)`. Idempotent Close via `closeOnce.Do` cancelling the sweeper ctx + 1ms wait on `sweepDone` (D-85). Get does lazy-expiration-on-read for user-visible TTL (D-81).
- **`transport_cache.go`** — `cacheTransport` RoundTripper consulting the supplied `Cache` for allowlisted paths only (D-83). `isCacheablePath` exact-matches `/Countries`, `/Languages`, `/Subdivisions`; holiday endpoints bypass entirely (RESIL-07). `cacheKey` encodes `method + " " + URL.Path + "?" + URL.Query().Encode()` (D-82). On a hit, builds a synthetic `*http.Response` with `Body=NopCloser(bytes.NewReader(cached))`, `ContentLength=len(cached)`, and `Request=req.WithContext(ctxWithCacheHitKey)` so observability layers can detect cache hits via `CacheHitContextKey`. On miss success, reads via `io.LimitReader(maxResponseBytes+1)`, drains+closes the original body (Pitfall HTTP-3), and caches when `len(buf) <= maxResponseBytes`.
- **Exported context-key surface** — `CacheHitContextKey` (var) + unexported `cacheHitKeyType` (type). Consumers read it via `req.Context().Value(openholidays.CacheHitContextKey)` and get `true` on cache hits; absence = miss.
- **`options.go`** — `WithCache(ttl time.Duration) Option` (D-79/D-80; `ttl <= 0` disables; uses `time.Now` literal) and `WithCacheBackend(c Cache) Option` (last-wins; nil = no-op). Both ship with full godoc citing the relevant decision IDs.
- **`config.go::buildTransport`** — edited in place per D-89 to insert `cacheTransport` ABOVE `loggingTransport` when `cfg.cache != nil`. The Phase 2 chain order is preserved verbatim when cache is unused. Comment block documents the inverse build-order convention.
- **`internal_test.go::allowedVars`** — `CacheHitContextKey` added (chronological-append position) per the planner-authorized deviation override.

## How it integrates

```text
req ──► [cacheTransport]? ──► loggingTransport ──► headerTransport ──► underlying
        ▲                                                                ▲
        │ cache hit short-circuits here                                  │
        │ (returns synthetic 200 OK +                                    │
        │  CacheHitContextKey == true)                                   │
        │                                                                │
client.Close ──► closeOnce.Do ──► c.cache.Close() ──► sweepCancel + wait sweepDone (≤ 1ms)
```

The cache integrates with `Client.Close()` exactly as Plan 02 stubbed it: `closeOnce.Do { c.closed.Store(true); if c.cache != nil { _ = c.cache.Close() } }`. This plan supplies the implementation that `c.cache.Close()` now invokes — the sweeper goroutine started lazily on the first Put exits cleanly within 1ms (D-85).

Strict-decoding composition (D-93) works because the decoder runs in `doJSONGet` AFTER `cacheTransport` returns: both fresh and cached bytes flow through the same `decoder.DisallowUnknownFields()` gate. The `TestCache_StrictDecodingComposes` test proves this end-to-end by exercising a 2-call sequence with `WithStrictDecoding(true) + WithCache(time.Hour)` against a server returning JSON with an unknown field — both calls fail at the decoder, and the server is hit exactly once (second call is a cache hit).

## Tests

| Test | File | Outcome |
| --- | --- | --- |
| `TestCacheInterface_Conformance` | `cache_test.go` | PASS — `var _ Cache = (*MemoryCache)(nil)` compiles + runtime conformance |
| `TestNewMemoryCache` | `cache_test.go` | PASS — non-nil + ttl field populated |
| `TestMemoryCache_GetPut` | `cache_test.go` | PASS — Get on empty returns `(nil, false)`; Put then Get round-trips |
| `TestMemoryCache_SweeperLazyStart` | `cache_test.go` | PASS — sweeper not spawned by NewMemoryCache or Get; spawned by first Put; stopped by Close |
| `TestMemoryCache_TTLEviction` | `cache_test.go` | PASS — fake clock advance past TTL makes Get return ok=false |
| `TestMemoryCache_CloseIdempotent` | `cache_test.go` | PASS — sequential + 100 concurrent Close all return nil; sweeper exits |
| `TestCacheTransport_PathAllowlist` | `transport_cache_test.go` | PASS — table over 7 path cases; cacheable vs bypass behavior verified |
| `TestCacheTransport_HolidayPathsBypass` | `transport_cache_test.go` | PASS — explicit RESIL-07 lock: /PublicHolidays + /SchoolHolidays bypass |
| `TestCacheTransport_HitMissBehavior` | `transport_cache_test.go` | PASS — miss-then-hit returns identical bytes; 503 never cached (Pitfall CACHE-1) |
| `TestCacheTransport_RawBytesKey` | `transport_cache_test.go` | PASS — different query → distinct entries; same path/query + different Host → same key (D-82) |
| `TestCacheHitContextKey_OnHit` | `transport_cache_test.go` | PASS — miss carries no key; hit carries `CacheHitContextKey == true` in `resp.Request.Context()` |
| `TestWithCache` | `options_test.go` | PASS — positive ttl populates a *MemoryCache; ttl ≤ 0 disables |
| `TestWithCache_NonPositiveTTLDisables` | `options_test.go` | PASS — explicit RESIL-07/D-80 lock |
| `TestWithCacheBackend` | `options_test.go` | PASS — non-nil verbatim; nil no-op; last-wins over WithCache(ttl) |
| `TestClient_CloseStopsSweeper` | `client_test.go` | PASS — sweeper exits after Close + CloseIdleConnections + srv.Close + 20ms grace (D-96) |
| `TestCache_StrictDecodingComposes` | `client_test.go` | PASS — D-93 strict gate fires on both fresh AND cached reads; 1 server hit total |
| `TestClient_NoCache_AllCallsHitNetwork` | `client_test.go` | PASS — default-off invariant: 3 calls → 3 server hits |
| `TestCache_PerClientIsolation` | `client_test.go` | PASS — D-82: two Clients each hit their own server exactly once |
| `TestNoInitOrGlobalState` | `internal_test.go` | PASS — allowlist now includes `CacheHitContextKey` |

Full unit suite: `go test -race -count=1 ./...` → **ok** (≈ 2.0s).
`go vet ./...` clean. `gofmt -l` on all touched files: clean.

## Deviations from Plan

### Planner-authorized (documented in plan)

1. **[Rule 2 — Critical Functionality] Add `CacheHitContextKey` to `internal_test.go::allowedVars`.**
   - **Found during:** Plan authoring (Pattern Mapper finding, lines 244-258 of 04-PATTERNS.md).
   - **Issue:** CONTEXT.md D-97 step 6 states "allowlist needs NO updates for Phase 4". CLIENT-10 AST audit, however, gates ALL exported package-level vars, not just sentinel errors — `CacheHitContextKey` is the Phase 4 context-key var and would otherwise fail the audit.
   - **Fix:** Added `"CacheHitContextKey": {}` to `allowedVars` (Task 4); comment block extended with a bullet citing the deviation.
   - **Files modified:** `internal_test.go`.
   - **Commit:** `0d372e8`.

2. **Compile-time conformance assertion placement (`var _ Cache = (*MemoryCache)(nil)`).**
   - The plan instructed hoisting this from `cache.go` to `cache_test.go` to avoid a `_` (blank identifier) package-level var triggering the AST audit. Implemented as instructed; the assertion lives only in the test file. Production `cache.go` declares NO package-level vars.

### Auto-fixed during execution

1. **[Rule 1 — Bug fix] `TestClient_CloseStopsSweeper` flakiness on connection-pool goroutines.**
   - **Found during:** Task 3 GREEN verification (test failed: "6 is not less than or equal to 3").
   - **Issue:** D-96's documented pattern is `before := runtime.NumGoroutine(); c.Countries(...); c.Close(); time.Sleep(10ms); assert.LessOrEqual(NumGoroutine, before)`. In practice the `httptest.Server` keep-alive worker goroutines + the `http.Transport`'s internal keep-alive connection management leave persistent goroutines that don't release on `Client.Close()` — the sweeper-leak signal is drowned out.
   - **Fix:** Added explicit `c.http.CloseIdleConnections()` + `srv.Close()` BEFORE the goroutine-delta assertion, and extended the grace from 10ms to 20ms. Comment block documents the test-harness teardown as the load-bearing prerequisite (Rule 1 per the executor protocol). The sweeper-stop invariant from D-96 is unchanged; only the test pattern is more robust.
   - **Files modified:** `client_test.go`.
   - **Commit:** `a11ee0a`.

## Clock-seam compromise (documented in D-86)

`WithCache(ttl)` uses `time.Now` literally because options run BEFORE Client construction; the Client's `nowFunc` cannot be picked up retroactively by the cache backend. Tests that need a fake clock route through `WithCacheBackend(newMemoryCacheWithClock(ttl, fc.Now))` instead. `TestMemoryCache_TTLEviction` exercises exactly this pattern.

## CL-XX numbering note for Plan 06

CONTEXT.md D-80 specifies "Recorded as new **CL-14** in PROJECT.md Key Decisions" for the cache surface, and D-91 specifies CL-15 for strict-decoding. However:

- **CL-14 is already taken** by a Phase 3 gap-closure deviation (Gold-Rule-3 exception in `school_holidays_test.go`).
- Plan 06 (PROJECT.md docs append) will use **CL-15** for the cache surface decision and **CL-16** for the strict-decoding decision.

This plan (04) does NOT touch PROJECT.md — the docs append is Plan 06's responsibility — but the renumbering is recorded here so downstream plans use the correct numbers.

## Self-Check: PASSED

- `cache.go` exists: FOUND
- `cache_test.go` exists: FOUND
- `transport_cache.go` exists: FOUND
- `transport_cache_test.go` exists: FOUND
- Commit `2d389a4` (Task 1 RED): FOUND
- Commit `da447ee` (Task 1 GREEN): FOUND
- Commit `354977b` (Task 2 RED): FOUND
- Commit `e66b89b` (Task 2 GREEN): FOUND
- Commit `7573b37` (Task 3 RED): FOUND
- Commit `a11ee0a` (Task 3 GREEN): FOUND
- Commit `0d372e8` (Task 4): FOUND
- `go test -race -count=1 ./...` exits 0: VERIFIED
- `TestNoInitOrGlobalState` passes with `CacheHitContextKey` allowlisted: VERIFIED
- `var _ Cache = (*MemoryCache)(nil)` lives in `cache_test.go` (test file): VERIFIED
- `transport_retry.go` audit still clean (no such file exists): VERIFIED (only test-side references in retry_test.go)
