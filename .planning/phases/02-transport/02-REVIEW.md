---
phase: 02-transport
reviewed: 2026-05-28T00:00:00Z
depth: standard
files_reviewed: 16
files_reviewed_list:
  - client.go
  - client_test.go
  - config.go
  - countries.go
  - countries_test.go
  - errors.go
  - errors_test.go
  - internal_test.go
  - options.go
  - options_test.go
  - testdata/countries.json
  - transport.go
  - transport_header_test.go
  - transport_logging_test.go
  - validate.go
  - validate_test.go
findings:
  critical: 0
  warning: 4
  info: 5
  total: 9
status: issues_found
---

# Phase 2: Code Review Report (Re-review)

**Reviewed:** 2026-05-28
**Depth:** standard
**Files Reviewed:** 16
**Status:** issues_found

## Summary

This is the re-review of the phase 02 transport layer after phases 03/04 landed
their changes. The 2026-05-27 review surfaced 1 Critical + 5 Warning + 4 Info.
Status of those prior findings:

- **CR-01** (oversize false positive on chunked responses) — **FIXED** in
  commit `20ccdf7`. The fix moved the boundary-truncation gate to
  `decoder.More()` and the mid-truncation gate to `limited.N == 0`. Both
  gates now live in `request.go` (phase 03 D-62 / D-63); the regression
  case is locked in `countries_test.go` ("CR-01 regression: trailing
  whitespace in separate chunk is NOT oversize"). Verified by reading
  `request.go:179-209`.
- **WR-01** (dead `isTwoASCIIUppers` / `isTwoASCIILowers` helpers) — **FIXED**
  in commit `fa28137` (phase 03). `validate.go` now only ships
  `isTwoASCIILetters`. Confirmed by `grep` over `*.go`.
- **WR-02** (`WithBaseURL("/")` produces empty baseURL) — **STILL PRESENT**.
  See WR-01 below.
- **WR-03** (`internal/` in `skipDirs` defeats CLIENT-10 for future internal
  packages) — **STILL PRESENT** (`internal_test.go:94`). Phase 02 originally
  flagged this as inherited from phase 01; W-03 follow-up remains open.
- **WR-04** (goroutine-leak audit via process-wide `runtime.NumGoroutine`)
  — **PARTIALLY FIXED**. The oversize subtest now uses the per-test
  `drainCountingTransport` (`countries_test.go:50-63`) which is fully
  deterministic and replaces the prior `runtime.NumGoroutine` sample. But
  `TestClient_CloseStopsSweeper` (`client_test.go:368`) still uses
  `runtime.NumGoroutine()` deltas with a `time.Sleep(20*time.Millisecond)`
  settle and a `LessOrEqual` assertion — see WR-02 below.
- **WR-05** (Gold Rule 3 — `TestClient_ConcurrentAccess` and
  `TestClient_ContextCancel` not bound to a production function) — **STILL
  PRESENT** plus four new violations added by phases 03/04. See WR-03 below.
- **IN-01** (duplicated `defaultBaseURL` literal) — **STILL PRESENT** and
  has spread to a fifth site (`update_fixtures_test.go:192`). See IN-01 below.
- **IN-02** (validator quotes with `%q` may include escapes) — accepted as
  correct behavior; no further action needed (out of scope, was Info).
- **IN-03** (`parseAPIMessage` does not handle plain-string JSON) — **STILL
  PRESENT** but now lives in `request.go:302` (out of phase 02 scope).
  Still worth flagging at Info level.
- **IN-04** (defer drain accounting comment) — **OBSOLETE**. The
  sentinel-byte read approach is gone; the drain logic now lives in
  `request.go:151-158` with a clean LimitReader-only bound.

New issues surfaced in this re-review:

- A new **dead-state defect**: `Client.requestHook` is populated by `NewClient`
  but never read by any production code; the real consumer is `cfg.hook`
  passed to `hookTransport` in `buildTransport`. The field is pure dead
  state and is misleading to readers and the test surface. Promoted to
  Warning because it indicates a structural confusion between the
  config-side and the Client-side of the hook plumbing.
- `cfg.cacheTTL` is similarly write-only (set by `WithCache`, read nowhere
  in production) and the comment on `config.go:45` says it would be
  "consumed by composeHTTPClient in Plan 04" but Plan 04 shipped without
  consuming it.
- `errors_test.go` was not updated when phase 03 added `ErrMalformedResponse`
  — the `all` slices in `TestSentinelErrors` and `TestSentinels_ErrorsIs`
  still enumerate exactly 6 sentinels, and the godoc comment claims "6
  exported sentinels" while there are now 7. The CLIENT-10 audit
  allowlist in `internal_test.go` *was* updated; the sentinel-identity
  tests were not. Promoted to Warning because the test surface silently
  desynced from the production sentinel surface.
- `TestClient_CloseStopsSweeper` still relies on a process-wide
  `runtime.NumGoroutine()` delta with a fixed-duration sleep, which is the
  exact pattern WR-04 flagged as flaky. The test header acknowledges the
  flake risk explicitly; flagging at Warning to track follow-up.

The package as a whole continues to follow CONTEXT.md D-24..D-94 faithfully:
shallow-copy of `*http.Client`, `req.Clone` in `headerTransport`, body
invariant in `loggingTransport`, defer drain-then-close, nil-ctx guard,
ASCII-shape check before case-fold, sentinel allowlist gated by AST audit.
The new RoundTripper composition (`hookTransport` outermost, `cacheTransport`
above `loggingTransport`) is well-documented and the chain ordering is
explicitly noted as load-bearing semantics.

## Structural Findings (fallow)

No `<structural_findings>` block was provided. No structural pre-pass
substrate to merge.

## Narrative Findings (AI reviewer)

## Warnings

### WR-01: `WithBaseURL("/")` (or any all-slash input) silently produces an empty baseURL

**File:** `options.go:58-65`
**Issue:**
`WithBaseURL` short-circuits `u == ""` as a no-op but does NOT short-circuit a
string that becomes empty after `strings.TrimRight(u, "/")`. Inputs `"/"`,
`"//"`, `"///"`, etc. all canonicalize to `""` and are assigned to
`cfg.baseURL`. Subsequent calls then build a URL of `"" + "/Countries" =
"/Countries"`, which `http.NewRequestWithContext` accepts as a relative URL.
The `c.http.Do(req)` call eventually fails with an opaque error
("`unsupported protocol scheme \"\"`" or "`http: no Host in request URL`"),
far from where the misconfiguration happened.

This is the original WR-02 finding from 2026-05-27. It is still unfixed.
Existing test coverage (`options_test.go:76-89`) covers
`"https://example.test///"` (which trims to a valid URL) but not the
all-slash → empty case. A caller pulling a base URL from an environment
variable that defaults to `"/"` will silently land here.

**Fix:**
After trim, check whether the result is empty and treat that as a no-op
(same as the empty-input branch):

```go
func WithBaseURL(u string) Option {
    return func(cfg *clientConfig) {
        trimmed := strings.TrimRight(u, "/")
        if trimmed == "" {
            return // empty or all-slashes — keep the default
        }
        cfg.baseURL = trimmed
    }
}
```

Add regression cases to `TestWithBaseURL`:

```go
{name: "single slash trims to empty is no-op (default kept)", in: "/", want: "https://openholidaysapi.org"},
{name: "all-slashes trims to empty is no-op (default kept)", in: "////", want: "https://openholidaysapi.org"},
```

### WR-02: `TestClient_CloseStopsSweeper` still uses process-wide `runtime.NumGoroutine` delta — flaky pattern

**File:** `client_test.go:368-389`
**Issue:**
The test asserts `runtime.NumGoroutine() <= before` after a 20 ms settle.
`runtime.NumGoroutine` is process-wide; sibling `t.Parallel()` tests in the
same binary (and the file ships several of them — `TestCache_StrictDecodingComposes`,
`TestClient_NoCache_AllCallsHitNetwork`, `TestCache_PerClientIsolation`,
`TestHook_FiresOnRetryAttempts`, `TestHook_SeesCacheHits`,
`TestHook_DoesNotFireOnDecodeError`) can run concurrently while a non-parallel
test executes (Go's test runner serializes non-parallel tests against each
other but pauses parallel ones; the runtime, GC, and httptest worker
goroutines remain shared across the binary).

The test header (`client_test.go:362-367`) acknowledges:

> Not t.Parallel() because runtime.NumGoroutine() delta checks are
> sensitive to other tests' goroutine churn (Phase 2 D-48 / D-96).

…which is the same flake risk the prior WR-04 flagged (`runtime.NumGoroutine`
+ `time.Sleep` is the documented anti-pattern). The fixed 20 ms settle is
small; the `LessOrEqual` direction tolerates higher counts but still flakes
when the sweeper hasn't yet exited within 20 ms on a loaded CI runner.

The complementary precedent is already in the codebase:
`TestClient_Countries` ("oversize" subtest) replaced an earlier
`runtime.NumGoroutine` check with the deterministic
`drainCountingTransport` (`countries_test.go:50-63`) for exactly this
reason.

**Fix:**
Three options, increasing investment:
1. Replace the `runtime.NumGoroutine` invariant with a per-test counter on
   the sweeper goroutine itself. The cache implementation could expose a
   test-only `sweeperRunning() bool` (kept under `*_test.go` build tag) or
   the `MemoryCache.Close` could send on a done-channel the test selects on.
2. Convert the assertion from "no goroutine leak via global counter" to "the
   sweeper goroutine exited within N seconds" using `runtime.Stack` and a
   bounded poll loop — still process-global but more diagnostic.
3. Add `go.uber.org/goleak` as a test-only dep with a Key Decisions entry
   in PROJECT.md (matches the prior WR-04 recommendation; deferred there
   because of the CL row required).

Recommend (1) — the smallest deterministic seam that exercises the actual
invariant (sweeper stopped).

### WR-03: Gold Rule 3 violations — six top-level test functions not bound to an exported production function

**File:** `client_test.go` (multiple)
**Issue:**
CLAUDE.md Gold Rule 3 is non-negotiable: "Exactly one TestXxx function per
exported production function." `client_test.go` declares ten top-level test
functions; six of them are NOT named after an exported production function
on `Client`:

- `TestClient_ConcurrentAccess` — covers `Client.Countries` concurrency
  (prior WR-05; unchanged)
- `TestClient_ContextCancel` — covers `Client.Countries` cancellation
  (prior WR-05; unchanged)
- `TestNewClientForTest` — covers the unexported test helper
  `newClientForTest`. Tests of unexported helpers should live as subtests
  of the production function that exercises them, or not as a top-level
  TestXxx.
- `TestWithStrictDecoding_RejectsUnknown` and
  `TestWithStrictDecoding_DefaultLenient` — these exercise `Client.Countries`
  with a specific Option active, but the Option itself (`WithStrictDecoding`)
  already has its dedicated `TestWithStrictDecoding` in `options_test.go`.
  These two should be subtests of `TestClient_Countries`.
- `TestClient_CloseStopsSweeper` — covers `Client.Close`; should be a subtest
  of `TestClient_Close`.
- `TestCache_StrictDecodingComposes` — cross-cutting cache+decode test;
  there is no exported `Client.Cache` function it binds to.
- `TestClient_NoCache_AllCallsHitNetwork` — same; cross-cutting integration.
- `TestCache_PerClientIsolation` — same.
- `TestHook_FiresOnRetryAttempts`, `TestHook_SeesCacheHits`,
  `TestHook_DoesNotFireOnDecodeError` — cross-cutting hook integration tests.

Prior WR-05 cited two violations; phases 03/04 added six more. The trend is
the wrong direction.

**Fix:**
Two acceptable forms:
1. Rename each cross-cutting test to bind to a specific production
   function it primarily exercises. Examples:
   `TestClient_Countries_ConcurrentAccess`,
   `TestClient_Countries_ContextCancel`,
   `TestClient_Close_StopsSweeper`,
   `TestClient_Countries_StrictDecodingRejectsUnknown`. Cache and hook
   integration tests move to dedicated `cache_test.go` / `transport_hook_test.go`
   files under one TestXxx per production function (TestNewMemoryCache, etc.).
2. Demote each to a subtest of an existing TestXxx and centralize the cross-
   cutting integration coverage under the production function whose contract
   it primarily tests.

Recommend (1) for the lowest-diff path — preserves the load-bearing intent
of CONTEXT D-47/D-48 (separate top-level CI rows) while making the
production-function binding explicit in the name.

### WR-04: `Client.requestHook` field and `cfg.cacheTTL` field are write-only dead state

**File:** `client.go:57`, `client.go:100`, `config.go:45`, `options.go:295`
**Issue:**
Two struct fields are set during construction but never read by any
production code:

1. `Client.requestHook RequestHookFunc` (`client.go:57`) is assigned from
   `cfg.hook` in `NewClient` (`client.go:100`). The actual hook is invoked
   by `hookTransport` in `transport_hook.go:104`, which receives `cfg.hook`
   directly through `buildTransport` (`config.go:175`). The `Client.requestHook`
   field is never read in any production file. `grep -n "c.requestHook\|client.requestHook"`
   over production code returns zero hits; the only readers are
   `options_test.go:426/434/442`, which assert *that the field is populated*
   — a test of the field's existence, not its effect.
2. `clientConfig.cacheTTL time.Duration` (`config.go:45`) is set by `WithCache`
   (`options.go:295`). The field is never read by `composeHTTPClient`,
   `buildTransport`, or any other production function. The doc comment on
   `config.go:45` claims it is "consumed by composeHTTPClient in Plan 04" —
   Plan 04 shipped without that consumption. The `MemoryCache` carries its
   own TTL internally (`mc.ttl` on `options_test.go:335`).

Both fields are vestigial. They will fail `staticcheck` (`U1000: unused
field`) once linting is enforced. They also mislead future readers who
expect them to be the effective source of truth (the prior CR for
`TestWithRequestHook` at `options_test.go:426` even asserts on
`c.requestHook` — a green test that proves nothing about the hook's effect).

**Fix:**
1. Remove `Client.requestHook` from the struct (`client.go:57`) and from
   `NewClient`'s constructor literal (`client.go:100`). Update
   `TestWithRequestHook` in `options_test.go:413-445` to assert on the
   buildTransport-produced chain rather than on `c.requestHook`. The
   simplest end-to-end assertion is: construct a Client with
   `WithRequestHook(...)`, dispatch one HTTP call, and verify the hook
   fired exactly once — but that requires an httptest server. A lower-cost
   alternative is to assert `c.http.Transport != nil` and rely on the
   existing `TestHook_*` end-to-end tests for hook-firing coverage.
2. Remove `clientConfig.cacheTTL` from `config.go:45`, remove the
   assignment in `WithCache` (`options.go:295`), and remove the assertion
   on `cacheTTL` in `options_test.go:325` (the test asserts on `mc.ttl`,
   which is the real source of truth, so the cfg field's removal does not
   weaken coverage). Update the godoc on `WithCache` to drop the
   "cacheTTL: ..." reference if any.

## Info

### IN-01: `TestSentinelErrors` and `TestSentinels_ErrorsIs` not updated after `ErrMalformedResponse` was added

**File:** `errors_test.go:13-32` and `errors_test.go:64-78`
**Issue:**
Phase 03 added `ErrMalformedResponse` to `errors.go` (commit visible in
`errors.go:46-61` and in the CLIENT-10 allowlist at `internal_test.go:79`).
The two sentinel-identity tests in `errors_test.go` were not updated:

- `TestSentinelErrors`'s godoc comment (`errors_test.go:13`) still says "each
  of the 6 exported sentinels"; the codebase now exports 7.
- The `all` slice (`errors_test.go:25-32`) enumerates exactly 6 entries,
  missing `ErrMalformedResponse`.
- `TestSentinels_ErrorsIs` has the same defect at `errors_test.go:71-78`.

Consequences: the sentinel-identity invariants (non-nil, prefix, distinct
identity, recoverable via `%w` wrap, no identity bleed) are not enforced
for `ErrMalformedResponse`. A future bug that, say, made `ErrMalformedResponse`
alias `ErrEmptyResponse` would not be caught by either test.

**Fix:**
Add `{"ErrMalformedResponse", ErrMalformedResponse}` to both `all` slices
and update the godoc comments from "6 exported sentinels" to "7 exported
sentinels". One-line append per test plus one comment edit.

### IN-02: `"https://openholidaysapi.org"` literal duplicated across five files

**File:** `config.go:99`, `client_test.go:62`, `options_test.go:80`,
`update_fixtures_test.go:192`, plus the doc comment in `doc.go:2`
**Issue:**
The prior IN-01 finding (three sites) has grown to five sites. When the
upstream changes URL (mirror, schema versioning), all five need updating
in lockstep. The `config.go:46-48` rationale ("an extracted const buys
nothing but the indirection cost") ignores the four test sites that
re-encode the constant — those are not indirection cost, they are
maintenance debt.

**Fix:**
Extract an unexported package-level const `defaultBaseURL =
"https://openholidaysapi.org"` in `config.go`. Constants are NOT in scope
of the CLIENT-10 AST audit (the audit filters on `ast.GenDecl.Tok ==
token.VAR`), so no allowlist update is required. Update the five sites to
reference the const. `doc.go` is a comment-only reference; leave as plain
text.

### IN-03: `WithMaxRetryWait(0)` documentation: "does NOT disable" is a Pitfall-quality footgun, not a defect

**File:** `options.go:218-220`
**Issue:**
The godoc says: *"A non-positive duration falls back to defaultMaxRetryWait
(60s) per D-74 — calling WithMaxRetryWait(0) does NOT disable the cap."*
This is internally consistent (test coverage at
`options_test.go:303-318` locks the behavior), but the asymmetry with
`WithTimeout(0)` (which means "disable timeout") and `WithRetry(0, ...)`
(which means "disable retry") is a footgun. A caller composing
`WithRetry(3, 100ms)` and `WithMaxRetryWait(0)` reasonably expects "no
per-attempt cap", but instead gets the 60s default — silently.

**Fix:**
Not required behaviorally. Either:
1. Add an explicit godoc warning: *"Note: this is asymmetric with
   WithTimeout(0) and WithRetry(0, _) — see Pitfall RETRY-X in
   RESEARCH.md."*
2. Accept the asymmetry as the documented semantics and move on.

Recommend (1) — one-line doc edit.

### IN-04: `parseAPIMessage` does not handle plain-string JSON error bodies

**File:** `request.go:302-321` (moved here from phase 02's `countries.go`)
**Issue:**
`parseAPIMessage` calls `json.Unmarshal(body, &env)` where `env` is a struct
of three string fields. If upstream returns a JSON scalar (e.g.,
`"plain error string"`) or a JSON array, `Unmarshal` returns an error and
`parseAPIMessage` returns `""`. `APIError.Message` ends up empty even though
the body had a meaningful textual error.

The docstring says "best-effort"; this is acceptable, but a single fallback
path "if Unmarshal-into-struct fails, try unmarshal-into-string" would
cover a common upstream variant for negligible cost.

This is the prior IN-03 finding, relocated to `request.go`. Listed as Info
because the live API verified at 2026-05-27 uses RFC 7807 ProblemDetails
(object shape).

**Fix:**
Optional improvement:
```go
// Final fallback: upstream returned a plain JSON string.
var plain string
if err := json.Unmarshal(body, &plain); err == nil {
    return plain
}
return ""
```
Note: this finding is technically outside phase 02 scope (the file is
`request.go`, introduced in phase 03), but the prior review tracked it
under phase 02 because that is where the helper originally lived. Carry-over.

### IN-05: `internal_test.go` skipDirs still hardcodes `"internal"` — pre-emptively defeats CLIENT-10 for any future internal/ package

**File:** `internal_test.go:84-95`
**Issue:**
This is the prior WR-03 finding, downgraded to Info because no `internal/`
package exists yet. The audit will silently skip any future `internal/` tree.

A future contributor adding `internal/cache/global.go` with
`var globalCache = ...` would slip past the CLIENT-10 invariant. CONTEXT.md
D-34 deferred this fix to phase 03/04, both of which have now shipped
without touching it.

**Fix:**
Either remove `"internal"` from `skipDirs` now (empty cost, restores the
invariant the day an `internal/` package lands), or leave a TODO with a
tracked issue ID pointing at the v0.1.0 cleanup milestone.

Recommend removal — the audit will only fail when an `internal/` package
adds a non-allowlisted var, which is exactly the moment the maintainer
should review.

---

_Reviewed: 2026-05-28_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
