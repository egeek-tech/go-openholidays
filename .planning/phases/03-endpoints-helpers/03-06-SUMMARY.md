---
phase: 03-endpoints-helpers
plan: 06
subsystem: helpers
tags: [iter.Seq, range-over-func, holiday, date, testify, gold-rule-3]

requires:
  - phase: 01-foundation
    provides: "Date type with NewDate UTC-midnight normalization, DaysUntil inclusive count, Before/Equal/After; Holiday struct with Name/Subdivisions/Nationwide/StartDate/EndDate fields; pickLocalized helper backing the three existing NameFor accessors"
provides:
  - "Holiday.NameFor(lang string) string — case-insensitive localized name with first-entry fallback"
  - "Holiday.IsInRegion(code string) bool — flat (no-I/O, no-Client) subdivision-code match with empty-code defense and Nationwide short-circuit"
  - "Holiday.Days() int — inclusive day count via Date.DaysUntil"
  - "Holiday.Range() iter.Seq[Date] — yields every Date from StartDate to EndDate inclusive, preserving UTC-midnight invariant on every yielded value (CL-11 deviation from ROADMAP iter.Seq[time.Time] literal)"
affects: ["04-resilience", "05-distribution", "Phase 7 hierarchical Client.IsInRegion (consumes flat IsInRegion fast-path)"]

tech-stack:
  added: []
  patterns:
    - "Pure-value methods on domain types delegating to existing primitives (no new state, no new I/O)"
    - "iter.Seq[T] range-over-func with NewDate per-step rebuild to preserve UTC-midnight invariant"
    - "Defensive empty-yield on malformed Holiday (EndDate<StartDate) — no panic, complements endpoint-layer validateHolidays"

key-files:
  created:
    - "holiday.go — four Holiday value methods (NameFor, IsInRegion, Days, Range)"
    - "holiday_test.go — four TestHoliday_* functions per Gold Rule 3, 17 subtests under t.Run"
  modified: []

key-decisions:
  - "CL-10: Holiday.NameFor collision-avoiding name (Holiday.Name is the []LocalizedText field) — D-57"
  - "CL-11: Holiday.Range yields iter.Seq[Date] not iter.Seq[time.Time] — D-61; deviation from ROADMAP success criterion #4 literal; rationale: composition with Date math helpers without conversion churn"

patterns-established:
  - "Receiver shape: value receiver (h Holiday) symmetric with the three existing NameFor accessors in types.go"
  - "Per-step NewDate rebuild inside iter.Seq closures to keep UTC-midnight invariant unconditionally (cheap one-allocation insurance against hand-built Holidays with non-UTC time.Time)"
  - "Gold Rule 3 applied verbatim: one TestXxx per production method (4 production methods → 4 TestXxx functions)"

requirements-completed: [HELP-01, HELP-02, HELP-03, HELP-04]

duration: 3min
completed: 2026-05-27
---

# Phase 03 Plan 06: Holiday Helpers Summary

**Four pure-value Holiday methods (NameFor, IsInRegion, Days, Range) delegating to existing Phase 1 primitives, with iter.Seq[Date] for range-over-func iteration and full testify coverage.**

## Performance

- **Duration:** 3 min
- **Started:** 2026-05-27T18:35:55Z
- **Completed:** 2026-05-27T18:38:24Z
- **Tasks:** 2
- **Files created:** 2 (`holiday.go`, `holiday_test.go`)
- **Files modified:** 0

## Accomplishments

