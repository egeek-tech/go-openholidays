---
phase: quick-260530-vtw
verified: 2026-05-30T21:31:34Z
status: passed
score: 7/7 must-haves verified
overrides_applied: 0
---

# Quick Task 260530-vtw Verification Report

**Task Goal:** Every exported endpoint/helper (PublicHolidays, SchoolHolidays, Countries, Languages, Subdivisions, Client.IsInRegion) exercised by the live integration suite with three-layer semantic assertions that FAIL when localization/regional behavior is silently broken — closing the 2026-05-30 languageIsoCode-lowercase bug class. Scope: PL + Germany + error paths.
**Verified:** 2026-05-30T21:31:34Z
**Status:** passed
**Re-verification:** No — initial verification

---

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|---------|
| 1 | All six exported endpoint/helper surfaces are exercised by the live integration suite | VERIFIED | `integration_test.go`: `TestIntegration_PublicHolidays` (line 105), `TestIntegration_SchoolHolidays` (249), `TestIntegration_Countries` (350), `TestIntegration_Languages` (424), `TestIntegration_Subdivisions` (493), `TestIntegration_ClientIsInRegion` (593), `TestIntegration_ErrorPaths` (666). Live run: all 22 subtests PASS in 1.154s. |
| 2 | A reintroduction of the 260530-dvc language-casing bug FAILS via exact localized-name pins AND raw-slice hasLang membership, never via NameFor(lang) != "" or NotEmpty(NameFor(lang)) | VERIFIED | Anti-pattern grep: `grep -nE 'NameFor\([^)]*\) *!= *""\|NotEmpty[^)]*NameFor' integration_test.go` returns nothing (exit 1 = no match). Pins present: `require.Equal(t, "Nowy Rok", hs[0].NameFor("PL"))` (line 143), `assert.Equal(t, "Neujahr", newYear.NameFor("DE"))` (183), `assert.Equal(t, "Deutschland", germany.NameFor("DE"))` (402), `assert.Equal(t, "Polen", poland.NameFor("DE"))` (403), `assert.Equal(t, "Śląskie", sk.NameFor("PL"))` (537), `assert.Equal(t, "Świętokrzyskie", sl.NameFor("PL"))` (538). hasLang guards pair every pin. `hasLang` body (lines 70-77) uses `strings.EqualFold` with no `entries[0]` fallback — returns false when the language is absent. |
| 3 | PL ferie-zimowe-per-województwo core value is asserted live: a non-nationwide school holiday with specific PL-XX code in a specific date window, with IsInRegion returning true for that województwo | VERIFIED | `TestIntegration_SchoolHolidays/PL_2025_ferie_zimowe_per_województwo_(core_value)` (lines 271-303): locates the 2025-01-20..02-02 cohort, asserts `Nationwide==false`, `Groups` empty, `IsInRegion("PL-SL")==true`, `IsInRegion("PL-SK")==false`. Subtest PASSES in live run. |
| 4 | Client.IsInRegion's hierarchical tree walk is exercised live in the TRUE direction via a DESCENDANT child code on a synthetic parent-only Holiday (NOT the flat fast-path) AND in the false direction | VERIFIED | `TestIntegration_ClientIsInRegion` (lines 593-650): fetches live DE tree via `c.Subdivisions(ctx, SubdivisionsRequest{CountryIsoCode: "DE"})`, locates a parent with `len(s.Children) > 0`, builds `Holiday{Subdivisions: []SubdivisionRef{{Code: parentCode}}}`, queries `c.IsInRegion(ctx, h, childCode)` where `childCode` is NOT in `h.Subdivisions` — forcing the tree walk. Both subtests PASS live. |
| 5 | Default `go test ./...` stays hermetic and green — issues zero live HTTP calls | VERIFIED | `go test ./...` exits 0 in 0.185s; integration file is compile-excluded by `//go:build integration` (line 1 of integration_test.go). No TestIntegration_* functions appear in the hermetic output. |
| 6 | One gated live run `OPENHOLIDAYS_LIVE=1 go test -tags=integration -count=1 -timeout=5m ./...` is GREEN | VERIFIED | Run completed: exit 0, 1.154s. All 22 subtests across 7 functions PASS. Well within the 5m timeout. |
| 7 | types.go SubdivisionRef.Code comment no longer falsely claims PL-SL is Śląskie; it states OpenHolidays Code is its own scheme (not ISO 3166-2) | VERIFIED | `types.go` lines 94-98: `"Code is the OpenHolidays subdivision code (its own scheme, NOT ISO 3166-2; any ISO 3166-2 value lives in Subdivision.IsoCode). The two schemes do not always agree: under the live API \"PL-SL\" resolves to Świętokrzyskie, whereas ISO 3166-2 assigns PL-SL to Śląskie (verified 2026-05-30 probe)."` Comment-only change — no function bodies or audit:ok lines in types.go touched. |

