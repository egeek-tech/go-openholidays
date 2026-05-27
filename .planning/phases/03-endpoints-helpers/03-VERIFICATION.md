---
phase: 03-endpoints-helpers
verified: 2026-05-27T20:25:00Z
status: passed
score: 5/5 success criteria verified
overrides_applied: 0
human_verification: []
re_verification:
  previous_status: gaps_found
  previous_score: 4/5
  gaps_closed:
    - "SC2-COMBINED — single integrated subtest exercising all 4 ferie zimowe cohorts against IsInRegion('PL-SL') now exists"
    - "CR-01-FIXTURE-INDENT — update_fixtures_test.go writer now emits 4-space indent matching all six fixtures"
    - "CR-02-TRAILING-NEWLINE — update_fixtures_test.go writer appends '\\n' so output matches the trailing-newline convention"
    - "WR-01-RANGE-FIRST-YIELD — Holiday.Range() first iteration is now normalized through NewDate, matching the godoc contract"
    - "WR-05-CAP-SHADOW — `cap := cap` shadow removed; loop variable renamed to `c`"
  gaps_remaining: []
  regressions: []
gaps: []
---

# Phase 03: Endpoints & Helpers Verification Report (Re-verification)

**Phase Goal:** All four remaining endpoints (Languages, Subdivisions, PublicHolidays, SchoolHolidays) ship with golden-fixture tests; Holiday helpers (Name/NameFor, IsInRegion, Days, Range) return correct values for the verified Polish 2025 data.

**Verified:** 2026-05-27T20:25:00Z
**Status:** passed
**Re-verification:** Yes — after gap closure (plans 03-09, 03-10, 03-11)

## Verification Audit Trail

