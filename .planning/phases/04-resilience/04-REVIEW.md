---
phase: 04-resilience
reviewed: 2026-05-28T00:00:00Z
depth: standard
files_reviewed: 16
files_reviewed_list:
  - cache.go
  - cache_test.go
  - client.go
  - client_test.go
  - clock_test.go
  - config.go
  - internal_test.go
  - options.go
  - options_test.go
  - request.go
  - retry.go
  - retry_test.go
  - transport_cache.go
  - transport_cache_test.go
  - transport_hook.go
  - transport_hook_test.go
findings:
  critical: 1
  warning: 3
  info: 4
  total: 8
status: issues_found
---

# Phase 04 (Resilience): Code Review Report — Round 3 Re-review

**Reviewed:** 2026-05-28
**Depth:** standard
**Files Reviewed:** 16
**Status:** issues_found

## Summary

Third pass on phase 04 (resilience). All 11 fixes from round 1 still hold; the 7 open findings from round 2 (3 WR + 4 IN) are unchanged because no remediation commits have landed since that review. The phase 02 round-3 fixes that touched files in this scope — `5b1c66c` (remove `Client.userAgent` + `Client.logger`), `b154c0e` (silence `fnv.Hash.Write` errcheck), `fe1d1f3` (remove `Client.closed`), `805fdf3` (test cleanup) — integrate cleanly. The `ctxSleep` function in `client.go` was NOT touched by phase 02 fixes, so WR-03 still applies as written.

The cross-phase fix verification surfaced one **new Critical** finding that the prior two passes missed: a data race on `Client.rand` (a `*math/rand/v2.Rand`) when concurrent endpoint calls invoke `computeBackoff` from the retry loop. `math/rand/v2.Rand` is NOT documented as safe for concurrent use, and the codebase has no synchronization around `c.rand` accesses. Current tests do not exercise this race because the only concurrent endpoint test (`TestClient_ConcurrentAccess`) deliberately omits `WithRetry`. A user calling endpoint methods concurrently with `WithRetry` enabled would trigger `-race` failures and could see corrupted jitter values in production.

The four prior IN findings remain valid; one Info-level note correcting a stale claim in the round-2 review's "Notes" section is also recorded below (the `Client.closed` field was removed in phase-02 round 3, so the previous review's race-cleanliness observation no longer matches the code).

### Prior-review status

**Round 1 (11 closed):**

| ID | Title | Status | Verification |
|----|-------|--------|--------------|
| CR-01 | Unbounded drain in `cacheTransport.RoundTrip` | FIXED | `transport_cache.go:152` wraps with `io.LimitReader(resp.Body, maxResponseBytes+1)` |
| CR-02 | Cache transport discards `*http.Response` on read error without context | FIXED | `transport_cache.go:160` wraps with `fmt.Errorf("openholidays: cache: read response body: %w", readErr)` |
| WR-01 | `computeBackoff` shift overflow at high attempt counts | FIXED | `retry.go:255-258` guards with `attempt < 63 && cfg.baseDelay > 0` + `exp > 0` predicate; covered by `retry_test.go` attempt-40 and attempt-63 subtests |
| WR-02 | `NewMemoryCache` accepts non-positive TTL silently | FIXED (doc) | `cache.go:102-111` documents the contract verbatim |
| WR-03 | Stale godoc reference to `sync.Cancel` | FIXED | `cache.go:66-67` |
| WR-04 | Goroutine-count test flake potential | PARTIALLY ADDRESSED | `TestClient_CloseStopsSweeper` uses deterministic `sweepDone`. Two cache tests still use `runtime.NumGoroutine` deltas — tracked as IN-01 |
| WR-05 | `newClientRand` fallback seeds only 8 of 32 ChaCha8 bytes | FIXED | `client.go:165-187` two-round FNV-128a fills all 32 bytes |
| WR-06 | Retry exhaustion on retryable-status responses skips "retry exhausted" prefix | FIXED | `request.go:168-170` |
| WR-07 | Retry loop reuses the same `*http.Request` across attempts | FIXED | `request.go:108` calls `req.Clone(ctx)` per attempt |
| WR-08 | Cache transport returns oversized buf to decoder | FIXED | `transport_cache.go:171-174` |
| WR-09 | `TestClient_ContextCancel` 200 ms ceiling fragile | FIXED | `client_test.go:335` ceiling bumped to 500 ms |

