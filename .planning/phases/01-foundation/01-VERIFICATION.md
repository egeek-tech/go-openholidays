---
phase: 01-foundation
verified: 2026-05-27T12:00:00Z
status: passed
score: 14/14 must-haves verified
overrides_applied: 0
roadmap_success_criteria_passed: 5/5
test_run:
  test_runs: 209
  failures: 0
  coverage_pct: 100.0
  race_clean: true
  fuzz_target: FuzzDateUnmarshal
  fuzz_duration: 10s
  fuzz_crashers: 0
  go_version_used: "go1.26.3 (matches >= 1.23 floor)"
  go_build: ok
  go_vet: ok
re_verification:
  previous_status: null
  previous_score: null
  initial_verification: true
follow_ups:
  - id: W-01
    severity: warning
    file: validate.go:28-34,49-55
    summary: "validateCountry/validateLanguage canonicalize before the ASCII shape check; Unicode code points that case-fold to ASCII (U+0130 İ, U+0131 ı, U+017F ſ, U+212A K) bypass the validator. Sentinel still applies upstream as 4xx, but client-side ASVS V5.1.3 guarantee documented in godoc is not delivered. Suggest fix-in-Phase-2 since validators are not yet wired to a network call."
  - id: W-02
    severity: warning
    file: version.go:7-10
    summary: "Godoc claims Version is overridable via `go build -ldflags '-X .Version=...'`, but `-X` only works on var, not const. Documented behavior does not match. Suggest either (a) drop the ldflags paragraph or (b) change to var Version = \"0.1.0\"."
  - id: W-03
    severity: warning
    file: internal_test.go:64-70
    summary: "CLIENT-10 AST audit skipDirs includes \"internal\". PROJECT.md says internal/ holds library code that should be subject to CLIENT-10. Remove the skip before any internal/* subpackage lands."
  - id: W-04
    severity: warning
    file: validate_test.go:241-299
    summary: "TestValidators_NoSensitiveData is a cross-cutting test that breaks the Gold Rule 3 one-test-per-prod-function shape. Either fold the leak-guard assertions into each per-validator test, or record an explicit Key Decisions exception."
---

# Phase 1: Foundation Verification Report

**Phase Goal:** Domain types, `Date`, errors, and validators exist as a zero-dependency package; `go.mod` declares Go 1.23; the public type contract is stable.
**Verified:** 2026-05-27
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement Summary

