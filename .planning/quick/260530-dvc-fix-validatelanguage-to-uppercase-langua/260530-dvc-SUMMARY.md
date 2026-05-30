---
phase: quick-260530-dvc
plan: 01
subsystem: validators
tags: [validate, language-iso, openholidays-api, case-sensitivity, tdd, D-21-reversal]

# Dependency graph
requires:
  - phase: 01-foundation
    provides: "validateLanguage / validateCountry shape-and-canonicalize validators (D-20, D-21, W-01 fold guard)"
provides:
  - "validateLanguage canonicalizing ISO 639-1 codes to UPPERCASE via strings.ToUpper (mirrors validateCountry)"
  - "Every language-bearing endpoint (public holidays, school holidays, countries, languages, subdivisions, ohcli --lang) now sends the uppercase languageIsoCode the case-sensitive upstream honors"
affects: [public_holidays, school_holidays, countries, languages, subdivisions, cmd/ohcli]

# Tech tracking
tech-stack:
  added: []
  patterns: ["Language ISO codes canonicalize to UPPERCASE on the wire, identical to country codes — the OpenHolidays API is case-sensitive and lowercase silently falls back to English"]

key-files:
  created: []
  modified:
    - validate.go
    - validate_test.go
    - public_holidays_test.go
    - countries_test.go
    - languages_test.go
    - subdivisions_test.go
    - cmd/ohcli/public_test.go
    - .planning/phases/01-foundation/01-CONTEXT.md
    - .planning/phases/01-foundation/01-VERIFICATION.md

key-decisions:
  - "Reversed D-21: validateLanguage now ToUpper, not ToLower. Verified live against https://openholidaysapi.org — lowercase languageIsoCode returns English-only names; uppercase returns the requested language. Original D-21 text preserved in 01-CONTEXT.md / 01-VERIFICATION.md for the audit trail (fix-forward ethos)."
  - "Single squash-safe commit for the whole RED→GREEN cycle (not one commit per task) because this repo uses merge commits, not squash — a separate intentionally-failing Task-1 commit would persist a red state on master and break per-commit CI / git bisect."

patterns-established:
  - "Language and country ISO codes share the same uppercase wire canonicalization; validateLanguage mirrors validateCountry exactly (ToUpper after the pre-fold isTwoASCIILetters shape guard)."

requirements-completed: [VALID-04]

# Metrics
duration: 12min
completed: 2026-05-30
---

# Quick 260530-dvc: Fix validateLanguage to uppercase languageIsoCode Summary

**One-character production fix made test-first: `validateLanguage` now canonicalizes ISO 639-1 language codes to UPPERCASE via `strings.ToUpper` (was `strings.ToLower`), because the OpenHolidays API is case-sensitive and a lowercase `languageIsoCode` silently returned English-only holiday names instead of the requested language. Reverses decision D-21.**

## Performance

- **Duration:** ~12 min
- **Started:** 2026-05-30
- **Completed:** 2026-05-30
- **Tasks:** 2 (RED + GREEN, committed as one)
- **Files modified:** 9 (1 production, 6 test, 2 planning records)

## Accomplishments

- **RED (Task 1):** Flipped the `validateLanguage` unit-test success cases and five endpoint wire-assertions to expect the UPPERCASE canonical form, and reworded every "lowercase"/"lowercased" subtest name, comment, and assertion message to "uppercase". Confirmed these FAIL against the still-lowercasing production code (proving they encode the correct contract). All reject/shape/W-01 cases and every non-language assertion left untouched and still passing.
- **GREEN (Task 2):** Applied the one-line production change `validate.go` `strings.ToLower(code)` → `strings.ToUpper(code)`, mirroring `validateCountry` exactly. Rewrote the `validateLanguage` doc comment and the in-body W-01 comment from lowercase to uppercase. The `isTwoASCIILetters` W-01 shape guard is unchanged and still first in the function (runs on original bytes before canonicalization). Annotated D-21 as REVERSED in `01-CONTEXT.md` and `01-VERIFICATION.md`, preserving the original text.
- All four gates green after the fix: `go test -race ./...`, `gofmt -l .` (empty), `golangci-lint run ./...` (0 issues), `go test -run Example ./...`.

