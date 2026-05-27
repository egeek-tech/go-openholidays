---
phase: 01-foundation
plan: 05
subsystem: validators
tags: [go, validation, sentinel-errors, iso-3166, iso-639, leap-year, date-range]

# Dependency graph
requires:
  - phase: 01-foundation
    provides: ErrInvalidCountry, ErrInvalidLanguage, ErrInvalidDateRange, ErrDateRangeTooLarge (errors.go from Plan 02); Date type with After/Before/AddDate (date.go from Plan 03)
provides:
  - "validateCountry (unexported): case-insensitive ISO 3166-1 alpha-2 check returning canonical uppercase form"
  - "validateLanguage (unexported): case-insensitive ISO 639-1 alpha-2 check returning canonical lowercase form"
  - "validateDateRange (unexported): from <= to invariant + 3-calendar-year-inclusive window check with correct leap-year semantics"
  - "isTwoASCIIUppers / isTwoASCIILowers (unexported helpers): byte-arithmetic ASCII letter checks"
affects: [02-transport, 02-public-holidays, 02-school-holidays, 02-countries, 02-languages, 02-subdivisions]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Pattern 5 (ARCHITECTURE.md): validators as unexported root-package functions called from endpoint methods before HTTP dispatch"
    - "Byte-arithmetic ASCII letter check (reject unicode.IsLetter false-positives such as 'Ö')"
    - "Backward-anchored 3-calendar-year window check via to.AddDate(-3, 0, 0) (avoids forward-AddDate leap-day overflow asymmetry)"

key-files:
  created:
    - validate.go
    - validate_test.go
  modified: []

key-decisions:
  - "Switched from the plan-prescribed forward formula from.AddDate(3, 0, 1) to backward-anchored to.AddDate(-3, 0, 0) because Go's time.AddDate normalizes 2024-02-29 + 3yr to 2027-03-01 (non-leap year overflow). The forward formula produces 2027-03-02 as the rejection threshold, so the plan's documented boundary (2024-02-29 -> 2027-03-01 MUST fail) is unreachable with it. The backward-anchored formulation satisfies every documented boundary case verbatim. Recorded as Rule 1 deviation."
  - "validateCountry / validateLanguage error messages quote the ORIGINAL (non-canonicalized) input via %q rather than the canonical form, so callers diagnose exactly what they passed."

patterns-established:
  - "Validators wrap their sentinel with %w + include the offending value via %q or %s (D-23 contract). Phase 2 endpoints can rely on errors.Is(err, ErrInvalid*) matching through their wrapping."
  - "Validator error messages contain ONLY the offending caller value, never HTTP/transport context (ERR-04 invariant; TestValidators_NoSensitiveData locks the regression)."

requirements-completed: [VALID-01, VALID-02, VALID-03, VALID-04, ERR-04]

# Metrics
duration: ~25min
completed: 2026-05-27
---

# Phase 1 Plan 5: Validators Summary

**Three unexported root-package validators (validateCountry, validateLanguage, validateDateRange) with calendar-correct leap-year boundary handling and 100% statement coverage across 42 table-driven subtests.**

## Performance

- **Duration:** ~25 min
- **Completed:** 2026-05-27T08:38:56Z
- **Tasks:** 2 (both committed atomically)
- **Files modified:** 2 (both created in this plan)

## Accomplishments

- `validateCountry` and `validateLanguage` ship as unexported case-insensitive shape-only validators that canonicalize their inputs (uppercase / lowercase) and reject non-ASCII letters via byte-arithmetic.
- `validateDateRange` ships with correct calendar-year semantics including the leap-year boundary: `2024-02-29 -> 2027-02-28` passes; `2024-02-29 -> 2027-03-01` rejects with `ErrDateRangeTooLarge`. Exact-3-year boundary `2025-01-01 -> 2028-01-01` passes; `+1d` rejects.
- `TestValidators_NoSensitiveData` locks the ERR-04 invariant that validator error messages contain only the offending caller value, never `http`/`://`/`Body:`/`Authorization` tokens.
- Full Phase 1 suite (`go test -race -cover ./...`) green at 100.0% statement coverage; `go vet ./...` and `gofmt -l` clean.

## Task Commits

Each task was committed atomically:

1. **Task 1 RED — failing smoke tests for validate.go** — `12389a9` (test)
2. **Task 1 GREEN — unexported validators in validate.go** — `383619a` (feat)
3. **Task 2 + leap-year boundary fix — expanded validate_test.go + corrected validateDateRange formula** — `a19fc9b` (fix)

## Files Created/Modified

- `validate.go` (116 lines) — three unexported validators + two unexported helpers; imports `fmt` and `strings` only.
- `validate_test.go` (299 lines) — four `TestXxx` functions covering VALID-01..VALID-04 + ERR-04; 42 distinct subtests under `t.Run`.

## Decisions Made

