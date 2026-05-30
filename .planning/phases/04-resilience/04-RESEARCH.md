# Phase 4: Resilience - Research

**Researched:** 2026-05-27
**Domain:** Resilience middleware for HTTP client SDK — retry, in-memory TTL cache, observability hook, strict-decoding, deterministic test clock
**Confidence:** HIGH

## Summary

Phase 4 wraps the stable Phase 3 endpoint pipeline (`doJSONGet[T any]` in `request.go`) with four cross-cutting behaviors: **retry** (inside the endpoint layer, not in the RoundTripper chain), **TTL cache** (as a RoundTripper that synthesizes responses on hit), **request hook** (outermost RoundTripper so cache hits are visible), and **strict-decoding** (a one-line gate inside `doJSONGet`). `Client.Close()` becomes load-bearing: it must stop the lazily-started cache sweeper goroutine via `sync.Once` + ctx cancellation, and is required to be idempotent and concurrent-safe.

Every locked decision in CONTEXT.md (D-73..D-97) has been cross-verified against the existing source code (`client.go`, `config.go`, `options.go`, `request.go`, `transport.go`, `internal_test.go`) and against the canonical research files (`PITFALLS.md` §RETRY/CACHE/CONC, `ARCHITECTURE.md` §Pattern 2/4). The implementation plan recommended in D-97 (strict-decoding → fake clock → retry → cache → hook → Key Decisions appendix) is reproducible: each step depends only on the prior step's exported surface and does not require changes to the Phase 3 endpoint method signatures.

**Primary recommendation:** Follow the D-97 ordering verbatim. Land strict-decoding first (smallest blast radius, no goroutines), then the fake-clock seam (no behavior change, unlocks deterministic tests), then retry (uses the seam), then cache (uses the seam, introduces the sweeper goroutine + `Client.Close()` wiring), then the hook (outermost in the chain so cache hits are observed), then the Key Decisions appendix (CL-14 + CL-15). No new exported sentinel errors; retry-exhausted errors wrap `lastErr` via `%w` so callers branch on the underlying sentinel.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Retry (RESIL-01..05; TEST-05):**

