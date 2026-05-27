---
phase: 03-endpoints-helpers
plan: 11
subsystem: testing
tags: [go, gap-closure, characterization-test, school-holidays, isinregion, ferie-zimowe, fixture, gold-rule-3, key-decisions]

requires:
  - phase: 03-endpoints-helpers
    provides: "testdata/school_holidays_pl_2025.json fixture (captured 2026-05-27) — 7 entries including 4 Ferie zimowe cohorts"
  - phase: 03-endpoints-helpers
    provides: "Client.SchoolHolidays endpoint + SchoolHolidaysRequest (Plan 03-05)"
  - phase: 03-endpoints-helpers
    provides: "Holiday.IsInRegion(code) flat helper (Plan 03-06)"
  - phase: 01-foundation
    provides: "Date.Format embedded from time.Time; Date.Equal calendar-correct comparison"
provides:
  - "TestClient_SchoolHolidays_IsInRegion_FerieZimowe in school_holidays_test.go — SC#2-integrated subtest exercising all 4 ferie zimowe cohorts against Holiday.IsInRegion(\"PL-SL\") in a single test scenario against the golden fixture"
  - "formatCohortName package-private helper for stable, human-readable cohort subtest names"
  - "CL-14 entry in .planning/PROJECT.md Key Decisions — Gold-Rule-3 narrow exception with explicit scope-limit wording"
affects: [04-resilience, 05-cli-release]

tech-stack:
  added: []  # test-only + docs; no runtime or new test deps
  patterns:
    - "Characterization test pattern: single integrated assertion against a golden fixture covering both positive and negative paths in one scenario, rather than compositional proof across separate test functions"
    - "Cohort-identity check (StartDate/EndDate match) BEFORE the behavioral assertion (IsInRegion) — fixture re-order fires loudly instead of silently testing the wrong cohort"
    - "Subtest naming via fmt.Sprintf'd helper that encodes index + date window + expected outcome — CI failure points at the exact regressed cohort without ambiguity"
    - "Narrow Gold-Rule-3 exception pattern: documented in TWO places (function godoc near the test + CL-14 in PROJECT.md), scope-limited to a single function, named explicitly, with rationale traceable to the originating gap_id"

key-files:
  created: []
  modified:
    - "school_holidays_test.go — added fmt import; appended new top-level TestClient_SchoolHolidays_IsInRegion_FerieZimowe (4 cohort subtests) and formatCohortName helper (111 insertions)"
    - ".planning/PROJECT.md — appended CL-14 row to Key Decisions table (1 insertion)"

key-decisions:
  - "CL-14 recorded: narrow Gold-Rule-3 exception for TestClient_SchoolHolidays_IsInRegion_FerieZimowe — explicit scope-limit prevents future generalization"
  - "Test placed as a NEW top-level function (not as a t.Run inside TestClient_SchoolHolidays) because VERIFICATION.md expected_after_fix names the function explicitly as the -run target"
  - "Cohort identity assertions (StartDate/EndDate) added BEFORE the IsInRegion call so a fixture re-order or upstream re-shuffle fires a precise failure instead of silently testing a different cohort"
  - "Used start.Format(\"2006-01-02\") (Date embeds time.Time.Format) for cohort subtest names — matches existing codebase pattern; no new code paths needed"

patterns-established:
  - "Pattern: SC-integrated characterization tests live in the endpoint test file (not the helper test file) and exercise the endpoint round-trip + helper composition in a single scenario"
  - "Pattern: documented exceptions to project Gold Rules require TWO discovery surfaces — godoc on the affected code AND a Key Decisions ledger row referencing the source gap_id"
  - "Pattern: cohort-identity precondition assertions (date-window match) in regression tests against re-orderable fixtures"

requirements-completed:
  - HELP-02  # Holiday.IsInRegion now has a single-scenario integrated proof against the golden school-holidays fixture
  - ENDPT-05 # SchoolHolidays endpoint exercised via the new integrated test (additive, no behavior change)

duration: ~12min
completed: 2026-05-27
---

