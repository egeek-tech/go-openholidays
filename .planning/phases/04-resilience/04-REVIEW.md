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
  warning: 0
  info: 5
  total: 5
status: issues_found
---

# Phase 04 (Resilience): Round-5 Code Review

**Reviewed:** 2026-05-28T00:00:00Z
**Depth:** standard
**Files Reviewed:** 16
**Status:** issues_found (Info-only — no Critical or Warning findings)

## Summary

Fifth-round adversarial re-review confirming all round-4 fixes (WR-01, IN-01, IN-02) are intact and structurally sound. No Critical or Warning findings remain in scope. Five Info items flag minor doc/style polish — three are stale source-line-number references in comments that no longer point to the cited code, one is a Go 1.22+ redundant loop-variable shadow flagged by CLAUDE.md, and one is a parseRetryAfter overflow edge that is benign in practice but mis-signals via its boolean return.

No new defects introduced by the round-4 path-wrapping changes. Behavior-affecting contracts (ctx-cancel path-carrying on all three surfaces; cumulative-budget documentation in `WithMaxRetryWait`; `TestNewMemoryCache` godoc scope) are mechanically verified intact below.

The codebase is functionally correct, race-clean (per the test suite design), and consistent with PROJECT.md / CLAUDE.md conventions. All Info items are documentation hygiene; none block release.

## Round-4 Fix Verification

| Round-4 Fix                              | Verification                                                                                                                                                                  | Status |
|------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|--------|
| WR-01 (loop-top ctx.Err() wrap)          | `request.go:115-117` wraps `ctxErr` via `fmt.Errorf("openholidays: GET %s: %w", path, ctxErr)`. `errors.Is(err, context.Canceled)` continues to hold via `%w`.                | OK     |
| WR-01 (mid-sleep sleepErr wrap)          | `request.go:167-169` wraps `sleepErr` via the same `fmt.Errorf("openholidays: GET %s: %w", path, sleepErr)` prefix. `errors.Is` chain preserved.                              | OK     |
| WR-01 (HTTP-layer httpErr branch)        | `request.go:203-206` continues to carry path via the canonical prefix on both the retry-exhausted (line 204) and single-attempt (line 206) sub-branches.                       | OK     |
| WR-01 test regression assertions         | `retry_test.go:473-476` (loop-top subtest) and `retry_test.go:523-526` (mid-sleep subtest) both assert `assert.Contains(err.Error(), "/Countries")` AND `"openholidays: GET"`. | OK     |
| IN-01 (WithMaxRetryWait godoc)           | `options.go:245-263` accurately describes the cumulative-budget relationship: per-attempt vs. cumulative; default `WithTimeout(15s)` acts as cumulative cap; `WithTimeout(0)` opts out; explicit example of "5 attempts × 60s cap ≈ 5 min worst case with WithTimeout(0)". No remaining misleading claims. | OK     |
| IN-02 (TestNewMemoryCache godoc)         | `cache_test.go:53-57` trims the godoc to what the test actually exercises (non-nil + ttl-matches-argument) AND cross-references `TestMemoryCache_SweeperLazyStart` for the lazy-start invariant.                                                                       | OK     |

All three round-4 fixes correctly applied. No regressions introduced.

## Narrative Findings (AI reviewer)

### Info

### IN-01: Stale comment line-number reference in `transport_cache.go:173`

**File:** `transport_cache.go:173`
**Issue:** The drain-defense comment claims "mirrors the analogous drain in request.go:116 — CR-01". After the WR-01 fix shifted lines in `request.go`, line 116 now contains the loop-top ctx-cancel return (`return zero, fmt.Errorf("openholidays: GET %s: %w", path, ctxErr)`), not a drain. The actual analogous drains in `request.go` are now at lines 144 (in-loop retry drain) and 213 (post-loop deferred drain).

Source-line references in comments are inherently fragile and drift silently. A future contributor following the cross-reference will land on unrelated code and be confused.

**Fix:** Replace the brittle line reference with a symbolic one:
```go
// (mirrors the analogous drain in request.go's retry loop / post-loop defer
//  — CR-01).
```

