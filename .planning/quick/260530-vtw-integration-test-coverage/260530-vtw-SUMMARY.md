---
phase: quick-260530-vtw
plan: 01
subsystem: testing
tags: [integration-test, openholidays, localization, subdivisions, isinregion, testify, live-api]

# Dependency graph
requires:
  - phase: 05-distribution
    provides: "build-tagged + env-gated integration_test.go (TEST-08) with two PL count-only canaries"
  - phase: 03-endpoints-helpers
    provides: "Client.PublicHolidays/SchoolHolidays/Countries/Languages/Subdivisions, Holiday.IsInRegion, Client.IsInRegion, NameFor accessors, pickLocalized"
provides:
  - "Live integration coverage of all six exported endpoint/helper surfaces with three-layer assertions (count canary + localized-name pin + hasLang anti-fallback membership)"
  - "hasLang(entries, lang) test helper — the Layer-3 anti-fallback guard for the 260530-dvc language-casing bug class"
  - "Live exercise of Client.IsInRegion's hierarchical tree walk in both directions (descendant DE-BY-AU → true through /Subdivisions; unrelated code → false)"
  - "Live ferie-zimowe-per-województwo core-value assertion (PL-SL in first window true, PL-SK false)"
  - "Corrected SubdivisionRef.Code doc comment (OpenHolidays Code is its own scheme, NOT ISO 3166-2)"
affects: [06-ci-nightly, release-readiness, future endpoint additions]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Three-layer integration assertion model (spec §3): Layer 1 count canary, Layer 2 exact localized-name pin, Layer 3 hasLang raw-slice membership"
    - "hasLang anti-fallback membership — never assert localization via NameFor(lang) != \"\" or NotEmpty(NameFor(...)); pickLocalized falls back to entries[0] and would pass under the bug"
    - "Tree-walk-true requires a DESCENDANT code (not in h.Subdivisions) to bypass the flat fast-path and force the /Subdivisions fetch; discover the parent/child pair from live data at runtime (mirrors hermetic findFirstWithChildren)"

key-files:
  created: []
  modified:
    - "integration_test.go — hasLang helper + 7 TestIntegration_<Endpoint> functions, all double-gated, fresh audit:ok marks"
    - "types.go — SubdivisionRef.Code doc comment corrected (comment-only; audit:ok marks untouched)"
    - ".planning/ROADMAP.md, .planning/phases/05-distribution/05-04-PLAN.md, 05-04-SUMMARY.md, 05-RESEARCH.md, 05-PATTERNS.md — re-pointed renamed-test references"

key-decisions:
  - "ZZ error path: observed live 2026-05-30 the upstream returns a 2xx with an EMPTY result (no *APIError) for the well-formed-but-unknown country ZZ — assert require.NoError + assert.Empty (Gold Rule 2: assert observed, not assumed 4xx)"
  - "Languages(lang=PL) localization: only the IsoCode \"PL\" entry carries a PL-language Name (\"polski\"); DE/EN entries return English-only names — the hasLang(\"PL\") assertion targets the PL entry specifically. No exact language name pinned (§6a gives none; Gold Rule 2)"
  - "Tree-walk-true case uses live DE pair parent=DE-BY (Bayern), child=DE-BY-AU (Augsburg), discovered at runtime — matches the hermetic client_isinregion_test.go pair"
  - "ErrorPaths uses require.ErrorIs (not assert.True(errors.Is(...))) per testifylint require-error rule; the errors import was dropped as a result"

patterns-established:
  - "Live integration tests assert scalars + semantics inline from a dated probe (§6a), not byte-fixtures — fixtures remain a hermetic-test concern"
  - "Every integration test double-gated (//go:build integration + OPENHOLIDAYS_LIVE=1), no t.Parallel, t.Context()+context.WithTimeout"

requirements-completed: [TEST-08]

# Metrics
duration: 9min
completed: 2026-05-30
---

# Phase quick-260530-vtw Plan 01: Live Integration Test Coverage Summary

**Extended the nightly live-API integration suite from two PL count-only canaries to all six exported endpoint/helper surfaces plus an error path, using a three-layer assertion model (count canary + exact localized-name pin + hasLang anti-fallback membership) so a reintroduction of the 260530-dvc language-casing bug now FAILS at least one assertion.**

## Performance

- **Duration:** ~9 min
- **Started:** 2026-05-30T21:17:52Z
- **Completed:** 2026-05-30T21:26:53Z
- **Tasks:** 4
- **Files modified:** 7 (integration_test.go, types.go, 5 planning docs)