- **D-73:** Two public retry options in v0.1.0: `WithRetry(maxAttempts int, baseDelay time.Duration) Option` (matches ROADMAP SC #1 literal) and `WithMaxRetryWait(d time.Duration) Option` (single escape-hatch knob). No `RetryConfig` struct, no per-status overrides.
- **D-74:** Retry DISABLED when `WithRetry` not called. `n <= 0` interpreted as DISABLED; `d <= 0` interpreted as `baseDelay = 250 * time.Millisecond`. Default `maxRetryWait = 60 * time.Second`.
- **D-75:** Retryable: HTTP `{408, 429, 500, 502, 503, 504}`; transport errors `net.Error.Timeout() == true`; connection resets via `errors.Is(err, syscall.ECONNRESET)`. `context.Canceled` / `context.DeadlineExceeded` NEVER retried.
- **D-76:** Retry-After parsed via `parseRetryAfter(h string, now time.Time) (time.Duration, bool)` (integer seconds via `strconv.Atoi`, HTTP-date via `http.ParseTime`). Per-attempt sleep = `max(retryAfter, jitterDelay)` capped at `maxRetryWait`. When absent: full-jitter exponential backoff via `math/rand/v2.Int64N(int64(min(maxRetryWait, baseDelay << attempt)))`.
- **D-77:** Retry layer lives in **`retry.go` at repo root**: `retryConfig`, `shouldRetry(resp, err) bool`, `parseRetryAfter`, `computeBackoff`. Wraps `c.http.Do(req)` + 4xx/5xx error-build inside `doJSONGet` via a `for attempt := 0; attempt < cfg.maxAttempts; attempt++` loop with `ctx.Err()` check at loop-top and `c.sleepFunc(ctx, computed)` between attempts. 2xx decode path runs once after the loop succeeds; mid-truncation / decode / oversize gates are NOT retried.
- **D-78:** Per-Client `*rand.Rand` initialized at `NewClient` via `rand.NewChaCha8(seed)` with `seed` from `crypto/rand.Read`. Stored as `c.rand`.

**Cache (RESIL-06..09; TEST-06):**

- **D-79:** Public `Cache` interface: `Get(key string) (value []byte, ok bool)` / `Put(key string, value []byte)` / `Close() error`. TTL owned by the cache (not per-Put).
- **D-80:** Two cache options: `WithCache(ttl time.Duration) Option` (default in-memory; `ttl <= 0` disables) and `WithCacheBackend(c Cache) Option` (last-wins override; `Client.Close()` calls `c.Close()`). Recorded as **CL-14**.
- **D-81:** Exported `type MemoryCache struct { /* unexported */ }`; `NewMemoryCache(ttl time.Duration) *MemoryCache`. Backing: `map[string]entry{ value []byte; expiresAt time.Time }` + `sync.RWMutex`. Lives in `cache.go` at repo root.
- **D-82:** Cache key = `req.Method + " " + req.URL.Path + "?" + req.URL.Query().Encode()`. Excludes Host (per-Client isolation per Pitfall CACHE-2).
- **D-83:** Cacheability gate in `cacheTransport.RoundTrip` (lives in `transport_cache.go`). Path-prefix allow-list: `/Countries`, `/Languages`, `/Subdivisions`. Only `err == nil && resp.StatusCode == 200` cache. Body read via `io.LimitReader(resp.Body, maxResponseBytes+1)`, then `resp.Body` replaced with `io.NopCloser(bytes.NewReader(buf))`.
- **D-84:** Sweeper goroutine: lazy start on first `Put` via `sync.Once`. Interval = `max(ttl/4, 30*time.Second)`. Stop via `cancel()` on sweeper ctx + tiny done-channel wait in `Close()`. Idempotent.
- **D-85:** `Client.Close()` flips existing `closed atomic.Bool` (Phase 2 D-40) inside `c.closeOnce.Do(...)`; calls `_ = c.cache.Close()` when cache non-nil. Errors from `cache.Close()` are intentionally swallowed.
- **D-86:** Cache↔clock wiring: public `NewMemoryCache(ttl)` uses `time.Now`. Sibling unexported `newMemoryCacheWithClock(ttl, now func() time.Time) *MemoryCache` used internally by `WithCache(ttl)` so production+tests share `c.nowFunc`.

**Hook (TRANS-05):**

- **D-87:** `WithRequestHook(func(*http.Request, *http.Response, error)) Option`. Function literal — no `Hook` interface.
- **D-88:** Fires after every real round trip AND every cache-hit synthetic response. Fires per retry attempt. Does NOT fire on decode errors or pre-HTTP failures.
- **D-89:** Hook is OUTERMOST RoundTripper. Revised chain: `req → hookTransport → cacheTransport → loggingTransport → headerTransport → underlying`.
- **D-90:** Synchronous; panics propagate (no recover).

**Strict-decoding (OBS-03):**

- **D-91:** `WithStrictDecoding(strict bool) Option`. Sets immutable `clientConfig.strictDecoding bool` → `Client.strict bool`. No per-call override. Recorded as **CL-15**.
- **D-92:** One-line gate in `doJSONGet`: `if c.strict { decoder.DisallowUnknownFields() }` BEFORE `decoder.Decode(&out)`.
- **D-93:** Cache hits re-run the strict decoder on every read — intended behavior, NOT a bug.

**Test seam:**

- **D-94:** Internal `nowFunc func() time.Time` + `sleepFunc func(context.Context, time.Duration) error` on Client. Defaults `time.Now` + `ctxSleep`. Test access via unexported `newClientForTest(...)` inside `package openholidays` `_test.go` files.
- **D-95:** Hand-rolled `fakeClock` in `clock_test.go`: `{ mu sync.Mutex; now time.Time }` with `Now()` + `Advance(d time.Duration)`.
- **D-96:** Goroutine-leak audit via `runtime.NumGoroutine()` delta (consistent with Phase 2 D-49). No `go.uber.org/goleak`.

**Plan sequencing:**

- **D-97:** Recommended order: (1) strict-decoding plumbing → (2) fake-clock seam → (3) retry layer → (4) cache layer → (5) hook RoundTripper → (6) PROJECT.md Key Decisions append (CL-14, CL-15).

### Claude's Discretion

- File layout: `retry.go`, `cache.go`, `transport_hook.go`, `transport_cache.go` at repo root with sibling `_test.go` files. `clock_test.go` for the shared fake-clock helper.
- Every new exported symbol gets a godoc starting with the symbol name (Gold Rule 1 + PROJECT.md).
- Every error string starts with `"openholidays: "` (Phase 1 D-23).
- Tests use testify (Gold Rule 3): one `TestXxx` per exported prod function; every case in `t.Run`; `require` for preconditions, `assert` for verifications.
- `package openholidays` (not `_test`) for tests that need the `nowFunc`/`sleepFunc` seam or `newClientForTest`.
- `httptest.NewServer` backs HTTP tests (no live network in unit tests, Phase 2 D-46).
- No new exported sentinels. Retry-exhausted errors wrap `lastErr` via `fmt.Errorf("openholidays: retry exhausted (%d attempts): %w", attempts, lastErr)`.
- 10 MiB cap + drain-and-close pattern from Phase 2/3 (D-25, D-45, D-62) extends to `cacheTransport`.
- Default values as named consts: `defaultMaxRetryWait`, `defaultBaseDelay`, `minSweeperInterval`.
- `cacheTransport` sets `resp.ContentLength = int64(len(cachedBytes))` on synthetic cache-hit responses so OBS-02 `bytes_in` is correct.
- Exported `CacheHitContextKey` (unique key type) lets consumers detect cache hits inside their hook via `req.Context().Value(openholidays.CacheHitContextKey)`.

### Deferred Ideas (OUT OF SCOPE)

- Negative caching (`WithNegativeCacheTTL`) — v0.2 if a real consumer asks.
- Persistent cache backends (Redis, BoltDB) — out of v0.1.0.
- Retry-After cumulative-budget cap (`WithMaxRetryBudget`) — consumers use `ctx.WithTimeout`.
- Per-status custom retry policies (`WithRetryStatus`) — v0.2.
- Async hook variant — explicit no.
- Per-call strict-decoding override — out.
- `WithCacheable(paths ...string)` to add custom cacheable endpoints — out.
- Distributed-tracing propagation in the SDK — out (zero-dep).
- Cache `Vary`-header awareness (RFC 7234) — out.
- Connection-pool tuning — consumers supply their own `*http.Client`.
- `go.uber.org/goleak` adoption — out; revisit if real flakes surface.
- Retry attempt counter wired into `loggingTransport.attempt` — Phase 5 / v0.2.

</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| RESIL-01 | `WithRetry(maxAttempts, baseDelay)` opt-in retry with exp backoff + full jitter using `math/rand/v2` | Confirmed via D-73/D-77/D-78; `math/rand/v2` available in Go 1.22+ (project floor is 1.23, verified `go.mod` line 3) `[VERIFIED: go.mod]` |
| RESIL-02 | Retry on 429, 5xx, transient net errors; not on other 4xx | D-75 matrix; `syscall.ECONNRESET` is stdlib (zero-dep preserved); `net.Error.Timeout()` is the canonical method `[VERIFIED: pkg.go.dev/net#Error]` |
| RESIL-03 | Retry-After in seconds + HTTP-date forms; uses larger of jitter delay and Retry-After | D-76 + Pitfall RETRY-2 verbatim formula; `http.ParseTime` accepts RFC 1123 + RFC 850 + ANSI C asctime per RFC 7231 §7.1.1.1 `[VERIFIED: pkg.go.dev/net/http#ParseTime]` |
| RESIL-04 | Retry loop is ctx-aware; `ctx.Done()` interrupts sleep | D-77 loop pseudocode with `ctx.Err()` at loop-top + `c.sleepFunc(ctx, d)` between attempts |
| RESIL-05 | Retry in endpoint layer (NOT a RoundTripper) | D-77 explicit; CONTEXT.md `<code_context>` confirms `doJSONGet` is the injection point |
| RESIL-06 | Public `Cache` interface | D-79 shape `Get/Put/Close`; cache-owned TTL |
| RESIL-07 | `WithCache(ttl)` opt-in; reference endpoints only | D-83 path-prefix allow-list `{/Countries, /Languages, /Subdivisions}` |
| RESIL-08 | Sweeper started lazily on first Set; stopped by `Client.Close()`; no leak | D-84 lazy `sync.Once`; D-85 `Close()` wiring; D-96 `runtime.NumGoroutine()` delta check |
| RESIL-09 | Raw bytes keyed by (method, path, query); decode on read | D-82 key formula; D-83 stores buffer; D-92 strict gate runs on every decode |
| TRANS-05 | `WithRequestHook(func(*http.Request, *http.Response, error))` | D-87 signature; D-88 contract; D-89 outermost placement |
| OBS-03 | `WithStrictDecoding(bool)` uses `DisallowUnknownFields` | D-91 immutable flag; D-92 one-line gate before Decode |
| TEST-05 | Retry/backoff tests with fake transport returning 429/500 then 200; verifies jitter, Retry-After, ctx-cancel | D-94 `nowFunc`+`sleepFunc`; D-95 `fakeClock` |
| TEST-06 | Cache tests cover TTL eviction, hit/miss, default-off; fake clock | D-86 `newMemoryCacheWithClock`; D-95 `fakeClock`; D-96 leak audit |

**CLIENT-08** is also affected: the existing Phase 2 stub `Close()` becomes load-bearing per D-85 (adds `closeOnce sync.Once`; calls `cache.Close()` inside the once-guarded block).

</phase_requirements>

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Retry loop (status + transport errors) | Endpoint layer (`doJSONGet`) | — | RESIL-05 explicit: must NOT be a RoundTripper so caller-supplied `*http.Client` retries do not double-fire. The 4xx/5xx error-build is post-status; only the `c.http.Do` + status-build path is retried. |
| Backoff computation + Retry-After parsing | Endpoint layer (`retry.go` helpers) | — | Pure functions called by the retry loop. Lives next to `doJSONGet` (one concern per file). |
| Cache lookup / store | Transport layer (`cacheTransport.RoundTrip`) | — | RESIL-09 + Pattern 4: cache lives BELOW endpoint layer so endpoints stay thin; raw bytes are cached, not decoded values. Hits short-circuit the chain. |
| TTL eviction (sweeper goroutine) | Cache layer (`MemoryCache`) | Client lifecycle | Sweeper goroutine is owned by `MemoryCache`; lifetime is bound to `Client.Close()` via the `Cache.Close()` interface call. |
| Observability hook | Transport layer (`hookTransport`) — outermost | — | Must see cache hits → must be ABOVE `cacheTransport`. Synchronous so consumers own their goroutines. |
| Strict-decoding gate | Endpoint layer (`doJSONGet`) | — | Applied AFTER cache returns bytes; ensures strict mode applies to cached entries too. One-line gate before `decoder.Decode`. |
| `Client.Close()` shutdown | Client lifecycle (`client.go`) | Cache layer | Reuses Phase 2 `closed atomic.Bool`; adds `closeOnce sync.Once`; calls `cache.Close()` inside the once-guarded block. |
| Deterministic test clock | Client (test seam) | — | Internal `nowFunc` + `sleepFunc` fields on `*Client`. Unexported; test access via `newClientForTest` in same-package `_test.go`. |

## Standard Stack

### Core (all stdlib; zero runtime deps preserved)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `net/http` | stdlib | `RoundTripper` chain; `http.ParseTime` for Retry-After RFC 7231 dates `[VERIFIED: pkg.go.dev/net/http]` | The community-default HTTP client. `http.ParseTime` accepts all three RFC 7231 §7.1.1.1 date formats (RFC 1123, RFC 850, ANSI C asctime). |
| `math/rand/v2` | stdlib (Go 1.22+) | Per-Client jitter source via `rand.NewChaCha8(seed)` + `Int64N` `[VERIFIED: pkg.go.dev/math/rand/v2]` | No global seed footgun; `Int64N` is the v2-idiomatic name (v1 was `Int63n`). |
| `crypto/rand` | stdlib | Seeds the per-Client `*rand.Rand` at `NewClient` (Pitfall RETRY-4) | Avoids fleet-wide thundering herds: each Client has an independent seed. |
| `sync` | stdlib | `sync.RWMutex` (MemoryCache); `sync.Once` (sweeper start + `Close`); `atomic.Bool` (reuse Phase 2 `closed`) | Pitfall CACHE-4 / CACHE-5 explicit: RWMutex over `sync.Map` for read-heavy + re-write-on-expiry workload. |
| `context` | stdlib | Cancellable sleep via `select { case <-ctx.Done(): ... case <-timer.C: ... }`; cache hit signal via context value | CONTEXT.md `<specifics>` recommends `CacheHitContextKey` (exported unique-type key per Go context-key convention). |
| `bytes`, `io` | stdlib | `bytes.NewReader(buf)`, `io.NopCloser`, `io.LimitReader`, `io.Copy(io.Discard, …)` for drain-and-close | Phase 2 D-45 / Pitfall HTTP-3 pattern extends to `cacheTransport` body buffering. |
| `encoding/json` | stdlib | `json.Decoder.DisallowUnknownFields()` for strict mode | One-line gate; documented in `pkg.go.dev/encoding/json#Decoder.DisallowUnknownFields`. |
| `syscall` | stdlib | `syscall.ECONNRESET` for connection-reset detection in `shouldRetry` | Cross-platform stdlib symbol; `errors.Is(err, syscall.ECONNRESET)` works on Linux/macOS/Windows via Go's syscall package abstraction `[VERIFIED: pkg.go.dev/syscall]`. |
| `strconv` | stdlib | `strconv.Atoi` for integer-seconds form of `Retry-After` | Pitfall RETRY-2 verbatim. |
| `time` | stdlib | `time.NewTimer`, `time.Duration`, comparison helpers | Phase 1/2 already-used. |

### Supporting (test-only — pre-approved per PROJECT.md)

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/stretchr/testify` | v1.11.1 (already in go.mod, line 5) `[VERIFIED: go.mod]` | Primary assertion library per Gold Rule 3 | Every Phase 4 test file (`require` for preconditions, `assert` for verifications). |
| `net/http/httptest` | stdlib | Fake transport for retry tests; cache-hit assertions | Phase 2 D-46 already-used pattern; Phase 4 retry tests need 429/500→200 sequence injection. |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Per-Client `*rand.Rand` + ChaCha8 seed | Global `math/rand/v2` | Same-seed-across-fleet thundering herd risk (Pitfall RETRY-4). Rejected. |
| `sync.RWMutex + map` | `sync.Map` | `sync.Map` is for "write-once read-many" or "disjoint key sets"; TTL caches re-write on expiry, hitting the wrong pattern (Pitfall CACHE-5). |
| Hand-rolled retry | `github.com/hashicorp/go-retryablehttp` / `github.com/cenkalti/backoff/v4` | Zero-dep policy + retry policy is ~80 LOC. Library wraps `*http.Client` (conflicts with `WithHTTPClient`). STACK.md HIGH confidence: "write our own". |
| Hand-rolled in-memory cache | `github.com/patrickmn/go-cache` | TTL pattern is ~50 LOC under RWMutex. Pulling an external cache surrenders control over eviction policy. STACK.md HIGH confidence. |
| `goleak.VerifyNone` | `runtime.NumGoroutine()` delta (D-96) | `goleak` is NOT in the pre-approved test-only allowlist. D-96 mirrors Phase 2 D-49 — consistent with prior practice. |
| Public `Clock` interface | Internal `nowFunc` + `sleepFunc` (D-94) | Public interface would add 1 option + 1 interface to v0.1.0. Internal fields keep the public surface unchanged. |
| Public `Hook` interface | Function literal (D-87) | Function shape is small and stable; interface adds public surface without payoff. |

**Installation:** None required. Every dependency is stdlib or already present in `go.mod` (testify v1.11.1).

**Version verification:**
- Project floor: Go 1.23 `[VERIFIED: go.mod line 3]`. Confirms `math/rand/v2` (Go 1.22+), `slog` (Go 1.21+), `iter.Seq` (Go 1.23+) all available.
- testify v1.11.1 `[VERIFIED: go.mod line 5]` — already in use throughout Phase 1-3 tests.

## Package Legitimacy Audit

> Phase 4 introduces **zero new external dependencies** (runtime or test-only). Every package referenced is either stdlib or already pinned in `go.mod` (testify v1.11.1). The Package Legitimacy Gate protocol is therefore informational only.

| Package | Registry | Age | Downloads | Source Repo | slopcheck | Disposition |
|---------|----------|-----|-----------|-------------|-----------|-------------|
| `net/http` | stdlib | — | — | golang.org | n/a | Approved (stdlib) |
| `math/rand/v2` | stdlib (Go 1.22+) | — | — | golang.org | n/a | Approved (stdlib) |
| `crypto/rand` | stdlib | — | — | golang.org | n/a | Approved (stdlib) |
| `syscall` | stdlib | — | — | golang.org | n/a | Approved (stdlib) |
| `github.com/stretchr/testify` | proxy.golang.org | 10+ years | de-facto Go standard | github.com/stretchr/testify | n/a | Already in go.mod (test-only) |

**Packages removed due to slopcheck [SLOP] verdict:** none.
**Packages flagged as suspicious [SUS]:** none.

No new packages installed → no slopcheck run needed. The CONTEXT.md zero-dep rule is preserved.

## Architecture Patterns

### System Architecture Diagram

```
                  Caller goroutine
                        │
                        ▼
        ┌─────────────────────────────────┐
        │  c.PublicHolidays(ctx, req)     │   endpoint method
        │  c.Countries(ctx, req)          │   (unchanged from Phase 3)
        │  ...                            │
        └─────────────────┬───────────────┘
                          │
                          ▼
        ┌─────────────────────────────────┐
        │  doJSONGet[T](ctx, c, path, q)  │   request.go
        │  ─────────────────────────────  │   (gains retry loop wrapper
        │  for attempt := 0; ...          │    + 1-line strict gate)
        │    if ctx.Err() != nil { … }    │   ← D-77 loop top
        │    resp, err := c.http.Do(req)  │
        │                  │              │
        │                  ▼              │
        │    [RoundTripper chain below]   │
        │                                 │
        │    if !shouldRetry(resp,err) {  │   retry.go
        │      break                      │   ← D-75 matrix
        │    }                            │
        │    delay := computeBackoff(...) │   ← D-76 + D-78
        │    c.sleepFunc(ctx, delay)      │   ← D-77 ctx-aware sleep
        │  end loop                       │
        │                                 │
        │  decoder := json.NewDecoder(…)  │
        │  if c.strict {                  │   ← D-92 one-line gate
        │    decoder.DisallowUnknownFields()
        │  }                              │
        │  decoder.Decode(&out)           │
        └─────────────────┬───────────────┘
                          │
                          ▼ (c.http.Do enters here)
        ┌─────────────────────────────────┐
        │  hookTransport (outermost)      │   transport_hook.go — D-89
        │  ─────────────────────────────  │
        │  resp, err := next.RoundTrip(r) │
        │  if hook != nil {               │
        │    hook(r, resp, err)           │   ← D-88 fires on every attempt
        │  }                              │      including cache hits
        │  return resp, err               │
        └─────────────────┬───────────────┘
                          │
                          ▼
        ┌─────────────────────────────────┐
        │  cacheTransport                 │   transport_cache.go — D-83
        │  ─────────────────────────────  │
        │  if path in allow-list {        │   {/Countries, /Languages,
        │    if cached := cache.Get(k) {  │    /Subdivisions}
        │      return synthRespFromCache  │   ← D-83 NopCloser body
        │    }                            │      + ContentLength set
        │  }                              │   ← <specifics> 7
        │  resp, err := next.RoundTrip(r) │
        │  if cacheable {                 │
        │    buf := LimitReader+ReadAll   │   ← D-83 + 10 MiB cap
        │    resp.Body = NopCloser(buf)   │
        │    cache.Put(k, buf)            │   ← D-83 success-only
        │  }                              │
        │  return resp, err               │
        └─────────────────┬───────────────┘
                          │
                          ▼
        ┌─────────────────────────────────┐
        │  loggingTransport (Phase 2)     │   transport.go (UNCHANGED)
        └─────────────────┬───────────────┘
                          │
                          ▼
        ┌─────────────────────────────────┐
        │  headerTransport (Phase 2)      │   transport.go (UNCHANGED)
        └─────────────────┬───────────────┘
                          │
                          ▼
        ┌─────────────────────────────────┐
        │  base transport (stdlib)        │   network
        └─────────────────────────────────┘


    Client lifecycle (parallel concern)
    ─────────────────────────────────────
        Client.Close() ──────► c.closeOnce.Do(func() {
                                   c.closed.Store(true)       ← Phase 2 D-40
                                   _ = c.cache.Close()         ← D-85
                               })
                                          │
                                          ▼
                               MemoryCache.Close():
                                   cancel(sweeperCtx)
                                   wait done-channel (1ms)
                                          │
                                          ▼
                               Sweeper goroutine (D-84):
                                   started lazily on first Put
                                   for { select {
                                       case <-ctx.Done(): exit
                                       case <-ticker.C: evict expired
                                   }}
```

### Recommended Project Structure (delta from Phase 3)

```
.
├── retry.go                   # NEW — retryConfig, shouldRetry, parseRetryAfter, computeBackoff
├── retry_test.go              # NEW — retry tests with fakeClock
├── cache.go                   # NEW — Cache interface, MemoryCache, NewMemoryCache, newMemoryCacheWithClock
├── cache_test.go              # NEW — TTL eviction, leak audit, fakeClock-driven tests
├── transport_cache.go         # NEW — cacheTransport RoundTripper
├── transport_cache_test.go    # NEW — path allow-list, success-only, synthetic response
├── transport_hook.go          # NEW — hookTransport RoundTripper
├── transport_hook_test.go     # NEW — fires per attempt, sees cache hits, sync contract
├── clock_test.go              # NEW — fakeClock test helper (test-only)
├── client.go                  # MODIFIED — gains closeOnce sync.Once, nowFunc, sleepFunc, rand, strict, cache fields; Close() body
├── config.go                  # MODIFIED — gains retry/cache/cacheTTL/hook/strictDecoding fields; buildTransport revised
├── options.go                 # MODIFIED — gains 6 new WithX options
├── request.go                 # MODIFIED — doJSONGet gains retry loop wrapper + 1-line strict gate
├── transport.go               # UNCHANGED — headerTransport + loggingTransport keep Phase 2 shape
├── internal_test.go           # UNCHANGED — no new exported sentinels in Phase 4
└── ... (all other Phase 1-3 files unchanged)
```

### Pattern 1: Retry inside `doJSONGet` (NOT a RoundTripper)

**What:** A `for attempt := 0; attempt < maxAttempts; attempt++` loop wraps the `c.http.Do(req)` + 4xx/5xx error-build steps INSIDE `doJSONGet`. The decode path runs once after the loop succeeds.

**When to use:** When the SDK must avoid double-firing with caller-supplied `*http.Client` retry middleware. RESIL-05 mandates this for go-openholidays.

**Example (anchored on the verified Phase 3 `doJSONGet` body in `request.go` lines 58–125):**

```go
// retry.go — D-77
type retryConfig struct {
    maxAttempts int
    baseDelay   time.Duration
    maxWait     time.Duration
}

func shouldRetry(resp *http.Response, err error) bool {
    // ctx.Err() handled by caller — NEVER retry on context.Canceled / DeadlineExceeded
    if err != nil {
        var netErr net.Error
        if errors.As(err, &netErr) && netErr.Timeout() {
            return true
        }
        if errors.Is(err, syscall.ECONNRESET) {
            return true
        }
        return false
    }
    switch resp.StatusCode {
    case 408, 429, 500, 502, 503, 504:
        return true
    }
    return false
}

func parseRetryAfter(h string, now time.Time) (time.Duration, bool) {
    if h == "" {
        return 0, false
    }
    if s, err := strconv.Atoi(h); err == nil && s >= 0 {
        return time.Duration(s) * time.Second, true
    }
    if t, err := http.ParseTime(h); err == nil {  // RFC 7231 §7.1.1.1 — RFC 1123 + RFC 850 + ANSI C asctime
        if d := t.Sub(now); d > 0 {
            return d, true
        }
    }
    return 0, false
}

func computeBackoff(attempt int, retryAfter time.Duration, cfg retryConfig, rnd *rand.Rand) time.Duration {
    // full jitter (AWS canonical) — math/rand/v2
    capped := cfg.maxWait
    if exp := cfg.baseDelay << attempt; exp < capped {
        capped = exp
    }
    if capped <= 0 {
        capped = time.Millisecond
    }
    jitter := time.Duration(rnd.Int64N(int64(capped)))
    if retryAfter > 0 && retryAfter > jitter {
        return min(retryAfter, cfg.maxWait)
    }
    return jitter
}
```

Inside `doJSONGet` (delta to the existing `request.go` lines 75–88):

```go
var (
    resp    *http.Response
    httpErr error
)
maxAttempts := 1
if c.retry.maxAttempts > 0 {
    maxAttempts = c.retry.maxAttempts
}
for attempt := 0; attempt < maxAttempts; attempt++ {
    if err := ctx.Err(); err != nil {
        return zero, err  // D-75: never retry ctx errors
    }
    resp, httpErr = c.http.Do(req)
    if !shouldRetry(resp, httpErr) {
        break
    }
    // last attempt — surface the error without sleeping
    if attempt == maxAttempts-1 {
        break
    }
    var retryAfter time.Duration
    if resp != nil {
        retryAfter, _ = parseRetryAfter(resp.Header.Get("Retry-After"), c.nowFunc())
        // drain+close the about-to-be-retried response (Pitfall HTTP-3)
        _, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes+1))
        _ = resp.Body.Close()
    }
    delay := computeBackoff(attempt, retryAfter, c.retry, c.rand)
    if err := c.sleepFunc(ctx, delay); err != nil {
        return zero, err  // ctx.Err() on cancel during sleep
    }
}
if httpErr != nil {
    // wrap with retry-exhaustion context if attempts > 1
    if maxAttempts > 1 {
        return zero, fmt.Errorf("openholidays: retry exhausted (%d attempts): %w", maxAttempts, httpErr)
    }
    return zero, fmt.Errorf("openholidays: GET %s: %w", path, httpErr)
}
// resp is now the final response — drain-and-close defer + 4xx/5xx + decode runs once
```

### Pattern 2: Cache as RoundTripper (`cacheTransport`)

**What:** A `cacheTransport struct { cache Cache; next http.RoundTripper }` checks the path-prefix allow-list, then either returns a synthetic response from cache OR forwards and stores the result.

**When to use:** RESIL-09 + ARCHITECTURE.md Pattern 4 explicit — cache raw bytes at the HTTP boundary so endpoint methods stay thin and strict-decoding gate applies on every read.

**Example:**

```go
// transport_cache.go
type cacheTransport struct {
    cache         Cache
    cacheablePath func(path string) bool  // returns true for /Countries, /Languages, /Subdivisions
    next          http.RoundTripper
}

