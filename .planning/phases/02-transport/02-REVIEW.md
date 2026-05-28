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
  warning: 1
  info: 3
  total: 4
status: issues_found
---

# Phase 2: Code Review Report (Re-review #2)

**Reviewed:** 2026-05-28
**Depth:** standard
**Files Reviewed:** 16
**Status:** issues_found

## Summary

This is the second re-review of phase 02 after the `--fix --all` pass landed
9 commits (`e4a4f0e`..`2f60b12`) addressing the 2026-05-28 review (0 Critical
+ 4 Warning + 5 Info).

### Re-verification of prior findings (all 9 hold)

| Prior ID | Subject | Status | Evidence |
| --- | --- | --- | --- |
| WR-01 | `WithBaseURL("/")` collapses to default | FIXED | `options.go:67-77` — second `trimmed == ""` no-op branch added; regression cases at `options_test.go:88-89` cover `"/"` and `"////"`. |
| WR-04 | Dead `Client.requestHook` + `cfg.cacheTTL` | FIXED | `client.go:52-66` no longer declares `requestHook`; `config.go:49-60` no longer declares `cacheTTL`. `TestWithRequestHook` (`options_test.go:489-522`) now asserts on `c.http.Transport.(*hookTransport)` instead of the removed field. `TestWithCache` (`options_test.go:387-417`) asserts on `mc.ttl`. |
| IN-01 | `ErrMalformedResponse` missing from sentinel-identity tests | FIXED | `errors_test.go:34` and `:81` include `{"ErrMalformedResponse", ErrMalformedResponse}`; godoc at `:12` updated to "7 exported sentinels". |
| IN-02 | `defaultBaseURL` const | FIXED | `config.go:44` declares the const; `client_test.go:69`, `options_test.go:82,88,89` reference it. |
| IN-03 | `WithMaxRetryWait` asymmetry doc | FIXED | `options.go:235-242` documents the asymmetry with `WithTimeout(0)` / `WithRetry(0,_)` explicitly. |
| IN-05 | `internal/` removed from skipDirs | FIXED | `internal_test.go:108-113` carries only `.planning`, `.git`, `.claude`, `testdata`. Comment block at `:103-107` records the rationale. |
| WR-03 part 1 | `TestWithStrictDecoding_*` siblings demoted | FIXED | `options_test.go:224,247` are subtests inside `TestWithStrictDecoding`. |
| WR-03 part 2 | `TestNewClientForTest` demoted | FIXED | `client_test.go:109,122,132` are subtests inside `TestNewClient` named "newClientForTest seam: …". |
| WR-02 + WR-03 part 3 | `TestClient_CloseStopsSweeper` deterministic | FIXED | `client_test.go:202-234` is a subtest of `TestClient_Close`; the `runtime.NumGoroutine()` + `time.Sleep(20ms)` pattern is gone, replaced by a select on the same-package-visible `*MemoryCache.sweepDone` channel with a 1s bounded wait. |

### Remaining known scope-deferred items

The prior review's WR-03 listed 8 sibling-test violations in `client_test.go`.
Three were fixed (the demotions above). Five cache/hook integration tests
(`TestCache_StrictDecodingComposes`, `TestClient_NoCache_AllCallsHitNetwork`,
`TestCache_PerClientIsolation`, `TestHook_FiresOnRetryAttempts`,
`TestHook_SeesCacheHits`, `TestHook_DoesNotFireOnDecodeError`) are
cross-cutting fixtures whose primary production targets live in phase 04
files (`cache.go`, `transport_cache.go`, `transport_hook.go`) — they are
explicitly out of phase 02 scope per the review brief and not re-flagged
here. Two pre-existing violations (`TestClient_ConcurrentAccess` and
`TestClient_ContextCancel`) likewise remain as known follow-up; flagged in
the prior review's WR-03 and tracked there, not duplicated here.

### New issues surfaced in this re-review

One previously-missed instance of the WR-04 dead-state pattern: `Client.userAgent`
and `Client.logger` are populated by `NewClient` but never read by any
production code path. The actual consumers are `headerTransport.userAgent`
(`transport.go:66`) and `loggingTransport.logger` (`transport.go:111`), each
populated directly from `cfg.userAgent` / `cfg.logger` inside `buildTransport`
(`config.go:170-171`). The Client-side fields exist only so tests can
construction-time assert on them — the same vestigial pattern that WR-04
removed for `Client.requestHook`. Promoted to Warning by parity with the
prior finding's classification.

