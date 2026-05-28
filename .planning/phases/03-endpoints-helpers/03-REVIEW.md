---
phase: 03-endpoints-helpers
reviewed: 2026-05-28T00:00:00Z
depth: standard
files_reviewed: 26
files_reviewed_list:
  - client_isinregion.go
  - client_isinregion_test.go
  - client_test.go
  - countries.go
  - countries_test.go
  - errors.go
  - holiday.go
  - holiday_test.go
  - internal_test.go
  - languages.go
  - languages_test.go
  - public_holidays.go
  - public_holidays_test.go
  - request.go
  - request_test.go
  - school_holidays.go
  - school_holidays_test.go
  - subdivisions.go
  - subdivisions_test.go
  - testdata/countries.json
  - testdata/languages.json
  - testdata/public_holidays_pl_2025.json
  - testdata/school_holidays_pl_2025.json
  - testdata/subdivisions_de.json
  - testdata/subdivisions_pl.json
  - update_fixtures_test.go
findings:
  critical: 0
  warning: 6
  info: 5
  total: 11
status: issues_found
---

# Phase 3: Code Review Report

**Reviewed:** 2026-05-28
**Depth:** standard
**Files Reviewed:** 26 (20 .go source files + 6 JSON fixtures)
**Status:** issues_found (no blockers; 6 warnings, 5 info)

## Summary

Phase 3 shipped five endpoint methods (`Countries`, `Languages`, `Subdivisions`, `PublicHolidays`, `SchoolHolidays`), the hierarchical `Client.IsInRegion`, four pure-value `Holiday` helpers (`NameFor`, `IsInRegion`, `Days`, `Range`), the generic `doJSONGet[T any]` pipeline, and the post-decode `validateHolidays` guard. The code is well-tested (one `TestXxx` per exported production function per Gold Rule 3; every case lives inside `t.Run`), the structural invariants hold (input validation before HTTP, sentinel-wrapped errors with `%w`, `*APIError` for non-2xx, cycle-bounded hierarchical walk), and URL/query construction routes through `url.Values.Encode()` so no injection vector is reachable. **No correctness or security blockers were found in the current state of the code.**

This is a re-review. A prior REVIEW.md (dated 2026-05-27, now overwritten) reported two BLOCKER-class defects in the fixture-refresh harness (CR-01: 2-space indent / fixtures 4-space; CR-02: missing trailing-newline append) and a WR-01 contract violation in `Holiday.Range` (first yield un-normalized). Verified at 2026-05-28: all three have been fixed — `update_fixtures_test.go:230` now uses `"    "` (four spaces), line 231 appends `pretty.WriteByte('\n')`, and `holiday.go:119` rebuilds the first `d` via `NewDate(...)` (regression test at `holiday_test.go:213-240` named "non-UTC StartDate yields UTC-midnight first Date (WR-01 regression)" locks the contract). The prior WR-05 (`cap := cap` shadow + builtin-shadow) was also fixed: the loop variable is now `c` over `[]capture` (`update_fixtures_test.go:194`).

The six warnings carried forward are: (1) the cycle-defense test only exercises a 2-node cycle whose normal exit and cap-driven exit are indistinguishable; (2) the +5 goroutine-slack ceiling in the oversize test is too tight when sibling top-level tests run with `t.Parallel`; (3) `Holiday.Days()` returns a negative count for hand-built malformed Holidays without documentation or a defensive guard; (4) the IsInRegion 5xx test asserts `StatusCode` but not `apiErr.Message`, leaving the title-fallback path uncovered for the IsInRegion-mediated /Subdivisions call; (5) the retry-exhausted error wrap in `doJSONGet` drops the path from the error prefix, making error-string parsing inconsistent between retry-disabled and retry-exhausted paths; (6) `Holiday.IsInRegion("")` returns `false` even when `Nationwide: true`, a semantic inversion of the natural "Nationwide always wins" intuition that creates a footgun for callers who don't validate input upstream.

The five info-level items cover stale doc references in `request.go`, dead/unreachable helper functions in `validate.go`, a too-lax ctx-cancel ceiling in `subdivisions_test.go`, a missing post-decode country-prefix consistency check in `Subdivisions`, and a small file-handle hygiene issue in the fixture-refresh harness.

## Narrative Findings (AI reviewer)

### Warnings

#### WR-01: IsInRegion cycle-defense test exercises only a 2-node cycle — cap-driven exit indistinguishable from normal exit