// CacheHitContextKey is the context-value key set by cacheTransport when a
// response is served from cache. Consumers can detect cache hits inside their
// WithRequestHook via req.Context().Value(openholidays.CacheHitContextKey).
type cacheHitKeyType struct{}
var CacheHitContextKey = cacheHitKeyType{}

func (t *cacheTransport) RoundTrip(req *http.Request) (*http.Response, error) {
    if !t.cacheablePath(req.URL.Path) {
        return t.next.RoundTrip(req)
    }
    key := cacheKey(req)  // D-82: req.Method + " " + req.URL.Path + "?" + req.URL.Query().Encode()
    if cached, ok := t.cache.Get(key); ok {
        // Signal cache hit to outer hookTransport via context value.
        // The request and synthetic response carry this value.
        ctxWithHit := context.WithValue(req.Context(), CacheHitContextKey, true)
        synth := &http.Response{
            StatusCode:    http.StatusOK,
            Status:        "200 OK",
            Header:        make(http.Header),
            Body:          io.NopCloser(bytes.NewReader(cached)),
            ContentLength: int64(len(cached)),  // <specifics> 7: correct bytes_in on cache hits
            Request:       req.WithContext(ctxWithHit),
        }
        return synth, nil
    }
    resp, err := t.next.RoundTrip(req)
    if err != nil || resp.StatusCode != http.StatusOK {
        return resp, err  // D-83: success-only cache
    }
    // Buffer the body so downstream decoder + cache both see it.
    limited := io.LimitReader(resp.Body, maxResponseBytes+1)
    buf, readErr := io.ReadAll(limited)
    // Drain-and-close the original body (Pitfall HTTP-3).
    _, _ = io.Copy(io.Discard, resp.Body)
    _ = resp.Body.Close()
    if readErr != nil {
        return nil, readErr  // surface as transport error
    }
    // Only store if within the 10 MiB cap (Pitfall HTTP-4).
    if len(buf) <= maxResponseBytes {
        t.cache.Put(key, buf)
    }
    resp.Body = io.NopCloser(bytes.NewReader(buf))
    resp.ContentLength = int64(len(buf))
    return resp, nil
}
```

### Pattern 3: Hook RoundTripper (outermost)

**What:** `hookTransport{hook RequestHookFunc; next http.RoundTripper}` fires the user's hook synchronously after every round trip — including cache hits returned by `cacheTransport`.

**Example:**

```go
// transport_hook.go
type RequestHookFunc func(*http.Request, *http.Response, error)

