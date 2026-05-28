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
  critical: 2
  warning: 9
  info: 0
  total: 11
status: issues_found
---

# Phase 04 (Resilience): Code Review Report

**Reviewed:** 2026-05-28
**Depth:** standard
**Files Reviewed:** 16
**Status:** issues_found

## Summary

Phase 04 lands retry/backoff, in-memory TTL cache, request hooks, strict JSON decoding, and a deterministic test clock. The implementation is generally clean, well-documented, and the test coverage is thorough.

Two **BLOCKER** defects were found:

1. `cacheTransport.RoundTrip` drains the upstream response body without a size cap when miss-path bodies overflow the LimitReader budget — a hostile/misbehaving server can stream unbounded bytes to a non-cancellable drain. Inconsistent with the analogous drain in `request.go` (which DOES cap at `maxResponseBytes+1`).
2. The same drain helper drops the upstream `*http.Response` entirely on a read error — caller loses status code, headers, and never sees a wrapped error explaining where the failure occurred.

Nine **WARNING**-level issues span correctness edge cases (shift-overflow in `computeBackoff`), API hygiene (`NewMemoryCache` accepts non-positive TTL), test-flake risk (a sweeper-leak goroutine-count test uses real sleeps), and stale godoc references ("sync.Cancel" — no such type exists).

The retry layer's pure-function helpers (`shouldRetry`, `parseRetryAfter`, `computeBackoff`) are well-tested but `computeBackoff`'s `cfg.baseDelay << attempt` can silently underflow at large attempt counts; the existing `<=0` defensive clamp masks the bug into a wrong-but-non-panicking 0-1ms sleep, which defeats the documented "cap at maxWait" guarantee.

The hook + cache + retry composition is correctly tested end-to-end. Strict-decoding correctly applies to cached bytes (D-93 lock in `TestCache_StrictDecodingComposes`).

## Critical Issues

### CR-01: Unbounded response-body drain in `cacheTransport.RoundTrip` (DoS vector)

**File:** `transport_cache.go:142-149`
**Issue:** After a successful (200) miss path, the code does:

```go
limited := io.LimitReader(resp.Body, maxResponseBytes+1)
buf, readErr := io.ReadAll(limited)
// Pitfall HTTP-3: drain any remaining bytes past the cap so the
// underlying connection can return to the keep-alive pool, then
// close. LimitReader does not advance the underlying reader past
// its cap — drain defensively.
_, _ = io.Copy(io.Discard, resp.Body)
_ = resp.Body.Close()
```

The second `io.Copy(io.Discard, resp.Body)` reads from the **unwrapped** `resp.Body` with **no upper bound**. A hostile or buggy upstream returning 50 GiB of body (after the first 10 MiB consumed by the `LimitReader`) will pin a goroutine on this drain for the duration of the transfer — denial-of-service against the calling Client. This is the exact failure `request.go:116` defends against in its retry-path drain by wrapping with `io.LimitReader(resp.Body, maxResponseBytes+1)`. The cache path's drain is the only unbounded reader in the codebase.

PROJECT.md's "response body capped at 10 MiB via `io.LimitReader`" constraint is violated here because the drain is post-LimitReader.

**Fix:**

```go
limited := io.LimitReader(resp.Body, maxResponseBytes+1)
buf, readErr := io.ReadAll(limited)
// Bounded drain: never read more than maxResponseBytes+1 past
// what we already buffered. A hostile upstream cannot pin this
// goroutine on an unbounded stream.
_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes+1))
_ = resp.Body.Close()
```

### CR-02: `cacheTransport.RoundTrip` discards `*http.Response` on body-read error

**File:** `transport_cache.go:150-152`
**Issue:**

```go
if readErr != nil {
    return nil, readErr
}
```

When `io.ReadAll(limited)` fails (network truncation mid-body, TLS reset, etc.), the helper returns `(nil, readErr)`. The original `resp` (with valid `StatusCode`, headers, and the just-closed `resp.Body`) is dropped. Two consequences:

1. The retry loop in `doJSONGet` sees `(nil, readErr)` and asks `shouldRetry(nil, readErr)` — the `nil`-response branch only retries `net.Error.Timeout()` and `syscall.ECONNRESET`. Any other body-read error (e.g. `io.ErrUnexpectedEOF` from a truncated chunked transfer) becomes a non-retryable terminal error even though the upstream did successfully reach status 200 first.
2. The error has no `"openholidays: cache:"` prefix, so consumers reading `err.Error()` cannot tell which layer surfaced the failure — diagnostics become harder.

