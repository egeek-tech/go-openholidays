# Phase 4: Resilience - Context

**Gathered:** 2026-05-27
**Status:** Ready for planning

<domain>
## Phase Boundary

Deliver retry, in-memory TTL cache, observability hook, and strict-decoding as transparent middleware that wraps the Phase 3 endpoint pipeline WITHOUT modifying any Phase 3 method signature. `Client.Close()` becomes load-bearing (stops the cache sweeper goroutine via `sync.Once` + ctx cancel).

- **Retry layer** (RESIL-01..05; TEST-05) — `WithRetry(maxAttempts int, baseDelay time.Duration) Option` + `WithMaxRetryWait(d time.Duration) Option`. Implemented inside `doJSONGet` (endpoint layer, NOT a RoundTripper) per RESIL-05 so caller-supplied `*http.Client` retries do not double-fire. Exponential backoff + full jitter using `math/rand/v2`; honors `Retry-After` (integer seconds + RFC 7231 HTTP-date); ctx-aware sleep returns `ctx.Err()` on cancel.
- **Cache layer** (RESIL-06..09; TEST-06) — `WithCache(ttl time.Duration) Option` enables the default in-memory cache; `WithCacheBackend(c Cache) Option` supplies a custom one and overrides. Exported `Cache` interface + exported `NewMemoryCache(ttl time.Duration) *MemoryCache` constructor. Caches raw response bytes keyed by `(method, path, canonical query)` for the three reference endpoints only (`/Countries`, `/Languages`, `/Subdivisions`); holiday endpoints are never cached by default. Decode happens on read so strict-decoding and cache coexist correctly.
- **Hook layer** (TRANS-05) — `WithRequestHook(func(*http.Request, *http.Response, error)) Option`. New RoundTripper placed OUTERMOST in the chain so it observes every round trip AND every cache-hit synthetic response. Synchronous-only contract documented (consumers wanting async own the goroutine and the leak — Pitfall CONC-2). Fires per retry attempt (retry is in `doJSONGet`, so each `c.http.Do` invocation re-enters the chain). Does NOT fire on decode errors or pre-HTTP ctx errors — HTTP-layer observability only.
- **Strict-decoding** (OBS-03) — `WithStrictDecoding(strict bool) Option` sets an immutable `strict` field on Client. `doJSONGet` reads it and conditionally calls `decoder.DisallowUnknownFields()` before `Decode`. Cache hits flow through the same decode path, so strict checks apply to cached bytes as well (consistent with ROADMAP SC #5 "decoding happens on read").
- **`Client.Close()` wiring (load-bearing)** — flips the existing `closed atomic.Bool` (Phase 2 D-40), invokes a `sync.Once`-guarded cancel of the sweeper ctx, then calls `cache.Close()`. Safe to call concurrently from multiple goroutines; idempotent.
- **Test seam (fake clock)** — Internal `nowFunc func() time.Time` and `sleepFunc func(ctx context.Context, d time.Duration) error` fields on `*Client`. Defaults to `time.Now` and a ctx-aware `select`-based sleep. Test access via an unexported `newClientForTest(...)` constructor inside same-package `_test.go` files. NOT exposed in the public API.

What this phase does NOT deliver: negative caching, persistent cache backends, distributed cache adapters (e.g., Redis), retry of non-idempotent verbs (we only ship GETs anyway), per-call strict-decoding override, working-day arithmetic, CLI, CI workflows, release tooling, additional endpoints. All of those are roadmap-deferred or out of v1 scope.

</domain>

<decisions>
## Implementation Decisions

### Retry (RESIL-01..05; TEST-05)

- **D-73:** Public retry surface ships exactly two options in v0.1.0: `WithRetry(maxAttempts int, baseDelay time.Duration) Option` (matches ROADMAP success criterion #1 literal signature) and `WithMaxRetryWait(d time.Duration) Option` as a single escape-hatch knob. No `RetryConfig` struct, no per-status overrides, no `WithRetryPolicy(...)`. If a real consumer needs richer knobs (per-status, per-endpoint), add them in v0.2 without breaking the positional signature.
- **D-74:** Defaults when `WithRetry` is NOT called: retry is DISABLED (opt-in for M1 per STATE.md). Defaults when `WithRetry(n, d)` IS called: `n <= 0` is interpreted as DISABLED (defensive; symmetry with `WithTimeout(0)` Phase 2 D-28); `d <= 0` is interpreted as `baseDelay = 250 * time.Millisecond` (FEATURES.md research recommendation). Default `maxRetryWait` when `WithMaxRetryWait` is not called: `60 * time.Second` (Pitfall RETRY-2 AWS guidance).
- **D-75:** Retryable conditions matrix (RESIL-02):
  - HTTP statuses: `{408, 429, 500, 502, 503, 504}` (Pitfall RETRY-1 explicit set; OpenAPI documents only 400/500 but the set is forward-defensive). Never retry on other 4xx.
  - Transport errors: `net.Error` with `Timeout() == true`; connection-reset errors detected via `errors.Is(err, syscall.ECONNRESET)` (`syscall` is stdlib — zero-dep policy preserved).
  - Genuine `context.Canceled` / `context.DeadlineExceeded` are NEVER retried (they propagate immediately as `ctx.Err()`).
- **D-76:** Retry-After handling (RESIL-03):
  - On any retryable response, read `resp.Header.Get("Retry-After")` via the existing `parseRetryAfter(h string, now time.Time) (time.Duration, bool)` helper (lives in `retry.go` per D-77; uses `strconv.Atoi` for the integer-seconds form and `http.ParseTime` for the RFC 7231 HTTP-date form).
  - When a `Retry-After` is parsed, the per-attempt sleep is `max(retryAfter, jitterDelay)` capped at `maxRetryWait`. This matches RESIL-03 "takes the larger of jitter delay and Retry-After" with the additional escape-hatch ceiling.
  - When absent, fall back to full-jitter exponential backoff: `delay := time.Duration(rand.Int64N(int64(min(maxRetryWait, baseDelay << attempt))))` using `math/rand/v2` per RESIL-01.
- **D-77:** Retry placement and file layout: a new file `retry.go` ships at repo root (alongside `request.go`) with:
  - `retryConfig` (unexported struct: `maxAttempts int`, `baseDelay`, `maxWait time.Duration`).
  - `shouldRetry(resp *http.Response, err error) bool` — pure function over D-75 matrix.
  - `parseRetryAfter(h string, now time.Time) (time.Duration, bool)` — both forms per D-76.
  - `computeBackoff(attempt int, retryAfter time.Duration, cfg retryConfig, rnd *rand.Rand) time.Duration` — full jitter + Retry-After, capped at `maxWait`.
  Retry wraps the `c.http.Do(req)` + 4xx/5xx error-build steps inside `doJSONGet` via a `for attempt := 0; attempt < cfg.maxAttempts; attempt++` loop with `ctx.Err()` check at loop-top and `c.sleepFunc(ctx, computed)` between attempts (Pitfall RETRY-3 ctx-aware). The 2xx decode path runs once after the loop succeeds; mid-truncation / decode-error / oversize gates from Phase 3 D-62 are not retried.
- **D-78:** Random source: a per-Client `*rand.Rand` initialized at `NewClient` time using `rand.NewChaCha8(seed)` with `seed` from `crypto/rand.Read` (Pitfall RETRY-4 — per-Client seed avoids fleet-wide thundering herds). Field `c.rand *rand.Rand` on Client. NOT exposed in public API.

### Cache (RESIL-06..09; TEST-06)

- **D-79:** Public cache surface (RESIL-06 mandate to expose a public `Cache` interface):
  ```go
  // Cache is the contract for any cache backend wired via WithCacheBackend.
  type Cache interface {
      Get(key string) (value []byte, ok bool)
      Put(key string, value []byte)
      Close() error
  }
  ```
  TTL is OWNED by the cache (not passed per `Put`) — the default in-memory impl applies a single uniform TTL fixed at construction. Custom backends that want per-entry TTL can wrap their own logic; the interface stays simple.
- **D-80:** Two cache options ship in v0.1.0:
  - `WithCache(ttl time.Duration) Option` — enables the default in-memory cache with the supplied TTL. Internally constructs `NewMemoryCache(ttl)` and stores it on `clientConfig.cache`. `ttl <= 0` disables the cache (defensive symmetry).
  - `WithCacheBackend(c Cache) Option` — supplies a custom cache; overrides any prior `WithCache(ttl)` if both are passed (last-wins per functional-options convention). When the caller supplies a backend, the Client does NOT own its goroutines — `Client.Close()` still calls `c.Close()` per the interface contract, but the backend is responsible for stopping its own sweepers/connections.
  - Recorded as new **CL-14** in PROJECT.md `Key Decisions` (cache is opt-in but the cache *interface* is part of v0.1.0 public surface).
- **D-81:** Exported in-memory implementation:
  ```go
  // MemoryCache is the default in-memory TTL cache returned by NewMemoryCache.
  type MemoryCache struct { /* unexported fields */ }

  func NewMemoryCache(ttl time.Duration) *MemoryCache
  func (m *MemoryCache) Get(key string) ([]byte, bool)
  func (m *MemoryCache) Put(key string, value []byte)
  func (m *MemoryCache) Close() error
  ```
  Backing storage is `map[string]entry{ value []byte; expiresAt time.Time }` under a `sync.RWMutex` (Pitfall CACHE-4 RWMutex over `sync.Map` — read-heavy + re-write-on-expiry workload). Lives in a new file `cache.go` at repo root.
- **D-82:** Cache key encoding (RESIL-09 raw bytes keyed by `(method, path, query)`):
  ```go
  key := req.Method + " " + req.URL.Path + "?" + req.URL.Query().Encode()
  ```
  `Query().Encode()` produces sorted keys for deterministic ordering (stdlib documents this). No `baseURL` in the key by design — Pitfall CACHE-2 is mitigated because the cache lives on `*Client`, not globally, so two Clients with different `baseURL`s have isolated caches automatically.
- **D-83:** Cacheability gate lives in the new `cacheTransport.RoundTrip` (RoundTripper layer per ARCHITECTURE.md Pattern 4 + the chain order constraint in D-89 below). Path-prefix allow-list: `/Countries`, `/Languages`, `/Subdivisions`. Non-allowed paths bypass cache entirely. Only successful responses cache (`err == nil && resp.StatusCode == 200` per Pitfall CACHE-1 — never cache errors). The body is read into a buffer via `io.LimitReader(resp.Body, maxResponseBytes+1)`, then `resp.Body` is replaced with `io.NopCloser(bytes.NewReader(buf))` so the downstream decoder still sees a fresh stream. The buffer is also stored in the cache.
- **D-84:** Sweeper goroutine (RESIL-08):
  - **Start:** lazy on first successful `Put` — never sooner. `MemoryCache` carries a `sync.Once` + `context.CancelFunc` pair; `Put` calls `once.Do(m.startSweeper)`.
  - **Interval:** `max(ttl/4, 30*time.Second)`. Short-TTL test caches sweep aggressively; long-TTL prod caches don't churn.
  - **Behavior:** on every tick, acquire `Lock`, iterate the map, delete entries where `time.Now().After(entry.expiresAt)`, release. The sweeper uses the Client's `nowFunc` indirectly via a constructor param so tests can fake time (D-86).
  - **Stop:** `MemoryCache.Close()` calls `cancel()` on the sweeper ctx (which is selected on `<-ctx.Done()` in the sweeper loop), then waits on a tiny done-channel (1 ms timeout) so `Close` returns after the sweeper exits. Returns `nil` always; idempotent (the `sync.Once` for start means a never-started sweeper is also fine to Close).
- **D-85:** `Client.Close()` (CLIENT-08) ordering — replaces the Phase 2 D-40 stub:
  ```go
  func (c *Client) Close() error {
      c.closeOnce.Do(func() {
          c.closed.Store(true)            // existing atomic.Bool from Phase 2
          if c.cache != nil {
              _ = c.cache.Close()         // best-effort; never returns the error
          }
      })
      return nil
  }
  ```
  - New field `c.closeOnce sync.Once` on Client struct.
  - The `closed` atomic.Bool stays — it's reused as the "this client has been Closed" flag readable from any goroutine without taking a mutex (the read path in `doJSONGet` can short-circuit with a `c.closed.Load() == true` check before dispatching).
  - Errors from `cache.Close()` are intentionally swallowed (`_ = ...`). The cache's contract is "best-effort cleanup" and a `Close()` failure on the cache shouldn't surface to a caller draining their `Client` in `defer client.Close()`.
- **D-86:** Cache <-> fake-clock wiring: `NewMemoryCache(ttl time.Duration)` exposes a public constructor that uses `time.Now`. Tests that need fake time use a sibling unexported `newMemoryCacheWithClock(ttl time.Duration, now func() time.Time) *MemoryCache`. The Client's `WithCache(ttl)` path internally calls `newMemoryCacheWithClock(ttl, c.nowFunc)` so production and tests share the same clock seam (`c.nowFunc`). Custom backends supplied via `WithCacheBackend(c Cache)` use their own clock — that's the caller's responsibility.

### Hook (TRANS-05)

- **D-87:** Public surface: `WithRequestHook(func(*http.Request, *http.Response, error)) Option`. One option, one type, no per-event flag or condition argument in v0.1.0. Hook signature stays a function literal (no `Hook` interface) — the function shape is small and stable; introducing an interface adds public surface without payoff.
- **D-88:** Hook scope (ROADMAP SC #4):
  - Fires AFTER every real HTTP round trip (so retries fire it N+1 times where N is the number of retries; each attempt has its own (req, resp, err) triple).
  - Fires on every CACHE HIT with the synthetic `*http.Response` returned by `cacheTransport` (so observability sees cache-hit counts uniformly).
  - Does NOT fire on decode errors (decode happens in `doJSONGet` after the RoundTripper chain returns — out of hook scope).
  - Does NOT fire on pre-HTTP failures (validation, nil-ctx, NewRequest build errors).
  - On retry-budget exhaustion the hook fires once per attempt (including the last failing attempt). The endpoint method then returns the wrapped error — hook does not see "final result", only per-attempt outcomes.
- **D-89:** Hook chain placement: `hookTransport` is placed OUTERMOST in the RoundTripper chain. Revised chain order with retry moved to the endpoint layer (per RESIL-05):
  ```
  req → hookTransport → cacheTransport → loggingTransport → headerTransport → underlying
  ```
  Rationale: cache hits short-circuit the chain at `cacheTransport`. Only a hook placed ABOVE cache observes them. STATE.md's older "retry → cache → hook → logging → header → base" notation predates the RESIL-05 retry-relocation decision; the revised chain above is the correct one. Logging and header remain in their Phase 2 positions.
- **D-90:** Synchronous contract: `WithRequestHook` documents the function is invoked synchronously on the calling goroutine's stack (Pitfall CONC-2). Consumers wanting async behavior own the goroutine and the leak. A hook that panics propagates the panic to the caller — the library does NOT recover (mirrors stdlib `http.Handler` convention).

### Strict-decoding (OBS-03)

- **D-91:** Public surface: `WithStrictDecoding(strict bool) Option`. Sets immutable `clientConfig.strictDecoding bool` which lands as `Client.strict bool` after `NewClient`. No per-call override and no runtime toggle — toggling at runtime would compromise the cache (cached bytes decoded with one mode could surface as a strict-failure after the toggle). Recorded as new **CL-15** in PROJECT.md `Key Decisions`.
- **D-92:** Wiring into `doJSONGet`: the existing `decoder := json.NewDecoder(limited)` line in `doJSONGet` (Phase 3 D-62) gains a one-line gate:
  ```go
  if c.strict {
      decoder.DisallowUnknownFields()
  }
  ```
  Order matters — call BEFORE `decoder.Decode(&out)`. The boundary/mid-truncation gates and `ErrEmptyResponse` paths are unchanged.
- **D-93:** Cache + strict interaction (ROADMAP SC #5 — "cache hits still work after WithStrictDecoding is added"): cache stores raw bytes; decode runs on read; therefore strict-mode applies to cached bytes on every read. This is the intended behavior, NOT a bug — a strict-mode client SHOULD surface a schema-drift response that landed in cache before the upstream added a new field. If a consumer wants "cache lenient + fresh strict" they must run two Clients. Documented in `docs/design.md` (deferred to Phase 5).

### Test seam (fake clock + TEST-05 + TEST-06)

- **D-94:** Internal time/sleep injection (NOT in public API):
  - `Client.nowFunc func() time.Time` defaults to `time.Now`.
  - `Client.sleepFunc func(ctx context.Context, d time.Duration) error` defaults to a ctx-aware helper:
    ```go
    func ctxSleep(ctx context.Context, d time.Duration) error {
        if d <= 0 { return nil }
        t := time.NewTimer(d)
        defer t.Stop()
        select {
        case <-ctx.Done(): return ctx.Err()
        case <-t.C: return nil
        }
    }
    ```
  - Test access via unexported `newClientForTest(opts ...Option, now func() time.Time, sleep func(ctx, d) error) *Client` defined inside same-package `_test.go` (build-tagged is NOT required because `_test.go` files are excluded from `go build` by default). Tests live in `package openholidays` (not `package openholidays_test`) for the parts that need this seam.
- **D-95:** Fake clock implementation: a tiny hand-rolled `fakeClock` struct in a new `clock_test.go` (test-only, lives at repo root):
  ```go
  type fakeClock struct {
      mu  sync.Mutex
      now time.Time
  }
  func (f *fakeClock) Now() time.Time { ... }
  func (f *fakeClock) Advance(d time.Duration) { ... }
  ```
  Tests inject `fc.Now` as `nowFunc` and a `fakeSleep` that synchronously advances `fc` (no real sleep). This satisfies TEST-05 (retry/backoff under deterministic clock) and TEST-06 (cache TTL eviction without real `time.Sleep`).
- **D-96:** Goroutine-leak audit (RESIL-08 + Pitfall CONC-2): TEST-05/TEST-06 verify `Client.Close()` actually stops the sweeper via a `runtime.NumGoroutine()` delta check (consistent with Phase 2 D-49 — avoids adding `go.uber.org/goleak` as a test-only dep). The test pattern:
  ```go
  before := runtime.NumGoroutine()
  c := NewClient(WithCache(1 * time.Millisecond))
  c.Countries(ctx, CountriesRequest{}) // forces a Put → sweeper starts
  assert.NoError(t, c.Close())
  time.Sleep(10 * time.Millisecond) // give the sweeper a moment to exit
  assert.LessOrEqual(t, runtime.NumGoroutine(), before)
  ```
  If a real consumer hits a flaky leak in the wild, evaluate adding `go.uber.org/goleak` then (currently NOT pre-approved per PROJECT.md test-only dep allowlist).

### Plan sequencing

- **D-97:** Suggested plan order (planner may refine):
  1. **Strict-decoding + Client config plumbing** — adds `strict` field on `Client` + `clientConfig`, threads through `WithStrictDecoding(b)`, gates `decoder.DisallowUnknownFields()` in `doJSONGet`. CL-15 row added. Smallest, lowest-risk; lands first so subsequent plans can rely on the field existing. (Covers OBS-03.)
  2. **Fake-clock test seam** — adds `nowFunc` + `sleepFunc` to Client, `newClientForTest`, `clock_test.go`. No production behavior change; unlocks deterministic tests for plans 3 + 4. (Covers internal test infrastructure.)
  3. **Retry layer** — adds `retry.go` with `retryConfig`, `shouldRetry`, `parseRetryAfter`, `computeBackoff`. Adds `WithRetry(maxAttempts, baseDelay)` + `WithMaxRetryWait(d)` options. Wraps `c.http.Do(req)` + status-error build inside `doJSONGet` with the retry loop. Per-Client `*rand.Rand` field. (Covers RESIL-01..05, TEST-05.)
  4. **Cache layer (default in-memory + interface)** — adds `cache.go` with the `Cache` interface, `MemoryCache` struct, `NewMemoryCache(ttl)`, sweeper goroutine. Adds `WithCache(ttl)` + `WithCacheBackend(c Cache)` options. Adds `cacheTransport` RoundTripper with path-prefix allow-list and success-only caching. Wires sweeper into Client.Close. CL-14 row added. (Covers RESIL-06..09, TEST-06, CLIENT-08 full wiring.)
  5. **Hook RoundTripper** — adds `transport_hook.go` with `hookTransport`. Adds `WithRequestHook(fn)` option. Updates `buildTransport` to place `hookTransport` outermost. (Covers TRANS-05.)
  6. **PROJECT.md Key Decisions append + sentinel allowlist refresh** — extends `internal_test.go` allowlist if any new sentinels are added (none planned — Phase 4 introduces no new exported sentinels). Adds CL-14 (Cache surface) and CL-15 (StrictDecoding immutability) rows.

### Claude's Discretion

The following are inferred from already-locked architecture and conventions; no need to re-ask:

- File layout: `retry.go`, `cache.go`, `transport_hook.go` ship at repo root. Test files `retry_test.go`, `cache_test.go`, `transport_hook_test.go`, `clock_test.go` (test seam shared by retry + cache tests) alongside.
- Every new exported symbol gets a godoc starting with the symbol name (Gold Rule 1 + PROJECT.md). Every error string starts with `"openholidays: "` (Phase 1 D-23 convention).
- Tests use testify (Gold Rule 3): one `TestXxx` per exported prod function; every case in `t.Run`; `require` for preconditions, `assert` for verifications. `cache_test.go` follows the table-driven pattern from Phase 3.
- `package openholidays` (not `_test`) for tests that need the `nowFunc`/`sleepFunc` seam or `newClientForTest`. Tests that don't can live in either; default to the production package for consistency.
- `httptest.NewServer` continues to back HTTP tests (no live network in unit tests, Phase 2 D-46). The cache's path-prefix allow-list is exercised against the existing `testdata/` fixtures from Phase 3.
- No new exported sentinels in Phase 4. The retry layer wraps existing errors via `fmt.Errorf("openholidays: retry exhausted (%d attempts): %w", attempts, lastErr)`. Total exported sentinel count after Phase 4: still 7 (unchanged from Phase 3).
- The 10 MiB cap + drain-and-close pattern from Phase 2/3 (D-25, D-45, D-62) extends to `cacheTransport` — when the cache reads the body to buffer it, the read goes through `io.LimitReader(resp.Body, maxResponseBytes+1)` and the same `defer drain+close` sequence applies BEFORE the body is replaced with `io.NopCloser(bytes.NewReader(buf))`.
- Default values are written as named consts in the relevant file (`defaultMaxRetryWait`, `defaultBaseDelay`, `minSweeperInterval`) — not hardcoded in option bodies.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project baseline (read first)
- `.planning/PROJECT.md` — locked constraints (zero runtime deps, 10 MiB cap, default timeout 15 s, slog Debug-only HTTP logging, ≤ 100 ms ctx cancellation), Key Decisions table (CL-01..CL-13 from Phases 1–3; CL-14 + CL-15 will be added by this phase).
- `.planning/REQUIREMENTS.md` — Phase 4 owns: RESIL-01..09 (9), TRANS-05 (1), OBS-03 (1), TEST-05..06 (2), CLIENT-08 full wiring (existing stub becomes load-bearing) = 13 requirements.
- `.planning/ROADMAP.md` §"Phase 4: Resilience" — goal + 5 success criteria (note: chain order in SC #4 is conceptual; the actual revised chain per D-89 places hook outermost and moves retry into the endpoint layer).
- `.planning/STATE.md` — running ledger of decisions inherited from Phases 1–3 (CL-01..CL-13). Confirms retry opt-in for M1, retry in endpoint layer (NOT a RoundTripper), strict decoding OFF by default, RoundTripper chain order constraints.
- `.planning/codebase/CONVENTIONS.md` — Gold Project Rules (English-only, verify-or-ask, testify + one-test-per-prod-func + t.Run).

### Architecture and patterns (read before writing retry / cache / hook code)
- `.planning/research/ARCHITECTURE.md` §"Pattern 2: RoundTripper Decorator Chain" (lines 266-326) — chain composition pattern; D-89 revises the order with retry moved to endpoint layer.
- `.planning/research/ARCHITECTURE.md` §"Pattern 4: Cache Inside the RoundTripper" (lines 383-405) — cache layer rationale, key derivation, path-prefix allow-list (D-82/D-83 follow this verbatim).
- `.planning/research/ARCHITECTURE.md` §"Data Flow — Request Flow" (lines 620-720) — full traversal pre-Phase-4; D-77 + D-83 + D-89 mutate the flow as described.
- `.planning/research/STACK.md` §"Recommended Stack" — confirms `math/rand/v2`, `sync.RWMutex`, hand-rolled retry/cache, `log/slog`, zero runtime deps.
- `.planning/research/FEATURES.md` §"Retry / Cache / Hook" — feature priorities + the on-by-default discussion (retry stays opt-in for M1 per STATE.md).

### Pitfalls (read before writing each layer)
- `.planning/research/PITFALLS.md` §"RETRY-1: Retrying 4xx (except 429)" — drives D-75 status allow-list.
- `.planning/research/PITFALLS.md` §"RETRY-2: Ignoring Retry-After" — drives D-76 `parseRetryAfter` shape.
- `.planning/research/PITFALLS.md` §"RETRY-3: Unbounded retry loop ignoring ctx" — drives D-77 retry-loop pseudocode (`ctx.Err()` at top of loop + `sleepCtx` not `time.Sleep`).
- `.planning/research/PITFALLS.md` §"RETRY-4: Same-jitter retries → thundering herd" — drives D-78 per-Client `*rand.Rand` with crypto-seeded ChaCha8.
- `.planning/research/PITFALLS.md` §"CACHE-1: Caching error responses" — drives D-83 success-only cache (status == 200, err == nil).
- `.planning/research/PITFALLS.md` §"CACHE-2: Cache keyed only by endpoint params" — drives D-82 (per-Client cache mitigates without putting baseURL in the key).
- `.planning/research/PITFALLS.md` §"CACHE-3: Memory leak — no TTL eviction loop" — drives D-84 sweeper goroutine + D-85 Close wiring.
- `.planning/research/PITFALLS.md` §"CACHE-4: Race on read-during-evict" — drives D-81 `sync.RWMutex` choice over `sync.Map`.
- `.planning/research/PITFALLS.md` §"CACHE-5: sync.Map for the wrong access pattern" — confirms D-81's RWMutex choice.
- `.planning/research/PITFALLS.md` §"CONC-2: Goroutine leaks in retry / hook paths" — drives D-84/D-85 sweeper lifetime + D-90 hook synchronous contract.
- `.planning/research/PITFALLS.md` §"LOG-1: Logging response bodies at Info level" — Phase 2 already-mitigated; relevant when hook receives `*http.Response` — consumers must not log body at Info above (documented in `WithRequestHook` godoc).
- `.planning/research/PITFALLS.md` §"JSON-1: Strict decoding by default" — drives D-91 strict-OFF default + immutability of the strict flag.
- `.planning/research/PITFALLS.md` §"TEST-3: Time-dependent tests without a fake clock" — drives D-94/D-95 internal `nowFunc`/`sleepFunc` seam.
- `.planning/research/PITFALLS.md` §"TEST-4: Race-flaky tests" — Phase 4 tests run under `-race`; the sweeper + Close path is the highest-risk vector and is explicitly race-tested per D-96.

### Phase 1–3 anchors (read for state inherited from prior phases)
- `.planning/phases/01-foundation/01-CONTEXT.md` — D-01..D-23 + CL-01..CL-06. Especially the errors model (which Phase 4 does NOT extend).
- `.planning/phases/02-transport/02-CONTEXT.md` — D-24..D-50 + CL-07. Especially D-29 (chain composition shape that D-89 revises), D-37 (HTTPClient shallow-copy), D-40 (Close stub that D-85 replaces), D-45 (drain-and-close pattern that D-83 inherits).
- `.planning/phases/03-endpoints-helpers/03-CONTEXT.md` — D-51..D-72 + CL-08..CL-13. Especially D-62 (`doJSONGet` pipeline that D-77 wraps with retry and that D-92 gates with strict-decoding), D-63 (Countries refactor through `doJSONGet`), D-65 (`validateHolidays` post-decode validation that runs AFTER cache reads).
- Phase 1–3 source files at repo root — every `*.go` file that the new options touch. Especially `client.go` (gains `closeOnce`, `nowFunc`, `sleepFunc`, `rand`, `strict`, `cache` fields), `config.go` (gains `retry`, `cache`, `hook`, `strict` config fields + chain-order update in `buildTransport`), `options.go` (gains 5 new WithX options), `request.go` (`doJSONGet` gains retry wrapper + strict gate).

### Upstream API (verify before writing tests)
- `https://openholidaysapi.org/swagger/v1/swagger.json` — OpenAPI 3 spec. Verified live 2026-05-27 in Phase 3 discussion; no rate-limit headers observed (so `Retry-After` tests must use a fake `httptest.Server` that injects the header — there is no live signal to capture). `Cache-Control` / `Vary` headers also unverified; Phase 4 ignores them and uses opt-in TTL only per RESIL-07.

### Gold Project Rules (apply everywhere)
- `CLAUDE.md` §"Project Rules (Gold)" — Rule 1 (English-only), Rule 2 (verify-or-ask), Rule 3 (testify + t.Run + one-test-per-prod-function).

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets (from Phases 1–3)

- `client.go` — `*Client` struct gains five new fields in Phase 4: `closeOnce sync.Once`, `nowFunc func() time.Time`, `sleepFunc func(context.Context, time.Duration) error`, `rand *rand.Rand` (math/rand/v2), `strict bool`, plus a `cache Cache` interface field (nil-able). The existing `closed atomic.Bool` is reused by `Close()` as the "Client has been Closed" flag.
- `config.go` — `clientConfig` gains five new option-fed fields: `retry retryConfig` (zero-value = disabled), `cache Cache` (nil = disabled), `cacheTTL time.Duration` (set by `WithCache(ttl)` for the default impl), `hook RequestHookFunc` (nil = no hook), `strictDecoding bool`. `buildTransport` is edited in place per the revised chain order in D-89 (Phase 2 D-29 explicitly anticipated this).
- `options.go` — gains five new exported `WithX` constructors: `WithRetry`, `WithMaxRetryWait`, `WithCache`, `WithCacheBackend`, `WithRequestHook`, `WithStrictDecoding`. (Six options total — `WithMaxRetryWait` is one of two retry options.)
- `transport.go` — `headerTransport` and `loggingTransport` remain unchanged. The `attempt` field still hardcoded to 1 — Phase 4's retry lives in `doJSONGet`, not in `loggingTransport`. (Future-work note: the attempt counter in `loggingTransport` could be wired from a context value set by the retry loop. Defer to Phase 5 docs or a v0.2 enhancement.)
- `request.go` — `doJSONGet` is the central injection point. The function gains: (a) a retry loop wrapping `c.http.Do(req)` + the 4xx/5xx error-build path (D-77); (b) a one-line strict-decoding gate before `Decode` (D-92). The mid-truncation / boundary-truncation / `ErrEmptyResponse` paths are NOT retried.
- `errors.go` — no new sentinels in Phase 4. The retry-exhausted message wraps `lastErr` via `fmt.Errorf("openholidays: retry exhausted (%d attempts): %w", attempts, lastErr)` so callers branch on the underlying sentinel (`ErrEmptyResponse`, `ErrResponseTooLarge`, `*APIError`, etc.).
- `internal_test.go` — Phase 1's `TestNoInitOrGlobalState` AST audit. Allowlist needs NO updates for Phase 4 (no new exported sentinels). The new package-level `var` audit would catch if anyone slips a global cache var; per PROJECT.md / CLIENT-10 we never do.

### Established Patterns (continue using)

- One concern per `*.go` file at repo root (Phase 2/3 convention): `retry.go`, `cache.go`, `transport_hook.go`. Tests as `*_test.go` siblings.
- Tests under `httptest.NewServer` with hand-crafted fixtures or `testdata/` reuse (Phase 2 D-46; Phase 3 D-67). No live network outside `//go:build integration` paths.
- testify-only assertions + `t.Run` per case + one `TestXxx` per exported prod function (Gold Rule 3). `require` for preconditions, `assert` for verifications.
- English-only invariant (Gold Rule 1). Polish strings in `testdata/` from real upstream responses remain the only exception.
- Error string convention: `"openholidays: "` prefix (Phase 1 D-23).
- godoc on every exported symbol starting with the symbol name.
- Per-Client immutability after `NewClient` (Pitfall CONC-1). The new `closeOnce sync.Once` and `cache` fields are write-once during construction; the cache's internal state is mutable but encapsulated behind its mutex.

### Integration Points

- **`doJSONGet` is the retry injection point** — retry wraps the `c.http.Do` + status-error build steps only. Decode, oversize, and empty-body gates run once per successful retry attempt.
- **`buildTransport` is the cache+hook+logging+header injection point** — D-89 revises the chain order to `req → hookTransport → cacheTransport → loggingTransport → headerTransport → underlying`. The function in `config.go` is edited in place (Phase 2 D-29 explicit).
- **`Client.Close()` becomes load-bearing** — replaces the Phase 2 D-40 stub. The atomic.Bool stays; `sync.Once` is added; `cache.Close()` is called inside the once-guarded block.
- **`cacheTransport.RoundTrip` reads body into a buffer** — must drain+close `resp.Body` per Pitfall HTTP-3 BEFORE replacing it with `io.NopCloser(bytes.NewReader(buf))`. `io.LimitReader(resp.Body, maxResponseBytes+1)` caps the buffer per D-83.
- **Cache lives on the Client, not globally** — two `Client`s with different `baseURL`s have isolated caches automatically (Pitfall CACHE-2 mitigation per D-82).
- **Strict-decoding gate is one line in `doJSONGet`** — `if c.strict { decoder.DisallowUnknownFields() }` before `decoder.Decode(&out)`. Phase 3's existing `validateHolidays` post-decode validation runs after, unchanged.
- **Hook synchronous contract** — `hookTransport.RoundTrip` invokes the user's function on the calling goroutine's stack; a panicking hook propagates the panic. Pitfall CONC-2 explicit.
- **Phase 5 consumes the stable Phase 3 endpoint surface** — `cmd/ohcli` calls `c.PublicHolidays(ctx, PublicHolidaysRequest{...})` etc. unchanged. The CLI may optionally use `WithCache(24*time.Hour)` for `Countries`/`Languages` calls but does not require any new public type beyond the options.

</code_context>

<specifics>
## Specific Ideas

- The retry loop in `doJSONGet` should write a structured slog Debug record at every retry attempt boundary so operators can correlate retried calls in production logs — leverage the same `c.logger.LogAttrs` path that `loggingTransport` uses, with fields `attempt`, `delay_ms`, `retry_after_ms`, `reason` (e.g., "status_429", "net_timeout", "conn_reset"). Lives in `retry.go` alongside the retry loop, NOT in `loggingTransport` (which is per-round-trip; retry is per-loop-iteration).
- `cacheTransport.RoundTrip` should set a context value on the in-flight request when serving from cache (e.g., `cacheHitCtxKey{}` → `true`) so the OUTER `hookTransport` can detect cache hits in its hook payload. The hook itself still receives `(*http.Request, *http.Response, error)` — but consumers who want to differentiate cache hits from real round trips can `req.Context().Value(openholidays.CacheHitContextKey)` (exported unique key type per Go context-key convention). The exported `CacheHitContextKey` type adds 1 line of public surface; worth the explicit observability hook.
- For `parseRetryAfter`, prefer `http.ParseTime` over `time.Parse(http.TimeFormat, h)` — `http.ParseTime` accepts RFC 1123, RFC 850, and ANSI C asctime per RFC 7231 §7.1.1.1, all three of which appear in the wild. The integer-seconds path uses `strconv.Atoi`, not `strconv.ParseInt(h, 10, 64)`, because the upstream conventional unit is seconds-as-int and the cleaner shape preserves intent.
- The cache key encoding (D-82) intentionally includes `req.URL.Path` rather than the full `req.URL.String()` to elide `req.URL.Host` — the host is determined by the Client's `baseURL` (per-Client isolation per Pitfall CACHE-2) and including it in the key would waste bytes. If a consumer ever points a single Client at two hosts via a custom `WithHTTPClient` Transport, the cache would still be correct because the `baseURL` field on Client is the only path-prefix used in request construction.
- Tests for retry under deterministic clock should assert specific outcomes (not just "took the right amount of time") — e.g., "after 3 retries with backoff 100/200/400 ms, total fake-clock advance is exactly 700 ms" — using the `fakeClock.Advance(d)` shape from D-95. This avoids the flake mode of "real clock said 720 ms, test asserts < 1000 ms" that Phase 2 D-48 already navigated.
- The `WithMaxRetryWait` knob should be documented on its godoc as "applies to each individual sleep, not the cumulative retry budget". A 5-attempt retry with 60 s cap can still take up to 5 minutes total — bounded only by ctx and `maxAttempts`. Consumers wanting a total-budget cap should pass a `ctx.WithTimeout(...)` themselves; the SDK does NOT enforce a cumulative cap (would conflict with the per-attempt cap semantics).
- Cache + retry composition: when the retry loop fires after a 5xx, the second attempt re-enters the RoundTripper chain. The cache layer sees the path is in the allow-list AND the status was 5xx on the first attempt (no cache write happened). The second attempt's 200 then writes to cache. No special-case logic required — the path-prefix + success-only gate composes correctly with retries.
- Document in `cacheTransport`'s godoc that consumers supplying a custom backend via `WithCacheBackend` are responsible for thread-safety of their backend — the RoundTripper does NOT serialize access. Pitfall CACHE-4 only applies to the default `MemoryCache` (which uses RWMutex internally).
- For OBS-02 `bytes_in` field (Phase 2 D-31): when `cacheTransport` returns a synthetic response from cache, `resp.ContentLength` should be set to `int64(len(cachedBytes))` so `loggingTransport`'s OBS-02 record shows the correct payload size on cache hits. This is a one-line set inside `cacheTransport.RoundTrip` when building the synthetic `*http.Response`. Cache hits then appear in logs with `status=200`, `duration_ms=~0`, `bytes_in=<actual>` — easy to spot in production.
- The exported `Cache` interface and `MemoryCache` type belong in `cache.go` (single file). The `cacheTransport` RoundTripper belongs in a separate `transport_cache.go` so the file naming pattern matches `transport_hook.go` (Phase 4 new) and `transport.go` (Phase 2 header + logging). Keeps "transport layer" files visually grouped at repo root.

</specifics>

<deferred>
## Deferred Ideas

- **Negative caching** — Pitfall CACHE-1 hints at "negative cache with short TTL to prevent stampedes on hard-error conditions (e.g., 404 on a non-existent country code)". Out of v0.1.0 scope. If a consumer reports a stampede pattern, add `WithNegativeCacheTTL(d time.Duration)` in v0.2.
- **Persistent cache backends (Redis, BoltDB)** — explicitly rejected for M1 per FEATURES.md "Anti-features". The exported `Cache` interface (D-79) anchors the future extension point — a consumer writing a Redis-backed `Cache` impl can pass it via `WithCacheBackend(redisCache)`. The library will NOT ship one in v0.1.0.
- **Retry-After cumulative-budget cap** — currently the per-attempt sleep is capped at `maxRetryWait` (default 60 s) but cumulative time is bounded only by `maxAttempts` × per-attempt + ctx deadline. A `WithMaxRetryBudget(d time.Duration)` total cap was considered. Rejected for v0.1.0: consumers wanting that supply a `ctx.WithTimeout(...)`. Add only if a real consumer asks.
- **Per-status custom retry policies** — `WithRetryStatus(code int, retryable bool)` was considered to let consumers add 418 or remove 429 from the retryable set. Rejected for v0.1.0 (D-73 explicit). Add in v0.2 if a consumer asks.
- **Async hook** — Pitfall CONC-2 documents synchronous-only. An `WithAsyncHook(fn, bufferSize int)` variant that spawns goroutines was considered. Rejected: goroutine ownership becomes the SDK's problem; the consumer can `go fn(req, resp, err)` inside their own sync hook if they want async.
- **Per-call strict-decoding override** — D-91 rejects this. If a real consumer needs "lenient for old endpoints, strict for new", they instantiate two `*Client`s (cheap; idempotent; honors the immutability guarantee). Add in v0.2 if real demand surfaces.
- **`WithCacheable(paths ...string)` to add custom cacheable endpoints** — D-83 hardcodes the path-prefix allow-list. A consumer wanting to cache `/PublicHolidays` for a year would need a custom `Cache` backend wrapper or a v0.2 option. The current default (holiday endpoints uncached) protects against the temporal-data trap from STATE.md / FEATURES.md.
- **Distributed-tracing propagation** — `WithRequestHook` lets consumers thread `trace.SpanContextFromContext(req.Context())` into their hook function. The SDK does NOT propagate OpenTelemetry context itself (zero-dep policy). Documented in `docs/design.md` (Phase 5 deliverable).
- **Cache `Vary`-header awareness** — RFC 7234 cache-control semantics (the `Vary: Accept-Language` use case) are out of v0.1.0 scope. The opt-in TTL cache is sufficient for the static reference data this library caches (`Countries`, `Languages`, `Subdivisions`).
- **Connection-pool tuning on the underlying `http.Transport`** — Phase 4 doesn't touch `cfg.httpClient.Transport`. A consumer who needs custom pool sizes / idle timeouts supplies their own `*http.Client` via `WithHTTPClient`. Out of Phase 4 scope.
- **`goleak` adoption** — Phase 4 uses `runtime.NumGoroutine()` delta checks per D-96. If real flakes surface, evaluate `go.uber.org/goleak` as a test-only dep in v0.2 (would require a `Key Decisions` entry per PROJECT.md test-only dep allowlist).
- **Retry attempt counter wired into `loggingTransport.attempt`** — Phase 2 D-31 hardcodes `attempt = 1`. The retry loop in `doJSONGet` could thread an attempt counter via a context value, and `loggingTransport` could read it. Deferred to v0.2 (or to a Phase 5 doc/quality task) — not blocking RESIL-* or TEST-05.

</deferred>

---

*Phase: 04-resilience*
*Context gathered: 2026-05-27*
