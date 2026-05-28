---
phase: 04-resilience
plan: 02
subsystem: client-config
tags: [strict-decoding, test-seam, client-fields, cache-interface, request-hook, retry-stub]
dependency_graph:
  requires:
    - 04-01 (fakeClock helper from clock_test.go — signature consumed by Client.sleepFunc)
    - Phase 1–3 Client struct, doJSONGet pipeline, options.go pattern
  provides:
    - Client.{retry, cache, strict, requestHook, nowFunc, sleepFunc, rand, closeOnce} fields (Plan 03/04/05 fill semantics)
    - Cache interface (D-79) — Plan 04 implements MemoryCache against it
    - RequestHookFunc (D-87) — Plan 05 wires WithRequestHook(fn) onto it
    - retryConfig stub (D-77) — Plan 03 fills maxAttempts/baseDelay/maxWait
    - WithStrictDecoding(bool) option + decoder.DisallowUnknownFields gate in doJSONGet (D-91 / D-92, OBS-03)
    - newClientForTest(now, sleep, opts...) test seam (D-94) — Plan 03 retry_test + Plan 04 cache_test consume it
  affects:
    - request.go (one-line strict gate inserted between NewDecoder and Decode)
    - Client.Close (now guarded by sync.Once and calls c.cache.Close when non-nil; cache is nil until Plan 04 wires WithCache)