The contract violation: `http.RoundTripper` documents that when `err != nil`, "the Response should be ignored," which is consistent — but the SDK loses the diagnostic signal that an HTTP response WAS produced.

**Fix:** Wrap the error with a layer prefix and consider whether to propagate the response (status + headers) to the caller for accurate retry semantics:

```go
if readErr != nil {
    return nil, fmt.Errorf("openholidays: cache: read response body: %w", readErr)
}
```

If retry-on-body-read-error semantics are desired, the response struct (sans Body) can also be returned with a synthetic empty body so `shouldRetry` sees the 200 status — but this changes contract and warrants a discussion.

## Warnings

### WR-01: `computeBackoff` silently produces ~0 ms sleep at high attempt counts (shift overflow)

**File:** `retry.go:248-253`
**Issue:**

```go
capped := cfg.maxWait
if exp := cfg.baseDelay << attempt; exp < capped {
    capped = exp
}
if capped <= 0 {
    capped = time.Millisecond
}
```

`time.Duration` is `int64`. With `cfg.baseDelay = 100 * time.Millisecond` (1e8 ns), `cfg.baseDelay << 33` overflows the sign bit and becomes negative. The comparison `exp < capped` (negative < positive maxWait) is true, so `capped` is overwritten with a garbage **negative** value. The `<= 0` guard then collapses to `time.Millisecond`.

Net effect: a caller using `WithRetry(50, 100*time.Millisecond)` + `WithMaxRetryWait(60*time.Second)` will, at attempts > ~33, get jitter in `[0, 1ms)` instead of the intended `[0, 60s)` cap. The retry loop then hammers the upstream at line speed — directly contradicting the documented "cap at maxWait" guarantee on `WithRetry` / `WithMaxRetryWait`, and aggravating the thundering-herd scenarios full-jitter is supposed to prevent.

Note: typical `maxAttempts` values are 3-10, so production callers are unaffected. But the bug is silent (no panic) and surfaces under stress.

**Fix:** Clamp by the cap BEFORE computing the shift, or saturate:

```go
capped := cfg.maxWait
if attempt < 63 && cfg.baseDelay > 0 {
    if exp := cfg.baseDelay << attempt; exp > 0 && exp < capped {
        capped = exp
    }
}
if capped <= 0 {
    capped = time.Millisecond
}
```

The `exp > 0` predicate rejects the negative-overflow case; the `attempt < 63` predicate is a defense-in-depth ceiling. Add a regression test in `retry_test.go::TestComputeBackoff` with `attempt=40` (or larger) asserting the result is still bounded by `maxWait`.

### WR-02: `NewMemoryCache` accepts non-positive TTL with no defense

**File:** `cache.go:101-103`
**Issue:** `NewMemoryCache(ttl)` does not validate ttl. A user calling `NewMemoryCache(0)` or `NewMemoryCache(-1*time.Hour)` constructs a cache where every Put produces an entry with `expiresAt == now` or `expiresAt < now`, so every Get on the just-stored key returns `(nil, false)`. The cache becomes silently useless and the sweeper still spawns on first Put — wasting one goroutine forever.

`WithCache(ttl)` correctly rejects `ttl <= 0` (`options.go:289-297`), but the exported constructor itself is unprotected. Custom-backend callers using `NewMemoryCache(...)` directly (e.g. via `WithCacheBackend(NewMemoryCache(myTTL))`) bypass that protection.

**Fix:** Either reject (panic in development is fine for "programmer error" per PROJECT.md, but contradicts library norms) or document the contract explicitly and treat non-positive TTL as "expire-everything-immediately" intentionally. A return-nil path is the cleanest:

```go
func NewMemoryCache(ttl time.Duration) *MemoryCache {
    if ttl <= 0 {
        // Documented: caller wanting "no caching" must not call NewMemoryCache
        // at all. We refuse to construct a useless cache.
        // Alternatively, panic with a clear message.
    }
    return newMemoryCacheWithClock(ttl, time.Now)
}
```

At minimum, update the godoc to spell out the behavior.

### WR-03: Stale godoc reference to non-existent `sync.Cancel`

**File:** `cache.go:60-67` (MemoryCache godoc)
**Issue:**