**File:** `client_isinregion_test.go:200-249`
**Severity:** Warning
**Issue:** The cycle subtest constructs two top-level subdivisions A and B where each declares the other as Children. `buildParentIndex` produces `parentIdx = {"DE-A": "DE-B", "DE-B": "DE-A"}` — `len(parentIdx) == 2`. The upward walk in `IsInRegion` loops with `i = 0..2` (3 iterations). At each iteration the lookup succeeds and the walk oscillates DE-A → DE-B → DE-A → DE-B; the loop exits via the `i <= len(parentIdx)` bound after 3 iterations. The 2-second test timeout would correctly catch a regression that *removed* the cap entirely (the loop would never exit). But the test does NOT distinguish between "loop exits because cap fired" (the intended behavior) and "loop exits because cycle length divides cleanly into cap budget" — a regression that *tightened* the cap to `len(parentIdx)-1 = 1` would still terminate (after 2 iterations), still return `(false, nil)`, and the test would still pass even though the cap is now wrong. A regression that *loosened* the cap to a number > 2 would also pass. The test catches only the binary case "cap exists vs cap removed entirely".

**Fix:** Add a second cycle test with a longer cycle so the cap upper-bound is meaningfully exercised:

```go
t.Run("5-node cycle terminates via len(parentIdx)+1 cap, not pure cycle math", func(t *testing.T) {
    t.Parallel()
    // 5-node deep cycle: A → B → C → D → E → A. parentIdx will have
    // 5 entries; len(parentIdx)+1 = 6 iterations cap. The cycle's
    // natural length is 5 — the cap fires on the 6th iteration.
    cyclic := []Subdivision{
        {Code: "X-A", Children: []Subdivision{
            {Code: "X-B", Children: []Subdivision{
                {Code: "X-C", Children: []Subdivision{
                    {Code: "X-D", Children: []Subdivision{
                        {Code: "X-E", Children: []Subdivision{{Code: "X-A"}}},
                    }},
                }},
            }},
        }},
    }
    // ... rest mirrors the existing 2-node cycle test
})
```

Without this stronger probe, a future contributor tightening the cap could silently break cycle defense on deeper malformed trees.

---

#### WR-02: `TestClient_Countries` oversize-subtest goroutine-slack +5 is too tight under sibling `t.Parallel`

**File:** `countries_test.go:259-262`
**Severity:** Warning
**Issue:** The oversize subtest at `countries_test.go:200-263` reads `runtime.NumGoroutine()` to detect a body-drain failure. The slack is `+5` (line 259: `const goroutineSlack = 5`). The subtest is intentionally NOT `t.Parallel()` to avoid interference from siblings of the same parent `TestClient_Countries`. However, `runtime.NumGoroutine()` is **process-global** — sibling top-level tests in OTHER `TestXxx` functions (e.g. `TestClient_ConcurrentAccess` spawns 50 concurrent Countries calls and is marked `t.Parallel()`; `TestClient_ContextCancel` holds a 30-second timeout server; `TestClient_Languages`, `TestClient_Subdivisions`, etc.) WILL be in flight when the baseline is captured. Their goroutine churn can easily exceed 5. Removing `t.Parallel()` from the subtest does NOT serialize against other top-level tests — Go's test runner serializes only same-parent subtests.

Empirically the test currently passes, but it's flake-prone on slow / loaded CI runners. The comment on line 251-256 acknowledges that "+5 detects any leak of 6+ goroutines" — fine in isolation, but the +5 ceiling cannot distinguish a 6-goroutine leak from baseline noise produced by sibling tests' transient HTTP transport pools.

**Fix:** Two options:
- (a) Widen the slack to +20 (the comment line 253 admits "a real drain failure would leak the transport's body-reader plus its parent and would show ≥ +10"). This loses some signal but eliminates the flake risk.
- (b) Replace `runtime.NumGoroutine()` measurement with a deterministic counting body — wrap the test transport in a `RoundTripper` that increments on body-Read and decrements on body-Close, and assert the counter is 0 after the call returns. This is the standard pattern for proving drain-then-close hygiene:

```go
// pseudo-code
type drainCheckingRT struct {
    base       http.RoundTripper
    openCount  atomic.Int32
}
// wraps resp.Body in a counter so test can assert == 0 post-call
```

