---
phase: 04-resilience
plan: 03
subsystem: retry-layer
tags: [retry, exponential-backoff, full-jitter, retry-after, ctx-cancel, fake-clock, D-73, D-74, D-75, D-76, D-77, D-78, RESIL-05, TEST-05]
dependency_graph:
  requires:
    - 04-01 (fakeClock helper from clock_test.go — drives retry sleeps deterministically)
    - 04-02 (Client.retry / Client.rand / Client.nowFunc / Client.sleepFunc fields, retryConfig stub, newClientForTest seam)
    - Phase 2 doJSONGet pipeline (request.go — the retry loop wraps c.http.Do here)
  provides:
    - shouldRetry, parseRetryAfter, computeBackoff (retry.go — pure helpers; consumed by doJSONGet retry loop)
    - retryConfig (filled struct — moved from config.go to retry.go)
    - defaultBaseDelay (250ms) + defaultMaxRetryWait (60s) named consts
    - WithRetry(maxAttempts, baseDelay) + WithMaxRetryWait(d) exported options
    - retry-wrapped doJSONGet (request.go — the actual loop body)
  affects:
    - request.go (c.http.Do(req) is now inside a for-loop with shouldRetry/computeBackoff/c.sleepFunc orchestration)
    - config.go (retryConfig moved to retry.go; file-level godoc updated)
tech-stack:
  added:
    - math/rand/v2 (retry.go — used by computeBackoff for full-jitter via the per-Client *rand.Rand from D-78)
    - net (retry.go — net.Error interface assertion via errors.As inside shouldRetry)
    - syscall (retry.go — syscall.ECONNRESET sentinel for conn-reset detection)
    - context (retry.go — added during deviation fix; errors.Is against context.Canceled / context.DeadlineExceeded inside shouldRetry)
  patterns:
    - Pure helper functions over (resp, err) and (h, now) — no Client dependency, trivially unit-testable
    - AWS canonical full-jitter exponential backoff (jitter = rand.Int64N(capped); capped = min(baseDelay << attempt, maxWait))
    - Retry-After promotion with hostile-cap (min(retryAfter, maxWait) guards threat T-04-05)
    - Ctx-aware retry loop (ctx.Err() at loop top + ctx-aware c.sleepFunc — Pitfall RETRY-3 / RESIL-04)
    - Endpoint-layer retry (NOT a RoundTripper — RESIL-05; structurally audited via TestRetry_NotARoundTripper)
key-files:
  created:
    - retry.go
    - retry_test.go
  modified:
    - config.go
    - options.go
    - options_test.go
    - request.go
decisions:
  - D-73 wired (public retry surface ships exactly WithRetry + WithMaxRetryWait — no RetryConfig struct, no per-status overrides)
  - D-74 wired (maxAttempts <= 0 disabled; baseDelay <= 0 → defaultBaseDelay 250ms; maxWait <= 0 → defaultMaxRetryWait 60s; WithRetry seeds maxWait when unset)
  - D-75 wired (retryable matrix: 408/429/500/502/503/504 + net.Error.Timeout() + syscall.ECONNRESET; ctx errors NEVER retried — guard added inside shouldRetry)
  - D-76 wired (parseRetryAfter accepts integer seconds + http.ParseTime; computeBackoff promotes retryAfter > jitter capped at maxWait; past-dated Retry-After rejected per Pitfall 9)
  - D-77 wired (retry loop in doJSONGet, NOT a RoundTripper; retry exhaustion wraps lastErr via fmt.Errorf %w so errors.Is/errors.As still match)
  - D-78 consumed (per-Client *rand.Rand from newClientRand drives computeBackoff jitter — prevents fleet-wide thundering herd per Pitfall RETRY-4)
metrics:
  duration: ~50m (one execution wave)
  completed: 2026-05-28
requirements_complete:
  - RESIL-01 (full-jitter exponential backoff via math/rand/v2 — computeBackoff in retry.go)
  - RESIL-02 (retryable-conditions matrix — shouldRetry in retry.go)
  - RESIL-03 (Retry-After honoring — parseRetryAfter + computeBackoff promotion)
  - RESIL-04 (≤ 100ms ctx cancellation through retry loop — TestRetry_CtxCancel locks both before-call + mid-sleep paths)
  - RESIL-05 (retry in endpoint layer, NOT a RoundTripper — TestRetry_NotARoundTripper structural audit)
  - TEST-05 (deterministic retry/backoff tests via fakeClock — TestRetry_E2E_429Then500Then200 + TestRetry_DeterministicClock + TestRetry_HonorsRetryAfter*)
---

# Phase 04 Plan 03: Retry Layer Summary