- **Backward-anchored 3-year window check.** Switched from `from.AddDate(3, 0, 1)` (plan-prescribed forward formula) to `to.AddDate(-3, 0, 0)` after probing Go's `time.AddDate` normalization. The forward formula maps `2024-02-29` to `2027-03-02` as the rejection threshold (because `2027-02-29` overflows to `2027-03-01`, then `+1 day` yields `2027-03-02`), which makes the plan's locked boundary `2024-02-29 -> 2027-03-01 MUST FAIL` unreachable. The backward formulation `from.Before(to.AddDate(-3, 0, 0))` satisfies every documented boundary including the leap-year case and preserves calendar arithmetic (no duration math).
- **%q quotes the ORIGINAL input, not the canonicalized form.** A caller that passes `"pl"` would see `"openholidays: invalid country code: \"pl\""` in a hypothetical rejection — useful for diagnosing typo'd input as the user typed it. Country codes happen to canonicalize cleanly, but the principle matters for malformed input like `" PL"`.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 — Bug] Corrected 3-calendar-year boundary formula for leap-day from values**

- **Found during:** Task 2 (running the expanded table-driven `TestValidateDateRange` against the production code committed in Task 1).
- **Issue:** Plan Task 1 mandated the formula `limit := Date{from.AddDate(3, 0, 1)}` plus `if !to.Before(limit)`. Plan Task 2 mandated the behavior `validateDateRange(2024-02-29, 2027-03-01) -> ErrDateRangeTooLarge`. These two requirements are mutually inconsistent because Go's `time.AddDate` normalizes overflow toward later dates: `2024-02-29` advanced by 3 years lands on `2027-03-01` (since 2027 is not a leap year), then advanced by 1 more day lands on `2027-03-02`. Under the forward formula, the rejection threshold for `from=2024-02-29` is `2027-03-02`, meaning `to=2027-03-01` is accepted — contradicting ROADMAP success criterion #4 verbatim text.
- **Verification of the inconsistency:** Ran a Go probe against `time.AddDate` directly. Confirmed `time.Date(2024, 2, 29, ...).AddDate(3, 0, 1).Format("2006-01-02") == "2027-03-02"`.
- **Fix:** Replaced the forward formula with `lowerBound := Date{to.AddDate(-3, 0, 0)}` plus `if from.Before(lowerBound)`. The backward formulation satisfies the boundary contract: for `from=2024-02-29, to=2027-02-28`, the lower bound is `2024-02-28`, and `from(2024-02-29).Before(2024-02-28)` is false (PASS); for `from=2024-02-29, to=2027-03-01`, the lower bound is `2024-03-01`, and `from(2024-02-29).Before(2024-03-01)` is true (FAIL with `ErrDateRangeTooLarge`).
- **Files modified:** validate.go (the `validateDateRange` body + its godoc, which now explains the asymmetry).
- **Verification of the fix:** All 42 subtests under `TestValidateDateRange` (including all six leap-year cases probed) pass. `go test -race -cover ./...` green at 100.0% coverage.
- **Committed in:** a19fc9b.
- **Plan-level follow-up needed:** Plan Task 1's `<verify><automated>` block grepped `grep -q 'AddDate(3, 0, 1)' validate.go`; that string is no longer in validate.go. The grep was a hint at the intended formula, not a behavior requirement, so this is a documentation issue with the plan itself rather than a regression. Phase 1 closing (Plan 06) should update PROJECT.md / CONTEXT.md D-22 to reflect the correct formula. Also note: the threat-model entry T-01-05-OFB references the forward formula by name — its wording needs revising to match the implementation.

---

**Total deviations:** 1 auto-fixed (Rule 1 — Bug).
**Impact on plan:** The Rule 1 fix is essential — without it, ROADMAP success criterion #4 verbatim text could not be satisfied. The deviation does not change the externally visible behavior the plan promised; it changes only the internal formula to deliver the promised behavior. No scope creep.

## Issues Encountered

- The internal inconsistency between Plan Task 1's prescribed formula and Plan Task 2's prescribed behavior surfaced as a test failure. Resolved via Rule 1 (auto-fix bug) after probing `time.AddDate` directly to verify the root cause (Gold Rule 2 — verify, don't guess). Documented in Deviations.

## User Setup Required

None — no external service configuration required.

## Next Phase Readiness

- Phase 2 endpoint methods can call `validateCountry`, `validateLanguage`, `validateDateRange` from the root package and rely on `errors.Is(err, ErrInvalid*)` matching through `%w` wrapping.
- Phase 1 Plan 06 (closure) should:
  1. Record CL-02 (case-insensitive country validator deviation from VALID-01 literal text) in PROJECT.md Key Decisions.
  2. Update D-22 in `.planning/phases/01-foundation/01-CONTEXT.md` to document the backward-anchored formula (and the leap-day rationale).
  3. Revise the T-01-05-OFB threat-model row in 01-05-PLAN.md (or its successor doc) to reference the actual implementation formula.

## Self-Check: PASSED

- Files exist: `validate.go` (FOUND), `validate_test.go` (FOUND).
- Commits exist: `12389a9` (FOUND), `383619a` (FOUND), `a19fc9b` (FOUND).
- `go test -race -cover ./...` — 100.0% coverage, green.
- `gofmt -l validate.go validate_test.go` — empty.
- `go vet ./...` — clean.

---
*Phase: 01-foundation*
*Completed: 2026-05-27*