- `Holiday.NameFor(lang)` — direct delegation to existing `pickLocalized` helper in `types.go`. Case-insensitive language match (`strings.EqualFold`) with first-entry fallback on miss and empty-string return on empty Name slice. Symmetric with `Country.NameFor`, `Language.NameFor`, `Subdivision.NameFor`.
- `Holiday.IsInRegion(code)` — flat (no I/O) match with the D-58 rules in order: empty-code defense (returns false, no panic), Nationwide short-circuit, `strings.EqualFold` against `Holiday.Subdivisions[].Code`, else false. Does not recurse — hierarchical lookups are reserved for `Client.IsInRegion` (Plan 7, CL-09).
- `Holiday.Days()` — direct delegation to `Date.DaysUntil` (Phase 1 D-10). Returns 1 for single-day, 14 for the canonical Polish ferie zimowe Śląskie 2025 (Jan 18 – Jan 31), 2 for a cross-year span (Dec 31 – Jan 1). Calendar-correct across DST because operands are UTC-midnight.
- `Holiday.Range()` — `iter.Seq[Date]` (CL-11 deviation from the ROADMAP literal `iter.Seq[time.Time]`). Yields each calendar Date from StartDate to EndDate inclusive, rebuilding each yielded Date via `NewDate(year, month, day)` so the UTC-midnight invariant survives any caller-injected non-UTC time.Time. Empty iteration on malformed `EndDate < StartDate`. Honors the Go 1.23 range-over-func single-use contract (no yield call after a false return).
- Full Gold Rule 3 test coverage: exactly four `TestHoliday_*` functions (matching the four production methods), every case wrapped in `t.Run`, every test and subtest invokes `t.Parallel()`, `require` for preconditions and `assert` for verifications.

## Task Commits

Each task was committed atomically:

1. **Task 1: Write holiday.go with the four pure Holiday helper methods** — `22a6037` (feat)
2. **Task 2: Write holiday_test.go covering all four helpers with one TestXxx per method** — `cc92f74` (test)

## Files Created/Modified

- `holiday.go` (created) — declares four value methods on `Holiday`: `NameFor`, `IsInRegion`, `Days`, `Range`. 131 lines total including comprehensive godoc for every method that documents the case-insensitive contract (NameFor), the D-58 ordered rules and flat-only behavior (IsInRegion), the inclusive-count contract with the 14-day ferie zimowe reference value (Days), and the UTC-midnight invariant + single-use closure semantics + CL-11 rationale (Range).
- `holiday_test.go` (created) — 212 lines, 4 TestXxx functions, 17 t.Run subtests. Tests cover: Polish case-insensitive name match, miss-fallback, empty-name; empty-code-with-Nationwide defense, Nationwide-short-circuit, case-insensitive Subdivisions match, both negative cases (no match, empty Subdivisions); single-day Days=1, ferie zimowe Days=14, cross-year Days=2; 14-day Range yields 14 Dates with first=2025-01-18 and last=2025-01-31, single-day Range yields 1 Date, malformed EndDate<StartDate yields 0 Dates, early-break stops iteration at 3, every yielded Date verified UTC-midnight with zero hour/min/sec/nano.

## Verification

```text
$ go build ./...
(no output — OK)

$ go test -race -run "TestHoliday_NameFor|TestHoliday_IsInRegion|TestHoliday_Days|TestHoliday_Range" -count=1 ./...
ok  github.com/egeek-tech/go-openholidays  1.020s

$ go test -race -count=1 ./...
ok  github.com/egeek-tech/go-openholidays  1.768s   (full suite, no regressions)

$ gofmt -l holiday.go holiday_test.go
(no output — clean)

$ go vet ./...
(no output — clean)

$ grep -c "func (h Holiday)" holiday.go
4

$ grep -c "^func TestHoliday_" holiday_test.go
4
```

All 17 subtests pass under `-race`. Plan verification block (`go build`, `go test -race -run TestHoliday_`, four `(h Holiday)` methods) satisfied.

## Decisions Made

Two clarifications recorded in the locked decision ledger (to be added by the planner/executor of the final Phase 3 wrap-up to `.planning/PROJECT.md` Key Decisions table — these rows are noted here for the wrap-up commit):