**Round 2 (7 open — no fix commits since review):** WR-01, WR-02, WR-03, IN-01, IN-02, IN-03, IN-04 — all re-confirmed against current source, see findings below (now renumbered as WR-02..WR-04, IN-02..IN-05 to make room for the new CR-01).

## Critical

### CR-01: Concurrent endpoint calls with `WithRetry` race on `Client.rand`

**File:** `client.go:67`, `client.go:169-197`, `retry.go:246-268`, `request.go:133`
**Issue:** `Client.rand` is a `*math/rand/v2.Rand` value shared across all goroutines that call endpoint methods on the same `*Client`. The `math/rand/v2` package documentation does not promise concurrent-safe access on `*Rand` — its safety depends on the wrapped `Source`. `rand.NewChaCha8(seed)` returns a `*ChaCha8` whose `Uint64()` method is not documented as concurrent-safe either, and the official `math/rand/v2` docs explicitly say users needing concurrency must wrap the source.

The retry loop in `doJSONGet` passes `c.rand` directly to `computeBackoff`:

```go
// request.go:133
delay := computeBackoff(attempt, retryAfter, c.retry, c.rand)
```

And `computeBackoff` calls `rnd.Int64N(int64(capped))` (retry.go:263). With two goroutines simultaneously executing endpoint methods on the same `*Client` while retry is enabled, both invocations of `Int64N` read and mutate the underlying ChaCha8 state without synchronization.

Why prior reviews missed this: the only existing concurrent-endpoint test is `TestClient_ConcurrentAccess` (`client_test.go:254-295`), and it deliberately does NOT enable `WithRetry`. The retry path's `computeBackoff` is therefore never exercised concurrently in CI, so `go test -race ./...` does not flag this race. The retry-related concurrent tests (`TestRetry_E2E_429Then500Then200`, etc.) use one client and one goroutine.

Practical impact:
- Under `-race`, a user running concurrent endpoint calls with retry will see test failures.
- Without `-race`, the worst observable symptom is corrupted jitter values (e.g., negative `Int64N` results would panic the stdlib at `n <= 0`, though ChaCha8 state corruption is more likely to produce skewed-but-valid jitter values silently).
- This violates the CLIENT-07 "safe for concurrent use from any goroutine" invariant that `Client`'s godoc (`client.go:30`) and PROJECT.md both assert.

The exact same concern is raised in the godoc itself: `retry.go:230-234` claims "Goroutine-safety of `*math/rand/v2.Rand` is established by the stdlib (math/rand/v2 docs)." This claim is incorrect — the stdlib docs for `Rand` say "The methods of Rand are not safe for concurrent use by multiple goroutines, but the global functions [...] are." (Verify: `go doc math/rand/v2.Rand` returns no concurrency guarantee; the package overview states the per-Rand methods are not safe.)

**Fix:** Two options, in increasing complexity:

1. **Guard `c.rand` with a mutex on the read path.** Add a `randMu sync.Mutex` to `Client`, and lock around the `computeBackoff` call:
   ```go
   c.randMu.Lock()
   delay := computeBackoff(attempt, retryAfter, c.retry, c.rand)
   c.randMu.Unlock()
   ```
   Lowest overhead; preserves per-Client jitter sequence.

2. **Use a per-call seeded `Rand`.** Construct a fresh `rand.Rand` per retry-loop entry, seeded from `c.rand` under a short critical section (or directly from `crypto/rand`). Avoids the mutex on the hot path at the cost of an allocation per attempt.

Option (1) is the minimal correctness fix. Also correct the stale claim in `retry.go:230-234` once chosen.