**One-liner:** A pure-helper retry layer (`shouldRetry`, `parseRetryAfter`, `computeBackoff`) drives a ctx-aware `for attempt := 0; attempt < maxAttempts; attempt++` loop inside `doJSONGet` per D-77 + RESIL-05 — retry is in the endpoint layer, NOT a RoundTripper, so caller-supplied `*http.Client` retry middleware does NOT double-fire.

## What Shipped

### `retry.go` (new file, 245 lines)

```go
// Pure helpers (no Client dependency, no HTTP dispatch).
func shouldRetry(resp *http.Response, err error) bool                         // D-75 matrix
func parseRetryAfter(h string, now time.Time) (time.Duration, bool)           // D-76 RFC 7231 §7.1.1.1
func computeBackoff(attempt int, retryAfter time.Duration,
                    cfg retryConfig, rnd *rand.Rand) time.Duration             // D-76 full-jitter

// Filled struct (moved from config.go).
type retryConfig struct {
    maxAttempts int
    baseDelay   time.Duration
    maxWait     time.Duration
}

// Named constants at top of file (D-74 defaults).
const defaultBaseDelay     = 250 * time.Millisecond
const defaultMaxRetryWait  = 60 * time.Second
```

`shouldRetry` matrix verbatim per D-75:

| Input | Result |
|-------|--------|
| `context.Canceled` (errors.Is) | `false` — D-75: NEVER retried |
| `context.DeadlineExceeded` (errors.Is) | `false` — D-75: NEVER retried (also satisfies `net.Error.Timeout()==true`; the ctx-error guard runs FIRST) |
| `net.Error` with `Timeout()==true` | `true` |
| Errors wrapping `syscall.ECONNRESET` (errors.Is) | `true` |
| Any other non-nil error | `false` |
| `*http.Response{StatusCode: 408/429/500/502/503/504}` | `true` |
| Any other status (200/201/400/401/403/404/...) | `false` |
| Both `resp == nil` and `err == nil` | `false` (defensive) |

`parseRetryAfter` accepts integer-seconds via `strconv.Atoi` and RFC 7231 HTTP-date (RFC 1123 + RFC 850 + ANSI C asctime) via `http.ParseTime`. Past-dated HTTP-dates are rejected (Pitfall 9 / threat T-04-06) so backoff never collapses to zero.

`computeBackoff` is the AWS canonical full-jitter formula: `capped := min(baseDelay << attempt, maxWait)`, `jitter := rand.Int64N(capped)` cast to `time.Duration`. When `retryAfter > jitter`, retryAfter wins, but is always capped at `maxWait` (threat T-04-05 guard against `Retry-After: 999999999`). Defensive against a zero `baseDelay` — `capped` forced to ≥ 1 ns so `rand.Int64N` never panics.

