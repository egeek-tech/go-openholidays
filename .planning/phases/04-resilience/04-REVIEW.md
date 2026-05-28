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
  critical: 0
  warning: 3
  info: 4
  total: 7
status: issues_found
---

# Phase 04 (Resilience): Code Review Report — Re-review

**Reviewed:** 2026-05-28
**Depth:** standard
**Files Reviewed:** 16
**Status:** issues_found

## Summary

This is the second pass on phase 04 (resilience). The first review (commit chain `56fa40f`..`d290bd6`) surfaced 2 Critical + 9 Warning findings; the re-review confirms **all 11 prior findings are addressed** in the current source. Subsequent phase-01/02/03 fix passes that touched files in this scope (`client.go`, `config.go`, `client_test.go`, `internal_test.go`, `options.go`, `options_test.go`, `request.go`) integrate cleanly and introduce no regressions.

The remaining findings are smaller, mostly pre-existing latent defects that the prior review missed plus one cosmetic regression introduced by a fix. None rise to BLOCKER severity. The codebase is in good shape; recommend addressing WR-01 (resp/body leak on final-attempt transport error with non-nil resp) before v1.0 because it is the only one that could leak resources in production, but it requires a custom `CheckRedirect` to trigger.

### Prior-review status (all 11 closed)

| ID | Title | Status | Verification |
|----|-------|--------|--------------|
| CR-01 | Unbounded drain in `cacheTransport.RoundTrip` | FIXED | `transport_cache.go:152` now wraps with `io.LimitReader(resp.Body, maxResponseBytes+1)` |
| CR-02 | Cache transport discards `*http.Response` on read error without context | FIXED | `transport_cache.go:160` now wraps with `fmt.Errorf("openholidays: cache: read response body: %w", readErr)` |
| WR-01 | `computeBackoff` shift overflow at high attempt counts | FIXED | `retry.go:255-258` adds `attempt < 63 && cfg.baseDelay > 0` guard + `exp > 0` predicate; covered by `retry_test.go` attempt-40 and attempt-63 subtests |
| WR-02 | `NewMemoryCache` accepts non-positive TTL silently | FIXED (doc) | `cache.go:102-111` documents the contract verbatim; intentionally not a code change per the "constructors never error" contract |
| WR-03 | Stale godoc reference to `sync.Cancel` | FIXED | `cache.go:66-67` now reads `sync.RWMutex and sync.Once trigger the standard go vet copy-lock warning` |
| WR-04 | Goroutine-count test flake potential | PARTIALLY ADDRESSED | `TestClient_CloseStopsSweeper` rewritten to use deterministic `sweepDone` channel (now a subtest of `TestClient_Close` per Gold Rule 3). Two cache-side tests (`TestMemoryCache_SweeperLazyStart`, `TestMemoryCache_CloseIdempotent`) still rely on `runtime.NumGoroutine()` deltas. Acknowledged design; see IN-01 below. |
| WR-05 | `newClientRand` fallback seeds only 8 of 32 ChaCha8 bytes | FIXED | `client.go:165-187` now uses two FNV-128a rounds with `(nanos, pid)` and `(pid, nanos)` to populate all 32 bytes |
| WR-06 | Retry exhaustion on retryable-status responses skips "retry exhausted" prefix | FIXED | `request.go:168-170` now wraps `*APIError` with the retry-exhausted prefix when `maxAttempts > 1 && shouldRetry(resp, nil)` |
| WR-07 | Retry loop reuses the same `*http.Request` across attempts | FIXED | `request.go:108` now calls `req.Clone(ctx)` per attempt |
| WR-08 | Cache transport returns oversized buf to decoder | FIXED | `transport_cache.go:171-174` now returns `ErrResponseTooLarge` when `len(buf) > maxResponseBytes`, never handing oversized bytes to the decoder |
| WR-09 | `TestClient_ContextCancel` 200 ms ceiling fragile | FIXED | `client_test.go:323` ceiling bumped to 500 ms with explanatory comment citing contract vs CI slack |

### New findings (this pass)

3 Warning + 4 Info. Listed in severity order, then file order.

## Warnings

### WR-01: `doJSONGet` leaks `resp.Body` on final-attempt transport error when both `resp` and `httpErr` are non-nil

**File:** `request.go:108-117, 138-150`
**Issue:** The deferred drain-and-close at `request.go:151-158` is registered AFTER the retry loop exits. Inside the loop, the in-attempt drain (lines 122-132) runs ONLY for non-final iterations (`attempt != maxAttempts-1`). On the final attempt, the code intentionally breaks before drain so the post-loop defer can take over — but the defer only runs when the function reaches that point.