type hookTransport struct {
    hook RequestHookFunc
    next http.RoundTripper
}

func (t *hookTransport) RoundTrip(req *http.Request) (*http.Response, error) {
    resp, err := t.next.RoundTrip(req)
    if t.hook != nil {
        t.hook(req, resp, err)  // D-90: synchronous; panics propagate; consumer owns goroutines
    }
    return resp, err
}
```

### Pattern 4: Lazy Sweeper via `sync.Once` (`MemoryCache`)

**What:** The sweeper goroutine starts on the first `Put` (not at `NewMemoryCache`), so a constructed-but-never-used cache spawns nothing. `Close()` cancels the sweeper ctx and waits ≤ 1 ms for the goroutine to exit.

**Example:**

```go
// cache.go
type MemoryCache struct {
    ttl          time.Duration
    nowFn        func() time.Time
    mu           sync.RWMutex
    entries      map[string]entry
    startOnce    sync.Once
    sweepCtx     context.Context
    sweepCancel  context.CancelFunc
    sweepDone    chan struct{}
    closeOnce    sync.Once
}

type entry struct {
    value     []byte
    expiresAt time.Time
}

const minSweeperInterval = 30 * time.Second

func NewMemoryCache(ttl time.Duration) *MemoryCache {
    return newMemoryCacheWithClock(ttl, time.Now)
}

func newMemoryCacheWithClock(ttl time.Duration, nowFn func() time.Time) *MemoryCache {
    ctx, cancel := context.WithCancel(context.Background())
    return &MemoryCache{
        ttl:         ttl,
        nowFn:       nowFn,
        entries:     make(map[string]entry),
        sweepCtx:    ctx,
        sweepCancel: cancel,
        sweepDone:   make(chan struct{}),
    }
}

func (m *MemoryCache) Get(key string) ([]byte, bool) {
    m.mu.RLock()
    e, ok := m.entries[key]
    m.mu.RUnlock()
    if !ok || m.nowFn().After(e.expiresAt) {
        return nil, false
    }
    return e.value, true
}

func (m *MemoryCache) Put(key string, value []byte) {
    m.mu.Lock()
    m.entries[key] = entry{value: value, expiresAt: m.nowFn().Add(m.ttl)}
    m.mu.Unlock()
    m.startOnce.Do(m.startSweeper)  // D-84: lazy
}

func (m *MemoryCache) startSweeper() {
    interval := m.ttl / 4
    if interval < minSweeperInterval {
        interval = minSweeperInterval
    }
    go m.sweepLoop(interval)
}

func (m *MemoryCache) sweepLoop(interval time.Duration) {
    defer close(m.sweepDone)
    t := time.NewTicker(interval)
    defer t.Stop()
    for {
        select {
        case <-m.sweepCtx.Done():
            return
        case <-t.C:
            m.evict()
        }
    }
}

func (m *MemoryCache) evict() {
    now := m.nowFn()
    m.mu.Lock()
    for k, e := range m.entries {
        if now.After(e.expiresAt) {
            delete(m.entries, k)
        }
    }
    m.mu.Unlock()
}