Option (b) is more work but removes flake AND eliminates the `time.Sleep(200 * time.Millisecond)` settle pause at line 257.

---

#### WR-03: `Holiday.Days()` returns negative counts for malformed Holidays without documentation or defensive guard

**File:** `holiday.go:83-85`, `date.go:154-164`, `holiday_test.go:99-128`
**Severity:** Warning
**Issue:** `Holiday.Days()` delegates to `Date.DaysUntil`, which is documented (date.go:148-149) to return a negative integer when the source date is strictly after the target. For a hand-built Holiday with `EndDate.Before(StartDate)`, `Days()` returns a negative count. The Holiday.Days godoc (holiday.go:73-82) covers only single-day (1) and the canonical 14-day ferie zimowe case — it never mentions the negative-return path. `TestHoliday_Days` has no malformed-input case. The `validateHolidays` godoc at `request.go:244-249` explicitly calls out: "a strictly-before EndDate would make Holiday.Range yield zero items and Holiday.Days return a negative count — both observable bugs for callers iterating returned holidays" — but that observation is buried in a separate file's comments and is gated only at the endpoint boundary. Callers who hand-construct a Holiday (e.g., for unit tests, or by merging two upstream queries) can reach the negative branch silently, and `if h.Days() > 14 { ... }` mis-classifies any Holiday with EndDate ≥ StartDate-14-days.

**Fix:** Two acceptable approaches:

(a) **Defensive clamp** — preferred:

```go
// Days returns the inclusive count of calendar days the holiday spans.
//
// For a single-day holiday (StartDate == EndDate), Days returns 1. For a
// multi-day holiday, Days returns the inclusive count from StartDate to
// EndDate.
//
// When EndDate is strictly before StartDate — a malformed Holiday the
// endpoint-layer validateHolidays would have rejected but a hand-built
// Holiday can carry — Days returns 0 (defensive: callers branching on
// h.Days() > N get a defined, non-negative value).
func (h Holiday) Days() int {
    if h.EndDate.Before(h.StartDate) {
        return 0
    }
    return h.StartDate.DaysUntil(h.EndDate)
}
```

(b) **Document the negative-return contract** if changing the surface is undesirable. Either way, add a `TestHoliday_Days` subtest:

```go
t.Run("EndDate before StartDate returns 0 (defensive)", func(t *testing.T) {
    t.Parallel()
    h := Holiday{
        StartDate: NewDate(2025, time.December, 25),
        EndDate:   NewDate(2025, time.January, 1),
    }
    assert.Equal(t, 0, h.Days(),
        "malformed hand-built Holiday must produce a defined non-negative value")
})
```

Without this lock, a future contributor could silently change the behavior.

---

#### WR-04: IsInRegion 5xx test asserts `StatusCode` but not `apiErr.Message` — title-fallback path uncovered on the IsInRegion-mediated /Subdivisions error

**File:** `client_isinregion_test.go:179-198`
**Severity:** Warning
**Issue:** The "transport error from Subdivisions surfaces verbatim" subtest sets `Content-Type: application/problem+json` and writes `{"title": "Internal Server Error"}`. It asserts `errors.As(err, &apiErr)` succeeds and `apiErr.StatusCode == 500`, but does **not** assert `apiErr.Message == "Internal Server Error"` or `apiErr.Path == "/Subdivisions"`. Every sibling endpoint test (`countries_test.go:107-126`, `languages_test.go:137-156`, `public_holidays_test.go:184-208`, `school_holidays_test.go:189-213`, `subdivisions_test.go:185-205`) DOES assert the title-fallback message. The IsInRegion test alone leaves the message-parse path uncovered when surfaced through the inner `c.Subdivisions` → `doJSONGet` → `buildAPIError` → `parseAPIMessage` chain. If `parseAPIMessage` silently regressed (e.g. lost the title-fallback when detail is absent), every other endpoint test would catch it, but the IsInRegion 5xx test alone would NOT.

The Path assertion matters too — IsInRegion's caller does NOT see `/IsInRegion` as the path on an *APIError (there is no such upstream path); they see the inner `/Subdivisions` path because the error is wrapped verbatim from the inner call. A regression that changed this (e.g., wrapping the *APIError to rewrite the path field) would silently break caller introspection — but the test does not lock this contract either.

**Fix:** Add the missing assertions symmetric with sibling tests:

```go
assert.Equal(t, "/Subdivisions", apiErr.Path,
    "Path must be /Subdivisions (the inner Subdivisions call) — IsInRegion does not rewrite the path")
assert.Equal(t, "Internal Server Error", apiErr.Message,
    "title must win when detail is absent (parseAPIMessage fallback)")
```

---

#### WR-05: Inconsistent error-prefix shape between retry-disabled and retry-exhausted paths in `doJSONGet`

**File:** `request.go:138-148, 157-169`
**Severity:** Warning
**Issue:** When the retry loop exits with `httpErr != nil`, the wrap depends on `maxAttempts`:
- `maxAttempts == 1` (retry disabled): `"openholidays: GET %s: %w"` — includes the request path.
- `maxAttempts > 1` (retry exhausted): `"openholidays: retry exhausted (%d attempts): %w"` — does NOT include the path.

The same asymmetry applies at lines 166-168 for the retryable-status branch: the retry-exhausted wrap drops the path. Consequences:
1. Callers using `errors.Is` / `errors.As` see consistent behavior (sentinels and `*APIError` still match), but callers parsing `err.Error()` for path-based log routing must handle two distinct shapes.
2. An operator searching for `"openholidays: GET /SchoolHolidays"` in error logs will miss every retry-exhausted occurrence of the same error against the same path.

This is a quality wart, not a correctness defect. Per the project's "Verify or ask" Gold Rule 2, I verified via `errors.As` that *APIError's inner Path field still carries the path — that's correct. But raw transport errors (DNS failure, connection refused) have no `Path` field at all, and the retry-exhausted path is the only one that strips the contextual path string.

**Fix:** Make both wraps carry the path:

```go
if httpErr != nil {
    if maxAttempts > 1 {
        return zero, fmt.Errorf("openholidays: GET %s: retry exhausted (%d attempts): %w", path, maxAttempts, httpErr)
    }
    return zero, fmt.Errorf("openholidays: GET %s: %w", path, httpErr)
}
// ... and at line 166-168:
if maxAttempts > 1 && shouldRetry(resp, nil) {
    return zero, fmt.Errorf("openholidays: GET %s: retry exhausted (%d attempts): %w", path, maxAttempts, apiErr)
}
```

After fix, `strings.Contains(err.Error(), path)` holds for every transport-layer error regardless of retry settings.

---

#### WR-06: `Holiday.IsInRegion("")` returns `false` even when `Nationwide: true` — silent semantic inversion of natural intuition

**File:** `holiday.go:58-71`, `client_isinregion.go:75-81`
**Severity:** Warning
**Issue:** Both `Holiday.IsInRegion("")` and `Client.IsInRegion(ctx, h, "")` return `false` for an empty code argument — even when `h.Nationwide == true`. The godoc explicitly documents this ("An empty code returns false (defensive — no panic)") and the test at `holiday_test.go:61-65` locks the behavior. But the precedence is **counter-intuitive**: a nationwide holiday applies to "everywhere by definition", so "does this holiday apply in '' (empty region)" is a question whose natural answer is either "true (it applies everywhere)" or "error (caller bug — empty input)". Silently answering `false` means a caller who writes `if h.IsInRegion(userSuppliedCode) { ... }` and forgets to validate `userSuppliedCode != ""` upstream silently misclassifies every nationwide holiday on empty input. The cost is non-zero: PL ferie test code, for example, builds a map of regions to "does this holiday apply", and an empty cell silently maps to `false` even for nationwide holidays.

**Fix:** Two options, in preference order:

(a) **Reorder the checks** to make `Nationwide` win over empty input (matches intuition):

```go
func (h Holiday) IsInRegion(code string) bool {
    if h.Nationwide {
        return true
    }
    if code == "" {
        return false
    }
    for _, s := range h.Subdivisions {
        if strings.EqualFold(s.Code, code) {
            return true
        }
    }
    return false
}
```

Apply the symmetric reorder to `Client.IsInRegion`. Update `TestHoliday_IsInRegion/empty_code_returns_false_even_when_Nationwide` to assert the new contract.

(b) **Keep the current order** but strengthen the godoc to call out the surprise:

```go
// IsInRegion ... PRECEDENCE NOTE: An empty code returns false EVEN WHEN
// h.Nationwide is true. Callers who want "Nationwide always wins"
// semantics must check h.Nationwide explicitly before calling IsInRegion.
```