## Task Commits

Per the orchestrator's explicit instruction, the entire RED→GREEN cycle is **one** commit (not one per task) — a separate intentionally-failing Task-1 commit would persist a red state on master under this repo's merge-commit (non-squash) workflow and break per-commit CI / git bisect. The RED observation was for verification only.

1. **Task 1 (RED) + Task 2 (GREEN) — single commit** — `075ce4e` (fix)
   - Subject: `fix(validate): canonicalize language ISO code to uppercase to match OpenHolidays API`
   - 9 files changed, 39 insertions(+), 35 deletions(-)

**Plan metadata:** PLAN.md / SUMMARY.md / STATE.md are committed by the orchestrator, not here.

## Files Created/Modified

- `validate.go` — `validateLanguage` returns `strings.ToUpper(code)` (was `ToLower`); doc comment + W-01 in-body comment reworded to uppercase; D-21 reversal noted in the doc comment. Shape guard unchanged.
- `validate_test.go` — `TestValidateLanguage` success cases assert uppercase canonical form (`pl`/`PL`/`Pl` → `PL`, `EN` → `EN`, `de` → `DE`); doc comment reworded. Reject-case slice and reject-loop assertions untouched.
- `public_holidays_test.go` — wire subtest renamed and asserts `languageIsoCode=PL`; inline comment reworded.
- `countries_test.go` — wire subtest asserts `languageIsoCode=PL`; comment + message reworded.
- `languages_test.go` — wire subtest asserts `languageIsoCode=EN`; doc-comment bullet + in-body comment reworded. Response-body `"isoCode":"en"` left as-is (it is a response field, not a wire param).
- `subdivisions_test.go` — wire subtest renamed and asserts `languageIsoCode=EN`; doc-comment bullet (D-55 ID kept) + in-body comment reworded. `countryIsoCode=PL` assertion untouched.
- `cmd/ohcli/public_test.go` — `TestCmdPublic` `--lang` subtest renamed, asserts `languageIsoCode=PL`, comment reworded, and CLI input arg changed from `--lang pl` to `--lang PL` so the wire expectation is unambiguous regardless of input case.
- `.planning/phases/01-foundation/01-CONTEXT.md` — D-21 entry annotated `[REVERSED 2026-05-30, quick task 260530-dvc]` (original line preserved).
- `.planning/phases/01-foundation/01-VERIFICATION.md` — D-21 row notes the reversal to uppercase (row kept; history not deleted).

## Decisions Made

- **D-21 REVERSED:** lowercase → uppercase. Ground truth was established live against the OpenHolidays API this session (`languageIsoCode=PL` → Polish; `=pl` → English; `DE`/`de` identical). The original D-21 text is preserved in both planning records consistent with the project's fix-forward ethos.
- **Single commit for the full TDD cycle** per the orchestrator's commit instruction (merge-commit repo; avoid landing a red state on master).
- Left the W-01 `rejectCases` slice and its loop assertions untouched in `validate_test.go` (plan instruction). The U+212A / U+0130 fold cases are rejected by the pre-fold shape guard regardless of canonicalization direction, so they remain valid under `ToUpper`.

## Deviations from Plan

None — plan executed exactly as written. The fix is the single `ToLower` → `ToUpper` change; all six test files reworded off "lowercase"; doc + W-01 comments corrected; D-21 annotated REVERSED with original text preserved. No `testdata/*.json`, no `update_fixtures_test.go`, no `CHANGELOG.md`, no CLI arg-handling, no `Holiday.NameFor`/`pickLocalized`, and no change to the `isTwoASCIILetters` guard or its position. English only (Gold Rule 1). testify assert/require, one-test-per-prod-function, `t.Run` per case all preserved (Gold Rule 3). No new dependencies (zero-runtime-dep rule).