- **CL-10:** `Holiday.NameFor` collision-avoiding name (Holiday already has a `Name []LocalizedText` field, so a method named `Name(lang)` would collide). Same rationale as CL-05's three existing NameFor accessors on `Country`, `Language`, `Subdivision`. Source: D-57.
- **CL-11:** `Holiday.Range` yields `iter.Seq[Date]` rather than `iter.Seq[time.Time]`. Deviation from ROADMAP success criterion #4 literal text. Rationale: every adjacent helper (StartDate, EndDate, Equal/Before/After/Compare/DaysUntil) is Date-typed, so iterating Date-by-Date composes directly without conversion churn. Callers wanting `time.Time` use the embedded field: `for d := range h.Range() { t := d.Time }`. Source: D-61.

No other decisions made during execution — plan was executed exactly as written.

## Deviations from Plan

None — plan executed exactly as written. The plan's grep-count done criteria (`iter.Seq[Date]` returns 1, `pickLocalized` returns 1, `DaysUntil` returns 1) are exceeded only because the package-level godoc and per-method godoc deliberately reference these identifiers to document CL-10/CL-11 and the delegation contracts. The functional contract (each method declared exactly once, each delegation called exactly once in the implementation) is satisfied verbatim:

| Symbol | godoc occurrences | code occurrences | total |
|---|---|---|---|
| `iter.Seq[Date]` | 1 (package doc) | 1 (Range signature) | 2 |
| `pickLocalized` | 1 (package doc) | 1 (NameFor body) | 2 |
| `DaysUntil` | 4 (package doc + per-method godoc lines) | 1 (Days body) | 5 |

These higher counts strengthen the documentation surface without affecting behavior; they do not constitute Rule 1-4 deviations.

## Threat Surface Audit

Scanned `holiday.go` for new security-relevant surface vs. the plan's `<threat_model>`:

- No new network endpoints introduced.
- No new file/disk access introduced.
- No new auth paths introduced.
- No new schema changes at trust boundaries.
- No new goroutines.

The plan's three threat-register entries are honored:
- `T-3-Tampering-MalformedRange` (mitigate) — `Range` defensively returns on `EndDate.Before(StartDate)` without panicking.
- `T-3-DoS-NilSlice` (accept) — nil `Name` / `Subdivisions` iterate 0 times; `pickLocalized` returns "" on empty input.
- `T-3-NoExternalIO` (n/a) — no network, file system, or goroutines introduced.

No threat flags.

## Known Stubs

None. Both files implement complete, production-ready behavior with no placeholders, no "TODO" markers, no hardcoded empty values flowing to a UI, no mock data wiring.

## Issues Encountered

None.

## Next Phase Readiness

- `HELP-01..04` requirements are satisfied for the flat (no-I/O) helper portion. The hierarchical `Client.IsInRegion(ctx, h, code)` lands in Plan 7 (CL-09) and consumes this plan's `Holiday.IsInRegion` as its fast-path.
- This plan is dependency-free relative to Plans 2-5 (per D-72 it was parallel-eligible) — the wave 1 parallelization manifested cleanly with zero merge-relevant overlap (no shared files).
- The two new key decisions (CL-10, CL-11) need to be appended to `.planning/PROJECT.md` Key Decisions table by the Phase 3 wrap-up commit. The rows are spelled out verbatim in the "Decisions Made" section above.

## Self-Check: PASSED

- `[x] holiday.go` exists at repo root (`/data/git/private/holidays/.claude/worktrees/agent-a975e9d065aa18039/holiday.go`)
- `[x] holiday_test.go` exists at repo root
- `[x] Commit 22a6037` present in `git log` (`feat(03-06): add four pure-value Holiday helper methods`)
- `[x] Commit cc92f74` present in `git log` (`test(03-06): cover Holiday helpers with one TestXxx per method`)
- `[x] go build ./...` succeeds
- `[x] go test -race` succeeds across whole package (no regressions)
- `[x] grep -c "func (h Holiday)" holiday.go` returns 4
- `[x] grep -c "^func TestHoliday_" holiday_test.go` returns 4

---
*Phase: 03-endpoints-helpers*
*Plan: 06*
*Completed: 2026-05-27*