## Accomplishments
- Added `hasLang(entries, lang)` — case-insensitive membership that does NOT fall back to `entries[0]`, the Layer-3 anti-fallback guard for the language-casing bug class.
- Renamed and broadened the two existing tests; added five new endpoint/helper tests. The suite now covers PublicHolidays, SchoolHolidays, Countries, Languages, Subdivisions, Client.IsInRegion, and a live error path — 7 `TestIntegration_*` functions, 20 `t.Run` subtests.
- Drove `Client.IsInRegion`'s hierarchical tree walk live in the TRUE direction via a descendant code (`DE-BY-AU` under a parent-only `DE-BY` synthetic Holiday), forcing the `/Subdivisions` fetch + `buildParentIndex` + upward walk — not the flat fast-path — and in the FALSE direction with an unrelated code.
- Asserted the PL ferie-zimowe-per-województwo core value live: the first cohort (2025-01-20..02-02) is non-nationwide, `Groups` empty, `IsInRegion("PL-SL")` true, `IsInRegion("PL-SK")` false.
- Corrected the misleading `SubdivisionRef.Code` doc comment (now states the OpenHolidays Code is its own scheme, NOT ISO 3166-2).

## §6a Values Pinned (verified live 2026-05-30)
- **Languages:** 30 (default count canary); PL/DE/EN present by uppercase IsoCode. `lang=PL`: only the IsoCode "PL" entry carries a PL Name ("polski") → hasLang("PL") on that entry.
- **Countries:** 36 (count canary); `lang=DE` → Germany="Deutschland", Poland="Polen" (pins) + hasLang("DE"); PL.OfficialLanguages contains "PL".
- **PublicHolidays DE 2025 (lang=DE):** 21 (count canary); New Year="Neujahr" (pin) + hasLang("DE"); "Heilige Drei Könige" Nationwide=false, IsInRegion("DE-BY")=true, IsInRegion("DE-HH")=false.
- **PublicHolidays PL:** 14 (existing canary kept); 2025-01-01 New Year="Nowy Rok" (existing pin preserved, not weakened).
- **SchoolHolidays PL 2025:** 7 (existing canary kept); first ferie-zimowe cohort 2025-01-20..02-02 non-nationwide, Groups empty, PL-SL true / PL-SK false.
- **Subdivisions PL:** 16 (count canary); codes `^PL-`; PL-SK="Śląskie", PL-SL="Świętokrzyskie" (pins, with inline non-ISO note); Category(PL)="województwo".
- **Subdivisions DE:** 16 (count canary); codes `^DE-`; DE-BY present; live tree carries `Subdivision.Children`.

## Probed Values (NOT pinned in §6a — Gold Rule 2, observed not guessed)
- **ZZ unknown-country error path:** upstream returns **HTTP 2xx with an empty result** (`err=<nil>, len=0`) — NO `*APIError`. ErrorPaths asserts `require.NoError` + `assert.Empty`.
- **Z malformed-country path:** deterministic `ErrInvalidCountry` from validate.go (one-letter code fails `isTwoASCIILetters` client-side, no network). Asserted via `require.ErrorIs(t, err, ErrInvalidCountry)`.
- **Languages(lang=PL) name localization:** no exact name pinned (none in §6a); hasLang("PL") on the IsoCode "PL" entry is the robust assertion.

## Live Tree-Walk Codes
- **parent = `DE-BY` (Bayern), child = `DE-BY-AU` (Augsburg)** — discovered at runtime from the live DE `/Subdivisions` tree (logged: `tree-walk target: parent="DE-BY" child="DE-BY-AU"`). Matches the hermetic `findFirstWithChildren` pair.

## Task Commits

Per the plan's two-commit structure (Task 4), committed directly on `test/integration-coverage`:

1. **Commit A (chore)** — `6a6684d` — types.go comment correction (NOT ISO 3166-2) + re-point of renamed-test references across the 5 planning files.
2. **Commit B (test)** — `3a8c653` — integration_test.go: hasLang + 7 TestIntegration_* functions with three-layer assertions.

_Plan/Summary/STATE artifacts are committed by the orchestrator (Step 8), not here. PLAN.md was pre-committed at `1f45575`._

## Files Created/Modified
- `integration_test.go` — added `hasLang`; renamed `TestIntegration_PublicHolidays_PL_2025`→`TestIntegration_PublicHolidays` and `_SchoolHolidays_PL_2025`→`_SchoolHolidays` with DE/regional/ferie-zimowe cases; added `TestIntegration_Countries`, `_Languages`, `_Subdivisions`, `_ClientIsInRegion`, `_ErrorPaths`. 8 fresh `// audit:ok 2026-05-30` marks (hasLang + 7 functions).
- `types.go` — `SubdivisionRef.Code` doc comment corrected (comment-only; the 5 existing audit:ok marks untouched).
- `.planning/ROADMAP.md`, `.planning/phases/05-distribution/{05-04-PLAN.md, 05-04-SUMMARY.md, 05-RESEARCH.md, 05-PATTERNS.md}` — renamed-test references re-pointed.

