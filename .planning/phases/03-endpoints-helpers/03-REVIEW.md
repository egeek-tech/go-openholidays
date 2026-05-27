---
phase: 03-endpoints-helpers
reviewed: 2026-05-27T00:00:00Z
depth: standard
files_reviewed: 25
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
  - testdata/languages.json
  - testdata/public_holidays_pl_2025.json
  - testdata/school_holidays_pl_2025.json
  - testdata/subdivisions_de.json
  - testdata/subdivisions_pl.json
  - update_fixtures_test.go
findings:
  critical: 2
  warning: 6
  info: 4
  total: 12
status: issues_found
---

# Phase 3: Code Review Report

**Reviewed:** 2026-05-27
**Depth:** standard
**Files Reviewed:** 25
**Status:** issues_found

## Summary

Phase 3 ships the four pure GET endpoint methods (Countries, Languages, Subdivisions, PublicHolidays, SchoolHolidays), the hierarchical Client.IsInRegion helper, four Holiday pure-value helpers (NameFor, IsInRegion, Days, Range), and the post-decode validateHolidays guard, all backed by a shared generic doJSONGet pipeline in request.go. The implementation is conscientious — typed sentinels, oversize-body gates, CR-01 regression coverage, cycle defense in the hierarchical walk, and one TestXxx per exported production function per Gold Rule 3.

Two BLOCKER-class defects exist in the fixture-refresh harness: (1) the `json.Indent` pretty-printer in `update_fixtures_test.go` uses a 2-space indent, but five of the six committed fixtures (`countries.json`, `languages.json`, `subdivisions_pl.json`, `subdivisions_de.json`, `public_holidays_pl_2025.json`) are stored with a 4-space indent; (2) the same writer never appends a trailing newline, while every committed fixture ends with `\n]\n`. The combined effect: drift-detection mode (`go test -tags=integration -run TestUpdateFixtures` without `-update`) reports DRIFT on every fixture on first run, and an `-update` capture run silently re-indents five fixtures from 4-space to 2-space — a noisy diff that would mask real upstream drift. This invalidates the documented drift-detection contract in the file header and CONTEXT D-67/D-68.

Six WARNINGs cover a contract mismatch in `Holiday.Range()` (godoc claims every yielded Date is rebuilt via NewDate, implementation yields the un-normalized StartDate on the first iteration), a 5xx test that mis-encodes its content type (`application/problem+json` body for `errors.As` extraction but no problem-json semantics tested), an `IsInRegion` cycle test that does not actually probe the cap boundary, a `TestClient_ConcurrentAccess` subtest goroutine-leak ceiling (+5) likely too tight when sibling t.Parallel tests run concurrently, the unused-cap `cap := cap` shadow that CLAUDE.md "What NOT to Use" explicitly forbids, and a `Days()` documentation gap on negative spans. INFO covers the `cap` Go-builtin shadow, dead-code defense-in-depth comments, and the lax 200 ms ctx-cancel ceiling on `subdivisions_test.go`.

## Critical Issues

### CR-01: Fixture writer indent (2-space) does not match committed fixtures (4-space)

**File:** `update_fixtures_test.go:226`
**Issue:** The fixture-refresh harness pretty-prints captured live responses with `json.Indent(&pretty, body, "", "  ")` (2-space indent). Five of the six committed fixtures (`testdata/countries.json`, `testdata/languages.json`, `testdata/subdivisions_pl.json`, `testdata/subdivisions_de.json`, `testdata/public_holidays_pl_2025.json`) are stored with 4-space indent (verified: line 2 of each is `    {`, while `school_holidays_pl_2025.json` is `  {`). Consequences:

1. **Drift-detection mode is permanently broken.** Lines 228-238 read the committed fixture and do `require.Equalf(t, string(committed), pretty.String(), "DRIFT: live response for %s differs from committed fixture", cap.fixture)`. Since the indent levels don't match, every fixture except `school_holidays_pl_2025.json` will report a false DRIFT on the very first run.
2. **An `-update` run silently re-indents five fixtures from 4-space to 2-space.** This produces a noisy diff in version control that masks any real upstream schema change — defeating the whole point of the drift-detection workflow (CONTEXT D-67/D-68).

The file header (lines 24-33) promises "byte-for-byte" comparison after normalization. The normalization is not consistent across the existing fixture corpus.

**Fix:** Pick one canonical indent and apply it to every fixture. Recommend 2-space (matches the writer code) and rewrite the five 4-space fixtures via `-update`, OR change the writer to use 4 spaces:

```go
require.NoError(t, json.Indent(&pretty, body, "", "    ")) // 4-space matches existing fixtures
```

Then add a CI guard (e.g., a unit test that asserts `bytes.HasPrefix(<fixture>, []byte("[\n    "))`) so future fixture refreshes cannot drift the indent without being caught.