# Phase 03 Plan 11: SC#2 Integrated Ferie Zimowe Characterization Test + CL-14 Summary

**Closes the SC2-COMBINED gap from 03-VERIFICATION.md by adding `TestClient_SchoolHolidays_IsInRegion_FerieZimowe` — a single integrated subtest that exercises `Holiday.IsInRegion("PL-SL")` on all four Ferie zimowe cohorts from the golden PL 2025 school-holidays fixture (cohort 1 → true; cohorts 2/3/4 → false), and records the narrow Gold-Rule-3 exception as CL-14 in PROJECT.md Key Decisions.**

## Performance

- **Duration:** ~12 min
- **Started:** 2026-05-27T~19:59Z (worktree spawn)
- **Completed:** 2026-05-27T20:11:42Z
- **Tasks executed:** 2/2 (Task 1 = test, Task 2 = docs)
- **Commits:** 2 (one per task, atomic)

## Task Execution

### Task 1 — Add SC#2-integrated subtest (commit `473a2e4`)

Added a new top-level function in `school_holidays_test.go`:

```go
func TestClient_SchoolHolidays_IsInRegion_FerieZimowe(t *testing.T)
```

The function loads `testdata/school_holidays_pl_2025.json` via the SchoolHolidays endpoint (httptest server), filters the 7 fixture entries down to the 4 "Ferie zimowe" cohorts (via `h.NameFor("pl") == "Ferie zimowe"`), and runs 4 t.Run subtests — one per cohort — each asserting (a) the cohort StartDate/EndDate match the expected window and (b) `h.IsInRegion("PL-SL")` returns the expected boolean.

**Cohort subtests (1-based numbering per VERIFICATION.md SC2-COMBINED):**

| Subtest name | Cohort dates | Expected `IsInRegion("PL-SL")` | Result |
|--------------|--------------|-------------------------------|--------|
| `cohort_1_2025-01-20_to_2025-02-02_matches_PL-SL` | 2025-01-20 .. 2025-02-02 | `true`  | PASS |
| `cohort_2_2025-01-27_to_2025-02-09_excludes_PL-SL` | 2025-01-27 .. 2025-02-09 | `false` | PASS |
| `cohort_3_2025-02-03_to_2025-02-16_excludes_PL-SL` | 2025-02-03 .. 2025-02-16 | `false` | PASS |
| `cohort_4_2025-02-17_to_2025-03-02_excludes_PL-SL` | 2025-02-17 .. 2025-03-02 | `false` | PASS |

Verification gate:

```
go test -race -count=1 -v -run TestClient_SchoolHolidays_IsInRegion_FerieZimowe ./...
  --- PASS: TestClient_SchoolHolidays_IsInRegion_FerieZimowe/cohort_1_2025-01-20_to_2025-02-02_matches_PL-SL
  --- PASS: TestClient_SchoolHolidays_IsInRegion_FerieZimowe/cohort_2_2025-01-27_to_2025-02-09_excludes_PL-SL
  --- PASS: TestClient_SchoolHolidays_IsInRegion_FerieZimowe/cohort_3_2025-02-03_to_2025-02-16_excludes_PL-SL
  --- PASS: TestClient_SchoolHolidays_IsInRegion_FerieZimowe/cohort_4_2025-02-17_to_2025-03-02_excludes_PL-SL
PASS
ok  	github.com/egeek-tech/go-openholidays	1.017s
```

The function's godoc explicitly records the Gold-Rule-3 narrow exception (CL-14) and references the SC2-COMBINED gap_id for traceability.

A package-private helper `formatCohortName(idx, start, end, wantPLSL)` builds the subtest names from `fmt.Sprintf("cohort_%d_%s_to_%s_%s", ...)`. The `fmt` import was added to the file (it was not previously imported).

### Task 2 — Record CL-14 in PROJECT.md Key Decisions (commit `9eda5ec`)

Appended one row to the `## Key Decisions` table in `.planning/PROJECT.md` immediately after the existing CL-06 row. The full row text (single line in markdown source) is:

```
| CL-14: Narrow Gold-Rule-3 exception — a second top-level TestXxx tied to `Client.SchoolHolidays` (`TestClient_SchoolHolidays_IsInRegion_FerieZimowe` in `school_holidays_test.go`) is permitted ALONGSIDE the existing `TestClient_SchoolHolidays` in order to satisfy the literal ROADMAP SC#2 wording — "correctly identifies the Śląskie ferie zimowe cohort while excluding the other three regional cohorts" — as a single integrated test scenario against the golden fixture rather than a compositional proof split across `school_holidays_test.go` (fixture-has-the-entry) and `holiday_test.go` (IsInRegion-logic-is-correct). Scope: THIS test only — future SchoolHolidays-related tests must continue to live inside the single `TestClient_SchoolHolidays` t.Run tree per Gold Rule 3. | Gold Rule 3 ("one TestXxx per exported production function") has no documented exception mechanism in CLAUDE.md or PROJECT.md, so without this entry future maintainers would believe the rule is silently broken when they discover the second test function. Recording the narrow exception keeps Gold Rule 3 enforceable for every other test by making the one allowed exception explicit, named, and scope-limited. Source: `.planning/phases/03-endpoints-helpers/03-VERIFICATION.md` gap `SC2-COMBINED` + `.planning/phases/03-endpoints-helpers/03-11-PLAN.md` Task 1. | Adopted in Phase 3 (gap closure) |
```

Verification gates:

- `grep -cE '^\| CL-14:' .planning/PROJECT.md` → 1
- Row contains literal `TestClient_SchoolHolidays_IsInRegion_FerieZimowe`
- Row contains literal `SC2-COMBINED`
- Row contains scope-limit wording `THIS test only`
- CL-01..06 untouched (still 6 rows present); total CL-rows now 7
- `git diff --stat .planning/PROJECT.md` → `1 file changed, 1 insertion(+)` (single-line diff, exactly as required)
- Row has 4 column-edge `|` characters (= 3 columns) — markdown table renders cleanly

## Gap Closure — SC2-COMBINED

The gap raised by VERIFICATION.md SC2-COMBINED was that the SC#2 success criterion ("correctly identifies the Śląskie ferie zimowe cohort while excluding the other three regional cohorts") was satisfied only compositionally — `school_holidays_test.go` proved the fixture contains the PL-SL entry, and `holiday_test.go` proved the `IsInRegion` logic in isolation. No single test exercised both the positive (cohort 1) AND the three negative paths (cohorts 2/3/4) against the golden fixture in one scenario.

The new `TestClient_SchoolHolidays_IsInRegion_FerieZimowe` closes that gap: a single test function, loading the golden fixture, calling `IsInRegion("PL-SL")` on all 4 cohorts, asserting the expected outcome for each.

The expected_after_fix gate from VERIFICATION.md:

> `go test -race -count=1 -run TestClient_SchoolHolidays_IsInRegion_FerieZimowe ./... passes; subtest exercises all 4 cohorts.`

is now satisfied: the `-run` target exists, exits 0, and the 4-cohort-subtest pass count is exactly 4 (asserted by the Task 1 verify gate `grep -cE '^    --- PASS: .../cohort_'`).

## Gold-Rule-3 Narrow Exception