### `request.go` retry loop (lines 75-141 — replaces the single `c.http.Do(req)` call)

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
    if ctxErr := ctx.Err(); ctxErr != nil {
        return zero, ctxErr // Pitfall RETRY-3 + D-75
    }
    resp, httpErr = c.http.Do(req)
    if !shouldRetry(resp, httpErr) {
        break
    }
    if attempt == maxAttempts-1 {
        break // last attempt — surface error without sleeping
    }
    var retryAfter time.Duration
    if resp != nil {
        retryAfter, _ = parseRetryAfter(resp.Header.Get("Retry-After"), c.nowFunc())
        // Drain+close mid-loop response per Pitfall HTTP-3 + nil-out resp
        // so the post-loop deferred drain doesn't double-close.
        _, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes+1))
        _ = resp.Body.Close()
        resp = nil
    }
    delay := computeBackoff(attempt, retryAfter, c.retry, c.rand)
    if sleepErr := c.sleepFunc(ctx, delay); sleepErr != nil {
        return zero, sleepErr // ctx.Err() during sleep
    }
}
if httpErr != nil {
    if maxAttempts > 1 {
        return zero, fmt.Errorf("openholidays: retry exhausted (%d attempts): %w", maxAttempts, httpErr)
    }
    return zero, fmt.Errorf("openholidays: GET %s: %w", path, httpErr)
}
// existing defer drain-and-close + 4xx/5xx + decode runs once after the loop
```

Notable invariants:

- The pre-existing deferred drain-and-close fires exactly once on the FINAL response — mid-loop bodies are drained+closed inline, then `resp` is nil'd to prevent the deferred drain from double-closing.
- Retry-exhaustion error wraps `lastErr` via `%w` so callers' `errors.Is(err, ErrEmptyResponse)` / `errors.As(err, &apiErr)` still match (D-77 + Phase 1 D-23).
- When `maxAttempts == 1` (retry disabled — the Phase 2 baseline), the error wrap matches the existing Phase 2 format verbatim: `fmt.Errorf("openholidays: GET %s: %w", path, httpErr)`.

### `options.go` (new exported options — appended after `WithStrictDecoding`)

```go
func WithRetry(maxAttempts int, baseDelay time.Duration) Option
func WithMaxRetryWait(d time.Duration) Option
```

Both options carry exhaustive godoc citing D-73/D-74/D-75/D-76/D-77, RESIL-05, Pitfall RETRY-1/RETRY-2/RETRY-3 + threats T-04-05/T-04-06. Defaults applied per D-74:

| Option call | Outcome |
|-------------|---------|
| `WithRetry(5, 100*time.Millisecond)` | `maxAttempts=5; baseDelay=100ms; maxWait=60s (seeded)` |
| `WithRetry(0, 100*time.Millisecond)` | `maxAttempts=0` → DISABLED (verbatim) |
| `WithRetry(3, 0)` | `maxAttempts=3; baseDelay=250ms` (defaultBaseDelay fallback) |
| `WithRetry(3, -1s)` | `maxAttempts=3; baseDelay=250ms` (defaultBaseDelay fallback) |
| `WithMaxRetryWait(10*time.Second)` | `maxWait=10s` |
| `WithMaxRetryWait(0)` | `maxWait=60s` (defaultMaxRetryWait fallback — does NOT disable cap) |
| `WithMaxRetryWait(-1s)` | `maxWait=60s` (defaultMaxRetryWait fallback) |
| `WithMaxRetryWait(5s)` then `WithRetry(3, 100ms)` | `maxWait=5s` (WithRetry preserves caller-supplied maxWait) |
| `WithRetry(3, 100ms)` then `WithMaxRetryWait(5s)` | `maxWait=5s` (last-wins) |

### `config.go` edits

- `retryConfig` type declaration moved from config.go to retry.go (all retry types colocated; file-level godoc updated to reflect the move).
- `clientConfig.retry retryConfig` field unchanged — the type alias is preserved by Go's package-level scope.

### `options_test.go` (new tests appended)

- **`TestWithRetry`** (7 subtests) — positive verbatim; `maxAttempts=0` stays disabled; `baseDelay<=0` falls back; negative baseDelay falls back; default Client disabled; last-wins on maxWait (both orderings).
- **`TestWithMaxRetryWait`** (3 subtests) — positive verbatim; zero fallback; negative fallback.

### `retry_test.go` (new file, 615 lines, 10 TestXxx functions)

| Test function | Subtests | What it locks |
|---------------|----------|---------------|
| `TestShouldRetry` | 19 (table-driven) | D-75 retryable-conditions matrix verbatim — every status + transport-error + ctx-error branch |
| `TestParseRetryAfter` | 6 | D-76 + Pitfall 9: integer seconds, RFC 1123 future date, RFC 1123 past date (rejected), ANSI C asctime, empty, garbage |
| `TestComputeBackoff` | 3 | Full-jitter ranges at attempt 0 / 3 / 10 (exponential ceiling clamped at maxWait) |
| `TestComputeBackoff_HonorsRetryAfter` | 2 | retryAfter > jitter promotes; retryAfter > maxWait is capped (threat T-04-05) |
| `TestRetry_E2E_429Then500Then200` | 1 | Marquee TEST-05: exactly 3 round trips on 429 → 500 → 200; payload decoded |
| `TestRetry_HonorsRetryAfterSeconds` | 1 | Integer-seconds Retry-After produces ≥ 2s of fake-clock advance |
| `TestRetry_HonorsRetryAfterDate` | 1 | HTTP-date Retry-After produces ≥ 30s of fake-clock advance |
| `TestRetry_CtxCancel` | 2 | Before-call + mid-sleep ctx cancel each return `errors.Is(err, context.Canceled)` within ≤ 200 ms (CI slack on the 100 ms target) |
| `TestRetry_NeverRetriesCtxErrors` | 2 | Direct pure-function lock: `shouldRetry(nil, context.Canceled/DeadlineExceeded) == false` |
| `TestRetry_DeterministicClock` | 1 | 3×503 → 200 produces ≤ 700 ms fake-clock advance with baseDelay=100ms (full-jitter ceiling) |
| `TestRetry_NotARoundTripper` | 2 | RESIL-05 structural audit: no `transport_retry.go` file; no `type retryTransport` declaration in production code |

All tests follow Gold Rule 3 etiquette: `t.Parallel()` at outer level and every leaf subtest; table-driven where ≥ 2 cases share setup; `require` for preconditions, `assert` for verifications; one TestXxx per concern. `roundTripperFunc` is NOT redeclared — same-package visibility carries it from `transport_header_test.go:19`.

A test-only `fakeNetErr` struct implements `net.Error` for the `shouldRetry` net.Error branch. A test-only `newTestRand()` builds a deterministic `*rand.Rand` (fixed 32-byte ChaCha8 seed) so range assertions in `TestComputeBackoff` are stable run-to-run.

## Verification Output

```text
go build ./...                                            -> exit 0
go vet ./...                                              -> exit 0
gofmt -l retry.go retry_test.go options.go options_test.go request.go config.go
                                                          -> (no output — clean)