func (m *MemoryCache) Close() error {
    m.closeOnce.Do(func() {
        m.sweepCancel()
        // Wait briefly for the sweeper to exit; tolerate "never started" via startOnce.
        select {
        case <-m.sweepDone:
        case <-time.After(time.Millisecond):
        }
    })
    return nil
}
```

### Anti-Patterns to Avoid

- **Putting retry as a RoundTripper:** Conflicts with caller-supplied `*http.Client` retry middleware (RESIL-05). Verified by reading `request.go` — retry wraps `c.http.Do(req)` inside `doJSONGet`.
- **Caching at the method call site:** Pattern 4 explicit anti-pattern. Caches typed `[]Holiday` instead of raw bytes, can't compose with `WithRequestHook`, requires repeating logic per endpoint.
- **`sync.Map` for the TTL cache:** Pitfall CACHE-5 — `sync.Map` is for "write-once read-many" or "disjoint key sets". TTL caches re-write on expiry (wrong pattern).
- **`time.Sleep(delay)` in the retry loop:** Pitfall RETRY-3 — uninterruptible. Use `select { case <-ctx.Done(): case <-timer.C: }` (this is what `ctxSleep` from D-94 does).
- **`math/rand` (v1) for jitter:** Requires manual seeding (footgun) + not concurrent-safe without `rand.New(rand.NewSource(...))` boilerplate. Use `math/rand/v2` (Go 1.22+).
- **`json.Decoder.UseNumber()` instead of `DisallowUnknownFields()`:** Wrong semantics. Strict mode is about rejecting unknown fields, not number representation.
- **Putting `hookTransport` BELOW `cacheTransport`:** Cache hits short-circuit the chain at `cacheTransport`; hook below cache would miss them. D-89 explicit.
- **Calling `cache.Close()` outside `closeOnce.Do(...)`:** Concurrent `Client.Close()` calls would race on the cancel-channel close. D-85 wraps the call in `c.closeOnce.Do(...)`.
- **Cache key including `req.URL.Host`:** Wastes bytes; per-Client isolation already mitigates Pitfall CACHE-2 (D-82 rationale).
- **Logging response body content in the hook godoc example:** Pitfall LOG-1 — hook example MUST document "do not log body at Info or above".
- **Failing to drain+close `resp.Body` before replacing with `NopCloser`:** Pitfall HTTP-3 — keep-alive connection won't return to pool. The `cacheTransport.RoundTrip` example above does this explicitly.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| `Retry-After` HTTP-date parsing | Hand-rolled multi-format parser | `http.ParseTime(h)` | Stdlib accepts all three RFC 7231 §7.1.1.1 forms (RFC 1123, RFC 850, ANSI C asctime). `[VERIFIED: pkg.go.dev/net/http#ParseTime]` |
| Concurrent-safe jitter source | Per-Client `sync.Mutex` + `math/rand` (v1) | `math/rand/v2.NewChaCha8(seed)` | `math/rand/v2` is goroutine-safe per source; per-Client seed avoids Pitfall RETRY-4 fleet-wide herd. |
| Ctx-aware sleep | `time.Sleep` + best-effort cancellation | `select { case <-ctx.Done(): case <-timer.C: }` (the `ctxSleep` helper from D-94) | The stdlib pattern is well-known; D-94 codifies it. |
| Synthetic `*http.Response` Body | Naked `*bytes.Reader` | `io.NopCloser(bytes.NewReader(buf))` | `*http.Response.Body` must satisfy `io.ReadCloser`. `NopCloser` is the stdlib idiom; downstream `defer resp.Body.Close()` works correctly. |
| Map-based concurrent cache | Naked `map[string]…` + `sync.Mutex` | `sync.RWMutex` + plain map (D-81) | Read-heavy + re-write-on-expiry — RWMutex wins per Pitfall CACHE-4/5. |
| Connection-reset detection | Substring match on `err.Error()` | `errors.Is(err, syscall.ECONNRESET)` | Stdlib syscall constant; portable across Linux/macOS/Windows because Go's `syscall` package abstracts the value. `[VERIFIED: pkg.go.dev/syscall]` |
| Goroutine-leak detection in tests | Manual sleep + asserts | `runtime.NumGoroutine()` delta (D-96) | Pre-approved per Phase 2 D-49. Avoids adding `go.uber.org/goleak` as a test-only dep. |
| Functional options framework | Generic options builder | Hand-rolled `Option func(*clientConfig)` (existing `options.go`) | Phase 2 already-established pattern. New options follow the same shape. |

**Key insight:** Phase 4 introduces zero new abstractions beyond what `net/http`, `sync`, `math/rand/v2`, and `context` provide. Every "library" candidate (retryablehttp, backoff, go-cache, civil) has been explicitly rejected in STACK.md.

## Runtime State Inventory

> Phase 4 is **greenfield** (it adds new files and extends existing ones; no rename/refactor/migration). This section is INFORMATIONAL — there is no runtime state to migrate.

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | None — Phase 4 introduces the cache but it has no prior incarnation to migrate from. | none |
| Live service config | None — the OpenHolidays upstream has no SDK-side configuration; client behavior is governed by `clientConfig` only. | none |
| OS-registered state | None — the library does not register anything with the OS; the cache sweeper is an in-process goroutine. | none |
| Secrets/env vars | None — OpenHolidays has no auth; the library reads no env vars. | none |
| Build artifacts | None — `go build` produces no on-disk artifacts beyond the consumer's binary. | none |

**Why this section is included even for greenfield:** The orchestrator step 2.5 mandates explicit answers for all five categories. "Nothing in category" verified by reading `client.go`, `config.go`, `options.go` — no env-var reads, no filesystem writes, no OS hooks.

## Common Pitfalls

### Pitfall 1: Retry double-fire when caller supplies an `*http.Client` with its own retry middleware

**What goes wrong:** SDK puts retry as a RoundTripper. Caller passes `WithHTTPClient(myClient)` where `myClient.Transport` is `retryablehttp.Client.HTTPClient.Transport` (or similar). Both retry layers fire; a single 503 becomes 5×5 = 25 requests; the upstream sees abusive behavior.

**Why it happens:** "Retry as transport" looks idiomatic and composes with the chain, but it composes WITH ITSELF when stacked.

**How to avoid:** D-77 places retry INSIDE `doJSONGet` (endpoint layer). The retry loop wraps `c.http.Do(req)` + 4xx/5xx error-build only — it never reaches the caller's transport chain. Plans 03 must enforce this.

**Warning signs:**
- A new file `transport_retry.go` exists with `retryTransport` struct.
- `buildTransport` mentions retry in the chain.
- A test where caller's `*http.Client` retries are observed AT THE SDK BOUNDARY.

### Pitfall 2: Cache stores error responses

**What goes wrong:** Pitfall CACHE-1 verbatim. A 503 lands in cache with the full TTL; consumers see the stale error for 24h.

**How to avoid:** `cacheTransport.RoundTrip` checks `err == nil && resp.StatusCode == 200` before `cache.Put(key, buf)`. D-83 explicit. Plans 04 task-level verification MUST include "503 response not cached" test.

### Pitfall 3: Sweeper goroutine leak when consumer does not call `Close()`

**What goes wrong:** `c := NewClient(WithCache(1*time.Hour))` — consumer uses, walks away, never calls `Close()`. Sweeper goroutine runs forever.

**How to avoid:**
- D-84 lazy-starts the sweeper ONLY on first `Put`. A cache that's constructed-but-unused leaks nothing.
- README + `WithCache` godoc must include the `defer client.Close()` pattern.
- D-96 goroutine-leak audit catches missing-Close in tests via `runtime.NumGoroutine()` delta.

**Warning signs:**
- A test that constructs a `WithCache` Client, never calls `Close()`, asserts `runtime.NumGoroutine()` does NOT increase → would fail without the lazy-start design.

### Pitfall 4: Cache hit bypasses strict-decoding

**What goes wrong:** Consumer enables `WithStrictDecoding(true)`. Cache returns bytes that were stored BEFORE the upstream added a new field. The strict decoder should fail (correct schema-drift surfacing) but the cache returns decoded values directly.

**How to avoid:** D-93 explicit — cache stores RAW BYTES, decode runs on read. The `cacheTransport` returns a synthetic `*http.Response` with `io.NopCloser(bytes.NewReader(buf))`; `doJSONGet` then applies the strict gate (D-92) to those same bytes. Plan 04 task-level verification MUST include "cached bytes + strict mode → decoder rejects unknown field" test.

### Pitfall 5: Retry sleep ignores ctx cancellation

**What goes wrong:** Pitfall RETRY-3. `time.Sleep(delay)` is uninterruptible; caller's 30s ctx times out at 30s but the retry sleeps another 60s. The CLIENT-09 ≤100ms cancellation contract is broken.

**How to avoid:** D-94 `ctxSleep` uses `select { case <-ctx.Done(): case <-timer.C: }`. The `c.sleepFunc(ctx, delay)` call returns `ctx.Err()` immediately on cancel. The retry loop checks `err := c.sleepFunc(...)` and returns `ctx.Err()` if non-nil.

### Pitfall 6: Hook panic kills the SDK

**What goes wrong:** Consumer's hook function panics on a nil resp (transport error case). SDK has no recover; the panic propagates to the caller's goroutine.

**How to avoid:** D-90 explicit — panics propagate. The `WithRequestHook` godoc MUST document: "Hooks are invoked synchronously on the calling goroutine. A panicking hook propagates to the caller. Use defer/recover inside your hook if needed." Mirrors stdlib `http.Handler` convention. This is INTENTIONAL — silent recovery would hide bugs.

### Pitfall 7: Body not drained before replacement in `cacheTransport`

**What goes wrong:** Pitfall HTTP-3 — `resp.Body` replaced with `NopCloser(bytes.NewReader(buf))` without draining the original. The original keep-alive connection doesn't return to the pool; a subsequent request opens a fresh TCP connection.

**How to avoid:** `cacheTransport.RoundTrip` does `io.Copy(io.Discard, resp.Body)` + `resp.Body.Close()` BEFORE assigning the NopCloser. The example code above shows this.

### Pitfall 8: Same-jitter retries from many Clients

**What goes wrong:** Pitfall RETRY-4 — 100 instances of a service deploy. Each calls `NewClient(WithRetry(...))`. Each `*rand.Rand` seeded from `time.Now().UnixNano()` — at deploy time these are within microseconds of each other. All 100 retry at the same millisecond. Self-inflicted DDoS.

**How to avoid:** D-78 — per-Client `*rand.Rand` seeded via `crypto/rand.Read` (8-byte ChaCha8 seed). Independent across instances. Verified by reading `pkg.go.dev/math/rand/v2#NewChaCha8` `[CITED: pkg.go.dev]`.

### Pitfall 9: `parseRetryAfter` returns negative durations

**What goes wrong:** Server sends `Retry-After: <HTTP-date in the past>`. Without a `t.Sub(now) > 0` guard, the function returns a negative `time.Duration`. The retry loop sleeps "negative time" (immediately) — looks like ignoring Retry-After.

**How to avoid:** Pitfall RETRY-2 verbatim formula includes `if d := t.Sub(now); d > 0 { return d, true }`. The `now` parameter takes the Client's `nowFunc()` so tests can drive deterministic past/present/future scenarios.

### Pitfall 10: Cache key collision between two Clients sharing process

**What goes wrong:** Pitfall CACHE-2 — two Clients, two `baseURL`s, both call `Subdivisions("PL", "")`. Without `baseURL` in the cache key, the second Client gets the first's cached bytes.

**How to avoid:** D-82 + D-79 — cache lives on the `Client`, not globally. Two `Client`s = two `MemoryCache` instances. The `baseURL` is NOT in the key by design; per-Client isolation handles it. Plan 04 task-level verification MUST include a "two Clients with different baseURLs don't share cache" test.

### Pitfall 11: Boundary-truncation false positive on cached bytes

**What goes wrong:** `doJSONGet` includes a "boundary truncation" gate via `decoder.More()` (existing in `request.go` line 121). If cached bytes have trailing whitespace from the original HTTP/2 chunked response, the synthetic response might trigger `decoder.More() == true` falsely.

**How to avoid:** Existing `request.go` D-24 comment confirms `decoder.More()` correctly ignores RFC 8259 whitespace (newlines, spaces, tabs). The gate fires ONLY when another JSON value is starting, not on trailing whitespace. Verified by reading `request.go` lines 112–123.

### Pitfall 12: `runtime.NumGoroutine()` flakiness from background runtime goroutines

**What goes wrong:** D-96 `runtime.NumGoroutine() <= before` assertion is flaky if Go runtime spawns/exits background goroutines (GC, finalizer, network poller) during the test window.

**How to avoid:** Tests use `assert.LessOrEqual(t, after, before)` not `assert.Equal` — accepts ≤. A 10ms `time.Sleep` after `Close()` gives the sweeper time to exit. If flake re-surfaces, the fallback is `go.uber.org/goleak` (deferred — see Deferred Ideas).

## Code Examples

Verified patterns from official sources and existing codebase:

### `math/rand/v2` per-Client seed (D-78)

```go
// In client.go NewClient:
// Source: pkg.go.dev/math/rand/v2#NewChaCha8 + crypto/rand docs
var seed [32]byte
if _, err := crand.Read(seed[:]); err != nil {
    // crand.Read is documented to never fail on a healthy system; if it does,
    // fall back to a time-based seed. The library does not return an error from
    // NewClient (CLIENT-01), so panicking would violate the contract.
    binary.LittleEndian.PutUint64(seed[:8], uint64(time.Now().UnixNano()))
}
c.rand = rand.New(rand.NewChaCha8(seed))
```

Note: `crand` is `crypto/rand`; `rand` is `math/rand/v2`.

### `http.ParseTime` for Retry-After HTTP-date form

```go
// Source: pkg.go.dev/net/http#ParseTime — "ParseTime parses a time header
// (such as the Date: header), trying each of the three formats allowed by
// HTTP/1.1: time.RFC1123, time.RFC850, and time.ANSIC."
t, err := http.ParseTime("Wed, 21 Oct 2026 07:28:00 GMT")  // RFC 1123
t, err = http.ParseTime("Wednesday, 21-Oct-26 07:28:00 GMT")  // RFC 850
t, err = http.ParseTime("Wed Oct 21 07:28:00 2026")  // ANSI C asctime
```

### `json.Decoder.DisallowUnknownFields` (D-92)

```go
// Source: pkg.go.dev/encoding/json#Decoder.DisallowUnknownFields
// Existing request.go line 92: decoder := json.NewDecoder(limited)
if c.strict {
    decoder.DisallowUnknownFields()
}
// Existing line 93: if decodeErr := decoder.Decode(&out); decodeErr != nil {
```

When strict mode is on and the upstream response has an unknown field, `Decode` returns an error like `json: unknown field "extra_unknown_field"`. The existing `request.go` error wrapping (`return zero, fmt.Errorf("openholidays: decode %s: %w", path, decodeErr)`) covers this case — no new sentinel needed.

### Per-Client `sync.Once` Close (D-85)

```go
// In client.go — replaces existing Phase 2 Close stub at lines 81-84:
// Source: existing client.go (verified by reading)
func (c *Client) Close() error {
    c.closeOnce.Do(func() {
        c.closed.Store(true)
        if c.cache != nil {
            _ = c.cache.Close()
        }
    })
    return nil
}
```

### `fakeClock` test helper (D-95)

```go
// clock_test.go (test-only — never compiled into production)
type fakeClock struct {
    mu  sync.Mutex
    now time.Time
}

func newFakeClock(t time.Time) *fakeClock {
    return &fakeClock{now: t}
}

func (f *fakeClock) Now() time.Time {
    f.mu.Lock()
    defer f.mu.Unlock()
    return f.now
}

func (f *fakeClock) Advance(d time.Duration) {
    f.mu.Lock()
    f.now = f.now.Add(d)
    f.mu.Unlock()
}

// fakeSleep is a sleepFunc that synchronously advances the clock — no real waiting.
func (f *fakeClock) Sleep(ctx context.Context, d time.Duration) error {
    if err := ctx.Err(); err != nil {
        return err
    }
    f.Advance(d)
    return nil
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `math/rand` v1 with manual seeding | `math/rand/v2` (Go 1.22+) — auto-seeded, goroutine-safe per source | Go 1.22 (Feb 2024) | Phase 4 uses v2 throughout. D-78 explicit. |
| `time.Sleep` in retry loops | `select { case <-ctx.Done(): case <-timer.C: }` | Predates Go 1.22 — established pattern since contexts | D-94 `ctxSleep` codifies this. |
| `sync.Map` as default concurrent map | `sync.RWMutex` + plain map for non-write-once-read-many workloads | Go 1.9 docs explicit | D-81 RWMutex. Confirmed by Pitfall CACHE-5. |
| `http.NewRequest` + `req.WithContext(ctx)` | `http.NewRequestWithContext(ctx, …)` | Go 1.13 | Already used in `request.go` line 68. |
| `*http.Client.Timeout` for per-request timeout | `context.WithTimeout` (avoids golang/go#49521) | Bug filed 2021; widely-known by 2023 | Already used in `request.go` lines 63–67. |

**Deprecated/outdated:**
- `math/rand` v1 — still works, but v2 is now preferred for new code. Phase 4 must use v2.
- `net.Error.Temporary()` — deprecated in Go 1.18+. RESIL-02 mentions it but D-75 has updated to `net.Error.Timeout()` + `errors.Is(err, syscall.ECONNRESET)` as the canonical detection. Plan 03 must NOT use `Temporary()`.

## Assumptions Log

> Every factual claim in this research has been verified against the existing codebase, the canonical research files (`PITFALLS.md`, `ARCHITECTURE.md`, `STACK.md`), or stdlib documentation.

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `errors.Is(err, syscall.ECONNRESET)` is portable across Linux/macOS/Windows because Go's `syscall` package abstracts the value. | Standard Stack + Pitfall 8 | LOW — `syscall.ECONNRESET` exists on all three platforms `[VERIFIED: pkg.go.dev/syscall]`. On Windows the constant maps to `WSAECONNRESET` (`syscall.Errno(10054)`); on Linux/macOS it maps to errno 104/54. The `errors.Is` chain follows because `*net.OpError.Err` wraps the syscall error. If a wrapping mismatch surfaces, the fallback is to check `errors.Is(err, syscall.ECONNRESET) \|\| strings.Contains(err.Error(), "connection reset")` — but the latter is a code smell. Plans 03 should add a test that injects a synthetic `&net.OpError{Op: "read", Err: syscall.ECONNRESET}` and asserts `shouldRetry` returns true. |
| A2 | `min` builtin (Go 1.21+) is available for `min(maxRetryWait, baseDelay << attempt)`. | Pattern 1 retry example | LOW — Go 1.21 added `min`/`max` as language builtins. Project floor is Go 1.23 (verified `go.mod` line 3). Safe to use. |
| A3 | `runtime.NumGoroutine()` delta is sufficient for D-96 leak detection in tests that close a single Client. | Pitfall 12 + TEST-06 | MEDIUM — Phase 2 D-49 already uses this pattern successfully; if Phase 4 sweeper tests show flakiness, evaluate `go.uber.org/goleak` (deferred). The 10ms `time.Sleep` after `Close()` is the established mitigation. |
| A4 | `http.ParseTime` accepts all three RFC 7231 §7.1.1.1 forms in current Go versions. | Standard Stack + Code Examples | LOW — stdlib documentation explicit; Pitfall RETRY-2 verbatim formula uses it. `[VERIFIED: pkg.go.dev/net/http#ParseTime]` |
| A5 | The existing Phase 3 `doJSONGet` "boundary truncation" gate via `decoder.More()` correctly handles trailing whitespace on cached bytes. | Pitfall 11 | LOW — Confirmed by reading `request.go` lines 112–123 (D-24 / CR-01 comment block). The gate is post-decode; it would fire on a SECOND JSON value, not trailing whitespace. |
| A6 | `crypto/rand.Read` does not return an error on healthy systems but the library must handle the error path defensively because `NewClient` cannot return an error (CLIENT-01). | Code Examples (per-Client seed) | LOW — stdlib documents the function may return an error but in practice does not on Linux/macOS/Windows when `/dev/urandom` / `BCryptGenRandom` is available. The fallback to time-based seed is a defensive guard that should be exercised under a test that mocks `crand.Read` (advanced; may be Plan 03 stretch). |
| A7 | `http.NopCloser(bytes.NewReader(buf))` produces a `*http.Response.Body` that satisfies the downstream `defer resp.Body.Close()` + `io.Copy(io.Discard, resp.Body)` drain in `request.go`. | Pattern 2 (cacheTransport) | LOW — `io.NopCloser` is the documented stdlib idiom for this; `defer resp.Body.Close()` becomes a no-op (NopCloser's Close returns nil); the drain reads from the in-memory buffer (instant). |

**No claim tagged `[ASSUMED]` in this research carries decision authority** — all assumptions are low-risk implementation details, and the planner will encode mitigation (A1 → portable-error test; A3 → 10ms sleep; A6 → defensive fallback path). Every `[CITED]` and `[VERIFIED]` claim is backed by an authoritative source (stdlib docs or the existing codebase).

## Open Questions

1. **Should the retry attempt counter thread through `loggingTransport.attempt`?**
   - What we know: `transport.go` line 116 hardcodes `slog.Int("attempt", 1)`. CONTEXT.md `<code_context>` calls this out as a "future-work note" deferring to v0.2.
   - What's unclear: Whether Phase 4 should land a context-value-based attempt counter so retried calls show `attempt=2,3,...` in slog records.
   - Recommendation: DEFER per CONTEXT.md `<deferred>` last bullet. Plan 03 may add a context-value hook (`retryAttemptCtxKey{}` unexported) and `loggingTransport` reads it via `req.Context().Value(...)` defaulting to 1 — but this is stretch goal, not requirement. Track as `[ ] OBS-V2-X` deferred ledger.

2. **Should cache `Put` block when the sweeper goroutine is mid-evict?**
   - What we know: `sync.RWMutex` — `Put` takes `Lock`, `evict` takes `Lock`. They serialize automatically. No deadlock risk.
   - What's unclear: Whether sweep cadence under aggressive write load (TEST-06 short-TTL test) measurably degrades `Put` latency.
   - Recommendation: Land the simple RWMutex design. If benchmarks (Phase 5 TEST-11) reveal contention, revisit. Phase 4 success criteria do not include a Put-latency target.

3. **Does the `CacheHitContextKey` exported type count as new public surface?**
   - What we know: `<specifics>` 2 introduces it; counts as 1 added line of public API per CONTEXT.md (line 280).
   - What's unclear: Whether the type should be `type cacheHitKeyType struct{}; var CacheHitContextKey = cacheHitKeyType{}` (private type, public var) or a fully-exported `type CacheHitKey struct{}`. The former is the Go idiom (e.g., `context.canceledCtx{}`).
   - Recommendation: Use the `type cacheHitKeyType struct{}` private-type-public-var pattern. Document on the var godoc that consumers should read it via `req.Context().Value(openholidays.CacheHitContextKey)`. No new exported type appears in the public surface — only the exported `var`. Plan 04 task verification MUST assert "cache-hit context value flows from cacheTransport synthetic resp → hookTransport's `(req, resp, err)` triple".

## Environment Availability

> Phase 4 has no new external dependencies. The existing Phase 1–3 environment (Go 1.23, testify v1.11.1) is sufficient.

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | Build + tests | ✓ | 1.23+ (verified `go.mod` line 3) | — |
| `github.com/stretchr/testify` | All Phase 4 tests | ✓ | v1.11.1 (verified `go.mod` line 5) | — |
| `httptest.NewServer` (stdlib) | Retry + cache tests | ✓ | stdlib | — |

**Missing dependencies with no fallback:** None.
**Missing dependencies with fallback:** None.

## Validation Architecture

> Nyquist validation is ENABLED per `.planning/config.json` (`workflow.nyquist_validation: true`). VALIDATION.md will be generated from this section.

### Test Framework

| Property | Value |
|----------|-------|
| Framework | `testing` (stdlib) + `github.com/stretchr/testify` v1.11.1 |
| Config file | none — Go test discovery via `_test.go` naming |
| Quick run command | `go test ./... -run 'TestRetry\|TestCache\|TestHook\|TestStrict\|TestClient_Close\|TestNewMemoryCache\|TestCacheTransport\|TestHookTransport' -race -count=1` |
| Full suite command | `go test ./... -race -cover -count=1` |
| Coverage target | ≥ 85% (TEST-10 — Phase 5 enforces in CI; Phase 4 should not regress current coverage) |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| RESIL-01 | `WithRetry(5, 250ms)` enables retry with exp backoff + full jitter | unit | `go test -run TestWithRetry -race -count=1` | ❌ Wave 0: `options_test.go` extension OR `retry_test.go::TestWithRetry_AppliesConfig` |
| RESIL-01 | `math/rand/v2.Int64N` produces in-range jitter delays | unit | `go test -run TestComputeBackoff -race -count=1` | ❌ Wave 0: `retry_test.go::TestComputeBackoff_FullJitter` |
| RESIL-02 | Retry on 408/429/500/502/503/504 only; not on 400/401/403/404 | unit (table-driven) | `go test -run TestShouldRetry -race -count=1` | ❌ Wave 0: `retry_test.go::TestShouldRetry` |
| RESIL-02 | Retry on `net.Error{Timeout: true}` and `syscall.ECONNRESET`-wrapped errors | unit | `go test -run TestShouldRetry_TransportErrors -race -count=1` | ❌ Wave 0: same file |
| RESIL-02 | NEVER retry on `context.Canceled` / `context.DeadlineExceeded` | unit | `go test -run TestShouldRetry -race -count=1` | ❌ Wave 0: `retry_test.go` (ctx-error cases live in `TestShouldRetry` since the 2026-05-30 audit) |
| RESIL-03 | `parseRetryAfter` accepts integer seconds and HTTP-date (3 forms) | unit (table-driven) | `go test -run TestParseRetryAfter -race -count=1` | ❌ Wave 0: `retry_test.go::TestParseRetryAfter` |
| RESIL-03 | `computeBackoff` returns `max(jitter, retryAfter)` capped at `maxRetryWait` | unit | `go test -run TestComputeBackoff -race -count=1` | ❌ Wave 0: same file (Retry-After cases folded into `TestComputeBackoff` by the 2026-05-30 audit) |
| RESIL-04 | Retry loop interrupted by `ctx.Done()` mid-sleep within < 100ms | unit + integration | `go test -run TestRetry_CtxCancel -race -count=1` | ❌ Wave 0: `retry_test.go` — uses `fakeClock` + real ctx cancel |
| RESIL-05 | Retry implemented in `doJSONGet`, NOT as a RoundTripper | unit (AST/structural) | `go test -run TestRetry_NotARoundTripper -race -count=1` | ❌ Wave 0: assert no `retryTransport` type exists; `internal_test.go` extension |
| RESIL-06 | `Cache` interface exported with Get/Put/Close shape | unit | `go test -run TestCacheInterface_Conformance -race -count=1` | ❌ Wave 0: `cache_test.go::TestCacheInterface_Conformance` — `var _ Cache = (*MemoryCache)(nil)` |
| RESIL-07 | `WithCache(ttl)` caches `/Countries`, `/Languages`, `/Subdivisions` only | unit | `go test -run TestCacheTransport_PathAllowlist -race -count=1` | ❌ Wave 0: `transport_cache_test.go` |
| RESIL-07 | `WithCache` does NOT cache `/PublicHolidays` / `/SchoolHolidays` | unit | `go test -run TestCacheTransport_HolidayPathsBypass -race -count=1` | ❌ Wave 0: same file |
| RESIL-07 | `WithCache(ttl <= 0)` disables cache | unit | `go test -run TestWithCache -race -count=1` | ❌ Wave 0: `options_test.go` (ttl <= 0 cases live in `TestWithCache` since the 2026-05-30 audit) |
| RESIL-08 | Sweeper starts lazily on first `Put` | unit | `go test -run TestMemoryCache_SweeperLazyStart -race -count=1` | ❌ Wave 0: `cache_test.go` — uses `runtime.NumGoroutine()` delta |
| RESIL-08 | `Client.Close()` stops sweeper; `runtime.NumGoroutine()` delta ≤ 0 | unit | `go test -run TestClient_CloseStopsSweeper -race -count=1` | ❌ Wave 0: `client_test.go` extension |
| RESIL-08 | `MemoryCache.Close()` idempotent (safe to call twice from concurrent goroutines) | unit | `go test -run TestMemoryCache_CloseIdempotent -race -count=1` | ❌ Wave 0: `cache_test.go` |
| RESIL-09 | Cache stores raw bytes keyed by `(method, path, query)`; decode on read | unit | `go test -run TestCacheTransport_RawBytesKey -race -count=1` | ❌ Wave 0: `transport_cache_test.go` — assert exact key shape |
| RESIL-09 | Strict-decoding applies to cached bytes on re-read | unit (composition) | `go test -run TestCache_StrictDecodingComposes -race -count=1` | ❌ Wave 0: integration test in `client_test.go` extension |
| TRANS-05 | `WithRequestHook` fires after every round trip including retries | unit | `go test -run TestHookTransport_FiresPerAttempt -race -count=1` | ❌ Wave 0: `transport_hook_test.go` |
| TRANS-05 | `WithRequestHook` fires on cache-hit synthetic response | unit (composition) | `go test -run TestHook_SeesCacheHits -race -count=1` | ❌ Wave 0: composition test in `client_test.go` extension |
| TRANS-05 | Hook does NOT fire on decode errors or pre-HTTP failures | unit | `go test -run TestHook_DoesNotFireOnDecodeError -race -count=1` | ❌ Wave 0: `client_test.go` extension |
| TRANS-05 | Hook is synchronous; panic propagates to caller | unit | `go test -run TestHookTransport_PanicPropagates -race -count=1` | ❌ Wave 0: `transport_hook_test.go` |
| OBS-03 | `WithStrictDecoding(true)` rejects unknown fields | unit | `go test -run TestWithStrictDecoding_RejectsUnknown -race -count=1` | ❌ Wave 0: `client_test.go` extension OR `request_test.go` extension |
| OBS-03 | `WithStrictDecoding(false)` (default) accepts unknown fields | unit | `go test -run TestWithStrictDecoding_DefaultLenient -race -count=1` | ❌ Wave 0: same file |
| TEST-05 | Fake transport returning 429→500→200 produces correct retry behavior | unit (table-driven) | `go test -run TestRetry_E2E -race -count=1` | ❌ Wave 0: `retry_test.go::TestRetry_E2E_429Then500Then200` |
| TEST-05 | Retry uses `fakeClock` (no real `time.Sleep`); total fake-clock advance matches expected | unit | `go test -run TestRetry_DeterministicClock -race -count=1` | ❌ Wave 0: `retry_test.go` |
| TEST-05 | `Retry-After` integer-seconds form respected | unit | `go test -run TestRetry_HonorsRetryAfterSeconds -race -count=1` | ❌ Wave 0: same file |
| TEST-05 | `Retry-After` HTTP-date form respected | unit | `go test -run TestRetry_HonorsRetryAfterDate -race -count=1` | ❌ Wave 0: same file |
| TEST-06 | Cache TTL eviction works under `fakeClock` (no real `time.Sleep`) | unit | `go test -run TestMemoryCache_TTLEviction -race -count=1` | ❌ Wave 0: `cache_test.go` |
| TEST-06 | Cache hit returns identical bytes; cache miss does HTTP round trip | unit | `go test -run TestCacheTransport_HitMissBehavior -race -count=1` | ❌ Wave 0: `transport_cache_test.go` |
| TEST-06 | Default-off behavior: no cache means every call hits the network | unit | `go test -run TestClient_NoCache_AllCallsHitNetwork -race -count=1` | ❌ Wave 0: `client_test.go` extension |
| CLIENT-08 (existing) | `Client.Close()` is idempotent and concurrent-safe | unit | `go test -run TestClient_Close -race -count=1` | ✅ existing `TestClient_Close` in `client_test.go` — Phase 4 extends with sweeper-stop assertion |
| Internal | `runtime.NumGoroutine()` delta is consistent with Phase 2 D-49 pattern | unit | `go test -run TestClient_CloseStopsSweeper -race -count=1` | ❌ Wave 0: see RESIL-08 above |
| Internal | `newClientForTest(opts..., now, sleep)` produces a `*Client` with the seam wired | unit | `go test -run TestNewClientForTest -race -count=1` | ❌ Wave 0: `client_test.go` extension |
| Internal | `fakeClock.Advance` is thread-safe under `-race` | unit | `go test -run TestFakeClock_RaceFree -race -count=1` | ❌ Wave 0: `clock_test.go::TestFakeClock_RaceFree` |
| CLIENT-10 (existing) | No new `init()` functions; no new package-level vars (no new exported sentinels) | unit (AST) | `go test -run TestNoInitOrGlobalState -race -count=1` | ✅ existing `internal_test.go::TestNoInitOrGlobalState` — Phase 4 adds NO entries to `allowedVars`; the test must remain green without modification |

### Sampling Rate

- **Per task commit:** `go test ./... -run 'Test{ConcernUnderEdit}' -race -count=1` (e.g., `-run TestRetry` after a retry-touching commit). Sub-30-second run.
- **Per wave merge:** `go test ./... -race -count=1` (full unit suite, no integration, no fuzz). Sub-3-minute run on a modern dev machine.
- **Phase gate:** Full suite green (`go test ./... -race -cover -count=1`) before `/gsd:verify-work`. Coverage gate ≥ 85% per TEST-10 (Phase 5 enforces; Phase 4 should not regress).

### Wave 0 Gaps

The following test files / fixtures / harnesses MUST exist before any Phase 4 task that adds production code can pass verification:

- [ ] `clock_test.go` — `fakeClock` struct with `Now()`, `Advance(d)`, and `Sleep(ctx, d) error` methods. Pattern from D-95. Lands first.
- [ ] `retry_test.go` — tests for `shouldRetry`, `parseRetryAfter`, `computeBackoff`, full E2E retry behavior under `httptest.Server` with `fakeClock` and `fakeSleep`. Requires `clock_test.go`.
- [ ] `cache_test.go` — tests for `MemoryCache` Get/Put/Close lifecycle, sweeper start/stop, TTL eviction, idempotent Close, `newMemoryCacheWithClock` clock-injected behavior. Requires `clock_test.go`.
- [ ] `transport_cache_test.go` — tests for `cacheTransport` path allow-list, success-only caching, synthetic response shape, body drain-and-close, `CacheHitContextKey` set on hit. Requires `cache_test.go`.
- [ ] `transport_hook_test.go` — tests for `hookTransport` firing per round trip, synchronous contract, panic propagation. Standalone (no fakeClock dependency).
- [ ] `client_test.go` extensions — composition tests: `TestClient_CloseStopsSweeper`, `TestHook_SeesCacheHits`, `TestCache_StrictDecodingComposes`, `TestClient_NoCache_AllCallsHitNetwork`, `TestNewClientForTest`. Lives in the existing `client_test.go` (no new file needed).
- [ ] Framework install: not needed — testify v1.11.1 already in `go.mod`.

*(Existing infrastructure adequate for: `httptest.NewServer` pattern from Phase 2 — already used throughout; AST audit pattern from `internal_test.go` — Phase 4 does not modify the allowlist; testify `require`/`assert` patterns from Phase 1-3.)*

## Security Domain

> `security_enforcement` is not explicitly set in `.planning/config.json` (absent = enabled). Required by the orchestrator.

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | OpenHolidays API has no auth — no credentials in the library at all. |
| V3 Session Management | no | No session state in an HTTP client SDK. |
| V4 Access Control | no | No authorization decisions; the library forwards what the server returns. |
| V5 Input Validation | yes | Phase 1 D-23 already covers country/language/date-range validation. Phase 4 adds NO new user-facing input fields. The retry `maxAttempts`, cache `ttl`, etc. are program-supplied, not user-input. |
| V6 Cryptography | yes (light) | `crypto/rand.Read` seeds the per-Client `*rand.Rand` (D-78). NO cryptographic guarantees claimed; this is jitter randomization, not security-critical. Phase 4 must NEVER use `math/rand` to seed an auth token or session ID. |
| V7 Error Handling | yes | Phase 1 D-23 sentinels carry no credentials or response bodies above 4 KiB (Phase 1 D-17). Phase 4 retry-exhausted wrapping `fmt.Errorf("openholidays: retry exhausted (%d attempts): %w", ...)` preserves these guarantees because it wraps `lastErr` whose body is already 4 KiB-bounded. |
| V8 Data Protection | yes | Cache stores raw response bytes — these are PUBLIC holiday data, no PII. Documented in `WithCache` godoc that consumers should NOT enable caching for endpoints that return user-specific data (not applicable to OpenHolidays). |
| V9 Communication | yes | Library uses HTTPS upstream (`https://openholidaysapi.org`). The default `*http.Client` uses stdlib TLS defaults. Phase 4 does not change this. |
| V10 Malicious Code | yes | Zero new external deps (verified Package Legitimacy Audit). Supply chain unchanged. |
| V11 Business Logic | n/a | Library is a thin SDK; no business logic in the library itself. |
| V12 Files & Resources | yes (light) | 10 MiB `LimitReader` cap (Phase 2 D-25) extended to `cacheTransport` per D-83. Hostile server cannot OOM the cache. |
| V13 API & Web Services | yes | The library IS an API client. Standard HTTPS + stdlib transport defaults. |
| V14 Configuration | yes | Functional options; immutable Client after construction (CONC-1 mitigation); no env vars; no global mutable state (CLIENT-10). |

### Known Threat Patterns for Go HTTP SDK Middleware

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Hostile server returns oversized body (OOM) | Denial of Service | `io.LimitReader(resp.Body, maxResponseBytes+1)` in `cacheTransport` (D-83). Cap = 10 MiB. |
| Hostile server returns `Retry-After: <past date>` to disable backoff | Denial of Service (self-inflicted) | `parseRetryAfter` guards `d := t.Sub(now); d > 0` (Pitfall 9). |
| Hostile server returns `Retry-After: 999999999` to hold a request indefinitely | Denial of Service | Per-attempt sleep capped at `maxRetryWait` (default 60s) per D-76. Caller's ctx provides total-time cap. |
| Cache poisoning via response that gets cached for the full TTL | Tampering | Cache-only-on-success gate: `err == nil && resp.StatusCode == 200` (D-83). 503s never cached (Pitfall CACHE-1). |
| Goroutine leak via missing `Client.Close()` | Denial of Service (memory/goroutine) | Lazy sweeper start (D-84) — unused cache spawns nothing. `Close()` idempotent (D-85). |
| Thundering herd via correlated jitter across fleet | Denial of Service (against upstream) | Per-Client `crypto/rand`-seeded ChaCha8 (D-78). |
| Hook leaking response body to operator logs at Info+ | Information Disclosure | `WithRequestHook` godoc MUST document "do not log resp.Body content at Info+ level" (Pitfall LOG-1). |
| Panic in hook crashes the Client goroutine | Denial of Service | D-90 explicit: panics propagate. Consumer wraps with `defer recover()` if needed. Mirrors stdlib `http.Handler` convention. |

## Sources

### Primary (HIGH confidence)
- `[VERIFIED: /data/git/private/holidays/go.mod line 3]` — Go 1.23 floor.
- `[VERIFIED: /data/git/private/holidays/go.mod line 5]` — testify v1.11.1.
- `[VERIFIED: /data/git/private/holidays/client.go lines 31-38]` — existing `Client` struct shape; confirms `closed atomic.Bool` available for D-85 reuse.
- `[VERIFIED: /data/git/private/holidays/config.go lines 26-32]` — existing `clientConfig` struct; confirms additive nature of D-79/D-80/D-87/D-91 fields.
- `[VERIFIED: /data/git/private/holidays/request.go lines 58-125]` — existing `doJSONGet` body; confirms retry-loop injection point (lines 75-88) and strict-gate injection point (line 92).
- `[VERIFIED: /data/git/private/holidays/transport.go lines 87-96]` — existing `buildTransport`; confirms revised chain order per D-89 is an in-place edit.
- `[VERIFIED: /data/git/private/holidays/internal_test.go lines 61-70]` — `allowedVars` allowlist; confirms NO modification needed for Phase 4 (no new exported sentinels).
- `[CITED: pkg.go.dev/net/http#ParseTime]` — RFC 7231 §7.1.1.1 forms (RFC 1123, RFC 850, ANSI C asctime).
- `[CITED: pkg.go.dev/math/rand/v2#NewChaCha8]` — per-Client jitter source.
- `[CITED: pkg.go.dev/encoding/json#Decoder.DisallowUnknownFields]` — strict-decoding mechanism.
- `[CITED: pkg.go.dev/syscall]` — `ECONNRESET` constant available on Linux/macOS/Windows.

### Secondary (MEDIUM confidence)
- `.planning/research/PITFALLS.md` §RETRY-1..4, §CACHE-1..5, §CONC-2, §JSON-1, §TEST-3..4 — all canonical research files verified as the source for D-73..D-97 decisions.
- `.planning/research/ARCHITECTURE.md` §Pattern 2 (RoundTripper chain), §Pattern 4 (Cache in RoundTripper) — confirms the chain composition pattern and the cache-as-transport idiom.
- `.planning/research/STACK.md` — confirms hand-rolled retry / cache / hook (no `retryablehttp`, no `backoff`, no `go-cache`) and `math/rand/v2` choice.

### Tertiary (LOW confidence)
None — every claim is backed by stdlib documentation, existing source code, or canonical research files.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — every package is stdlib or already-pinned; verified line-by-line against `go.mod` and existing source.
- Architecture: HIGH — D-77/D-83/D-89/D-92 are all backed by direct reads of `request.go`, `config.go`, `transport.go`, and the canonical ARCHITECTURE.md Pattern 2/4 sections.
- Pitfalls: HIGH — all 12 pitfalls are mapped to specific PITFALLS.md sections (RETRY-1..4, CACHE-1..5, CONC-2, LOG-1, HTTP-3/4, JSON-1) with the CONTEXT.md decisions that mitigate them.
- Validation Architecture: HIGH — 30+ test cases mapped to specific requirement IDs, with Wave 0 gaps explicitly identified.

**Research date:** 2026-05-27
**Valid until:** 2026-06-26 (30 days for a stable stdlib-driven Phase; revisit if a new Go release surfaces a relevant `net/http` or `math/rand/v2` change).