Option (a) is safer (eliminates the footgun); option (b) preserves the current contract for callers that may already depend on it. Given the library is pre-1.0 (`v0.x`), option (a) is the recommended pre-1.0 break.

---

### Info

#### IN-01: Stale forward-reference in `doJSONGet` godoc

**File:** `request.go:55-58`
**Severity:** Info
**Issue:** The godoc on `doJSONGet` reads "PublicHolidays and SchoolHolidays will call a separate validateHolidays helper (D-65, lands in Plan 04) on the decode result before returning." But `validateHolidays` is declared 200 lines lower in the SAME file (`request.go:269-286`), and Phase 3 Plan 04 has shipped — the "lands in Plan 04" future-tense reference is stale.

**Fix:** Update to present tense:

```go
// Post-decode validation is the caller's responsibility — doJSONGet does
// NOT inspect the decoded value. PublicHolidays and SchoolHolidays call
// the validateHolidays helper (declared below in this file) on the
// decode result before returning (D-65 / CL-12).
```

---

#### IN-02: Dead-code helpers `isTwoASCIIUppers` / `isTwoASCIILowers` in `validate.go`

**File:** `validate.go:123-143`
**Severity:** Info
**Issue:** After the W-01 reorder, `validateCountry` and `validateLanguage` both call `isTwoASCIILetters` and never call `isTwoASCIIUppers` or `isTwoASCIILowers`. The function comments on lines 123 and 135 acknowledge: "Currently unreachable from validateCountry/validateLanguage after the W-01 reorder; retained as defense-in-depth and for direct testing." Comments do not satisfy `unused` / `deadcode` linters. If `validate_test.go` exercises them directly, the rationale stands; if not, they should be deleted. (Note: `validate_test.go` is out of this phase's review scope; verifying requires a follow-up Read.)

**Fix:** Either (a) confirm `TestIsTwoASCIIUppers` and `TestIsTwoASCIILowers` exist in `validate_test.go`, or (b) delete the 16 lines of unreachable code. Defense-in-depth comments without test or runtime coverage age into pure dead code.

---

#### IN-03: ctx-cancel ceiling in `subdivisions_test.go` is 200 ms (180 ms slack vs the 100 ms CLIENT-09 contract)

**File:** `subdivisions_test.go:259-266`
**Severity:** Info
**Issue:** The subtest cancels ctx at +20 ms and asserts elapsed ≤ 200 ms — leaving 180 ms slack against the CLIENT-09 ≤ 100 ms contract. If a future change pushed cancellation latency to 150 ms, the assertion still passes. Sibling tests have looser ceilings too (`languages_test.go:213` uses 1 s; `countries_test.go:234` uses 500 ms with WR-09 comment acknowledging the loosening). The cumulative effect is that no ctx-cancel test in the repo actually verifies the 100 ms contract — they all verify a much looser bound for CI flake tolerance.

**Fix:** Either tighten this test to ~150 ms (20 ms cancel + 100 ms target + 30 ms scheduler slack) to make it the contract-locking test, or add an explicit microbenchmark target that locks the 100 ms contract under controlled conditions. The current state where every cancel test is loose is a quality wart — there is no regression detector for the headline ≤ 100 ms contract.

---

#### IN-04: `Subdivisions` does not validate that returned `Code` matches requested country prefix

**File:** `subdivisions.go:88-103`, `client_isinregion.go:98-102`
**Severity:** Info
**Issue:** `Client.Subdivisions` returns the upstream tree verbatim with no post-decode check that each `Subdivision.Code` starts with the requested country code. `Client.IsInRegion` then indexes this tree by `Subdivision.Code` and walks it. A hostile or buggy upstream that returns a tree with mixed-country codes (e.g., a `DE-BY` entry leaks into a response for `countryIsoCode=PL`) would silently corrupt the hierarchical walk. The current 7-case IsInRegion test does not exercise "upstream returns codes inconsistent with the requested country". This is a low-probability scenario given the curated upstream, but it's an uncovered defense-in-depth gap.

**Fix:** Document the trust assumption in the godoc:

```go
// Subdivisions ... Trust model: the upstream is assumed to return only
// subdivisions belonging to the requested country. The library does NOT
// post-decode-verify the country prefix on Subdivision.Code values; a
// hostile or buggy upstream that returns mixed-country codes would
// produce undefined behavior in downstream helpers (in particular
// Client.IsInRegion's hierarchical walk).
```

Alternative: add a post-decode pass that filters or rejects mismatched-country entries. This is a v0.2 deviation candidate (current scope is intentionally trust-the-upstream).

---

#### IN-05: Temp-file handle leak on abort-before-`tmp.Close()` in fixture-refresh harness

**File:** `update_fixtures_test.go:250-262`
**Severity:** Info
**Issue:** The flow is:

```go
tmp, err := os.CreateTemp(tmpDir, c.fixture+".tmp-*")
require.NoError(t, err)
defer func() { _ = os.Remove(tmp.Name()) }()

_, err = tmp.Write(pretty.Bytes())
require.NoError(t, err)              // ← if this fires, tmp is never closed
require.NoError(t, tmp.Close())
```

If `tmp.Write` returns an error, `require.NoError` aborts the test before `tmp.Close()` runs. The file handle leaks until the test binary exits. Not a correctness defect (the test fails loudly anyway and the deferred `os.Remove` still runs), but the pattern is sloppy enough that future maintenance copying the idiom will inherit the leak.

**Fix:** Defer the close alongside the remove:

```go
tmp, err := os.CreateTemp(tmpDir, c.fixture+".tmp-*")
require.NoError(t, err)
defer func() {
    _ = tmp.Close() // best-effort; Close on already-closed file is harmless
    _ = os.Remove(tmp.Name())
}()

_, err = tmp.Write(pretty.Bytes())
require.NoError(t, err)
require.NoError(t, tmp.Close())
```

---

## Cross-cutting Observations (not findings)

These observations are NOT bugs — they are noted so the next reviewer doesn't independently re-derive them:

1. **URL/query construction is injection-safe.** Every dynamic value (CountryIsoCode, LanguageIsoCode, dates, SubdivisionCode, GroupCode) is passed through `url.Values.Set` and the query is built via `q.Encode()` which properly URL-encodes the values. A hostile caller cannot inject `&extra=evil` via shape-tolerant fields like SubdivisionCode/GroupCode.

2. **`validateHolidays` correctly rejects malformed responses with sentinel-wrapped errors.** Three invariants (zero StartDate, zero EndDate, EndDate < StartDate) all wrap `ErrMalformedResponse` with descriptive messages including the offending holiday ID and the path. `errors.Is` matching works through the wrap.

3. **`IsInRegion` cycle-defense terminates correctly** on the 2-node cycle test (limited as it is per WR-01). The bound `len(parentIdx)+1` is mathematically sound: any cycle path through a graph of N nodes has at most N distinct nodes, so a walk of N+1 steps must revisit a node and is bounded.

4. **`validateDateRange` uses backward-from-`to` AddDate(-3, 0, 0) to avoid the forward-overflow leap-day asymmetry** (Pitfall 3). Tested via `validate_test.go` (out of this phase's scope).

5. **`*APIError` carries the request path, status code, and a 4 KiB body cap.** Construction is centralized in `buildAPIError` (request.go:219-228), so every endpoint that goes through `doJSONGet` gets the same shape automatically.

6. **`Holiday.Range` correctly handles cross-year boundaries** via `time.Time.AddDate(0, 0, 1)`; the UTC-midnight invariant is preserved because both operands of every comparison are normalized to UTC midnight via `Date.toUTCMidnight()`.

7. **No global mutable state introduced.** The CLIENT-10 AST audit in `internal_test.go` continues to pass; `allowedVars` is the closed allowlist for the seven exported sentinels + `errEmptyDate` + `CacheHitContextKey`.

8. **Concurrent use is race-safe.** All Client fields touched in Phase 3 methods are read-only after `NewClient`. `TestClient_ConcurrentAccess` exercises 50 parallel `Countries` calls under `-race`.

9. **Context propagation works end-to-end.** Every endpoint passes ctx through to `doJSONGet` → `http.NewRequestWithContext` → `req.Clone(ctx)` → `c.http.Do`. Cancellation interrupts in-flight HTTP and surfaces `context.Canceled` via `errors.Is`.

10. **Prior REVIEW.md remediations verified.** CR-01 (4-space indent), CR-02 (trailing newline), WR-01 (Range first-yield normalization), WR-05 (`cap := cap` shadow + builtin shadow) all confirmed fixed in the current code.

---

_Reviewed: 2026-05-28_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
_Adversarial stance: applied (FORCE) — assumed defects, surfaced what was provable; prior critical findings verified fixed; no new blockers; 6 quality warnings carried forward._