go test -race -count=1 ./...                              -> ok 1.815s
go test -race -count=1 -run TestNoInitOrGlobalState ./... -> ok 1.025s
go test -race -count=1 -run 'TestShouldRetry|TestParseRetryAfter|TestComputeBackoff|TestRetry_E2E|TestRetry_HonorsRetryAfter|TestRetry_CtxCancel|TestRetry_NeverRetriesCtxErrors|TestRetry_DeterministicClock|TestRetry_NotARoundTripper' ./...
                                                          -> ok 1.026s
find . -maxdepth 2 -name 'transport_retry.go'             -> (no output — RESIL-05 confirmed)
grep -RIn --include='*.go' 'type retryTransport' .        -> only retry_test.go (audit code itself)
```

Go toolchain: `go1.26.3-X:nodwarf5 linux/amd64`.

## Commits

| Commit | Subject | Files |
|--------|---------|-------|
| `bfafe40` | feat(04-03): add retry.go helpers (shouldRetry, parseRetryAfter, computeBackoff) + retryConfig | retry.go (+245), config.go (-13 net) |
| `f20055b` | feat(04-03): add WithRetry + WithMaxRetryWait options with D-74 defaults | options.go (+105), options_test.go (+117) |
| `964ce61` | feat(04-03): wrap c.http.Do in retry loop inside doJSONGet + comprehensive retry_test.go | request.go (+71, -9), retry.go (+25 ctx-error guard), retry_test.go (+613) |

## Net Lines Modified (production + tests)

| File | Insertions | Deletions |
|------|-----------:|----------:|
| `retry.go` (new) | 270 | 0 |
| `retry_test.go` (new) | 613 | 0 |
| `options.go` | 105 | 0 |
| `options_test.go` | 117 | 0 |
| `request.go` | 71 | 9 |
| `config.go` | 5 | 13 |
| **Total** | **1181** | **22** |

## Deviations from Plan

### [Rule 1 — Bug] `shouldRetry` ctx-error guard added inside the helper

**Found during:** Task 3 first run of `TestShouldRetry` — case `context.DeadlineExceeded not retryable` failed with the original implementation (the net.Error branch claimed DeadlineExceeded as retryable because it satisfies `net.Error` with `Timeout() == true`).

**Root cause:** Go's stdlib makes `context.DeadlineExceeded` implement `net.Error`:
```go
var ne net.Error
errors.As(context.DeadlineExceeded, &ne) // → true; ne.Timeout() → true
errors.As(context.Canceled, &ne)         // → false
```
The original `shouldRetry` ordered the `net.Error.Timeout()` check before any explicit ctx-error guard, which directly violated D-75 ("ctx errors NEVER retried"). The plan's godoc anticipated this (the "Critical: defensive false return" note) but the implementation relied on the retry loop's loop-top `ctx.Err()` check, which the pure-function test bypasses.

**Fix:** Added an explicit ctx-error guard at the top of `shouldRetry`:
```go
if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
    return false
}
```
This makes the pure function correct in isolation — matching the plan's explicit behavior contract — and is strictly more defensive than the original code (the retry loop's ctx.Err() check still fires too, just earlier).

**Files modified:** `retry.go` only (added `context` import; added the two-line guard at the top of `shouldRetry`'s err-branch). Documented in godoc with the rationale.

**Why Rule 1 (not Rule 4):** No architectural change. Same function signature, same matrix, same callers. The change makes the existing contract correct under a stdlib behavior that the plan documented but the code didn't enforce.

### [Plan note → preserved as-is] Mid-loop `resp = nil` after drain-and-close

The plan's pseudocode in `<action>` Task 3 step 3 sub-bullet says: "If resp != nil: parse Retry-After; drain+close per Pitfall HTTP-3." It does NOT explicitly say "then nil-out resp". The implementation nil's out `resp` after drain-and-close so the post-loop deferred drain-and-close (which runs on the FINAL response) cannot double-close a body already returned to the pool. This is a defensive correctness invariant — drain+close + nil-out is the safe pattern; the plan's pseudocode implies it via the surrounding "decode runs once" comment but I'm calling it out explicitly here.

**Why this is NOT a deviation:** Same observable behavior. The plan's intent ("decode runs once after the loop succeeds") is mechanically preserved.

No auth gates. No checkpoints triggered.

## Authentication Gates

None encountered — retry tests use `httptest.NewServer` exclusively. No live network IO; no secrets.

## Known Stubs

None for this plan's deliverables. The retry layer is fully wired and tested end-to-end.

The `Client.cache Cache` and `Client.requestHook RequestHookFunc` fields remain stubs from Plan 02 — those are Plan 04 (cache) and Plan 05 (hook) deliverables and are explicitly out of scope here.

## Threat Surface Scan

This plan introduces the retry loop and helpers that mitigate the four retry-class threats declared in PLAN.md `<threat_model>`:

| Threat ID | Mitigation in this plan |
|-----------|-------------------------|
| T-04-05 (hostile `Retry-After: 999999999`) | `computeBackoff` applies `min(retryAfter, cfg.maxWait)` — verified by `TestComputeBackoff_HonorsRetryAfter/retryAfter_>_maxWait_capped_at_maxWait` |
| T-04-06 (hostile past-dated `Retry-After`) | `parseRetryAfter` guards `d := t.Sub(now); d > 0` — verified by `TestParseRetryAfter/RFC_1123_past_date_rejected` |
| T-04-07 (thundering herd via correlated jitter) | per-Client `*rand.Rand` from `newClientRand` (Plan 02) consumed by `computeBackoff`; cross-instance independence guaranteed by `crypto/rand`-seeded ChaCha8 |
| T-04-08 (retry loop ignores ctx cancellation) | loop-top `ctx.Err()` check + ctx-aware `c.sleepFunc` — verified by `TestRetry_CtxCancel` (both before-call and mid-sleep paths) |
| T-04-09 (retry double-fire when caller's `*http.Client` retries) | retry in `doJSONGet`, NOT a RoundTripper — verified by `TestRetry_NotARoundTripper` structural audit (no `transport_retry.go` file; no `retryTransport` struct in production code) |

No new threat surface beyond what the plan's `<threat_model>` already identifies. No new network endpoints, no auth paths, no file access patterns beyond the existing `httptest.NewServer` paths shared with Phase 1-3.

## Deferred Issues

### `countries_test.go` trailing-blank-line gofmt warning (pre-existing — out of scope)

`gofmt -l countries_test.go` reports a single trailing blank line at EOF that originates from commit `9730014` (Phase 3 Plan 01 — `refactor(03-01): retrofit Countries to (ctx, CountriesRequest) via doJSONGet`). This is NOT introduced by this plan and falls outside the SCOPE BOUNDARY (only auto-fix issues directly caused by the current task's changes). Logged here so a future plan touching `countries_test.go` (e.g., the Plan 04 cache integration tests) can fix it in passing.

The full Phase 4 plan-03 file set (`retry.go`, `retry_test.go`, `options.go`, `options_test.go`, `request.go`, `config.go`) is `gofmt`-clean.

## Self-Check: PASSED

- Created files:
  - `retry.go` at repo root: **FOUND** (270 lines).
  - `retry_test.go` at repo root: **FOUND** (613 lines).
- Modified files:
  - `config.go`: **FOUND** (retryConfig declaration removed; godoc updated).
  - `options.go`: **FOUND** (WithRetry + WithMaxRetryWait appended).
  - `options_test.go`: **FOUND** (TestWithRetry + TestWithMaxRetryWait appended).
  - `request.go`: **FOUND** (retry loop wrapping `c.http.Do(req)`; `time` import added).
- Commits exist:
  - `bfafe40`: **FOUND** in `git log`.
  - `f20055b`: **FOUND** in `git log`.
  - `964ce61`: **FOUND** in `git log`.
- Verification:
  - `go build ./...` exits 0.
  - `go vet ./...` exits 0.
  - `go test -race -count=1 ./...` exits 0 (full suite green; 1.815s wall).
  - `go test -race -count=1 -run TestNoInitOrGlobalState ./...` exits 0 (CLIENT-10 AST audit unchanged — only consts added, no new `var`).
  - All 10 retry tests pass under `-race`.
  - `find . -maxdepth 2 -name 'transport_retry.go'` produces no output (RESIL-05 confirmed).
  - `grep -RIn --include='*.go' 'type retryTransport' .` finds only retry_test.go (the audit code itself).