| Run | Date | Status | Score | Notes |
|-----|------|--------|-------|-------|
| 1 (initial) | 2026-05-27T19:31:11Z | gaps_found | 4/5 | 5 gaps identified: SC2-COMBINED, CR-01, CR-02, WR-01, WR-05 |
| 2 (re-verify) | 2026-05-27T20:25:00Z | **passed** | **5/5** | All 5 gaps closed by plans 03-09, 03-10, 03-11; no new gaps introduced |

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | `c.PublicHolidays(ctx, PublicHolidaysRequest{CountryIsoCode:"PL", ValidFrom:2025-01-01, ValidTo:2025-12-31})` against the golden fixture returns exactly 14 typed Holiday structs including Dec 24 Christmas Eve without panics or decode errors | VERIFIED | `testdata/public_holidays_pl_2025.json` has 14 entries; `TestClient_PublicHolidays/happy_path_PL_2025_returns_14_holidays_incl._Wigilia_2025-12-24` PASSES; Christmas Eve at `startDate: 2025-12-24` with `name=[{language:PL,text:Wigilia Bożego Narodzenia}]` |
| 2 | `c.SchoolHolidays(ctx, SchoolHolidaysRequest{CountryIsoCode:"PL",...})` against the golden fixture returns 7 periods, and `holiday.IsInRegion("PL-SL")` correctly identifies the Śląskie ferie zimowe cohort while excluding the other three regional cohorts | **VERIFIED (was PARTIAL)** | New `TestClient_SchoolHolidays_IsInRegion_FerieZimowe` (school_holidays_test.go:362) loads the fixture via httptest, filters to 4 Ferie zimowe entries, and asserts `IsInRegion("PL-SL")` per cohort in a single integrated scenario. 4 cohort subtests PASS: cohort_1 (2025-01-20..2025-02-02) → true; cohorts 2/3/4 → false. Fixture verified: 7 entries, exactly 4 "Ferie zimowe", subdivisions match the cohort assertions |
| 3 | `holiday.NameFor("pl")` returns the Polish localized name; `holiday.NameFor("xx")` falls back to the first available LocalizedText entry (not empty string) | VERIFIED | `TestHoliday_NameFor` PASSES: matches Polish case-insensitively returns `"Wigilia"`; falls back to first entry on miss returns `"Wigilia"`; returns empty on empty Name returns `""`. CL-10 documents the NameFor (not Name) rename |
| 4 | `holiday.Days()` returns 14 for the 14-day Śląskie ferie zimowe period; `holiday.Range()` iterates exactly 14 dates inclusively from StartDate to EndDate | VERIFIED | `TestHoliday_Days/14-day_ferie_zimowe_Śląskie_returns_14` PASSES; `TestHoliday_Range/14-day_ferie_zimowe_yields_14_Dates_inclusive` PASSES. CL-11 documents the iter.Seq[Date] deviation. NEW subtest `TestHoliday_Range/non-UTC_StartDate_yields_UTC-midnight_first_Date_(WR-01_regression)` PASSES — guards the godoc contract |
| 5 | Each endpoint has a table-driven unit test covering happy path + ≥4 error paths; all fixtures in `testdata/` come from captured live responses and a `-update` flag regenerates them | VERIFIED | Languages (7 subtests); Subdivisions (9); PublicHolidays (13); SchoolHolidays (11 + new 4-cohort SC#2 = 15). All 6 fixtures live-captured. `update_fixtures_test.go` ships `//go:build integration` + `-update` via `flag.Bool`. Writer now emits byte-identical output to on-disk format (4-space indent + trailing `\n`). All 6 fixtures uniform |

**Score:** 5/5 truths fully verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `languages.go` | Languages endpoint + LanguagesRequest type | VERIFIED | Unchanged from initial verification |
| `languages_test.go` | TestClient_Languages with table-driven subtests | VERIFIED | Unchanged |
| `testdata/languages.json` | Live-captured fixture (4-space, trailing newline) | VERIFIED | Format uniform with other 5 fixtures |
| `subdivisions.go` | Subdivisions endpoint + SubdivisionsRequest type | VERIFIED | Unchanged |
| `subdivisions_test.go` | TestClient_Subdivisions with subtests | VERIFIED | Unchanged |
| `testdata/subdivisions_pl.json` | 16 województwa flat (4-space, trailing newline) | VERIFIED | Format uniform |
| `testdata/subdivisions_de.json` | 16 Bundesländer with Children (4-space, trailing newline) | VERIFIED | Format uniform |
| `public_holidays.go` | PublicHolidays endpoint + PublicHolidaysRequest type | VERIFIED | Unchanged |
| `public_holidays_test.go` | TestClient_PublicHolidays + TestValidateHolidays | VERIFIED | Unchanged |
| `testdata/public_holidays_pl_2025.json` | 14 PL 2025 public holidays incl. Wigilia (4-space, trailing newline) | VERIFIED | Format uniform |
| `school_holidays.go` | SchoolHolidays endpoint + SchoolHolidaysRequest type | VERIFIED | Unchanged |
| `school_holidays_test.go` | TestClient_SchoolHolidays + **NEW** TestClient_SchoolHolidays_IsInRegion_FerieZimowe | **VERIFIED (UPDATED)** | 450 lines; NEW SC#2-integrated function at line 362 with 4 cohort subtests + `formatCohortName` helper at line 443; explicit CL-14 godoc reference |
| `testdata/school_holidays_pl_2025.json` | 7 PL 2025 school periods incl. ferie zimowe PL-SL (4-space, trailing newline) | **VERIFIED (RE-INDENTED)** | Format re-indented from 2-space to 4-space; semantic content unchanged (SHA-256 of sort_keys JSON = b82894877e6442a2c57dcbfe77cb158e8de624dd4067c504dd8592d0908ee6e9 before == after per 03-09-SUMMARY) |
| `holiday.go` | Holiday value methods: NameFor, IsInRegion, Days, Range (with WR-01 fix) | **VERIFIED (UPDATED)** | Line 119 now reads `d := NewDate(h.StartDate.Year(), h.StartDate.Month(), h.StartDate.Day())` — first-iteration normalization through NewDate matches subsequent iterations and matches the godoc contract |
| `holiday_test.go` | TestHoliday_NameFor + TestHoliday_IsInRegion + TestHoliday_Days + TestHoliday_Range (with NEW WR-01 regression subtest) | **VERIFIED (UPDATED)** | 241 lines; TestHoliday_Range now has 6 subtests including new `non-UTC StartDate yields UTC-midnight first Date (WR-01 regression)` at line 213; uses `time.FixedZone("CET", 3600)` |
| `client_isinregion.go` | Client.IsInRegion + splitCountryFromSubdivision + buildParentIndex | VERIFIED | Unchanged |
| `client_isinregion_test.go` | TestClient_IsInRegion | VERIFIED | Unchanged |
| `update_fixtures_test.go` | Build-tagged integration test with -update flag (with CR-01, CR-02, WR-05 fixes) | **VERIFIED (UPDATED)** | Line 230 `json.Indent(&pretty, body, "", "    ")` (4-space, was 2-space); line 231 `pretty.WriteByte('\n')` (new trailing newline append); line 194 `for _, c := range captures` (was `for _, cap := range captures` with `cap := cap` shadow); `type capture struct` declaration preserved |
| `testdata/countries.json` | Live-captured fixture | **VERIFIED (RE-INDENTED)** | Re-indented from 2-space to 4-space (matches post-fix writer); semantic content unchanged (SHA-256 stable per 03-09-SUMMARY) |
| `.planning/PROJECT.md` | Key Decisions table with **NEW** CL-14 row | **VERIFIED (UPDATED)** | Line 104 contains CL-14 row referencing `TestClient_SchoolHolidays_IsInRegion_FerieZimowe`, `SC2-COMBINED`, and "Scope: THIS test only" — narrow exception properly recorded |
| `request.go` | `doJSONGet[T any]` + `validateHolidays` + moved helpers | VERIFIED | Unchanged |
| `errors.go` | `ErrMalformedResponse` seventh sentinel | VERIFIED | Unchanged |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `school_holidays_test.go TestClient_SchoolHolidays_IsInRegion_FerieZimowe` | `testdata/school_holidays_pl_2025.json` | `os.ReadFile` + httptest.NewServer | VERIFIED | Line 365: `os.ReadFile(filepath.Join("testdata", "school_holidays_pl_2025.json"))`; httptest server at lines 369-373 |
| `school_holidays_test.go SC2 subtest` | `holiday.go Holiday.IsInRegion` | direct method call inside the loop body | VERIFIED | Line 432: `got := h.IsInRegion("PL-SL")` — assertion lives INSIDE the function body |
| `holiday.go Range first-iteration` | `date.go NewDate` | explicit normalization | VERIFIED | Line 119: `d := NewDate(h.StartDate.Year(), h.StartDate.Month(), h.StartDate.Day())` |
| `holiday_test.go WR-01 subtest` | `holiday.go Range first-iteration normalization` | hand-built non-UTC Holiday yields UTC-midnight first Date | VERIFIED | Line 213-240: `time.FixedZone("CET", 3600)` constructs non-UTC StartDate; `first.Location() == time.UTC` asserted |
| `update_fixtures_test.go json.Indent` | `testdata/*.json` on-disk format | 4-space indent string | VERIFIED | Line 230: `json.Indent(&pretty, body, "", "    ")` |
| `update_fixtures_test.go pretty buffer` | `os.Rename` atomic write | trailing `\n` appended | VERIFIED | Line 231: `pretty.WriteByte('\n')` between `json.Indent` and the `if !*updateFixtures` branch |
| `.planning/PROJECT.md CL-14 row` | `school_holidays_test.go TestClient_SchoolHolidays_IsInRegion_FerieZimowe` | Key Decisions row names test + cites SC2-COMBINED + scope-limit | VERIFIED | PROJECT.md line 104 contains literal function name, `SC2-COMBINED`, and "Scope: THIS test only" |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|-------------------|--------|
| `school_holidays_test.go SC2 subtest` | `ferieZimowe []Holiday` | `c.SchoolHolidays` decoding fixture bytes served by httptest | Yes — 4 Holiday values with subdivisions populated; cohort 0 has PL-SL, others do not | FLOWING |
| `holiday.go Range (post-WR-01 fix)` | first yielded `Date` | `NewDate(h.StartDate.Year(), h.StartDate.Month(), h.StartDate.Day())` | Yes — UTC-midnight Date even for non-UTC StartDate | FLOWING |
| `update_fixtures_test.go pretty buffer` | `pretty.Bytes()` for drift-detect or atomic write | `json.Indent(..., "    ")` + `WriteByte('\n')` of live response body | Yes — byte-identical to committed fixture format | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Full test suite with race detector | `go test -race -count=1 ./...` | `ok github.com/egeek-tech/go-openholidays 1.951s` | PASS |
| `go build ./...` | `go build ./...` | exit 0, no output | PASS |
| `go vet ./...` | `go vet ./...` | exit 0, no output | PASS |
| Integration build clean | `go vet -tags=integration ./... && go build -tags=integration ./...` | exit 0, no output (post-WR-05 rename compiles cleanly) | PASS |
| SC2-COMBINED integrated test | `go test -race -count=1 -v -run TestClient_SchoolHolidays_IsInRegion_FerieZimowe ./...` | 4 cohort subtests PASS: cohort_1 matches PL-SL, cohorts 2-4 exclude PL-SL | PASS |
| WR-01 regression subtest | `go test -race -count=1 -v -run 'TestHoliday_Range/non-UTC' ./...` | PASS (was UNRUNNABLE before plan 03-10) | PASS |
| TestHoliday_Range subtest count = 6 | `go test -race -count=1 -v -run TestHoliday_Range ./...` | 6 subtests PASS (was 5 before plan 03-10) | PASS |
| Fixture format uniformity | bash loop over 6 fixtures checking `^    [^ ]` second-line + final byte `0a` | All 6 fixtures: 4-space indent + trailing newline | PASS |
| `testdata/school_holidays_pl_2025.json` semantic content | python3 fixture parse | 7 entries; 4 "Ferie zimowe" cohorts; only cohort 0 has PL-SL subdivision | PASS |
| `update_fixtures_test.go` writer indent | `grep -nF 'json.Indent(&pretty, body, "", "    ")' update_fixtures_test.go` | exactly 1 match at line 230 | PASS |
| `update_fixtures_test.go` trailing newline | `grep -nF "pretty.WriteByte('\\n')" update_fixtures_test.go` | exactly 1 match at line 231 | PASS |
| `update_fixtures_test.go` cap shadow removed | `grep -nE '^\s*cap := cap' update_fixtures_test.go` | no lines | PASS |
| `update_fixtures_test.go` loop variable renamed | `grep -nE 'for _, c := range captures' update_fixtures_test.go` | exactly 1 line at 194 | PASS |
| `holiday.go` WR-01 fix in place | `grep -cE 'd := NewDate\(h\.StartDate\.Year\(\), h\.StartDate\.Month\(\), h\.StartDate\.Day\(\)\)' holiday.go` | 1 (at line 119) | PASS |
| `holiday.go` WR-01 old line gone | `grep -cE '^\s*d := h\.StartDate\s*$' holiday.go` | 0 | PASS |
| PROJECT.md CL-14 row present | `grep -cE '^\| CL-14:' .planning/PROJECT.md` | 1 (at line 104) | PASS |
| PROJECT.md scope-limit wording | grep "THIS test only" in CL-14 row | found | PASS |

### Probe Execution

Step 7c: SKIPPED — no `scripts/*/tests/probe-*.sh` files defined for this phase. The integration-tagged `update_fixtures_test.go` drift-detection mode requires live network access (`OPENHOLIDAYS_LIVE=1`) and is documented as a manual follow-up in 03-09-SUMMARY.md; static verification has already proven the writer output bytes will match the committed fixture format.

### Requirements Coverage

| Requirement | Description | Status | Evidence |
|-------------|-------------|--------|---------|
| ENDPT-02 | `Languages(ctx) ([]Language, error)` | SATISFIED | `languages.go` + TestClient_Languages (unchanged from initial verification) |
| ENDPT-03 | `Subdivisions(ctx, country, lang) ([]Subdivision, error)` | SATISFIED | `subdivisions.go` + TestClient_Subdivisions (unchanged) |
| ENDPT-04 | `PublicHolidays(ctx, PublicHolidaysRequest) ([]Holiday, error)` | SATISFIED | `public_holidays.go` + 13 subtests pass (unchanged) |
| ENDPT-05 | `SchoolHolidays(ctx, SchoolHolidaysRequest) ([]Holiday, error)` | SATISFIED | `school_holidays.go` + 11 original + new 4-cohort SC#2 integrated subtest pass |
| HELP-01 | `Holiday.Name(lang) string` with fallback | SATISFIED | Implemented as `Holiday.NameFor(lang)` per CL-10 (unchanged) |
| HELP-02 | `Holiday.IsInRegion(subdivisionCode) bool` + Client.IsInRegion hierarchical | SATISFIED | Both flat and hierarchical variants exist. SC2-COMBINED test now exercises IsInRegion against the golden fixture in one integrated scenario |
| HELP-03 | `Holiday.Days() int` inclusive day count | SATISFIED | Returns 14 for Śląskie ferie zimowe Jan 18–31 (unchanged) |
| HELP-04 | `Holiday.Range()` (iter.Seq) | SATISFIED | Returns `iter.Seq[Date]` per CL-11; **WR-01 fix** ensures first iteration is UTC-midnight matching the godoc contract; regression subtest gates the fix |
| TEST-01 | Unit tests per endpoint: happy path + ≥4 error paths | SATISFIED | Languages 4+; Subdivisions 4+; PublicHolidays 5+; SchoolHolidays 5+ (unchanged) |
| TEST-02 | No live network in unit tests — `httptest.NewServer` only | SATISFIED | `update_fixtures_test.go` build-tagged with `//go:build integration`; default `go test ./...` makes no live calls |
| TEST-03 | Golden fixtures + `-update` flag regeneration | SATISFIED | All 6 fixtures present, all 4-space + trailing newline; `-update` mechanism repaired (CR-01, CR-02 closed); writer-to-on-disk byte-equivalence now holds |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (none) | – | – | – | All anti-patterns from initial verification (CR-01, CR-02, WR-01, WR-05) closed by plans 03-09 and 03-10 |

**No debt markers (TBD/FIXME/XXX/TODO/HACK/PLACEHOLDER) found in any phase-modified file** (`holiday.go`, `holiday_test.go`, `school_holidays_test.go`, `update_fixtures_test.go`).

### Gap Closure Audit

Each of the 5 gaps from the initial verification is independently confirmed closed:

| Gap ID | Severity | Plan | Verification |
|--------|----------|------|--------------|
| SC2-COMBINED | medium | 03-11 | NEW `TestClient_SchoolHolidays_IsInRegion_FerieZimowe` at `school_holidays_test.go:362` exercises 4 cohorts in one integrated scenario; all 4 cohort subtests pass under `go test -race -count=1 -v -run TestClient_SchoolHolidays_IsInRegion_FerieZimowe ./...`; cohort identity assertions (StartDate/EndDate match) added as preconditions before the IsInRegion check so a fixture re-order fails loudly; PROJECT.md CL-14 records the Gold-Rule-3 narrow exception with scope-limit wording |
| CR-01-FIXTURE-INDENT | blocker | 03-09 | `update_fixtures_test.go:230` now uses `"    "` (4-space) indent; matches all 6 on-disk fixtures |
| CR-02-TRAILING-NEWLINE | blocker | 03-09 | `update_fixtures_test.go:231` appends `pretty.WriteByte('\n')` between `json.Indent` and the `if !*updateFixtures` branch; matches the trailing-newline convention of all 6 on-disk fixtures |
| WR-01-RANGE-FIRST-YIELD | warning | 03-10 | `holiday.go:119` now reads `d := NewDate(h.StartDate.Year(), h.StartDate.Month(), h.StartDate.Day())`; matches the godoc contract; NEW regression subtest using `time.FixedZone("CET", 3600)` proves the first yield is UTC-midnight |
| WR-05-CAP-SHADOW | warning | 03-09 | `update_fixtures_test.go:194` now reads `for _, c := range captures`; the `cap := cap` shadow line and its justifying comment are deleted; `type capture struct` declaration preserved; `go vet -tags=integration ./...` clean |

### Human Verification Required

None. The previous SC#2 human-UAT item is now satisfied automatically by `TestClient_SchoolHolidays_IsInRegion_FerieZimowe`; 03-HUMAN-UAT.md may be updated to mark the item as resolved (verification surface artifact only — not a code change).

### Gaps Summary

**No gaps remain.** All 5 gaps from the initial verification (1 medium SC#2 + 2 blockers + 2 warnings) are closed at the source level. The phase goal is fully achieved:

- All four endpoints (Languages, Subdivisions, PublicHolidays, SchoolHolidays) ship with golden-fixture tests and pass under `go test -race -count=1`
- All four Holiday helpers (NameFor, IsInRegion, Days, Range) return correct values for PL 2025 data
- The SC#2 integrated assertion against the golden fixture exists and passes
- `Holiday.Range()` first-iteration UTC-midnight normalization matches the godoc contract; regression subtest gates the fix
- `update_fixtures_test.go` writer output bytes match the on-disk fixture format (4-space indent + trailing `\n`), all 6 fixtures uniform
- `cap` builtin-shadow gone; integration build clean
- Gold-Rule-3 narrow exception recorded in PROJECT.md Key Decisions as CL-14
- `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`, `go test -race -count=1 ./...` all exit 0
- No debt markers (TBD/FIXME/XXX/TODO/HACK/PLACEHOLDER) introduced in phase-modified files

### Tracking State

- **ROADMAP.md:** Phase 3 marked as Complete (`[x]`); plan count 11/11; Phase 4 row remains "Not started" with `0/0` plans — correct.
- **STATE.md:** Reports `completed_phases: 2` and current focus "Phase 03 — endpoints-helpers, EXECUTING". The roadmap progress table contradicts this (Phase 3 shown as Complete). STATE.md is stale — this is a tracking-file artifact, NOT a Phase 3 code/goal issue. Recommendation: STATE.md should be reconciled in the next roadmap-helper invocation. Not a gap because the source of truth (ROADMAP.md) and the codebase are aligned.
- **Worktree commits merged:** `8a79659` (CR-01/CR-02/WR-05 fix), `2157589` (fixture re-indent), `2584162` (WR-01 fix), `848f6c3` (WR-01 test), `473a2e4` (SC#2 test), `9eda5ec` (CL-14 docs), plus three worktree-merge commits and the wave-0 tracking-update commit `7a2d9bf`. All present in `git log`.

---

_Re-verified: 2026-05-27T20:25:00Z_
_Verifier: Claude (gsd-verifier)_
_Previous verification: 2026-05-27T19:31:11Z (status: gaps_found, 4/5 → now 5/5 passed)_