### IN-02: Stale comment line-number reference in `request.go:175-176`

**File:** `request.go:175-176`
**Issue:** The WR-02 comment says "The post-loop defer at line 167 only registers AFTER this block". Line 167 of `request.go` is now `if sleepErr := c.sleepFunc(ctx, delay); sleepErr != nil {` (the mid-sleep ctx-cancel check added by WR-01). The post-loop defer is at line 208.

Same drift class as IN-01. The intent of the comment (explaining why we drain manually in the httpErr branch instead of relying on the defer) remains correct, but the cited line number is misleading.

**Fix:** Replace with a symbolic reference:
```go
// reproducible via user-supplied custom RoundTrippers (e.g.
// third-party tracing middleware). The post-loop drain defer (below)
// only registers AFTER this block, so on the final-attempt
// httpErr branch the response body would leak ...
```

### IN-03: Stale comment line-number references in `client.go:196-197`

**File:** `client.go:196-197`
**Issue:** The errcheck-discard idiom comment says `see client_test.go:339, countries_test.go:94/170/254/280/316/322/350/369 (IN-01)`. Line 339 of `client_test.go` is `srv := httptest.NewServer(http.HandlerFunc(...))` — NOT an `_, _ = h.Write(...)` discard. The cited countries_test.go lines were not inspected as part of this review, but the client_test.go reference is definitely stale, so the entire batch is suspect.

**Fix:** Either remove the source-line citations entirely (the surrounding comment already explains the project-wide idiom adequately), or replace with a symbolic reference like "see the `_, _ = ...` discard idiom used throughout `*_test.go`".

### IN-04: Redundant loop-variable shadowing in `transport_cache_test.go`

**File:** `transport_cache_test.go:98` and `transport_cache_test.go:130`
**Issue:** Two loop bodies open with `c := c` (line 98) and `path := path` (line 130). Go 1.22+ scopes loop variables per iteration automatically, making these shadows redundant. CLAUDE.md "What NOT to Use" explicitly flags this:

> `tc := tc` shadow in table-driven loops (Go 1.22+ code) — Redundant since Go 1.22 scopes loop vars per iteration. Drop it; linters may flag it.

The project's `.golangci.yml` enables `gocritic` and `revive`, both of which can be configured to flag this. Even if not currently enabled, the pattern is style-non-compliant with CLAUDE.md.

**Fix:** Remove both lines:
```go
// transport_cache_test.go:97-99 — remove the c := c line
for _, c := range cases {
    t.Run(c.name, func(t *testing.T) {
        // ...

// transport_cache_test.go:129-131 — remove the path := path line
for _, path := range []string{"/PublicHolidays", "/SchoolHolidays"} {
    t.Run(path+" two calls produce two next-handler hits", func(t *testing.T) {
        // ...
```

### IN-05: `parseRetryAfter` overflow on large integer-seconds inputs returns misleading `(negative, true)`

**File:** `retry.go:200-202`
**Issue:** `parseRetryAfter` does:
```go
if s, err := strconv.Atoi(h); err == nil && s >= 0 {
    return time.Duration(s) * time.Second, true
}
```

For very large integer inputs (e.g. a 12+ digit Retry-After like `"999999999999"`), `s * 1e9` (nanoseconds per second) overflows int64. The result is `time.Duration` wrapping to a negative value, but the function returns `(negative, true)` — the boolean signals success while the duration is invalid.

In practice, this is benign: `computeBackoff` (retry.go:271) guards with `if retryAfter > 0 && retryAfter > jitter` — a negative `retryAfter` falls through to the jitter path, so the request still backs off correctly. But:
1. The signal contract of `parseRetryAfter` is violated (true should mean "I parsed a valid duration").
2. The `TestParseRetryAfter` table has no overflow case, so this isn't locked.
3. RFC 7231 §7.1.1.1 has no upper bound on delta-seconds; a hostile or buggy upstream sending `"99999999999"` is within the protocol but exploits this gap.