**Score:** 7/7 truths verified

---

## Check Results

### Check 1: `go build -tags=integration ./...`

**Result:** EXIT 0 — CLEAN.

### Check 2: `go vet -tags=integration ./...`

**Result:** EXIT 0 — CLEAN.

### Check 3: `gofmt -l .`

**Result:** EXIT 0, empty output — no files need formatting. CLEAN.

### Check 4: golangci-lint

- `golangci-lint run` — `0 issues.` EXIT 0.
- `golangci-lint run --build-tags=integration` — `0 issues.` EXIT 0.

Both CLEAN.

### Check 5: Default hermetic `go test ./...`

**Result:** `ok github.com/egeek-tech/go-openholidays 0.185s` EXIT 0. Integration file is compile-excluded; zero live HTTP calls issued. CLEAN.

### Check 6: Anti-pattern absent

`grep -nE 'NameFor\([^)]*\) *!= *""|NotEmpty[^)]*NameFor' integration_test.go`

**Result:** No matches (exit 1 from grep = pattern absent). VERIFIED. The 260530-dvc bug class cannot pass silently through a NameFor fallback assertion.

### Check 7: Gated live run

`OPENHOLIDAYS_LIVE=1 go test -tags=integration -count=1 -timeout=5m ./...`

**Result:** `ok github.com/egeek-tech/go-openholidays 1.154s` EXIT 0. All 22 subtests PASS. Duration 1.154s — well inside 5m.

All subtest results:
- TestIntegration_PublicHolidays: PASS (0.25s) — 5 subtests all PASS
- TestIntegration_SchoolHolidays: PASS (0.16s) — 4 subtests all PASS
- TestIntegration_Countries: PASS (0.05s) — 2 subtests all PASS
- TestIntegration_Languages: PASS (0.07s) — 2 subtests all PASS
- TestIntegration_Subdivisions: PASS (0.10s) — 3 subtests all PASS
- TestIntegration_ClientIsInRegion: PASS (0.10s) — 2 subtests all PASS
- TestIntegration_ErrorPaths: PASS (0.03s) — 2 subtests all PASS

### Check 8: All six endpoint/helper surfaces covered + Client.IsInRegion tree-walk

Seven `TestIntegration_*` functions present (lines 105, 249, 350, 424, 493, 593, 666 of integration_test.go):
`PublicHolidays`, `SchoolHolidays`, `Countries`, `Languages`, `Subdivisions`, `ClientIsInRegion`, `ErrorPaths`.

`TestIntegration_ClientIsInRegion` tree-walk positive case (lines 611-638): discovers `parentCode` and `childCode` from the live DE tree at runtime via `len(s.Children) > 0`; builds `Holiday{Subdivisions: []SubdivisionRef{{Code: parentCode}}}` (only parent in Subdivisions); calls `c.IsInRegion(ctx, h, childCode)` where `childCode` is NOT in `h.Subdivisions` — this bypasses all fast-paths and forces `/Subdivisions` fetch + `buildParentIndex` + upward walk. Negative case uses `"ZZ-XX-NEVER"` (also forces tree walk, returns false). VERIFIED.

### Check 9: `hasLang` body verified — no `entries[0]` fallback

`hasLang` body (integration_test.go lines 70-77):
```
for _, e := range entries {
    if strings.EqualFold(e.Language, lang) {
        return true
    }
}
return false
```
The function returns `true` only on a genuine language match; returns `false` (not `entries[0]`) on miss. The two occurrences of `entries[0]` in the file are in the godoc comment (explaining what the function does NOT do) — not in the function body. VERIFIED.

### Check 10: Rule 5 audit:ok marks

`grep -c '// audit:ok 2026-05-30' integration_test.go` returns **8**.

Positions and corresponding functions:
- Line 55: `hasLang`
- Line 79: `TestIntegration_PublicHolidays`
- Line 221: `TestIntegration_SchoolHolidays`
- Line 338: `TestIntegration_Countries`
- Line 409: `TestIntegration_Languages`
- Line 478: `TestIntegration_Subdivisions`
- Line 572: `TestIntegration_ClientIsInRegion`
- Line 652: `TestIntegration_ErrorPaths`

Each mark is placed above the godoc comment with a blank line separator, per Rule 5 convention. Marks = 8 >= required minimum of 8. VERIFIED.

`types.go` audit:ok marks (lines 42, 201, 228, 270, 278): all dated `2026-05-30`, all untouched by the comment-only SubdivisionRef.Code change. VERIFIED.