tech-stack:
  added:
    - context (client.go — for ctxSleep)
    - crypto/rand (client.go, alias `crand` — newClientRand seed)
    - encoding/binary (client.go — fallback seed in newClientRand)
    - math/rand/v2 (client.go — per-Client *rand.Rand via NewChaCha8)
    - sync (client.go — sync.Once on closeOnce)
  patterns:
    - Functional option for opt-in feature flag (WithStrictDecoding mirrors WithTimeout's "verbatim" convention)
    - sync.Once-guarded idempotent shutdown (Close)
    - Per-Client ChaCha8-seeded jitter source (D-78 / Pitfall RETRY-4)
    - Internal nowFunc/sleepFunc seam pattern (D-94 / TEST-3) for deterministic tests
key-files:
  created: []
  modified:
    - client.go
    - config.go
    - options.go
    - request.go
    - client_test.go
    - options_test.go
decisions:
  - D-91 wired (WithStrictDecoding immutable, OFF by default, no per-call or runtime toggle)
  - D-92 wired (DisallowUnknownFields gate BEFORE decoder.Decode in doJSONGet)
  - D-78 wired (crypto/rand → ChaCha8 per-Client jitter source via newClientRand; time-based fallback per CLIENT-01)
  - D-85 wired (Close uses sync.Once + best-effort c.cache.Close)
  - D-94 wired (Client.nowFunc/sleepFunc default to time.Now/ctxSleep; newClientForTest seam in client_test.go)
  - D-77 stub (retryConfig declared as empty struct in config.go; Plan 03 fills fields)
  - D-79 wired (full Cache interface with Get/Put/Close declared in config.go; MemoryCache impl in Plan 04)
  - D-87 wired (RequestHookFunc declared in config.go; WithRequestHook + hookTransport in Plan 05)
metrics:
  duration: ~30m (one execution wave)
  completed: 2026-05-28
requirements_complete:
  - OBS-03 (WithStrictDecoding option + strict gate in doJSONGet — surfaces upstream schema drift)
  - CLIENT-08 (Close uses sync.Once + cache.Close hook — Plan 04 supplies the cache, but the Close shape is now load-bearing)
---

# Phase 04 Plan 02: Phase 4 Client State & Strict-Decoding Plumbing Summary

**One-liner:** All Phase 4 Client state plus the opt-in strict-decoding gate land atomically — Plans 03/04/05 now plug into pre-existing nil/zero-valued retry/cache/hook fields.

## What Shipped

### Client struct (client.go) — 8 new fields appended
```go
type Client struct {
    // existing Phase 1–2 fields unchanged:
    http        *http.Client
    baseURL     string
    userAgent   string
    logger      *slog.Logger
    timeout     time.Duration
    closed      atomic.Bool
    // Phase 4 additions (D-77 / D-78 / D-79 / D-85 / D-87 / D-91 / D-94):
    retry       retryConfig                                // D-77; zero-value = disabled
    cache       Cache                                      // D-79; nil = disabled (wired in Plan 04)
    strict      bool                                       // D-91; immutable after NewClient
    requestHook RequestHookFunc                            // D-87; nil = no hook (wired in Plan 05)
    nowFunc     func() time.Time                           // D-94; defaults to time.Now
    sleepFunc   func(context.Context, time.Duration) error // D-94; defaults to ctxSleep
    rand        *rand.Rand                                 // D-78; per-Client ChaCha8-seeded
    closeOnce   sync.Once                                  // D-85; guards cache.Close inside Close()
}
```
NewClient initializes the seam defaults explicitly (`nowFunc: time.Now`, `sleepFunc: ctxSleep`, `rand: newClientRand()`); `closeOnce` and `closed` keep their zero values.

### Two unexported helpers (client.go)
- **`ctxSleep(ctx, d) error`** — D-94 verbatim. Non-positive d returns nil immediately; otherwise `select` on `ctx.Done()` and `time.NewTimer(d).C`. Used as the default `Client.sleepFunc` so the retry loop in Plan 03 inherits ctx-aware sleep without re-implementing it.
- **`newClientRand() *rand.Rand`** — D-78 + RESEARCH §"math/rand/v2 per-Client seed" verbatim. `crypto/rand.Read` fills a 32-byte ChaCha8 seed; on the rare error path it falls back to `binary.LittleEndian.PutUint64(seed[:8], time.Now().UnixNano())` because `NewClient` must not return an error (CLIENT-01).

### Close (client.go) — sync.Once-guarded
```go
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
Phase 2's `closed.Store(true)` no-op stub is now wrapped in `sync.Once.Do`, and a best-effort `c.cache.Close()` runs inside the once-guarded block. Until Plan 04 wires `WithCache`, `c.cache` is nil and the inner branch is dead — but the shape is load-bearing.

### Three new types (config.go)
- **`type Cache interface { Get(string) ([]byte, bool); Put(string, []byte); Close() error }`** — D-79 full method set declared NOW (not deferred to Plan 04) so `Client.Close` can call `c.cache.Close()` without a build error. Plan 04 ships `MemoryCache` implementing this contract.
- **`type RequestHookFunc func(*http.Request, *http.Response, error)`** — D-87 verbatim. Plan 05's `WithRequestHook` accepts this shape.
- **`type retryConfig struct{}`** — D-77 stub. Plan 03 will edit it in place to add `maxAttempts`, `baseDelay`, `maxWait`. Empty-struct field on Client is zero-cost.

### clientConfig (config.go) — 5 new fields
```go
type clientConfig struct {
    // existing fields unchanged
    retry          retryConfig
    cache          Cache
    cacheTTL       time.Duration
    hook           RequestHookFunc
    strictDecoding bool
}
```
`defaultConfig` and `composeHTTPClient` and `buildTransport` are explicitly unchanged — Plan 04 (cache) and Plan 05 (hook) will edit `buildTransport` in place per D-89.

### WithStrictDecoding (options.go)
```go
func WithStrictDecoding(strict bool) Option {
    return func(cfg *clientConfig) {
        cfg.strictDecoding = strict
    }
}
```
"false is stored verbatim" matches the WithTimeout convention (no defensive special-case). Godoc names D-91, D-92, D-93, CL-15, and Pitfall JSON-1. Immutable after NewClient by design — no runtime toggle exists.

### Strict-decoding gate (request.go::doJSONGet)
Three lines inserted between `decoder := json.NewDecoder(limited)` and `if decodeErr := decoder.Decode(&out); ...`:
```go
if c.strict {
    decoder.DisallowUnknownFields()
} // D-92: applied BEFORE Decode; runs on every call including cache hits (D-93).
```
The boundary/mid-truncation gates, `ErrEmptyResponse`, and post-decode `validateHolidays` paths are unchanged.

### newClientForTest (client_test.go)
Same-package, unexported test seam. Wraps NewClient and replaces `nowFunc`/`sleepFunc` only when caller supplies non-nil overrides. Plan 03 retry_test and Plan 04 cache_test will consume this seam plus the `fakeClock` from clock_test.go (Plan 01).

```go
func newClientForTest(now func() time.Time, sleep func(context.Context, time.Duration) error, opts ...Option) *Client {
    c := NewClient(opts...)
    if now != nil { c.nowFunc = now }
    if sleep != nil { c.sleepFunc = sleep }
    return c
}
```

## Tests Added

| Test | Subtests | What it locks |
|------|----------|---------------|
| `TestWithStrictDecoding` (options_test.go) | 3 | true verbatim, false verbatim, default-off invariant (Pitfall JSON-1) |
| `TestNewClientForTest` (client_test.go) | 3 | non-nil overrides take effect; nil leaves defaults; options pass through to NewClient |
| `TestWithStrictDecoding_RejectsUnknown` (client_test.go) | 1 | wire-level: httptest.Server returns unknown field → decoder surfaces error containing field name + "openholidays: decode" wrap |
| `TestWithStrictDecoding_DefaultLenient` (client_test.go) | 1 | default Client accepts unknown JSON fields — Pitfall JSON-1 anti-regression lock |

All tests use testify (Gold Rule 3 primary), `t.Parallel()` at every leaf, `require` for preconditions, `assert` for verifications. One TestXxx per exported production function (`WithStrictDecoding`); the two wire-level tests are extra coverage owned by the OBS-03 contract rather than a specific exported function, and `newClientForTest` is treated as a "production function in the test-tree" per the plan's note.

## Verification Output

```
go build ./...                                     -> exit 0
go vet ./...                                       -> exit 0
gofmt -l client.go config.go options.go request.go client_test.go options_test.go
                                                   -> (no output)
go test -race -count=1 ./...                       -> ok 1.85s
go test -race -count=1 -run TestNoInitOrGlobalState ./...
                                                   -> ok 1.02s
go test -race -count=1 -run 'TestWithStrictDecoding|TestWithStrictDecoding_RejectsUnknown|TestWithStrictDecoding_DefaultLenient|TestNewClientForTest' ./...
                                                   -> ok (4 TestXxx funcs, 8 subtests, all PASS)
```

Go toolchain: `go1.26.3-X:nodwarf5 linux/amd64`.

## Commits

| Commit | Subject | Files |
|--------|---------|-------|
| `ade4c9a` | feat(04-02): declare Cache interface, RequestHookFunc, retryConfig stub + clientConfig fields | config.go (+52) |
| `49b20d6` | feat(04-02): extend Client struct + add ctxSleep/newClientRand + rewrite Close with sync.Once | client.go (+106, -25 net) |
| `7cbf5d1` | feat(04-02): add WithStrictDecoding option + strict-decode gate in doJSONGet + newClientForTest seam | options.go (+26), request.go (+3), client_test.go (+113), options_test.go (+33) |

## Net Lines Modified

| File | Insertions | Deletions |
|------|-----------:|----------:|
| client.go | 106 | 25 |
| client_test.go | 113 | 0 |
| config.go | 52 | 0 |
| options.go | 26 | 0 |
| options_test.go | 33 | 0 |
| request.go | 3 | 0 |
| **Total** | **333** | **25** |

## Deviations from Plan

### [Rule 3 — Blocking issue resolved] Task 2 committed BEFORE Task 1

**Found during:** Pre-commit verification of Task 1.
**Issue:** Task 1's `<done>` criterion requires `go build ./...` to exit 0, but Task 1's `Client` struct edit references `retryConfig`, `Cache`, and `RequestHookFunc` — three types whose declarations the plan assigns to Task 2 (`config.go`). The plan lists Task 1 first in the document, but each task's commit must build cleanly in isolation.
**Fix:** Committed Task 2's `config.go` (types + clientConfig fields) FIRST as `ade4c9a`, then Task 1's `client.go` (struct + helpers + Close rewrite) as `49b20d6`. Both commits independently build/vet/test green.
**Why this is Rule 3, not Rule 4:** No architectural change — same types, same files, same final state. Reordering two intra-plan commits to satisfy "each commit builds" is the smallest possible fix.
**Files modified by deviation:** None additional. Only the commit ORDER changed.

No other deviations. No auth gates. No checkpoints triggered.

## Threat Surface Scan

No new network endpoints, auth paths, file access patterns, or schema changes at trust boundaries beyond what the plan's `<threat_model>` already identifies (T-04-02, T-04-03, T-04-04). The strict-decoding gate is the only behavior change visible to upstream JSON — and it is precisely the mitigation listed for T-04-02. No additional threat flags raised.

## Known Stubs (intentional, future-plan-resolved)

| Stub | File | Reason | Resolved by |
|------|------|--------|-------------|
| `retryConfig struct{}` | config.go | Plan 03 owns retry semantics — empty stub avoids forward-reference | Plan 04-03 |
| `Client.requestHook RequestHookFunc` (always nil) | client.go | Plan 05 owns the WithRequestHook option | Plan 04-05 |
| `Client.cache Cache` (always nil) | client.go | Plan 04 owns the WithCache + WithCacheBackend options | Plan 04-04 |
| `Client.retry retryConfig` (zero-value) | client.go | Plan 03 owns WithRetry / WithMaxRetryWait | Plan 04-03 |

These are not "stubs that prevent the plan's goal from being achieved" — Plan 02's goal is to land the FIELD/TYPE plumbing so subsequent plans drop into existing slots. The strict-decoding behavior (OBS-03) ships fully wired and tested.

## Self-Check: PASSED

- Created files: none (plan only modifies existing files).
- Modified files exist: client.go, config.go, options.go, request.go, client_test.go, options_test.go — all present.
- Commits exist: `ade4c9a`, `49b20d6`, `7cbf5d1` — all in `git log`.
- All verification commands from the plan pass exit 0.
- `internal_test.go::allowedVars` is byte-identical to the base commit (no new package-level vars introduced).