Tests to add:
- A `TestClient_ConcurrentRetry_RaceClean` (or extend `TestClient_ConcurrentAccess`) that runs 50 parallel `Countries()` calls under `WithRetry(3, _)` against an `httptest.Server` returning 503 → 200 sequences, then run with `-race`. This locks the regression.

## Warnings

### WR-02: `doJSONGet` leaks `resp.Body` on final-attempt transport error when both `resp` and `httpErr` are non-nil

**File:** `request.go:108-117, 138-150`
**Issue:** The deferred drain-and-close at `request.go:151-158` is registered AFTER the retry loop exits. Inside the loop, the in-attempt drain (lines 122-132) runs ONLY for non-final iterations (`attempt != maxAttempts-1`). On the final attempt the code intentionally breaks before drain so the post-loop defer can take over — but the defer only runs when the function reaches that point.

Walk the failure path:

1. Final attempt (or first attempt with `!shouldRetry(resp, httpErr)`) returns `(resp, httpErr)` where both are non-nil. Per Go's `net/http` docs this is the documented behavior when a caller-supplied `CheckRedirect` returns an error (`http.Client.Do` doc: "A non-nil Response with a non-nil error only occurs when CheckRedirect fails").
2. Loop breaks (either on `!shouldRetry` or on the `attempt == maxAttempts-1` guard).
3. `httpErr != nil` at line 138 → return at line 147 or 149.
4. The defer at line 151 was never reached, so `resp.Body` is leaked.

For the standard `CheckRedirect` case, net/http's docs say the Response.Body is already closed — so this is technically defensible. But the same `(resp != nil, err != nil)` shape can be produced by user-supplied custom RoundTrippers (e.g., third-party tracing middleware that returns a non-nil resp with a wrapped error), and the in-loop drain at lines 122-132 explicitly demonstrates that the codebase considers this shape real. The omission of the same defense on the final attempt is inconsistent.

**Fix:** Add a defensive drain-and-close on the post-loop error path, mirroring the in-loop pattern:

```go
if httpErr != nil {
    if resp != nil && resp.Body != nil {
        _, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes+1))
        _ = resp.Body.Close()
    }
    if maxAttempts > 1 {
        return zero, fmt.Errorf("openholidays: GET %s: retry exhausted (%d attempts): %w", path, maxAttempts, httpErr)
    }
    return zero, fmt.Errorf("openholidays: GET %s: %w", path, httpErr)
}
```

### WR-03: `doJSONGet` reports "retry exhausted (N attempts)" even when only 1 attempt ran

**File:** `request.go:146-148`
**Issue:** When `c.retry.maxAttempts > 1` AND the first `c.http.Do` returns a non-retryable error (e.g., DNS resolution failure, TLS handshake error — neither `net.Error.Timeout()` nor `syscall.ECONNRESET`), the loop breaks on `!shouldRetry(resp, httpErr)` at attempt 0. Only ONE round trip occurred. But the post-loop wrap at line 146-148 unconditionally fires the "retry exhausted (%d attempts)" prefix because `maxAttempts > 1`:

```go
if maxAttempts > 1 {
    return zero, fmt.Errorf("openholidays: GET %s: retry exhausted (%d attempts): %w", path, maxAttempts, httpErr)
}
```

A user calling `c := NewClient(WithRetry(5, 100*time.Millisecond))` and getting a DNS error sees `"openholidays: GET /Countries: retry exhausted (5 attempts): ..."` even though only 1 attempt ran. This is misleading and could confuse operators triaging logs.

The same issue does NOT apply to the `*APIError` wrap on lines 168-170 because that branch correctly checks `shouldRetry(resp, nil)` first — non-retryable statuses (400, 404) do not get the prefix. The bug is specific to the `httpErr` branch.

**Fix:** Track the actual attempt count and only apply the "retry exhausted" prefix when more than one attempt actually ran:

```go
var attemptsMade int
for attempt := 0; attempt < maxAttempts; attempt++ {
    if ctxErr := ctx.Err(); ctxErr != nil {
        return zero, ctxErr
    }
    attemptReq := req.Clone(ctx)
    resp, httpErr = c.http.Do(attemptReq)
    attemptsMade = attempt + 1
    if !shouldRetry(resp, httpErr) {
        break
    }
    // ... existing logic
}
if httpErr != nil {
    if attemptsMade > 1 {
        return zero, fmt.Errorf("openholidays: GET %s: retry exhausted (%d attempts): %w", path, attemptsMade, httpErr)
    }
    return zero, fmt.Errorf("openholidays: GET %s: %w", path, httpErr)
}
```

Alternatively: only apply the prefix when `shouldRetry` returned true at least once.

### WR-04: `ctxSleep(d <= 0)` does not check ctx, asymmetric with `fakeClock.Sleep`

**File:** `client.go:143-155` vs `clock_test.go:63-69`
**Issue:** Production `ctxSleep` short-circuits on `d <= 0` and returns nil immediately:

```go
func ctxSleep(ctx context.Context, d time.Duration) error {
    if d <= 0 {
        return nil
    }
    // ... timer-based sleep
}
```

Test `fakeClock.Sleep` checks `ctx.Err()` first, regardless of `d`:

```go
func (f *fakeClock) Sleep(ctx context.Context, d time.Duration) error {
    if err := ctx.Err(); err != nil {
        return err
    }
    f.Advance(d)
    return nil
}
```

Both functions implement the same `Client.sleepFunc` contract. If `computeBackoff` ever returns `d == 0`, production code does not surface ctx cancellation while the test code does. Current `computeBackoff` floor at `retry.go:260-262` forces `capped >= time.Millisecond`, so jitter is in `[0, 1ms)` — but `rnd.Int64N(int64(time.Millisecond))` can return 0, making this reachable.

The retry loop already checks `ctx.Err()` at the top of every iteration, so a cancelled-ctx caller would see ctx.Err on the NEXT iteration after a no-op sleep — but if the no-op sleep is the LAST one (between final attempts), the post-loop block runs without seeing the cancellation. Practical impact: minimal, but the asymmetry is a latent inconsistency that may surface under future jitter-formula changes.

Verified that this finding still applies — phase 02 round 3 did NOT modify `ctxSleep`.

**Fix:** Make `ctxSleep` check `ctx.Err()` before the `d <= 0` short-circuit, matching `fakeClock.Sleep`:

```go
func ctxSleep(ctx context.Context, d time.Duration) error {
    if err := ctx.Err(); err != nil {
        return err
    }
    if d <= 0 {
        return nil
    }
    t := time.NewTimer(d)
    defer t.Stop()
    select {
    case <-ctx.Done():
        return ctx.Err()
    case <-t.C:
        return nil
    }
}
```

## Info

### IN-02: `runtime.NumGoroutine()` flake risk remains in two cache tests

**File:** `cache_test.go:100-137` (`TestMemoryCache_SweeperLazyStart`), `cache_test.go:169-207` (`TestMemoryCache_CloseIdempotent` concurrent subtest)
**Issue:** Per the round-1 WR-04 close-out, `TestClient_CloseStopsSweeper` was rewritten to use a deterministic `sweepDone` channel (excellent improvement at `client_test.go:228-243`). The two `cache_test.go` tests above still rely on `runtime.NumGoroutine()` delta comparisons with `time.Sleep(5*time.Millisecond)` grace windows. The tests are intentionally NOT parallel and tolerate `+1` goroutine slack, which is the right approach — but on heavily-loaded CI runners or under `-race` with active GC, counts can drift.

The deterministic alternative is to use `MemoryCache.sweepDone` (already exposed via same-package visibility, as `client_test.go:228` does).

**Fix:** Optional. Either:

1. Replace `runtime.NumGoroutine()` checks with `select { case <-mc.sweepDone: ... case <-time.After(...): ... }` patterns.
2. Accept the residual flake and monitor CI rates.

The current state is documented and stable enough for v0.x; flagged at Info severity for visibility.

### IN-03: Cache hit synthetic response omits standard HTTP protocol fields

