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
  warning: 1
  info: 2
  total: 3
status: issues_found
---

# Phase 04 (Resilience): Round-4 Code Review

**Reviewed:** 2026-05-28T00:00:00Z
**Depth:** standard
**Files Reviewed:** 16
**Status:** issues_found

## Summary

Fourth-round adversarial re-review of phase-04 resilience layer (retry, cache,
hook, strict JSON, deterministic clock). All 8 round-3 fixes are mechanically
verified intact (see Round-3 Fix Verification section). One new WARNING
surfaced: a path-carrying contract gap in two context-cancellation surfaces
of `doJSONGet`. Two INFO items document minor doc/test alignment polish.

No new BLOCKER/CRITICAL issues. No regressions caused by round-3 fixes —
`Client.randMu` adds correct serialization with negligible contention (held
for a single `Int64N` call); cache defensive-copy contract is uniformly
applied on Get and Put; cache-hit synthetic responses correctly populate
the new HTTP/1.1 protocol fields and Content-Type header.

## Round-3 Fix Verification

| Fix    | Verification                                                                                                               | Status |
|--------|----------------------------------------------------------------------------------------------------------------------------|--------|
| CR-01  | `client.go:68` declares `randMu sync.Mutex`. `request.go:149-151` brackets the single `c.rand.Int64N` call inside `computeBackoff` under `c.randMu.Lock/Unlock`. `retry.go:236-241` documents the caller-serialization contract. The previously misleading "stdlib goroutine-safety" comment is replaced with a correct note about caller-supplied synchronization. | OK |
| WR-02  | `request.go:168-171` drains and closes `resp.Body` on the post-loop `httpErr != nil` branch using `io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes+1))` then `resp.Body.Close()`. Test `TestClient_FinalAttemptRespBodyDrained` (client_test.go:405) exercises the CheckRedirect-rejection shape. | OK |
| WR-03  | `request.go:99` declares `attemptsRan`; line 118 sets it inside every loop iteration; line 188 gates the retry-exhausted prefix on `attemptsRan > 1`; line 220 mirrors the same gate for the retryable-status branch. Test `TestClient_RetryExhaustedPrefix` (client_test.go:522) locks both branches. | OK |
| WR-04  | `client.go:152-154` checks `ctx.Err()` BEFORE the `d <= 0` short-circuit. Test `TestCtxSleep` (client_test.go:455) covers cancelled-ctx with d > 0, d == 0, d < 0, live-ctx with d <= 0, live-ctx with d > 0, and mid-sleep cancellation. | OK |
| IN-03  | `transport_cache.go:148-150` sets `Proto: "HTTP/1.1"`, `ProtoMajor: 1`, `ProtoMinor: 1` on the synthetic cache-hit response. Test at transport_cache_test.go:184-210 asserts all three fields. | OK |
| IN-04  | `transport_cache.go:143-144` allocates a Header map and sets `Content-Type: application/json`; line 151 attaches it to the synthetic response. Same test asserts the header value via `resp.Header.Get("Content-Type")`. | OK |
| IN-02  | `cache_test.go:144-156` documents the `runtime.NumGoroutine` design choice for `TestMemoryCache_SweeperLazyStart` (no "sweeper started" channel available; only "sweeper exited" via `sweepDone`). `cache_test.go:230-237` documents the same for `TestMemoryCache_CloseIdempotent` 100-goroutine concurrent-close test. | OK |
| IN-05  | `cache.go:160-163` `MemoryCache.Get` returns `make([]byte, len(e.value))` + `copy(out, e.value)`. `cache.go:179-181` `MemoryCache.Put` copies `value` into `stored` before storing. `config.go:62-83` Cache interface godoc documents the copy contract on both sides. Tests at cache_test.go:93-133 exercise both Get-side and Put-side caller-mutation scenarios. | OK |

All 8 fixes are correctly applied and structurally sound.

## Narrative Findings (AI reviewer)

### Warnings

### WR-01: `doJSONGet` ctx-cancellation paths bypass the WR-05 path-carrying contract

**File:** `request.go:104-107` and `request.go:152-154`
**Issue:** Two of three context-cancellation surfaces in `doJSONGet` return `ctxErr` / `sleepErr` raw, without the `"openholidays: GET %s: %w"` wrap that the WR-03 / WR-05 comment at `request.go:184-187` explicitly identifies as load-bearing: "The path is carried in BOTH branches (WR-05) so operator log routing via `strings.Contains(err.Error(), path)` is consistent regardless of whether retry was enabled or the failure happened on attempt 1."

The three ctx-cancel surfaces produce inconsistent error shapes:

1. **Loop-top ctx.Err() check** (line 105): `return zero, ctxErr` — raw `context.Canceled` / `context.DeadlineExceeded`. No `openholidays:` prefix, no path.
2. **HTTP-layer ctx cancellation** (line 156-191, via `httpErr != nil`): wrapped via `fmt.Errorf("openholidays: GET %s: %w", path, httpErr)` — path IS carried.
3. **Mid-sleep ctx cancellation** (line 152-154): `return zero, sleepErr` — raw `context.Canceled`. No prefix, no path.

Operators routing log entries by `strings.Contains(err.Error(), "/Countries")` will silently miss the loop-top and during-sleep cancellation cases. The `errors.Is(err, context.Canceled)` contract is preserved on all three paths (and the existing tests assert only that), but the WR-05 path-carrying contract is broken on two of three.