The threat T-04-05 documented in `retry.go:65,226` mentions `Retry-After: 999999999` (9 digits, `~31.7 years`, but `s * 1e9 = 9.99e17 < MaxInt64`) — so the documented threat is below overflow. The 12-digit case is the actual overflow boundary.

**Fix:** Add an overflow guard:
```go
if s, err := strconv.Atoi(h); err == nil && s >= 0 {
    d := time.Duration(s) * time.Second
    if d < 0 {
        // overflow — reject so the caller falls back to jitter.
        return 0, false
    }
    return d, true
}
```

Add a regression test case to `TestParseRetryAfter`:
```go
{
    name:   "12-digit integer overflow rejected (defense vs. T-04-05 extreme)",
    h:      "999999999999",
    now:    now,
    wantD:  0,
    wantOK: false,
},
```

## Out-of-Scope Observations (NOT findings — surfacing for awareness)

These are NOT findings; they are surfacing items so future phases can decide whether to address them explicitly.

1. **`transport_cache.go` drain consumes up to 2× `maxResponseBytes` from a hostile upstream.** After `io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))` (line 166), an additional `io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes+1))` runs on the same underlying `resp.Body` (line 174). LimitReaders are independent — each reads up to `maxResponseBytes+1` from the underlying stream. Total bytes pulled from the network is bounded by `2 * (maxResponseBytes+1) ~= 20 MiB`. Documentation at line 167-173 claims "bounded by maxResponseBytes+1" which is inaccurate. Security-wise this is fine (still bounded, still finite); the comment overstates the cap by 2×. Out of scope per v0.x performance-not-in-scope rule, but worth a doc tweak in a future cleanup.

2. **Double "openholidays:" prefix on cache-transport-originated errors flowing through `doJSONGet`.** When `cacheTransport.RoundTrip` returns `nil, fmt.Errorf("openholidays: cache: response exceeded ...")` (WR-08) or `nil, fmt.Errorf("openholidays: cache: read response body: %w", readErr)` (CR-02), `doJSONGet`'s post-loop httpErr branch wraps with `fmt.Errorf("openholidays: GET %s: %w", path, httpErr)`. Result: `openholidays: GET /Countries: openholidays: cache: response exceeded 10485760 bytes: openholidays response too large`. Cosmetic, not a bug. `errors.Is(err, ErrResponseTooLarge)` still works. Worth normalizing to a single prefix in a future pass.

3. **`time.NewTicker(interval)` panic potential in `MemoryCache.startSweeper`.** Currently safe because `startSweeper` clamps `interval` to `minSweeperInterval (30s)` if it would be smaller. But the clamp happens BEFORE `time.NewTicker` only because of the `if interval < minSweeperInterval` test — if a future refactor reorders these, a `ttl <= 0` cache (allowed by the godoc, behavior is "useless cache") would cause `time.NewTicker(ttl/4)` to receive a non-positive duration and panic. Defensive comment or assertion at the top of `startSweeper` would lock the invariant. Out of scope.

4. **Several `NewClient(...)` test sites do not register `t.Cleanup(func() { _ = c.Close() })`.** Examples: `client_test.go:69, 94, 106, 197, 270, 597, 605` etc. None of these clients wire `WithCache(...)`, so the missing `Close` doesn't leak goroutines. Acceptable, but inconsistent with other test sites in the same file that DO defer Close. Out of scope (no defect).

5. **`shouldRetry(nil, nil)` defensive false-return** (`retry.go:170`) is correct but unreachable under the documented `c.http.Do` contract. Defense-in-depth, no action needed.

6. **Cache transport `resp.StatusCode != http.StatusOK` short-circuit on line 159 implicitly assumes `resp != nil` when `err == nil`.** Per Go's `RoundTripper` contract this is guaranteed, but a malformed custom RoundTripper supplied via `WithHTTPClient` could violate it. A nil-resp guard inside the OR would be defense-in-depth. Out of scope (not a documented threat).

---

_Reviewed: 2026-05-28T00:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