**File:** `transport_cache.go:126-133`
**Issue:** The synthetic cache-hit response sets `StatusCode`, `Status`, `Header`, `Body`, `ContentLength`, and `Request`. It does NOT set `Proto`, `ProtoMajor`, `ProtoMinor`, or `TransferEncoding`. A user-supplied `WithRequestHook` reading any of these fields on a cache-hit response gets the zero value (`""` for Proto, 0 for ProtoMajor/Minor, nil slice for TransferEncoding).

`hookTransport.RoundTrip` IS outermost and DOES see this synthetic response (it's the whole point of D-88). Users wiring metrics like "count HTTP/2 requests" on `resp.ProtoMajor == 2` would silently misreport cache hits as HTTP/0.0.

**Fix:** Either populate the synthetic response's protocol fields from `req.Proto` / `req.ProtoMajor` / `req.ProtoMinor` (most accurate), or document the gap on the `WithRequestHook` godoc. Suggested doc addition to `config.go::RequestHookFunc`:

```
// Hooks reading cache-hit responses (CacheHitContextKey == true) must
// treat protocol-level fields (resp.Proto, resp.ProtoMajor, resp.ProtoMinor,
// resp.TransferEncoding) as unset — the synthetic response only populates
// status, headers, body, and ContentLength.
```

### IN-04: Cache hit synthetic response has empty `Header`, omits `Content-Type`

**File:** `transport_cache.go:129`
**Issue:** `Header: make(http.Header)` produces an empty header on cache-hit responses. The original 200 response would carry `Content-Type: application/json` (and possibly `Cache-Control`, `ETag`, etc.). Hooks that introspect headers see them on cache misses but not on cache hits — same family of issue as IN-03.

The downstream JSON decoder in `doJSONGet` does not check Content-Type, so functionally this is fine for the SDK's own consumers. But user hooks that key on `resp.Header.Get("Content-Type")` will misbehave on cache hits.

**Fix:** Optional. Either:

1. Store the response headers alongside the body in `cacheTransport`'s Put path (changes Cache contract from `[]byte` to a richer envelope — non-trivial).
2. Synthesize a minimal header (`synth.Header.Set("Content-Type", "application/json")`) so the common case works.
3. Document the gap explicitly.

For v0.x, option (3) is the minimum bar.

### IN-05: `Cache.Get` exposes the underlying byte slice — caller mutation corrupts the cache

**File:** `cache.go:144-152`
**Issue:** `MemoryCache.Get` returns `e.value`, which is the same `[]byte` reference stored in `m.entries[key]`:

```go
func (m *MemoryCache) Get(key string) ([]byte, bool) {
    m.mu.RLock()
    e, ok := m.entries[key]
    m.mu.RUnlock()
    if !ok || m.nowFn().After(e.expiresAt) {
        return nil, false
    }
    return e.value, true
}
```

If any caller mutates the returned slice (e.g., `buf[0] = 'x'`), the cached entry is corrupted for all subsequent reads. The only in-tree consumer is `cacheTransport.RoundTrip` (line 130-131), which wraps the slice in `bytes.NewReader` (read-only) and passes it to the JSON decoder. The decoder does not write to its input. So the in-tree path is safe today.

However, the `Cache` interface (`config.go:76-80`) is exported and intended for third-party implementations and consumers. A user implementing `WithCacheBackend(myCache)` may legitimately call `myCache.Get(key)` for diagnostics — the contract on returned-slice mutability is undocumented and dangerous.

**Fix:** Either:

1. Document on the `Cache` godoc that returned slices are read-only references to internal storage and MUST NOT be mutated.
2. Have `MemoryCache.Get` return a copy: `out := make([]byte, len(e.value)); copy(out, e.value); return out, true` — costs an allocation per hit but eliminates the footgun.

For an SDK targeting OSS consumers, (1) is the minimum.

## Files Reviewed (compact summary)

| File | Lines | Issues This Pass |
|------|-------|------------------|
| `cache.go` | 240 | IN-05 |
| `cache_test.go` | 208 | IN-02 |
| `client.go` | 198 | CR-01, WR-04 |
| `client_test.go` | 637 | — |
| `clock_test.go` | 149 | — |
| `config.go` | 191 | — |
| `internal_test.go` | 261 | — |
| `options.go` | 413 | — |
| `options_test.go` | 565 | — |
| `request.go` | 322 | WR-02, WR-03 |
| `retry.go` | 269 | CR-01 (stale godoc claim) |
| `retry_test.go` | 646 | — |
| `transport_cache.go` | 180 | IN-03, IN-04 |
| `transport_cache_test.go` | 307 | — |
| `transport_hook.go` | 108 | — |
| `transport_hook_test.go` | 288 | — |

## Notes (observations, not findings)

- **Phase 02 round-3 fixes integration:** Verified clean. Commits `5b1c66c` (remove `Client.userAgent` / `Client.logger`), `b154c0e` (errcheck on `fnv.Hash.Write`), `fe1d1f3` (remove `Client.closed`), `805fdf3` (test cleanup) all compose with the phase 04 code. The transport chain accessors (`headerTransportFromChain` / `loggingTransportFromChain` in `options_test.go:137-156`) correctly walk the new chain after the dead fields were removed. No regressions found.
- **`Client.closed` removal correctness:** The round-2 review's Notes section asserted "`Client.closed` uses `atomic.Bool`" — that field was removed in commit `fe1d1f3`. The current `Client` struct (lines 58-69 of `client.go`) carries only the documented post-construction state (`closeOnce sync.Once`). Endpoint dispatch does not consult any post-Close flag, which matches the documented contract on `Close` (idempotent shutdown hook, NOT a gate on subsequent endpoint calls). PASS.
- **English-only rule (Gold Rule 1):** All identifiers, comments, godoc strings, and test names verified English. PASS.
- **Test conventions (Gold Rule 3):** `testify` + `require`/`assert` consistent; `t.Run` wraps every leaf case; `t.Parallel()` applied with documented exceptions for goroutine-count tests. One TestXxx per exported prod function. PASS.
- **Zero runtime dependencies:** No non-stdlib imports in production files. `hash/fnv`, `encoding/binary`, `os` in `client.go::newClientRand` are stdlib. PASS.
- **No `init()` and no unexpected package-level vars:** `internal_test.go::TestNoInitOrGlobalState` mechanically locks this. `CacheHitContextKey` and `Version` correctly listed in `allowedVars`. PASS.
- **Exported symbol godoc:** Every exported symbol has a doc comment. PASS.
- **`io.LimitReader` 10 MiB cap:** Applied in `doJSONGet` (lines 125, 156, 174), `cacheTransport` miss-path (line 143), and `cacheTransport` drain (line 152). PASS.
- **`slog.Default()` default + no body logging above Debug:** `loggingTransport` emits only at `slog.LevelDebug` and never reads the body; `hookTransport` does not log at all. PASS.
- **`-race` cleanliness:** `MemoryCache` uses `sync.RWMutex`; `fakeClock` uses `sync.Mutex`; `Client.closeOnce` uses `sync.Once`. **However:** `Client.rand` is shared across goroutines without synchronization — see CR-01. The current test suite does not exercise the race because no concurrent-endpoint test enables `WithRetry`.
- **Context cancellation ≤ 100 ms (CLIENT-09):** Verified via `TestClient_ContextCancel` (500 ms CI ceiling) and `TestRetry_CtxCancel` (200 ms ceiling). The fakeClock-based retry tests are tighter and more representative of the actual contract.
- **Strict-decoding composition (D-93):** `TestCache_StrictDecodingComposes` locks the contract that strict mode applies to cached bytes on every read (server hit once, strict gate fires twice). PASS.
- **Retry-NotARoundTripper structural audit:** `TestRetry_NotARoundTripper` scans for `type retryTransport` declarations and `transport_retry.go`. The audit matches `TrimLeft(line, " \t")` prefix `"type retryTransport"` per-line — would also catch a `type (\n retryTransport ...\n)` multi-line block. Sound.

---

_Reviewed: 2026-05-28_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