Walk the failure path:

1. Final attempt (or first attempt with `!shouldRetry(resp, httpErr)`) returns `(resp, httpErr)` where both are non-nil. Per Go's `net/http` docs, this is the documented behavior when a caller-supplied `CheckRedirect` returns an error (`http.Client.Do` doc: "A non-nil Response with a non-nil error only occurs when CheckRedirect fails").
2. Loop breaks (either on `!shouldRetry` or on the `attempt == maxAttempts-1` guard).
3. `httpErr != nil` at line 138 → return at line 147 or 149.
4. The defer at line 151 was never reached, so `resp.Body` is leaked.

The in-loop drain at lines 122-132 handles the same case for non-final iterations, but it skips the final iteration:

```go
if attempt == maxAttempts-1 {
    break
}
// drain-and-close prior resp, then sleep
```

So a retry-enabled client whose user supplied `WithHTTPClient(&http.Client{CheckRedirect: func(...) error { return ... }})` — and whose final attempt encounters a redirect — leaks the response body.

Practical impact: rare in practice (requires custom `CheckRedirect` returning an error), but a real resource leak. The existing in-loop drain logic explicitly demonstrates that the codebase considers `(resp != nil, err != nil)` a real path; the omission of the same defense on the final attempt is inconsistent.

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

The same drain must guard the equivalent path in `cacheTransport.RoundTrip` at `transport_cache.go:137-139` — the non-200 / non-cacheable bypass returns `(resp, err)` without draining. That one is caller-responsibility (the outer chain eventually reaches `doJSONGet`'s defer), so it's OK as-is — but only if the outer chain always reaches that defer. With the leak path above, it does not.

### WR-02: `doJSONGet` reports "retry exhausted (N attempts)" even when only 1 attempt ran

**File:** `request.go:146-148`
**Issue:** When `c.retry.maxAttempts > 1` AND the first `c.http.Do` returns a non-retryable error (e.g., DNS resolution failure, TLS handshake error — neither `net.Error.Timeout()` nor `syscall.ECONNRESET`), the loop breaks on `!shouldRetry(resp, httpErr)` at attempt 0. Only ONE round trip occurred. But the post-loop wrap at line 146-148 unconditionally fires the "retry exhausted (%d attempts)" prefix because `maxAttempts > 1`:

```go
if maxAttempts > 1 {
    return zero, fmt.Errorf("openholidays: GET %s: retry exhausted (%d attempts): %w", path, maxAttempts, httpErr)
}
```

A user calling `c := NewClient(WithRetry(5, 100*time.Millisecond))` and getting a DNS error sees `"openholidays: GET /Countries: retry exhausted (5 attempts): ..."` even though only 1 attempt ran. This is misleading and could confuse operators triaging logs (e.g., "why are we exhausting 5 attempts on every DNS failure?" — they're not).

The same issue applies to the `*APIError` wrap on line 168-170 if the non-retryable status path is exercised — `shouldRetry(resp, nil)` is checked, so for non-retryable statuses (400, 404, etc.) the prefix is correctly NOT applied. So this finding is specific to the `httpErr` branch.

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

Alternatively: only apply the prefix when `shouldRetry` returned true at least once (i.e., we actually attempted a retry, not just exited on a non-retryable on attempt 0).

### WR-03: `ctxSleep(d <= 0)` does not check ctx, asymmetric with `fakeClock.Sleep`

**File:** `client.go:139-151` vs `clock_test.go:63-69`
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

These functions implement the same `Client.sleepFunc` contract. If `computeBackoff` ever returns `d == 0` (currently it cannot because the cap floor at `retry.go:260-262` forces `capped >= time.Millisecond`, so jitter is in `[0, 1ms)` — but `rnd.Int64N(int64(time.Millisecond))` could return 0), production code does not surface ctx cancellation while the test code does. This is a silent contract divergence.

The retry loop already checks `ctx.Err()` at the top of every iteration (`request.go:97-99`), so a cancelled-ctx caller would see ctx.Err on the NEXT iteration after a no-op sleep — but if the no-op sleep is the LAST one (between final attempts), the post-loop block runs without seeing the cancellation. Practical impact: minimal (sleeps of 0 are extremely rare), but the asymmetry is a latent inconsistency that may surface under future jitter formula changes.

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

### IN-01: `runtime.NumGoroutine()` flake risk remains in two cache tests

**File:** `cache_test.go:100-137` (`TestMemoryCache_SweeperLazyStart`), `cache_test.go:169-207` (`TestMemoryCache_CloseIdempotent` concurrent subtest)
**Issue:** Per the prior review's WR-04 close-out: `TestClient_CloseStopsSweeper` was rewritten to use a deterministic `sweepDone` channel (excellent improvement, see `client_test.go:228-233`). The two `cache_test.go` tests above still rely on `runtime.NumGoroutine()` delta comparisons with `time.Sleep(5*time.Millisecond)` grace windows. These tests are intentionally NOT parallel and tolerate `+1` goroutine slack, which is good — but on heavily loaded CI runners or under `-race` with active GC, the counts can drift.

The phase-03 `drainCountingTransport` pattern (commit `735cf1d`) does NOT directly apply here — that pattern counts RoundTripper invocations, not goroutines. The deterministic alternative for these two tests would be a `sweeperStopped() <-chan struct{}` test seam on `MemoryCache` analogous to what `TestClient_CloseStopsSweeper` already does on `sweepDone`.

**Fix:** Optional. Either:

1. Expose `MemoryCache.sweepDone` to same-package tests (already exposed since it's unexported on a same-package type — `cache_test.go` could `select` on it directly the way `client_test.go:228` does).
2. Accept the residual flake and monitor CI rates.

The current state is documented and stable enough for v0.x; flagging at Info severity for visibility, not as a defect requiring a fix.

### IN-02: Cache hit synthetic response omits standard HTTP fields

**File:** `transport_cache.go:126-133`
**Issue:** The synthetic cache-hit response sets `StatusCode`, `Status`, `Header`, `Body`, `ContentLength`, and `Request`. It does NOT set `Proto`, `ProtoMajor`, `ProtoMinor`, or `TransferEncoding`. A user-supplied `WithRequestHook` reading any of these fields on a cache-hit response gets the zero value (`""` for Proto, 0 for ProtoMajor/Minor, nil slice for TransferEncoding).

The `hookTransport.RoundTrip` IS outermost and DOES see this synthetic response (it's the whole point of D-88). Users wiring metrics (e.g., "count HTTP/2 requests") on `resp.ProtoMajor == 2` would silently misreport cache hits as HTTP/0.0.

**Fix:** Either populate the synthetic response's protocol fields from `req.Proto` / `req.ProtoMajor` / `req.ProtoMinor` (most accurate) or document the gap on the `WithRequestHook` godoc. The current `RequestHookFunc` godoc at `config.go:82-91` says "On a transport error resp is nil — implementations MUST nil-check" but does NOT mention that cache-hit responses have zero-valued protocol fields. Adding one sentence covers the contract; the alternative is to forward `req.ProtoMajor` etc. on the synthetic response. Suggested doc addition:

```
// Hooks reading cache-hit responses (CacheHitContextKey == true) must
// treat protocol-level fields (resp.Proto, resp.ProtoMajor, resp.ProtoMinor,
// resp.TransferEncoding) as unset — the synthetic response only populates
// status, headers, body, and ContentLength.
```

### IN-03: Cache hit synthetic response has empty `Header`, omits `Content-Type`

**File:** `transport_cache.go:129`
**Issue:** `Header: make(http.Header)` produces an empty header on cache-hit responses. The original 200 response would carry `Content-Type: application/json` (and possibly `Cache-Control`, `ETag`, etc.). Hooks that introspect headers see them on cache misses but not on cache hits — same family of issue as IN-02.

The downstream JSON decoder in `doJSONGet` does not check Content-Type, so functionally this is fine for the SDK's own consumers. But user hooks that key on `resp.Header.Get("Content-Type")` will misbehave on cache hits.

**Fix:** Optional. Either:

1. Store the response headers alongside the body in `cacheTransport`'s Put path (changes Cache contract from `[]byte` to a richer envelope — non-trivial).
2. Synthesize a minimal header (`resp.Header.Set("Content-Type", "application/json")`) so the common case works.
3. Document the gap explicitly.

For v0.x, option (3) (documentation) is the minimum bar. Mention in `WithRequestHook` godoc that synthetic cache-hit responses have empty headers.

### IN-04: `Cache.Get` exposes the underlying byte slice — caller mutation corrupts the cache

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

If any caller mutates the returned slice (e.g., `buf[0] = 'x'`), the cached entry is corrupted for all subsequent reads. The only in-tree consumer is `cacheTransport.RoundTrip` (line 130-131), which wraps the slice in `bytes.NewReader` (read-only) and passes it to the JSON decoder. The decoder does not write to its input. So the in-tree path is safe.

However, the `Cache` interface (`config.go:76-80`) is exported and intended for third-party implementations and consumers. A user implementing `WithCacheBackend(myCache)` may legitimately call `myCache.Get(key)` for diagnostics — the contract on returned-slice mutability is undocumented and dangerous.

**Fix:** Either:

1. Document on the `Cache` godoc that returned slices are read-only references to internal storage and MUST NOT be mutated.
2. Have `MemoryCache.Get` return a copy: `out := make([]byte, len(e.value)); copy(out, e.value); return out, true` — costs an allocation per hit but eliminates the footgun.

For an SDK targeting OSS consumers, (1) is the minimum. (2) is safer but more allocation-heavy on the hot path.

## Files Reviewed (compact summary)

| File | Lines | Issues This Pass |
|------|-------|------------------|
| `cache.go` | 240 | IN-04 |
| `cache_test.go` | 208 | IN-01 |
| `client.go` | 188 | WR-03 |
| `client_test.go` | 625 | — |
| `clock_test.go` | 149 | — |
| `config.go` | 191 | — |
| `internal_test.go` | 261 | — |
| `options.go` | 413 | — |
| `options_test.go` | 523 | — |
| `request.go` | 322 | WR-01, WR-02 |
| `retry.go` | 269 | — |
| `retry_test.go` | 646 | — |
| `transport_cache.go` | 180 | IN-02, IN-03 |
| `transport_cache_test.go` | 307 | — |
| `transport_hook.go` | 108 | — |
| `transport_hook_test.go` | 288 | — |

## Notes (not findings, observations only)

- **English-only rule (Gold Rule 1):** All identifiers, comments, godoc strings, and test names verified English. PASS.
- **Test conventions (Gold Rule 3):** `testify` + `require`/`assert` is used consistently; `t.Run` wraps every leaf case; `t.Parallel()` applied appropriately (with documented exceptions for goroutine-count tests). PASS. Demotion of `TestNewClientForTest` and `TestClient_CloseStopsSweeper` to subtests is correct per Gold Rule 3 (one TestXxx per exported production function).
- **Zero runtime dependencies:** No non-stdlib imports in production files. The addition of `hash/fnv` and `encoding/binary` and `os` in `client.go::newClientRand` are all stdlib. PASS.
- **No `init()` and no unexpected package-level vars:** `internal_test.go::TestNoInitOrGlobalState` mechanically locks this. `CacheHitContextKey` and `Version` are correctly listed in `allowedVars`. The removal of `"internal"` from `skipDirs` (`internal_test.go:108-113`) is a defensible defense-in-depth change. PASS.
- **Exported symbol godoc:** Every exported symbol verified to have a doc comment. PASS.
- **`Accept: application/json` and `User-Agent: go-openholidays/<Version>`:** Injected by `headerTransport`; not changed by Phase 4. `Version` promotion from `const` to `var` (phase 01 fix `251dc9f`) is correctly read-once by `defaultConfig()` and never mutated by library code. PASS.
- **`io.LimitReader` 10 MiB cap:** Applied in `doJSONGet` (lines 125, 156, 174), `cacheTransport` miss-path read (line 143), and `cacheTransport` drain (line 152, the CR-01 fix). PASS.
- **`slog.Default()` default + no body logging above Debug:** `loggingTransport` continues to emit only at `slog.LevelDebug` and never reads the body; the new `hookTransport` does not log at all (consumer's responsibility). PASS.
- **`-race` cleanliness:** No new global mutable state; `MemoryCache` uses `sync.RWMutex`; `fakeClock` uses `sync.Mutex`; `Client.closed` uses `atomic.Bool`. Visually race-clean (running `go test -race ./...` recommended as confirmation).
- **Context cancellation ≤ 100 ms (CLIENT-09):** The 500 ms ceiling in `TestClient_ContextCancel` is generous for CI slack but the contract is a target of 100 ms verified by microbenchmark. The `fakeClock`-based retry tests use `200 ms` ceilings which are tighter and more representative of the actual contract.
- **Strict-decoding composition (D-93):** `TestCache_StrictDecodingComposes` locks the contract that strict mode applies to cached bytes on every read (server hit once, strict gate fires twice). PASS.
- **Retry-NotARoundTripper structural audit:** `TestRetry_NotARoundTripper` scans for `type retryTransport` declarations and the absence of `transport_retry.go`. The audit reads source files line-by-line and matches `TrimLeft(line, " \t")` prefix `"type retryTransport"` — note it skips itself by filename (`retry_test.go`). The audit is sound for direct declarations; a future contributor declaring `type retryTransport struct` via a multi-line `type (\n...\n)` block would also be caught because the prefix match runs per-line.

---

_Reviewed: 2026-05-28_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