The plan's line numbers were approximate against the live files; all edits were matched by exact string content, not line number. The `cmd/ohcli` test function is `TestCmdPublic` (the plan referenced it by file + subtest location, which matched).

## Issues Encountered

None. The only nuance: the Task-1 verify regex (`TestValidateLanguage|TestClient_*`) does not match the CLI test function name `TestCmdPublic`, so the CLI subtest's RED/GREEN was confirmed with a separate `-run TestCmdPublic` invocation rather than relying on the filtered run reporting "no tests to run" for `cmd/ohcli`.

## RED → GREEN Evidence

**RED (Task 1, against the lowercasing production code) — all flipped tests failed as required:**

- `TestValidateLanguage/accept/uppercase_passes_through` (expected `PL`, got `pl`)
- `TestValidateLanguage/accept/lowercase_canonicalizes`
- `TestValidateLanguage/accept/mixed_case_canonicalizes`
- `TestValidateLanguage/accept/additional_happy_case_uppercase_EN`
- `TestValidateLanguage/accept/additional_happy_case_lowercase_de`
- `TestClient_PublicHolidays/optional_LanguageIsoCode_is_canonicalized_to_uppercase_on_the_wire` (expected `PL`, got `pl`)
- `TestClient_Countries/optional_LanguageIsoCode_sent_in_query_when_non-empty` (expected `PL`, got `pl`)
- `TestClient_Languages/optional_LanguageIsoCode_sent_as_query_param_when_non-empty` (expected `EN`, got `en`)
- `TestClient_Subdivisions/uppercase_languageIsoCode_reaches_the_wire` (expected `EN`, got `en`)
- `TestCmdPublic/--lang_reaches_the_wire_as_uppercase_canonical_form` (expected `PL`, got `pl`)

All reject / shape / W-01 / non-language assertions stayed green during RED.

**GREEN (Task 2, after `ToUpper`):**

```
ok  	github.com/egeek-tech/go-openholidays	1.770s
ok  	github.com/egeek-tech/go-openholidays/cmd/ohcli	1.044s
```

(Every previously-RED test now passes; a targeted `-run` over all ten reported zero failures.)

## Verification (plan gates)

- `go test -race ./...` → **green** (both packages).
- `gofmt -l .` → **empty** (clean).
- `golangci-lint run ./...` → **0 issues** (govet, errcheck, staticcheck, gosec, revive, gocritic).
- `go test -run Example ./...` → **green**.
- Goal-backward: `ohcli public PL 2025 --lang PL` now sends `languageIsoCode=PL`; upstream returns Polish names; `NameFor("PL")` (case-insensitive) finds the PL entry — user sees Polish, not English.

## User Setup Required

None — no external service configuration. Ground truth was verified live earlier this session; no network call was made during execution (hermetic unit + httptest mocks only).

## Self-Check: PASSED

- FOUND: `validate.go` (commit 075ce4e) — `return strings.ToUpper(code), nil` at the `validateLanguage` success path.
- FOUND: `validate_test.go` — `canonOK: "PL"` success cases.
- FOUND: 5 endpoint wire-assertion test files asserting uppercase `languageIsoCode`.
- FOUND: D-21 REVERSED annotation in `01-CONTEXT.md` and `01-VERIFICATION.md` (original text preserved).
- FOUND commit: `075ce4e` (verified via `git show --stat`: exactly the 9 expected files, no deletions, no stray additions).

## Next Phase Readiness

The language filter now works correctly on every endpoint. No blockers. The unrelated untracked files in the tree (GSD-PROJECT-BRIEF.md, docs/superpowers/, 01-UAT.md, 04-PATTERNS.md) were deliberately not staged.

---
*Phase: quick-260530-dvc*
*Completed: 2026-05-30*