Every observable truth required by the Phase 1 goal is present, substantive, wired, and runnable in the codebase. All 5 ROADMAP success criteria pass with literal program proof (criterion #1, #2, #3 each verified by a standalone `go run` against the actual exported API; criteria #4 and #5 verified via the package test suite and toolchain). All 14 requirement IDs are claimed by PLAN frontmatter and each has matching production + test evidence. All 6 CL-XX clarifications are recorded in PROJECT.md Key Decisions.

The advisory code review (`01-REVIEW.md`) flagged 4 Warnings and 7 Info; 0 Critical. None blocks the Phase 1 goal. They are carried forward in the `follow_ups` frontmatter for Phase 2 to consume.

## Goal Achievement

### ROADMAP Success Criteria (5/5 PASS)

| # | Criterion | Status | Evidence |
|---|-----------|--------|----------|
| 1 | `var d Date; json.Unmarshal([]byte("\"2025-12-24\"")…)` round-trips through `MarshalJSON`; `null` and empty strings produce errors, not silent zero values | PASS | Standalone `go run` against the package: `ROUNDTRIP_OK`, `NULL_REJECTED_OK`, `EMPTY_REJECTED_OK`. Also locked by `TestDate_MarshalJSON/roundtrip_locks_roadmap_criterion_1`, `TestDate_UnmarshalJSON/null_returns_errEmptyDate`, `TestDate_UnmarshalJSON/empty_json_string_returns_errEmptyDate`. Code: `date.go:88-105` (UnmarshalJSON), `date.go:70-77` (MarshalJSON). |
| 2 | `errors.Is(fmt.Errorf("country %q: %w", "ZZZ", ErrInvalidCountry), ErrInvalidCountry)` returns `true` for every sentinel | PASS | Standalone `go run` looped through all 5 sentinels with literal `fmt.Errorf("country %q: %w", "ZZZ", s)`: `SENTINEL_IS_OK (5 sentinels matched)`. Locked by `TestSentinels_ErrorsIs/<sentinel>/recoverable-through-wrap` × 5. CL-01 expansion (5 sentinels vs ROADMAP-literal 4) is recorded in PROJECT.md Key Decisions row CL-01. |
| 3 | `errors.As(err, &apiErr)` extracts a `*APIError` with populated `StatusCode`, `Path`, `Body` fields from a wrapped error chain | PASS | Standalone `go run` constructs `&APIError{StatusCode:404, Path:"/Subdivisions", Body:[]byte("hello"), Message:"msg"}`, wraps with `fmt.Errorf("transport: %w", original)`, calls `errors.As` and asserts all four fields preserved: `APIERROR_AS_OK`. Locked by `TestAPIError_ErrorsAs/populated-fields-survive-wrap`. Code: `errors.go:57-62` (struct shape — Message added per ROADMAP allows additional fields). |
| 4 | `validate*` functions reject 1-letter, lowercase-via-three-letter, `validFrom > validTo`, and date windows > 3 years with the correct sentinel; no global state, no `init()` side effects | PASS | `TestValidateCountry/reject/<one letter\|three letters\|...>` (note CL-02 ratifies lowercase ACCEPT with canonicalization to uppercase — recorded in PROJECT.md CL-02 row). `TestValidateDateRange/reject/from after to by one day` (ErrInvalidDateRange), `.../exact 3 years plus 1 day` (ErrDateRangeTooLarge), `.../leap-year 3 years plus 1 day` (ErrDateRangeTooLarge — CL-06 boundary). `TestNoInitOrGlobalState/no_init_and_no_unexpected_package_vars` passed. Sanity `grep -rn 'func init' --include='*.go'` finds only doc-comment matches in `internal_test.go`, zero production `func init` declarations. |
| 5 | `go build ./...` and `go vet ./...` succeed on Go 1.23 with the `go 1.23` directive in `go.mod` | PASS | `go.mod:3` declares `go 1.23`. `go build ./...` → exit 0. `go vet ./...` → exit 0. Runner toolchain is `go1.26.3` (matches `>=1.23` floor; CI matrix per CONTEXT.md is 1.23/1.24/stable). |

**Score:** 5/5 ROADMAP success criteria verified

### Requirement Coverage (14/14 PASS)

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| TYPES-01 | 01-04 | Holiday struct with all upstream fields | PASS | `types.go:94-138` (13 fields including schema-drift Quality). `TestHoliday_JSON/{single-day…,multi-day…,decode with schema-drift Quality…,decode tolerates unknown extra…}` all pass. |
| TYPES-02 | 01-03 | Custom `Date` type wrapping `time.Time` with JSON marshaling | PASS | `date.go:37-39` declares `type Date struct { time.Time }`; `MarshalJSON`/`UnmarshalJSON` at `date.go:70-105`. `TestDate_*` (10 tests + 1 fuzz) pass at 100% coverage. |
| TYPES-03 | 01-04 | LocalizedText, SubdivisionRef, GroupRef value types with JSON tags | PASS | `types.go:47-78`. `TestLocalizedText_JSON`, `TestSubdivisionRef_JSON`, `TestGroupRef_JSON` round-trip pass. |
| TYPES-04 | 01-04 | HolidayType typed string enum with constants | PASS | `types.go:23-40` ships 6 PascalCase values (Public, Bank, Optional, School, BackToSchool, EndOfLessons). CL-04 expands from REQUIREMENTS.md's 4 (which included spurious "Observance") to the 6 verified-upstream values; recorded in PROJECT.md Key Decisions CL-04. `TestHolidayType_constants` × 6 pass. |
| TYPES-05 | 01-04 | NameFor(lang) localized accessor on Country/Language/Subdivision | PASS | `types.go:163-165, 180-182, 220-222` — three NameFor methods sharing `pickLocalized`. CL-05 ratifies the rename from REQUIREMENTS.md's `Name(lang)` (would collide with `Name []LocalizedText` field); recorded in PROJECT.md CL-05. `TestCountry_NameFor`, `TestLanguage_NameFor`, `TestSubdivision_NameFor` all pass with exact/case-insensitive/fallback/empty cases. |
| ERR-01 | 01-02 | Sentinel errors `ErrInvalidCountry`, `ErrInvalidLanguage`, `ErrDateRangeTooLarge`, `ErrEmptyResponse` | PASS | `errors.go:17-36`. Phase 1 ships 5 sentinels (adds `ErrInvalidDateRange` per CL-01). `TestSentinelErrors/<sentinel>` × 5 pass. |
| ERR-02 | 01-02 | `*APIError{StatusCode, Path, Body}` leaf type usable via `errors.As` | PASS | `errors.go:57-105` defines `APIError` with `StatusCode, Path, Body []byte, Message`. `Error()` and `Is()` methods present; no `Unwrap` per D-16. Construction lands in Phase 2 per D-19; Phase 1 ships type + methods + tests (matches plan must_haves). `TestAPIError_ErrorsAs/populated-fields-survive-wrap` passes. |
| ERR-03 | 01-02 | Sentinels and `*APIError` inspectable via `errors.Is`/`errors.As` | PASS | `TestSentinels_ErrorsIs` (5×) + `TestAPIError_Is` (8 cases including wildcard, status mismatch, path/body/message ignored, non-APIError target) + `TestAPIError_ErrorsAs` all pass. |
| ERR-04 | 01-02, 01-05 | Error messages do not leak secrets / transport context | PASS | `(*APIError).Error()` (`errors.go:76-81`) omits Body. `TestAPIError_Error/body-never-in-error-string` locks this. Validator errors quote only caller-supplied input. `TestValidators_NoSensitiveData` (3 subtests) locks the no-leak invariant (note W-04 about test shape — non-blocking). |
| VALID-01 | 01-05 | Country code is 2 letters | PASS | `validate.go:28-34` (`validateCountry`). CL-02 relaxes to "case-insensitive, canonicalize to uppercase" — recorded in PROJECT.md Key Decisions CL-02. `TestValidateCountry/accept/{uppercase,lowercase,mixed,DE,fr}` and `reject/{empty,one-letter,three-letters,letter+digit,whitespace,non-ASCII,…}` all pass. |
| VALID-02 | 01-05 | validFrom <= validTo | PASS | `validate.go:82-85`. `TestValidateDateRange/reject/{from after to by one day,from year after to year}` pass with `ErrInvalidDateRange`. |
| VALID-03 | 01-05 | Window <= 3 years | PASS | `validate.go:91-94` uses backward-anchored `to.AddDate(-3, 0, 0)` (CL-06 — recorded in PROJECT.md). `TestValidateDateRange/accept/{exact 3 years inclusive, leap-year 3 years inclusive}` PASS; `reject/{exact 3 years plus 1 day, leap-year 3 years plus 1 day, extreme 25-year}` all reject with `ErrDateRangeTooLarge`. |
| VALID-04 | 01-05 | Language code shape check | PASS | `validate.go:49-55` (`validateLanguage`). Case-insensitive accept, canonicalize to lowercase. `TestValidateLanguage/{accept×5,reject×10}` all pass. |
| CLIENT-10 | 01-06 | No `init()` side effects, no global mutable state | PASS | `internal_test.go:80-196` is an AST-walking audit. `TestNoInitOrGlobalState/no_init_and_no_unexpected_package_vars` passes. Allowlist exposes 6 entries (5 exported sentinels + `errEmptyDate`). Sanity grep `grep -rn 'func init'` finds only doc-comment matches, zero production declarations. **Caveat W-03**: `skipDirs` currently includes `"internal"` — the audit's blind spot for the `internal/` subtree is recorded as a follow-up; for Phase 1 with no `internal/*` subpackages yet, the invariant holds. |

**Score:** 14/14 requirements verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `go.mod` | module path + `go 1.23` directive | VERIFIED | `module github.com/egeek-tech/go-openholidays`, `go 1.23` (line 3), only test-only `testify` require. |
| `go.sum` | testify + transitive sums only | VERIFIED | testify v1.11.1 + davecgh/go-spew + pmezard/go-difflib + gopkg.in/yaml.v3 (all indirect/test-only). |
| `LICENSE` | MIT at repo root | VERIFIED | Present at `/data/git/private/holidays/LICENSE`. |
| `doc.go` | Package-level godoc | VERIFIED | `doc.go:1-15` — single `// Package openholidays` block. |
| `version.go` | exported Version identifier | VERIFIED | `const Version = "0.1.0"` (see W-02 caveat — ldflags claim is incorrect for const). |
| `errors.go` | 5 sentinels + *APIError | VERIFIED | `errors.go:17-36` (sentinels), `errors.go:57-105` (APIError type + Error + Is). |
| `date.go` | Date type + NewDate/ParseDate + JSON methods + comparisons + DaysUntil | VERIFIED | 173 lines, all exported symbols carry godoc, 100% coverage. |
| `types.go` | Holiday/Country/Language/Subdivision/LocalizedText/SubdivisionRef/GroupRef + HolidayType ×6 + NameFor ×3 | VERIFIED | 243 lines, all symbols documented, 100% coverage. |
| `validate.go` | validateCountry/validateLanguage/validateDateRange | VERIFIED | 116 lines, all unexported per ARCHITECTURE.md, 100% coverage. |
| `internal_test.go` | AST audit for CLIENT-10 | VERIFIED | `TestNoInitOrGlobalState` passes; closed allowlist of 6 vars enforced. |
| Test files | one TestXxx per production function, t.Run per case, testify | VERIFIED | `date_test.go`, `errors_test.go`, `types_test.go`, `validate_test.go` all conform (modulo W-04 cross-cutting test shape note). |

### Key Link Verification

| From | To | Via | Status | Details |
|------|-----|-----|--------|---------|
| `Date.UnmarshalJSON` | `errEmptyDate` | %w wrap | WIRED | `date.go:90,97` — `null` and empty string both `fmt.Errorf("openholidays: %w", errEmptyDate)`. `TestDate_UnmarshalJSON/{null_returns_errEmptyDate,empty_json_string_returns_errEmptyDate}` assert `errors.Is(err, errEmptyDate)`. |
| Validators | sentinels | `fmt.Errorf("%w: %q", ErrX, code)` | WIRED | `validate.go:31,52,84,93`. `TestValidate*` cases assert `errors.Is(err, Err*)` matches through the wrap. |
| `(*APIError).Is` | status-code branching | typed switch on target | WIRED | `errors.go:96-105`. `TestAPIError_Is` (8 cases) locks every branch including wildcard. |
| `Holiday.StartDate/EndDate` | `Date` JSON shape | embedded JSON method | WIRED | `types.go:98-101` typed as `Date`; `TestHoliday_JSON` round-trips through `json.Marshal`/`json.Unmarshal` end-to-end. |
| `Country.NameFor`/`Language.NameFor`/`Subdivision.NameFor` | `pickLocalized` | shared helper | WIRED | `types.go:163-165,180-182,220-222,233-243`. Three NameFor tests pass with identical contract. |
| `TestNoInitOrGlobalState` | every production `.go` file at repo root | filepath.WalkDir + go/parser | WIRED | `internal_test.go:80-196`. Sanity guard at line 187 asserts `filesSeen >= 4` so a broken walk would fail loudly. |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Module builds | `cd /data/git/private/holidays && go build ./...` | exit 0 | PASS |
| Module vets | `cd /data/git/private/holidays && go vet ./...` | exit 0 | PASS |
| Full test suite race+cover | `go test -race -cover ./...` | `ok ... coverage: 100.0% of statements` | PASS |
| Total t.Run cases | `go test -race -v ./... | grep -c '^=== RUN\\|--- (PASS|FAIL)'` | 209 lines, 0 FAIL | PASS |
| ROADMAP #1 literal roundtrip | standalone `go run` against package | `ROUNDTRIP_OK / NULL_REJECTED_OK / EMPTY_REJECTED_OK` | PASS |
| ROADMAP #2 literal `errors.Is` × 5 sentinels | standalone `go run` | `SENTINEL_IS_OK (5 sentinels matched)` | PASS |
| ROADMAP #3 literal `errors.As` with populated fields | standalone `go run` | `APIERROR_AS_OK` | PASS |
| Zero non-stdlib production imports | `go list -deps -f '{{if not .Standard}}{{.ImportPath}}{{end}}' ./...` | only `github.com/egeek-tech/go-openholidays` (self) | PASS |
| Fuzz panic-freedom (CL-03) | `go test -fuzz=FuzzDateUnmarshal -fuzztime=10s ./...` | 1,380,046 execs, 0 crashers, PASS | PASS |
| No `init()` in production | `grep -rn 'func init' --include='*.go' --exclude-dir=.planning --exclude-dir=.claude .` | only doc-comment hits in `internal_test.go` | PASS |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (none) | — | TBD/FIXME/XXX/TODO/HACK/PLACEHOLDER/"coming soon"/"not yet implemented" | — | Clean grep across all production AND test files. No debt markers anywhere in the Phase 1 surface. |

### PROJECT.md Key Decisions Verification

| Row | Status | Evidence |
|-----|--------|----------|
| CL-01 | PRESENT | `.planning/PROJECT.md:98` — "Phase 1 ships 5 sentinel errors instead of ROADMAP-literal 4 (adds ErrInvalidDateRange)" |
| CL-02 | PRESENT | `.planning/PROJECT.md:99` — "validateCountry is case-insensitive (canonicalizes input to uppercase)" |
| CL-03 | PRESENT | `.planning/PROJECT.md:100` — "FuzzDateUnmarshal ships in Phase 1 instead of Phase 5" |
| CL-04 | PRESENT | `.planning/PROJECT.md:101` — "HolidayType ships 6 PascalCase upstream-verified values" |
| CL-05 | PRESENT | `.planning/PROJECT.md:102` — "Country/Language/Subdivision NameFor(lang) renamed from TYPES-05's literal Name(lang)" |
| CL-06 | PRESENT | `.planning/PROJECT.md:103` — "validateDateRange uses backward-anchored to.AddDate(-3, 0, 0)" |

All 6 CL-XX rows are recorded in the Key Decisions table per the phase-closeout contract.

### Decision Traceability (D-01..D-23)

| Decision | Subject | Verification |
|----------|---------|--------------|
| D-01 | module path `github.com/egeek-tech/go-openholidays` | `go.mod:1` matches |
| D-02 | package name `openholidays` | All files declare `package openholidays` |
| D-03 | const `Version = "0.1.0"` | `version.go:10` matches |
| D-04 | MIT LICENSE at repo root | `LICENSE` present |
| D-05 | `type Date struct { time.Time }` | `date.go:37-39` matches |
| D-06 | unexported `errEmptyDate` sentinel | `date.go:24`, allowlisted in `internal_test.go:56` |
| D-07 | MarshalJSON/UnmarshalJSON contract | `date.go:70-105` matches |
| D-08 | `String()` shadows `time.Time.String` | `date.go:113-115` matches |
| D-09 | Equal/Before/After/Compare via toUTCMidnight | `date.go:122-142` matches |
| D-10 | DaysUntil inclusive count | `date.go:154-164` matches |
| D-11 | NewDate/ParseDate constructors | `date.go:44-63` matches |
| D-12 | FuzzDateUnmarshal panic-freedom | `date_test.go:353-366` matches; passes with 0 crashers |
| D-13 | 5 exported sentinels | `errors.go:17-36` matches |
| D-14 | sentinel prefix "openholidays: " | All 5 messages verified by `TestSentinelErrors` |
| D-15 | APIError.Is matches by StatusCode, wildcard at 0 | `errors.go:96-105` matches; `TestAPIError_Is` 8 cases |
| D-16 | APIError has no Unwrap method | Confirmed by grep: no `Unwrap()` on `*APIError` in `errors.go` |
| D-17 | 4 KiB Body cap | DEFERRED to Phase 2 (per plan 02 must_haves / phase boundary); not enforced in Phase 1, type accepts `[]byte` of any size. Phase 2 will cap on construction from `*http.Response`. |
| D-18 | Error() format (with/without Message) | `errors.go:76-81` matches; `TestAPIError_Error` 3 cases |
| D-19 | APIError construction from `*http.Response` deferred to Phase 2 | Phase 1 ships type + methods only; matches plan must_haves |
| D-20 | validateCountry canonicalizes to uppercase | `validate.go:28-34` matches |
| D-21 | validateLanguage canonicalizes to lowercase — REVERSED to uppercase 2026-05-30 (quick 260530-dvc); API is case-sensitive, lowercase returned English | `validate.go` matches (now `strings.ToUpper`) |
| D-22 | validateDateRange uses `time.AddDate` (corrected to backward-anchored per CL-06) | `validate.go:91` matches |
| D-23 | Validator errors quote offending value with %q / include from=/to= dates | `validate.go:31,52,84,93` matches; `TestValidators_NoSensitiveData` locks the no-transport-leak invariant |

### Human Verification Required

None for Phase 1 — this phase ships types, errors, validators, and a build/vet/test pipeline. Every must-have is observably true via grep, AST audit, runnable test, or standalone `go run`. No UI surface, no real-time behavior, no external service integration.

### Code Review Follow-ups (Advisory, Non-Blocking)

`01-REVIEW.md` flagged 4 Warnings and 7 Info; 0 Critical. None blocks Phase 1 goal achievement. Carried forward in the frontmatter `follow_ups` block:

- **W-01** (`validate.go:28-34, 49-55`) — Validators canonicalize via `strings.ToUpper`/`ToLower` *before* the ASCII shape check. Four Unicode code points (U+0130, U+0131, U+017F, U+212A) fold to ASCII and bypass the check. The validators are unexported and unused at runtime in Phase 1; Phase 2 must fix this before wiring validators into HTTP dispatch.
- **W-02** (`version.go:7-10`) — Godoc ldflags example contradicts the `const` declaration. Either drop the paragraph or change to `var`.
- **W-03** (`internal_test.go:64-70`) — `skipDirs` includes `"internal"`. Remove before any `internal/*` subpackage lands in Phase 2.
- **W-04** (`validate_test.go:241-299`) — `TestValidators_NoSensitiveData` is a cross-cutting test that breaks Gold Rule 3's one-test-per-prod-function shape. Either fold into per-validator subtests or add a Key Decisions exception.

The 7 Info-level findings (I-01..I-07) are listed in `01-REVIEW.md`; all are stylistic/polish items appropriate to address opportunistically. None block Phase 1 acceptance.

### Gaps Summary

No gaps. Every observable truth is supported by code that exists, is substantive, is wired, and produces real behavior under test. All 5 ROADMAP success criteria pass with literal-program proof. All 14 requirement IDs are claimed and verified. All 6 CL-XX clarifications are recorded in PROJECT.md.

---

_Verified: 2026-05-27_
_Verifier: Claude (gsd-verifier, goal-backward)_