### Check 11: No stale `_PL_2025` test name references in target planning files

`grep -rn 'TestIntegration_PublicHolidays_PL_2025|TestIntegration_SchoolHolidays_PL_2025'` in the five target planning files (05-04-PLAN.md, 05-04-SUMMARY.md, 05-RESEARCH.md, 05-PATTERNS.md, ROADMAP.md): **no matches** (exit 1). CLEAN.

Matches found only in `260530-vtw-PLAN.md` itself (task instructions describing the rename operation, automated verify commands) and `260530-vtw-SUMMARY.md` (historical prose about what was renamed) — both are the PLAN/SUMMARY for this quick task and are not live `-run` references.

### Check 12: No DE fixtures added; integration.yml and update_fixtures_test.go unchanged; CHANGELOG.md unchanged

- `testdata/*_de_*.json`: no such files exist. CLEAN.
- `git diff HEAD~2 HEAD` does not include `integration.yml`, `update_fixtures_test.go`, or `CHANGELOG.md`. CLEAN.

---

## Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `integration_test.go` | hasLang + 7 TestIntegration_* functions with three-layer assertions; audit:ok marks | VERIFIED | 705 lines; `func hasLang(` at line 70; all 7 TestIntegration_* functions present; 8 audit:ok marks at correct positions. |
| `types.go` | SubdivisionRef.Code comment corrected, no function bodies changed | VERIFIED | Lines 94-98 contain "NOT ISO 3166-2" wording. Function bodies and audit:ok lines in types.go are untouched. |

---

## Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| integration_test.go assertions | `hasLang(entries, lang)` | Layer-3 anti-fallback membership | WIRED | `hasLang(` called at lines 184, 318, 333, 404, 472, 539, 551 — covering all localized endpoints |
| TestIntegration_SchoolHolidays ferie-zimowe case | `Holiday.IsInRegion` | `firstCohort.IsInRegion("PL-SL")` / `IsInRegion("PL-SK")` | WIRED | Lines 298-303: positive PL-SL + negative PL-SK assertions in `PL_2025_ferie_zimowe_per_województwo` subtest |
| TestIntegration_PublicHolidays DE regional case | `Holiday.Subdivisions + Nationwide==false + IsInRegion("DE-BY")` | Heilige Drei Könige assertion | WIRED | Lines 207-217: `Nationwide=false`, `Subdivisions` non-empty, `IsInRegion("DE-BY")==true`, `IsInRegion("DE-HH")==false` |
| TestIntegration_ClientIsInRegion descendant case | `Client.IsInRegion(ctx, h, childCode)` hierarchical tree walk | `c.Subdivisions(ctx, SubdivisionsRequest{CountryIsoCode: "DE"})` + `Children[0].Code` | WIRED | Lines 607-638: live DE tree fetch, `len(s.Children) > 0` parent discovery, synthetic parent-only Holiday, `childCode` triggers tree walk |

---

## Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| All 7 TestIntegration_* functions exist | `grep -nE '^func TestIntegration_' integration_test.go` | 7 matches at lines 105, 249, 350, 424, 493, 593, 666 | PASS |
| Anti-pattern absent (no NameFor fallback) | `grep -nE 'NameFor\([^)]*\) != ""\|NotEmpty[^)]*NameFor' integration_test.go` | no output (exit 1) | PASS |
| 8 audit:ok marks | `grep -c '// audit:ok 2026-05-30' integration_test.go` | 8 | PASS |
| Hermetic run | `go test ./...` | ok in 0.185s | PASS |
| Gated live run | `OPENHOLIDAYS_LIVE=1 go test -tags=integration -count=1 -timeout=5m ./...` | ok in 1.154s, all 22 subtests PASS | PASS |
| Tree-walk DE fetch wired | `grep -q 'c\.Subdivisions(.*CountryIsoCode: "DE"' integration_test.go` | match found (exit 0) | PASS |

---

## Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| TEST-08 | 260530-vtw-PLAN.md | Live integration suite covering all endpoints with semantic assertions | SATISFIED | All 6 endpoint/helper surfaces covered, 22 subtests PASS live, three-layer assertions implemented |

---

## Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| — | — | — | — | No anti-patterns found |

No debt markers (TBD, FIXME, XXX), placeholders, or stub patterns found in the modified files.

---

## Human Verification Required

None. All checks are fully verifiable programmatically.

---

## Gaps Summary

No gaps. All 7 must-have truths are VERIFIED, all artifacts are substantive and wired, all key links function, the live run is GREEN, and the build/vet/lint/format gates are CLEAN.

---

_Verified: 2026-05-30T21:31:34Z_
_Verifier: Claude (gsd-verifier)_