---

### CR-02: Fixture writer does not append trailing newline, breaking drift detection

**File:** `update_fixtures_test.go:226,252`
**Issue:** `json.Indent` does not emit a trailing newline (verified empirically: `json.Indent` writes `"[\n  1,\n  2,\n  3\n]"` — no terminal `\n`). However every committed fixture ends with `\n]\n` (a trailing newline at EOF — confirmed via `tail -c 5` on all five `testdata/*.json` files: all show `]0a`). Consequences:

1. **Drift-detection mode reports DRIFT on every fixture even when the live response is byte-identical** because `string(committed)` carries a trailing `\n` that `pretty.String()` lacks.
2. **An `-update` run strips the trailing newline from every fixture,** producing a noisy diff that POSIX tools (cat, vi, most diff viewers, git's last-line warning) will flag.

This compounds CR-01 — even after the indent is unified, drift detection still won't pass without this fix.

**Fix:** Append a newline after `json.Indent`:

```go
var pretty bytes.Buffer
require.NoError(t, json.Indent(&pretty, body, "", "    "))
pretty.WriteByte('\n')
```

Apply BOTH to the drift comparison (line 235) and the overwrite write (line 252). After fix, run `-update` once to renormalize every committed fixture to the same canonical shape.

## Warnings

### WR-01: Holiday.Range first-iteration yield does not honor godoc UTC-midnight invariant

**File:** `holiday.go:114-131`
**Issue:** The godoc at lines 96-100 states: "Every yielded Date is rebuilt via NewDate(year, month, day), so each yielded value is at UTC midnight regardless of the receiver's internal time.Time location." The implementation does NOT honor this for the first iteration: line 119 `d := h.StartDate` yields the receiver's `StartDate` verbatim — if a hand-built Holiday was constructed with a struct literal carrying a non-UTC time, the first yielded Date has the original location. Only subsequent iterations rebuild via `NewDate(...)` on lines 127-128.

The existing test "every yielded Date is UTC midnight" (holiday_test.go:197-211) does not catch this because both StartDate and EndDate are constructed via `NewDate(...)` which is already UTC midnight.

**Fix:** Normalize the initial `d` via `NewDate(...)`:

```go
return func(yield func(Date) bool) {
    if h.EndDate.Before(h.StartDate) {
        return
    }
    d := NewDate(h.StartDate.Year(), h.StartDate.Month(), h.StartDate.Day())
    for {
        if !yield(d) {
            return
        }
        if !d.Before(h.EndDate) {
            return
        }
        next := d.AddDate(0, 0, 1)
        d = NewDate(next.Year(), next.Month(), next.Day())
    }
}
```

Add a holiday_test.go subtest that constructs a Holiday with a struct literal carrying a non-UTC StartDate (e.g. `time.Date(2025, 1, 18, 23, 30, 0, 0, time.FixedZone("X", 3600))`) and asserts the first yield is at UTC midnight.

---

### WR-02: IsInRegion cycle-defense test does not actually probe the iteration cap

**File:** `client_isinregion_test.go:200-249`
**Issue:** The cycle subtest creates two top-level subdivisions A and B where each declares the other as Children. `buildParentIndex` walks the tree as follows: at the root, parent="", so `idx[A]=` is NOT set (line 156-158 skips when parent=="" ); then it recurses into A's Children=[B] with parent=A, so `idx[B]=A`; then it recurses into B's Children=[A] with parent=B, so `idx[A]=B`. Result: `parentIdx = {A: B, B: A}` — 2 entries, as the test comment claims.

The upward walk starts at `current = "DE-A"`, iterates `i = 0..len(parentIdx) = 0..2` (3 iterations max). Trace:
- i=0: no match for "DE-A" against ["DE-X"]; lookup "DE-A" → parent="DE-B"; current="DE-B"
- i=1: no match for "DE-B"; lookup → parent="DE-A"; current="DE-A"
- i=2: no match for "DE-A"; lookup → parent="DE-B"; current="DE-B"
- loop exits cleanly via the `i <= 2` bound.

The test "terminates after 3 iterations and returns (false, nil)" — but this is normal loop exit, NOT cap enforcement. To verify the test would catch a regression that removed the cap, REMOVE the cap (replace `for i := 0; i <= len(parentIdx); i++` with `for {}`) — the loop would indeed never terminate, and the 2-second timeout would fire. So the regression-detection assertion DOES work via timeout. But the comment in the test (line 207: "terminates after 3 iterations") is misleading: 3 iterations is what `len(parentIdx)+1 = 3` produces, but a TRUE cycle in a 2-node loop only NEEDS 2 iterations to detect — the test does not actually exercise the case where cap < cycle-length. A tighter cap (say, `len(parentIdx)-1 = 1`) would still terminate but with a different result.

**Fix:** Either (a) tighten the docstring to acknowledge the cap is upper-bound-only, OR (b) add a second cycle test with a 5-node cycle (A→B→C→D→E→A) where cycle length > 2 — that genuinely exercises the cap mechanism. Recommend (b) for stronger regression coverage:

```go
t.Run("5-node cycle terminates via cap (not normal loop exit)", func(t *testing.T) {
    cyclic := []Subdivision{
        {Code: "X-A", Children: []Subdivision{{Code: "X-B", Children: []Subdivision{
            {Code: "X-C", Children: []Subdivision{{Code: "X-D", Children: []Subdivision{
                {Code: "X-E", Children: []Subdivision{{Code: "X-A"}}}}}}}}}}},
    }
    // ... probe with 2s timeout
})
```

---

### WR-03: TestClient_ConcurrentAccess goroutine slack +5 is too tight under sibling t.Parallel

**File:** `countries_test.go:259-262`
**Issue:** The oversize subtest reads `runtime.NumGoroutine()` and asserts `afterGoroutines <= baseGoroutines + 5`. The intent is to detect a body-drain failure (which would leak ≥10 goroutines per call). Risk: sibling top-level tests (`TestClient_ConcurrentAccess` — 50 goroutines doing HTTP calls; `TestClient_ContextCancel` — long-lived hung handler) are marked `t.Parallel()` and may be in flight when the baseline goroutine count is captured, then complete before the after-count is captured. NumGoroutine variance from those siblings could easily exceed 5.

The non-parallel subtest is supposed to run while parallel siblings are paused — but only siblings of the SAME outer test (`TestClient_Countries`). Siblings under OTHER top-level tests (TestClient_ConcurrentAccess, TestClient_ContextCancel, TestClient_Languages, TestClient_Subdivisions, …) may execute concurrently because Go's test runner serializes only the parent function call, not the global subtest pool.

**Fix:** Either (a) widen the slack to +20 (which the test comment acknowledges as the leak threshold), OR (b) use a more deterministic signal — wrap the test transport in a counter that increments on body-Read and decrements on body-Close, then assert the counter is 0 after the call. The latter is the standard pattern for proving drain-then-close without measuring goroutines:

```go
type countingBody struct {
    io.ReadCloser
    closed atomic.Bool
}
// ... assert b.closed.Load() == true
```

If the goroutine-count approach is kept, document the +20 ceiling in the inline comment.

---

### WR-04: Holiday.Days() does not document negative-span behavior; tests don't cover it

**File:** `holiday.go:73-85`, `holiday_test.go:99-128`
**Issue:** `Holiday.Days()` delegates to `Date.DaysUntil`, which returns a negative integer when the operand is reversed (date.go:144-164 godoc). For a hand-built Holiday with `EndDate.Before(StartDate)`, `Days()` returns a negative count. The Holiday.Days godoc (holiday.go:73-82) only documents single-day (1), multi-day (inclusive count from StartDate to EndDate), and "14" for the canonical ferie zimowe example — never the negative path. The test (holiday_test.go:99-128) does not cover the reversed-date case.

This matters because `validateHolidays` (request.go:185-202) rejects EndDate < StartDate as `ErrMalformedResponse`, so endpoint-returned Holidays cannot reach the negative case — but callers building Holidays by hand (e.g. constructing for unit-test purposes, or merging two upstream queries) absolutely can, and the negative `Days()` value will silently propagate into calculations.

**Fix:** Either (a) add a docstring sentence on `Holiday.Days()`: "When EndDate is strictly before StartDate (a malformed hand-built Holiday), Days returns a negative count; endpoint-returned Holidays are validated against this case before they reach the caller." OR (b) clamp negative results to 0 with a leading guard in `Days()`. Recommend (a) — silent clamping breaks the symmetry with `DaysUntil`. Add a `Days_negative_span` test subtest that documents the observable behavior.

---

### WR-05: `cap := cap` shadow directly violates CLAUDE.md "What NOT to Use"

**File:** `update_fixtures_test.go:192`
**Issue:** Line 192 has `cap := cap // pin loop variable for the closure even though Go 1.22+ per-iteration scoping makes this strictly redundant`. CLAUDE.md "What NOT to Use" table explicitly lists: `tc := tc shadow in table-driven loops (Go 1.22+ code) | Redundant since Go 1.22 scopes loop vars per iteration | Drop it; linters may flag it`. The module declares `go 1.23` per project constraints. The shadow is acknowledged as redundant in the comment but kept anyway — a direct CLAUDE.md violation.

Compounding issue: the loop variable is named `cap`, which shadows Go's built-in `cap()` function. Any future contributor extending the loop body to use the `cap()` builtin (e.g. for sizing a slice) would silently get the struct value instead of the function. `revive` and `gocritic` may flag this.

**Fix:** Remove the shadow line AND rename the loop variable:

```go
for _, capture := range captures {
    t.Run(capture.fixture, func(t *testing.T) {
        url := baseURL + capture.path
        // ...
    })
}
```

This eliminates both the CLAUDE.md violation and the builtin-shadow risk in one change.

---

### WR-06: 5xx test in IsInRegion uses problem+json content-type but does not check Message

**File:** `client_isinregion_test.go:181-198`
**Issue:** The "transport error from Subdivisions surfaces verbatim" subtest sets `Content-Type: application/problem+json` and writes `{"title": "Internal Server Error"}`. The test asserts `errors.As(err, &apiErr)` and `apiErr.StatusCode == 500` but does NOT assert `apiErr.Message == "Internal Server Error"`. If `parseAPIMessage` silently regressed and returned "" for valid problem+json bodies, this test would still pass — but every sibling endpoint test (`countries_test.go:120-126`, `languages_test.go:137-156`, `public_holidays_test.go:184-208`, `school_holidays_test.go:188-212`, `subdivisions_test.go:185-205`) DOES assert the title-fallback message. The IsInRegion test alone leaves the message-parse path uncovered when surfaced through `IsInRegion` → `Subdivisions` → `doJSONGet`.

**Fix:** Add the title-fallback assertion symmetric with sibling tests:

```go
assert.Equal(t, "Internal Server Error", apiErr.Message,
    "title must win when detail is absent")
assert.Equal(t, "/Subdivisions", apiErr.Path,
    "Path must be /Subdivisions (inner Subdivisions call) not /IsInRegion")
```

## Info

### IN-01: `cap` loop variable shadows Go's built-in `cap()` function

**File:** `update_fixtures_test.go:191-258`
**Issue:** Even after removing the redundant `cap := cap` line (see WR-05), the loop variable name `cap` shadows the built-in `cap()` function across the entire loop body. While the body does not currently invoke `cap()`, any future maintenance would face a footgun. Tracked separately from WR-05 because WR-05's recommended fix (rename to `capture`) closes this issue as a side effect.

**Fix:** Subsumed by WR-05. Rename `cap` to `capture` throughout the for-range body.

---

### IN-02: Dead-code defense-in-depth comments on unreachable helpers

**File:** `validate.go:123-143` (out of Phase 3 scope, surfacing as it ships with the package)
**Issue:** `isTwoASCIIUppers` and `isTwoASCIILowers` are marked "Currently unreachable from validateCountry/validateLanguage after the W-01 reorder; retained as defense-in-depth and for direct testing." If they have no test callers either, `unused` / `deadcode` linters will flag them. The comment is well-intentioned but the linter does not read comments.

**Fix:** Not in Phase 3 scope. Flagged for the next phase to either add unit tests that exercise these helpers directly OR delete them (the W-01 fix means they are never re-introduced).

---

### IN-03: ctx-cancel ceiling in subdivisions_test.go too lax to detect a 100 ms regression

**File:** `subdivisions_test.go:265-266`
**Issue:** The assertion `assert.LessOrEqual(t, elapsed, 200*time.Millisecond, "cancellation must interrupt in-flight HTTP within ≤ 100 ms (CLIENT-09); slack budget 200 ms ...")` allows the call to take up to 200 ms after `cancel()` fires at 20 ms — that is 180 ms slack against a 100 ms contract. If a future change pushed cancellation to 150 ms, this test would still pass even though the contract is broken. Sibling tests (`languages_test.go:213` uses 1 s ceiling for an immediately-cancelled ctx; `countries_test.go:209` uses 200 ms but for a 50 ms cancel — same 150 ms slack pattern).

**Fix:** Tighten to e.g. 150 ms (20 ms cancel + 100 ms target + 30 ms scheduler slack). Or add a separate stricter assertion gated by a CI-only build tag if CI flake risk is the reason for the loose ceiling.

---

### IN-04: `t.Cleanup(srv.Close)` ordering vs body-Close drain in oversize test

**File:** `countries_test.go:235-238`
**Issue:** The streaming server's handler writes until `target = 11 << 20`. When the client closes the connection mid-stream, the server's `w.Write` returns an error, the handler returns (line 229: `if err != nil { return }`), and the goroutine exits. `t.Cleanup(srv.Close)` then runs in LIFO order after the test body completes. The 200 ms sleep at line 257 is to allow this teardown. Comment line 252-256 acknowledges "A1 explicitly allows empirical loosening; +5 detects any leak of 6+ goroutines." This is the same risk as WR-03 surfaced from a different angle — flagged here as info for awareness, not action.

**Fix:** Subsumed by WR-03's recommendation to switch to a counting-body-close approach, which removes the need for `time.Sleep` settling at all.

---

_Reviewed: 2026-05-27_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