```
// Instances are constructed via NewMemoryCache (or newMemoryCacheWithClock
// inside tests) and stopped via Close. The zero value is NOT usable —
// fields are populated by the constructor; copying a MemoryCache by value
// is not supported (sync.RWMutex / sync.Once / sync.Cancel triggers the
// standard go vet copy-lock warning).
```

There is no `sync.Cancel` type in the Go standard library. The intended reference is either `context.CancelFunc` (a function, not a lock) or `sync.Once`. The `go vet` copy-lock warning is actually triggered by `sync.Mutex` / `sync.RWMutex` / `sync.Once`, not by a function value.

**Fix:** Replace `sync.Cancel` with `sync.Once` or rewrite the parenthetical:

```
// fields are populated by the constructor; copying a MemoryCache by value
// is not supported (sync.RWMutex and sync.Once trigger the standard go vet
// copy-lock warning).
```

### WR-04: Goroutine-count assertion tests have inherent flake potential

**File:** `cache_test.go:100-137`, `cache_test.go:182-207`, `client_test.go:363-384`
**Issue:** `TestMemoryCache_SweeperLazyStart`, `TestMemoryCache_CloseIdempotent` (concurrent subtest), and `TestClient_CloseStopsSweeper` use `runtime.NumGoroutine()` deltas to verify sweeper start/stop. Real timing is used (`time.Sleep(5*time.Millisecond)`, etc.). These checks are:

1. Inherently sensitive to other tests' goroutine churn (the comments acknowledge this — tests are marked non-parallel as mitigation).
2. Sensitive to Go runtime internal goroutines that fluctuate (gc workers, sweep workers, timer goroutines, especially when running with `-race`).
3. Sensitive to `httptest.Server` keep-alive pool goroutines that linger past `srv.Close` (acknowledged in `client_test.go:354-359`).

The mitigations applied (`srv.Close()` before assert, `CloseIdleConnections`, fixed sleeps) reduce flake but don't eliminate it. On a busy CI runner or under `-race`, `runtime.NumGoroutine()` can return slightly higher than `before` even when the sweeper IS gone.

PROJECT.md research-doc recommends `go.uber.org/goleak` (which would require adding a test-only dep) OR accepting flake. The current approach is the latter.

**Fix:** No code change strictly required; the design choice is documented. Recommend monitoring CI flake rates on these specific tests; if the rate climbs, propose adding `goleak` as an approved test-only dep (PROJECT.md "Any further test-only dep requires explicit user approval"). Alternatively, change the assertion from "exact goroutine count" to "the sweepDone channel is closed" by exposing an unexported `func (m *MemoryCache) sweeperStopped() <-chan struct{}` test seam.

### WR-05: `client.go::newClientRand` fallback seeds only 8 of 32 ChaCha8 bytes

**File:** `client.go:158-165`
**Issue:**

```go
func newClientRand() *rand.Rand {
    var seed [32]byte
    if _, err := crand.Read(seed[:]); err != nil {
        // Defensive fallback per CLIENT-01: NewClient must not error.
        binary.LittleEndian.PutUint64(seed[:8], uint64(time.Now().UnixNano()))
    }
    return rand.New(rand.NewChaCha8(seed))
}
```

On the `crand.Read` error path, only the first 8 bytes of `seed` are populated with `time.Now().UnixNano()`; the remaining 24 bytes stay zero. ChaCha8 will still produce deterministic output but with substantially reduced effective entropy (effectively 64 bits of state diversity, not 256). Crucially, two Clients constructed within the same nanosecond on the same machine would seed identically — defeating the fleet-wide jitter property the comment claims.

PROJECT.md's RETRY-4 mitigation depends on per-Client jitter randomness; the fallback weakens that guarantee.

Practical impact: `crand.Read` essentially never fails on a healthy OS; this is a defense-in-depth concern, not a live bug. But the comment "still per-Client unique within a nanosecond" overstates the fallback's diversity.

**Fix:** Fill more bytes of the seed (e.g., combine nanosecond timestamp + goroutine ID + pointer of a stack variable). Or use multiple independent entropy hashes:

```go
if _, err := crand.Read(seed[:]); err != nil {
    h := fnv.New128a()
    var tb [8]byte
    binary.LittleEndian.PutUint64(tb[:], uint64(time.Now().UnixNano()))
    h.Write(tb[:])
    var pb [8]byte
    // Use the address of a stack variable as an extra entropy source.
    binary.LittleEndian.PutUint64(pb[:], uint64(uintptr(unsafe.Pointer(&seed))))
    h.Write(pb[:])
    sum := h.Sum(nil)
    copy(seed[:], sum)
    copy(seed[16:], sum) // repeat to fill 32 bytes
}
```