## Decisions Made
See `key-decisions` frontmatter. Headline: the ZZ path is 2xx-empty (not a 4xx APIError) per the live probe; Languages localization asserted via hasLang on the IsoCode "PL" entry (only that entry carries a PL Name).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] golangci-lint (integration build tag) findings in TestIntegration_ErrorPaths**
- **Found during:** Task 4 (verification gate)
- **Issue:** `golangci-lint run --build-tags=integration` flagged (a) godoclint: bare `errors.Is` in the godoc should be `[errors.Is]`; (b) testifylint error-is-as: `assert.True(t, errors.Is(...))` should be `assert.ErrorIs`; then (c) testifylint require-error: error assertions should use `require`, so `assert.ErrorIs` → `require.ErrorIs`.
- **Fix:** Bracketed `[errors.Is]` in the godoc; switched the assertion to `require.ErrorIs(t, err, ErrInvalidCountry)` and dropped the now-redundant `require.Error`; removed the now-unused `"errors"` import.
- **Files modified:** integration_test.go (Task 4, committed in `3a8c653`)
- **Verification:** `golangci-lint run` and `golangci-lint run --build-tags=integration` both report 0 issues; `go vet -tags=integration ./...` clean.

**2. [Rule 3 - Blocking] gofmt reflow of a list-item comment in TestIntegration_SchoolHolidays godoc**
- **Found during:** Task 4 (gofmt gate)
- **Issue:** A godoc list continuation line beginning with `+` (`//     + date window`) was reinterpreted by gofmt's list formatter as a new bullet, which would have corrupted the prose.
- **Fix:** Reworded the sentence to avoid the leading `+` continuation ("distinguished by their Subdivisions and date window, NOT by group codes") — meaning preserved exactly.
- **Files modified:** integration_test.go (Task 4, committed in `3a8c653`)
- **Verification:** `gofmt -l .` returns empty.

---

**Total deviations:** 2 auto-fixed (both Rule 3 - blocking lint/format issues directly caused by the new test code).
**Impact on plan:** Both fixes were lint/format hygiene on the new code; no behavior change, no scope creep. All §6a pins, the tree-walk-true requirement, and the anti-fallback rules are intact.

## Issues Encountered
- The anti-pattern guard initially matched a literal `NotEmpty(NameFor(...))` string inside the `hasLang` doc comment (explanatory prose, not an assertion). Reworded the comment so the guard grep is clean while preserving the explanation of why the bug slipped past a non-empty check.

## Verification Results
- `go test ./...` (no tags, no env): GREEN and hermetic (zero live HTTP calls — integration file compile-excluded).
- `go build -tags=integration ./...`: clean.
- `go vet -tags=integration ./...`: clean.
- `gofmt -l .`: empty.
- `golangci-lint run`: 0 issues. `golangci-lint run --build-tags=integration`: 0 issues. **golangci-lint ran locally (v2.12.2).**
- **`OPENHOLIDAYS_LIVE=1 go test -tags=integration -count=1 -timeout=5m ./...`: GREEN.** Measured live-run duration ~1.2s (package test time; ~20 serial sub-second calls — far inside the 5m timeout).
- Anti-pattern grep (`NameFor(...) != ""` / `NotEmpty(...NameFor...)`): finds nothing.
- `// audit:ok 2026-05-30` marks in integration_test.go: 8 (hasLang + 7 test functions), each above its godoc.
- Five planning files: zero stale `TestIntegration_*_PL_2025` references.
- No `testdata/*_de_*.json` added; `update_fixtures_test.go`, `.github/workflows/integration.yml`, and `CHANGELOG.md` all untouched.

## User Setup Required
None - no external service configuration required. The nightly `.github/workflows/integration.yml` (unchanged) supplies both gates; new `//go:build integration` tests join automatically.

## Next Phase Readiness
- The nightly integration run now surfaces localization and regional-behavior regressions, not just count drift. The 260530-dvc bug class is regression-locked live.
- Forbidden untracked files (ohcli, GSD-PROJECT-BRIEF.md, 01-UAT.md, 04-resilience/04-PATTERNS.md, RESUME-AFTER-COMPACT.md, the two specs) were left unstaged as instructed.

## Self-Check: PASSED

- Files exist: integration_test.go, types.go, 260530-vtw-SUMMARY.md — all FOUND.
- Commits reachable: `6a6684d` (chore), `3a8c653` (test) — both FOUND on `test/integration-coverage`.
- SUMMARY correctly left untracked (orchestrator commits it in Step 8); no modified tracked files remain in the working tree.

---
*Phase: quick-260530-vtw*
*Completed: 2026-05-30*