CLAUDE.md Gold Rule 3 states "exactly one TestXxx per exported production function". `Client.SchoolHolidays` previously had one — `TestClient_SchoolHolidays`. This plan adds a second, `TestClient_SchoolHolidays_IsInRegion_FerieZimowe`, but only because the gap closure required honoring the VERIFICATION.md expected_after_fix function name verbatim (the orchestrator's gap-closure check uses that exact `-run` target).

To prevent this exception from generalizing:

1. The function godoc on `TestClient_SchoolHolidays_IsInRegion_FerieZimowe` explicitly names the exception (CL-14), the source gap (SC2-COMBINED), and the scope-limit wording ("THIS test only").
2. The CL-14 row in PROJECT.md Key Decisions records the same exception, in the project's authoritative decisions ledger, with the same scope-limit wording.
3. Both locations name the exact function being excepted — no other test gets the same allowance.

A future maintainer who discovers the second test function and asks "did someone silently break Gold Rule 3?" will find the answer in either the function godoc or the PROJECT.md ledger.

## Files Changed

| File | Change | Insertions | Deletions | Commit |
|------|--------|------------|-----------|--------|
| `school_holidays_test.go` | Added `fmt` import; appended `TestClient_SchoolHolidays_IsInRegion_FerieZimowe` (4 cohort subtests with cohort-identity precondition + `IsInRegion("PL-SL")` assertion) and `formatCohortName` helper | 111 | 0 | `473a2e4` |
| `.planning/PROJECT.md` | Appended CL-14 row to `## Key Decisions` table (Gold-Rule-3 narrow exception with scope-limit wording) | 1 | 0 | `9eda5ec` |

Total: 2 files modified, 112 insertions, 0 deletions.

## Verification

| Check | Result |
|-------|--------|
| `go vet ./...` | exit 0, clean |
| `go build ./...` | exit 0, clean |
| `go test -race -count=1 ./...` | exit 0, all tests pass |
| `go test -race -count=1 -run TestClient_SchoolHolidays_IsInRegion_FerieZimowe ./...` | exit 0, 4 cohort subtests PASS |
| `grep -cE 'func TestClient_SchoolHolidays_IsInRegion_FerieZimowe\(t \*testing\.T\)' school_holidays_test.go` | 1 |
| `grep -cE 'func TestClient_SchoolHolidays\(t \*testing\.T\)' school_holidays_test.go` | 1 (existing test preserved) |
| `awk '/^func TestClient_SchoolHolidays_IsInRegion_FerieZimowe/,/^}/' school_holidays_test.go \| grep -cE '\.IsInRegion\("PL-SL"\)'` | 1 (assertion lives inside the new function body) |
| `grep -cE '^\| CL-14:' .planning/PROJECT.md` | 1 |
| `grep -cE '^\| CL-0[1-6]:' .planning/PROJECT.md` | 6 (existing CL-01..06 untouched) |
| `grep -cE '^\| CL-' .planning/PROJECT.md` | 7 (six pre-existing + new CL-14) |
| CL-14 row contains `TestClient_SchoolHolidays_IsInRegion_FerieZimowe` | yes |
| CL-14 row contains `SC2-COMBINED` | yes |
| CL-14 row contains scope-limit wording `THIS test only` | yes |
| CL-14 row has 4 column-edge `\|` (= 3 columns) | yes |

## Deviations from Plan

None — plan executed exactly as written. The plan's `<action>` block specified two near-equivalent placement options for the `Format` call (`start.Time.Format(...)` via the embedded `time.Time` field vs. `start.Format(...)` via the embedded method); I chose `start.Format("2006-01-02")` because `Date` embeds `time.Time` and the embedded `Format(layout)` method works directly on the `Date` receiver — matching the existing codebase pattern. This was an in-scope option presented by the plan, not a deviation.

The plan also noted that `fmt` was not currently imported by `school_holidays_test.go`; I verified that by grep before writing and added the import. The `fmt` import addition is in-scope per the plan ("Add `\"fmt\"` to the existing import block at the top of school_holidays_test.go").

## Known Stubs

None.

## Threat Flags

None — this plan adds a test and a docs entry. No new network endpoints, auth paths, file access patterns, or schema changes at trust boundaries.

## Self-Check: PASSED

- school_holidays_test.go modified — FOUND (113 lines added, contains the new function and the `formatCohortName` helper)
- .planning/PROJECT.md modified — FOUND (CL-14 row appended after CL-06)
- Commit 473a2e4 — FOUND in git log
- Commit 9eda5ec — FOUND in git log
- Full test suite green — VERIFIED
- 4 cohort subtests pass under exact VERIFICATION.md `-run` target — VERIFIED

---

_Plan 03-11 SC2-COMBINED gap closure complete. Test commit `473a2e4`; docs commit `9eda5ec`._