(Or just document the fallback's reduced entropy honestly: "fallback weakens fleet-wide jitter to a 64-bit space; crypto/rand.Read failure is effectively never observed in practice.")

### WR-06: Retry exhaustion on retryable-status responses skips the "retry exhausted" prefix

**File:** `request.go:128-139` and `148-150`
**Issue:** When the retry loop exits with a final response that is a retryable status code (e.g. 503 after all `maxAttempts` attempts return 503), `httpErr == nil` and `resp.StatusCode >= 400`. The flow falls through to `buildAPIError(resp, path)` at line 149, which produces a `*APIError` WITHOUT the "openholidays: retry exhausted (N attempts)" prefix that line 136 applies on the `httpErr != nil` path.

Result: callers branching on `strings.Contains(err.Error(), "retry exhausted")` or expecting `errors.As(err, &APIError)` + a wrap-message see different behavior depending on whether the failure was a transport error or a retryable-status response. The documented contract on `WithRetry` doesn't promise either way, but the inconsistency is surprising.

**Fix:** After the retry loop, when `maxAttempts > 1` AND the final response is a retryable status, wrap the `*APIError` with the same retry-exhausted prefix:

```go
if resp.StatusCode >= 400 {
    apiErr := buildAPIError(resp, path)
    if maxAttempts > 1 && shouldRetry(resp, nil) {
        return zero, fmt.Errorf("openholidays: retry exhausted (%d attempts): %w", maxAttempts, apiErr)
    }
    return zero, apiErr
}
```

### WR-07: `request.go` retry path reuses the same `*http.Request` across attempts

**File:** `request.go:69-128`
**Issue:** `req` is built once outside the loop and reused for every `c.http.Do(req)` invocation. For GETs with no body, this is functionally safe (the stdlib client and `headerTransport`'s `req.Clone` defend against header mutation). However:

- If a future endpoint method ever uses a POST or PUT with a `req.Body`, the body will be consumed after the first attempt and subsequent attempts will send empty bodies (silent corruption).
- The `req.URL.RawQuery = q.Encode()` setup happens once; if any RoundTripper mutates `req.URL` in-flight (none currently do), subsequent attempts inherit the mutation.

Defensive practice in retrying HTTP clients is to clone the request per attempt or to use `req.GetBody` to rewind. The library currently has no body-sending endpoints, but this is a latent bug for future evolution.

**Fix:** Either document explicitly "this retry loop assumes GET-only / bodyless requests" or clone per attempt:

```go
for attempt := 0; attempt < maxAttempts; attempt++ {
    if ctxErr := ctx.Err(); ctxErr != nil {
        return zero, ctxErr
    }
    attemptReq := req.Clone(ctx) // safe to clone for every attempt
    resp, httpErr = c.http.Do(attemptReq)
    // ...
}
```

(Cost: one extra `req.Clone` per attempt — negligible for retry-loop frequencies.)

### WR-08: `transport_cache.go` `resp.Body` replaced after `bytes.NewReader(buf)` even when `buf` exceeds the cap

**File:** `transport_cache.go:158-163`
**Issue:**

```go
if len(buf) <= maxResponseBytes {
    t.cache.Put(key, buf)
}
resp.Body = io.NopCloser(bytes.NewReader(buf))
resp.ContentLength = int64(len(buf))
return resp, nil
```

When the upstream sent more than `maxResponseBytes`, `io.ReadAll(limited)` returns `maxResponseBytes+1` bytes. The cache correctly skips Put (line 158), but `resp.Body` is still replaced with the oversized buffer. The downstream decoder in `doJSONGet` will then read `maxResponseBytes` from this buffer and either:

1. Find truncated JSON (if the JSON spans the cap) → return `ErrResponseTooLarge` via the `limited.N == 0` gate — correct.
2. Find valid JSON followed by extra trailing bytes (impossible because `maxResponseBytes` is well above any realistic response) → `decoder.More()` returns true → return `ErrResponseTooLarge` — correct.

So the *behavior* is correct, but the comment at line 154-156 ("a buf longer than maxResponseBytes... indicates upstream truncation territory and MUST NOT be cached — the downstream decoder's mid-truncation gate would reject it on every read") is misleading because the over-cap buf IS still served to the downstream decoder on this single call (just not cached for future calls).

**Fix:** Either document this more precisely, or short-circuit with an oversize error from the transport layer:

```go
if len(buf) > maxResponseBytes {
    // Upstream exceeded the response cap; surface the same error
    // doJSONGet would produce, but avoid handing the oversize buf
    // to the downstream decoder.
    return nil, fmt.Errorf("openholidays: cache: response exceeded %d bytes: %w",
        maxResponseBytes, ErrResponseTooLarge)
}
t.cache.Put(key, buf)
resp.Body = io.NopCloser(bytes.NewReader(buf))
resp.ContentLength = int64(len(buf))
return resp, nil
```

The cleaner contract: the cache transport never returns an oversized response — either it caches the bytes (within cap) or it errors. This avoids subtle decoder behavior dependent on the cap.

### WR-09: `TestClient_ContextCancel` ceiling of 200 ms is fragile

**File:** `client_test.go:228-232`
**Issue:** The test asserts `elapsed < 200*time.Millisecond` for ctx-cancel to interrupt. CLIENT-09's contract is "≤ 100 ms target". A 2x slack is reasonable, but on a heavily-loaded CI runner (high GC pressure, contended goroutines) 200 ms is achievable and would manifest as a test flake. PROJECT.md does not enumerate a CI runner spec, so the slack budget is a judgment call.

A 500 ms ceiling would be safer and still well below the 30-second `WithTimeout` test setup.

**Fix:** Bump the ceiling to 500 ms or 1 second. The contract-level "≤ 100 ms" is what should be verified in a more controlled microbenchmark; an integration test under arbitrary scheduling conditions is a smoke-check.

```go
assert.Less(t, elapsed, 500*time.Millisecond,
    "ctx cancel must interrupt in-flight HTTP within 500ms (CLIENT-09 target 100ms; CI slack)")
```

## Files Reviewed (compact summary)

| File | Lines | Issue Density |
|------|-------|---------------|
| `cache.go` | 229 | WR-02, WR-03 |
| `cache_test.go` | 208 | WR-04 |
| `client.go` | 166 | WR-05 |
| `client_test.go` | 667 | WR-09 |
| `clock_test.go` | 149 | — |
| `config.go` | 179 | — |
| `internal_test.go` | 243 | — |
| `options.go` | 392 | — |
| `options_test.go` | 446 | — |
| `request.go` | 300 | WR-06, WR-07 |
| `retry.go` | 260 | WR-01 |
| `retry_test.go` | 616 | — |
| `transport_cache.go` | 165 | **CR-01**, **CR-02**, WR-08 |
| `transport_cache_test.go` | 307 | — |
| `transport_hook.go` | 108 | — |
| `transport_hook_test.go` | 288 | — |

## Notes (not findings, observations only)

- **English-only rule (Gold Rule 1):** All identifiers, comments, godoc strings, and test names verified English. PASS.
- **Test conventions (Gold Rule 3):** `testify` + `require`/`assert` is used consistently; `t.Run` wraps every leaf case; `t.Parallel()` applied appropriately (with documented exceptions for goroutine-count tests). PASS.
- **Zero runtime dependencies:** No non-stdlib imports in production files. PASS.
- **No `init()` and no unexpected package-level vars:** `internal_test.go::TestNoInitOrGlobalState` mechanically locks this. The new `CacheHitContextKey` is properly listed in `allowedVars`. PASS.
- **Exported symbol godoc:** Every exported symbol verified to have a doc comment. PASS.
- **`Accept: application/json` and `User-Agent: go-openholidays/<Version>`:** Injected by `headerTransport`; not changed by Phase 4. PASS (out of scope but verified intact).
- **`io.LimitReader` 10 MiB cap:** Applied in `doJSONGet` and `cacheTransport` miss-path read. **NOT applied in the cacheTransport drain — see CR-01.**
- **`slog.Default()` default + no body logging above Debug:** `loggingTransport` continues to emit only at `slog.LevelDebug` and never reads the body; the new `hookTransport` does not log at all (consumer's responsibility). PASS.
- **`-race` cleanliness:** No new global mutable state; `MemoryCache` uses `sync.RWMutex`; `fakeClock` uses `sync.Mutex`; `Client.closed` uses `atomic.Bool`. Visually race-clean (running `go test -race ./...` recommended as confirmation).

---

_Reviewed: 2026-05-28_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