Existing tests don't catch this: `TestRetry_CtxCancel` (retry_test.go:440) and `TestClient_ContextCancel` (client_test.go:580) only assert `errors.Is(err, context.Canceled)`. The WR-03 test (client_test.go:522) does assert path-carrying, but only on the non-ctx-cancel branches.

**Fix:** Wrap both raw returns with the same prefix as the non-ctx-cancel branches, e.g.:
```go
// request.go:104-107
if ctxErr := ctx.Err(); ctxErr != nil {
    return zero, fmt.Errorf("openholidays: GET %s: %w", path, ctxErr)
}

// request.go:152-154
if sleepErr := c.sleepFunc(ctx, delay); sleepErr != nil {
    return zero, fmt.Errorf("openholidays: GET %s: %w", path, sleepErr)
}
```
Then add subtests to `TestRetry_CtxCancel` asserting `assert.Contains(t, err.Error(), "/Countries", ...)` so the contract is mechanically locked.

Note: `errors.Is(err, context.Canceled)` continues to hold under the wrap because `fmt.Errorf("...%w", ...)` preserves the unwrap chain.

### Info

### IN-01: `WithMaxRetryWait` godoc overstates "no cumulative budget" claim

**File:** `options.go:243-249`
**Issue:** The godoc says "The ceiling applies to each individual sleep, NOT the cumulative retry budget. Five attempts with a 60s cap can still take ~5 minutes total. Consumers wanting a cumulative cap supply `ctx.WithTimeout(ctx, totalBudget)` themselves — the SDK does not enforce a cumulative budget".

This is misleading in the presence of the default `WithTimeout(15*time.Second)` (config.go:118). With default settings, the per-request `context.WithTimeout` applied in `doJSONGet:64-68` IS a 15s cumulative budget covering both the HTTP attempts and the inter-attempt sleeps. So "the SDK does not enforce a cumulative budget" is false by default — only callers who supply `WithTimeout(0)` opt out.

**Fix:** Clarify the doc:
```
// The ceiling applies to each individual sleep, NOT a cumulative retry
// budget independent of the request timeout. Note that the SDK's default
// per-request timeout (15s via WithTimeout) IS itself a cumulative budget
// covering all retry attempts and inter-attempt sleeps — callers wanting
// truly unbounded retries must pass WithTimeout(0). Five attempts with
// a 60s cap and WithTimeout(0) can still take ~5 minutes total; consumers
// wanting a cumulative cap supply ctx.WithTimeout(ctx, totalBudget)
// themselves.
```

### IN-02: `TestNewMemoryCache` godoc claims coverage that lives elsewhere

**File:** `cache_test.go:53-65`
**Issue:** The TestNewMemoryCache godoc claims to cover "the constructor: returns a non-nil *MemoryCache configured with the supplied TTL, no goroutines spawned yet." But the "no goroutines spawned yet" assertion is in `TestMemoryCache_SweeperLazyStart` (cache_test.go:157), not here. The single subtest at line 58 only asserts non-nil + ttl-field-matches-argument.

This is a small doc inconsistency that future contributors might trip on when looking for the "constructed-but-unused MemoryCache spawns no goroutines" lock.

**Fix:** Trim the godoc claim to match what the test actually exercises:
```go
// TestNewMemoryCache covers the constructor: returns a non-nil *MemoryCache
// configured with the supplied TTL. The "no goroutines spawned yet"
// lazy-start invariant is locked separately in TestMemoryCache_SweeperLazyStart.
```

## Out-of-Scope Observations (NOT findings — surfacing for awareness)

These are NOT findings; they are flagged so future phases can decide whether
to address them explicitly.

1. **Cache-hit body is a defensive copy passed to `bytes.NewReader`, then drained by `doJSONGet`'s deferred `io.Copy(io.Discard, ...)`** — this is correct under the IN-05 copy contract but means every cache hit allocates `2 × len(cached)` bytes (one for the `Get` defensive copy, one because `bytes.NewReader` doesn't alias the slice into the io.Reader interface — wait, `bytes.NewReader` does alias; only `Get` copies). Net: one extra copy per Get vs. an interface that returned a `[]byte` and trusted the caller. Documented in cache.go:149-152. Out of scope per v0.x performance-not-in-scope rule.

2. **Sweeper uses real `time.NewTicker` even when `nowFn` is a fake clock** — deterministic eviction tests must rely on `Get`'s lazy-on-read path (which uses `nowFn`); the sweeper's eviction is observable only by wall-clock-waiting for a real ticker tick. Documented in cache.go:23-26 and acknowledged in TestMemoryCache_TTLEviction's design.

3. **Post-Close `Put` on a `MemoryCache` still stores entries and (uselessly) spawns a sweeper goroutine that exits immediately on the cancelled ctx** — undefined behavior per the Cache interface contract (config.go:62-83 does not specify Put/Close ordering). Not a defect; surfacing as an awareness item if v1 wants to tighten the contract.

4. **`buildAPIError` reads `resp.Body` up to `apiErrorBodyCap` (4 KiB)** — the deferred `io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes+1))` at `request.go:198` then drains anything past 4 KiB, bounded by 10 MiB. Correct, just worth noting that error responses consume up to (4 KiB + 10 MiB) of read budget total. Out of scope.

5. **`shouldRetry(nil, nil)` returns `false`** — defensive, documented at `retry.go:117-119`. The `doJSONGet` loop calls `c.http.Do` whose stdlib contract is `(resp != nil, err == nil)` OR `(resp == nil OR resp != nil, err != nil)` — never `(nil, nil)`. Defense-in-depth is fine.

---

_Reviewed: 2026-05-28T00:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