Two minor quality items: an ignored `(int, error)` return on `hash.Hash.Write`
calls in the `newClientRand` fallback (would trip `errcheck` once
`.golangci.yml` is restored — currently only a `_backup` file exists), and a
few vacuous `require.NotNil` calls on freshly-constructed values in
`errors_test.go`. Both Info.

The package as a whole continues to follow CONTEXT.md D-24..D-94 faithfully:
shallow-copy of `*http.Client`, `req.Clone` in `headerTransport`, body
invariant in `loggingTransport`, defer drain-then-close, nil-ctx guard,
ASCII-shape check before case-fold, sentinel allowlist gated by AST audit.
All prior fixes hold under a focused re-read.

## Structural Findings (fallow)

No `<structural_findings>` block was provided. No structural pre-pass
substrate to merge.

## Narrative Findings (AI reviewer)

## Warnings

### WR-01: `Client.userAgent` and `Client.logger` are write-only dead state (WR-04 pattern, second instance)

**File:** `client.go:55-56` (struct field declarations), `client.go:98-99`
(NewClient constructor literal)
**Issue:**
Two Client struct fields are assigned during construction but never read by
any production code path. `grep -rn 'c\.userAgent\|c\.logger' *.go | grep -v
_test.go` returns zero hits outside the assignment site itself; the only
readers are test files asserting that the option mechanism mutated the
field (`client_test.go:71,75`, `options_test.go:110,118,134,148`).

The actual production consumers of both values are the transport-layer
decorator structs constructed in `buildTransport`:

- `headerTransport.userAgent` (`transport.go:66`) — populated from
  `cfg.userAgent` at `config.go:170`.
- `loggingTransport.logger` (`transport.go:111`) — populated from
  `cfg.logger` at `config.go:171`.

The Client-side mirror of these two fields adds no behavior: removing them
from the `*Client` struct and from the `NewClient` constructor literal
would not change any production code path. They will fail `staticcheck`
(`U1000: unused field`) the day `.golangci.yml` is enabled (currently only
a `.golangci.yml_backup` exists in the repo root). They also mislead future
readers who reasonably expect them to be the effective source of truth — a
contributor reading `c.userAgent` and assuming a future call would actually
use it would be wrong.

This is the same dead-state defect that the WR-04 fix removed for
`Client.requestHook`: the prior fix removed the Client-side mirror and
updated `TestWithRequestHook` to assert on the buildTransport-produced
chain (`options_test.go:489-522`). The same fix applies here.

**Fix:**

1. Remove `userAgent` and `logger` from the `Client` struct (`client.go:55-56`).
2. Remove the corresponding lines from the `NewClient` constructor literal
   (`client.go:98-99`).
3. Update the four affected test assertions to assert on the
   buildTransport-produced chain:
   - `client_test.go:71,75` (the "defaults applied" subtest of `TestNewClient`).
   - `options_test.go:110,118` (`TestWithUserAgent`).
   - `options_test.go:134,148` (`TestWithLogger`).

For each, the lowest-cost replacement is a type-assertion on the chain.
Recommended pattern (mirrors `TestWithRequestHook`):

```go
// userAgent assertion via headerTransport
ht := unwrapTransport[*headerTransport](c.http.Transport)
require.NotNil(t, ht, "headerTransport must be in chain")
assert.Equal(t, "go-openholidays/"+Version, ht.userAgent)

// logger assertion via loggingTransport
lt := unwrapTransport[*loggingTransport](c.http.Transport)
require.NotNil(t, lt, "loggingTransport must be in chain")
assert.Same(t, customLogger, lt.logger)
```

`unwrapTransport[T]` is a small test-only helper that walks the
`next http.RoundTripper` chain looking for a layer of type `T`. Add it to
`internal_test.go` or a new `transport_test_helpers_test.go`.

Alternatively the simplest fix is to land a wire-level assertion — a
single-request httptest server whose handler reads `r.Header.Get("User-Agent")`
— in place of the field assertion. That removes the need for the helper
and exercises the actual observable behavior.

## Info

### IN-01: `fnv.Hash.Write` return values ignored in `newClientRand` fallback

**File:** `client.go:178-179`, `client.go:182-183`
**Issue:**
The four `h1.Write(...)` / `h2.Write(...)` calls in the `newClientRand`
crypto/rand fallback discard the `(int, error)` return. `hash.Hash`
embeds `io.Writer`, and stdlib documents that `hash.Hash.Write` "never
returns an error", but `errcheck` (mandated by CLAUDE.md) does not treat
documented-never-errors specially — it flags every unchecked error
return. Once `.golangci.yml_backup` is restored to an active
`.golangci.yml` and `errcheck` runs in CI, these four lines will fail.

**Fix:**
Discard explicitly per the project convention (`_, _ = ...`):

```go
_, _ = h1.Write(tb[:])
_, _ = h1.Write(pb[:])
...
_, _ = h2.Write(pb[:])
_, _ = h2.Write(tb[:])
```

This is the same idiom already used at `client_test.go:339`
(`_, _ = w.Write(body)`) and at multiple sites in
`countries_test.go:94,170,254,280,316,322,350,369`.

### IN-02: `Client.closed` flag is set but never read by production code

**File:** `client.go:58`, `client.go:124` (write site), no production read site
**Issue:**
The `closed atomic.Bool` field is documented at `client.go:35-37` and
`client.go:118-121` as the "mechanical guarantee" that subsequent
operations observe the close. In practice, no production code path reads
`c.closed`: `grep -rn 'c\.closed\|client\.closed' *.go | grep -v _test.go`
returns only the single Store at `client.go:124`. The endpoint methods in
`request.go`, `countries.go`, `languages.go`, `subdivisions.go`,
`public_holidays.go`, `school_holidays.go` do not check `closed` before
dispatching the HTTP call. As a result, `Countries(ctx, ...)` called after
`Close()` still proceeds to dispatch the request through `c.http.Do`.

This is not strictly a bug — the godoc for `Close` does not promise that
post-Close calls fail, only that Close itself is idempotent. But the
`atomic.Bool` and its careful comments imply a "fail subsequent calls"
contract that does not actually exist. The field is documented as a
test-observable race-safety invariant only — that is the same WR-04
dead-state pattern, downgraded to Info here because:

1. Tests do read it (`client_test.go:163,167,179,198,200`) — removing the
   field would require rewriting those subtests.
2. The Close godoc's "subsequent calls return nil unchanged" sentence
   refers to subsequent `Close` calls, not subsequent endpoint calls — so
   the field's only documented purpose is to back the
   `closed.CompareAndSwap`-style test observation, which is structurally
   redundant given the `sync.Once`-guarded body.

**Fix:**
Either of:

1. Make `closed` load-bearing: gate every endpoint method on
   `if c.closed.Load() { return ..., errors.New("openholidays: client closed") }`
   at the top of `doJSONGet`. Surfaces post-Close misuse loudly and gives
   the field a real production purpose. Add a new sentinel
   `ErrClientClosed` (would need an allowlist entry in `internal_test.go`).
2. Remove the field and the test assertions on it; rely on `sync.Once`
   for the idempotency invariant (which it already enforces). Lower
   churn, but loses the test-observable race-safety check.

Recommend (1) — gating endpoint methods on `closed` is the documented
real-world expectation of "I called Close, this Client is dead" and the
fix is one if-check per endpoint plus one sentinel.

### IN-03: Vacuous `require.NotNil` on freshly-constructed values in `errors_test.go`

**File:** `errors_test.go:89`, `errors_test.go:151`, `errors_test.go:222`,
`errors_test.go:253`
**Issue:**
Several `require.NotNil` assertions check values that were just built by a
`fmt.Errorf("...: %w", ...)` call or a `&APIError{...}` struct literal.
`fmt.Errorf` with a non-nil `%w` argument cannot return nil; a pointer to
a struct literal cannot be nil. These four assertions are defensive but
vacuous — they cannot fail, so they provide no signal. Consequences are
purely cosmetic (test report carries a few extra rows that always pass).

**Fix:**
Optional cleanup — remove the four `require.NotNil` lines. They cannot
catch any real regression because the construction sites cannot produce
nil.

---

_Reviewed: 2026-05-28_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
