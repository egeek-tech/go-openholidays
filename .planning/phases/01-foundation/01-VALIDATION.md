---
phase: 1
slug: foundation
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-05-27
---

# Phase 1 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go stdlib `testing` + `github.com/stretchr/testify` v1.11.1 (test-only) |
| **Config file** | None — Go's `testing` package needs no config |
| **Quick run command** | `go test -race ./...` |
| **Full suite command** | `go test -race -cover ./...` |
| **Fuzz command (Phase 1)** | `go test -fuzz=FuzzDateUnmarshal -fuzztime=30s ./...` |
| **Estimated runtime** | ~5 seconds (unit + race), 30 seconds (fuzz cycle) |

---

## Sampling Rate

- **After every task commit:** Run `go test -race -run TestXxx ./...` for the affected file (subsecond)
- **After every plan wave:** Run `go test -race -cover ./...` (full suite — under 5 seconds)
- **Before `/gsd:verify-work`:** Full suite + `go test -fuzz=FuzzDateUnmarshal -fuzztime=30s ./...` + `gofmt -d ./...` + `go vet ./...` all green
- **Max feedback latency:** 5 seconds

---

## Per-Requirement Verification Map

| Req ID | Behavior | Test Type | Automated Command | File Exists | Status |
|--------|----------|-----------|-------------------|-------------|--------|
| TYPES-01 | `Holiday` decodes canonical OpenHolidays JSON; nullable `Comment`/`Subdivisions`/`Groups`/`Tags` handled; `Quality` tolerated as schema drift | unit (table-driven) | `go test -race -run 'TestHoliday_JSON' ./...` | ❌ W0 (`types_test.go`) | ⬜ pending |
| TYPES-02 | `Date.MarshalJSON`/`UnmarshalJSON` round-trip; rejects `null` and `""`; emits `"0001-01-01"` for zero `Date{}` | unit + fuzz | `go test -race -run 'TestDate_MarshalJSON\|TestDate_UnmarshalJSON\|TestNewDate\|TestParseDate' ./...` and `go test -fuzz=FuzzDateUnmarshal -fuzztime=30s ./...` | ❌ W0 (`date_test.go`) | ⬜ pending |
| TYPES-03 | `LocalizedText`/`SubdivisionRef`/`GroupRef` decode with verified field names | unit | `go test -race -run 'TestLocalizedText_JSON\|TestSubdivisionRef_JSON\|TestGroupRef_JSON' ./...` | ❌ W0 (`types_test.go`) | ⬜ pending |
| TYPES-04 | `HolidayType` constants exist for all 6 upstream values (`Public`, `Bank`, `Optional`, `School`, `BackToSchool`, `EndOfLessons`); wire-format match | unit | `go test -race -run 'TestHolidayType' ./...` | ❌ W0 (`types_test.go`) | ⬜ pending |
| TYPES-05 | `Country.NameFor(lang)` / `Language.NameFor(lang)` / `Subdivision.NameFor(lang)` return localized text; falls back to first entry on miss; returns `""` only when slice is empty; case-insensitive language match | unit (table-driven) | `go test -race -run 'TestCountry_NameFor\|TestLanguage_NameFor\|TestSubdivision_NameFor' ./...` | ❌ W0 (`types_test.go`) | ⬜ pending |
| ERR-01 | All 5 sentinels non-nil; error message starts with `"openholidays: "`; distinct identities | unit | `go test -race -run 'TestSentinelErrors' ./...` | ❌ W0 (`errors_test.go`) | ⬜ pending |
| ERR-02 | `errors.As(wrappedErr, &apiErr)` extracts `*APIError` with populated `StatusCode`, `Path`, `Body`, `Message` fields | unit | `go test -race -run 'TestAPIError_ErrorsAs' ./...` | ❌ W0 (`errors_test.go`) | ⬜ pending |
| ERR-03 | `errors.Is(fmt.Errorf("country %q: %w", "ZZZ", ErrInvalidCountry), ErrInvalidCountry) == true` for every sentinel | unit (table-driven) | `go test -race -run 'TestSentinels_ErrorsIs' ./...` | ❌ W0 (`errors_test.go`) | ⬜ pending |
| ERR-04 | Validator error messages carry only the offending code/date value (no secrets) | unit | `go test -race -run 'TestValidators_NoSensitiveData' ./...` | ❌ W0 (`validate_test.go`) | ⬜ pending |
| VALID-01 | `validateCountry` accepts `"PL"`/`"pl"`/`"Pl"`; canonicalizes to `"PL"`; rejects `""`/`"P"`/`"POL"`/`"P1"`/non-ASCII/whitespace | unit (table-driven) | `go test -race -run 'TestValidateCountry' ./...` | ❌ W0 (`validate_test.go`) | ⬜ pending |
| VALID-02 | `validateDateRange(from=2025-12-31, to=2025-01-01)` returns wrapped `ErrInvalidDateRange`; equal dates accepted | unit (table-driven) | `go test -race -run 'TestValidateDateRange' ./...` | ❌ W0 (`validate_test.go`) | ⬜ pending |
| VALID-03 | `validateDateRange(2025-01-01, 2028-01-01)` accepted (exactly 3 years); `+1d` rejected; leap-year boundary (`2024-02-29 → 2027-02-28` pass; `→ 2027-03-01` fail) | unit (table-driven) | `go test -race -run 'TestValidateDateRange' ./...` | ❌ W0 (`validate_test.go`) | ⬜ pending |
| VALID-04 | `validateLanguage` accepts `"pl"`/`"PL"`/`"Pl"`; canonicalizes to `"pl"`; rejects `""`/`"p"`/`"pol"`/`"p1"`/non-ASCII | unit (table-driven) | `go test -race -run 'TestValidateLanguage' ./...` | ❌ W0 (`validate_test.go`) | ⬜ pending |
| CLIENT-10 | No `func init()` in `*.go` files (excluding tests); no package-level `var` outside the 5 sentinels; `Version` is `const` | unit (AST scan) + manual grep | `go test -race -run 'TestNoInitOrGlobalState' ./...` + `grep -rn 'func init' --include='*.go'` returns no matches | ❌ W0 (`internal_test.go`) | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] **`go.mod` / `go.sum` rewrite** — delete stray POC files, `go mod init github.com/egeek-tech/go-openholidays`, `go mod edit -go=1.23`, `go get github.com/stretchr/testify@v1.11.1`, `go mod tidy`
- [ ] `errors_test.go` — TestSentinelErrors, TestAPIError_Error, TestAPIError_Is, TestAPIError_ErrorsAs, TestSentinels_ErrorsIs (ERR-01 .. ERR-03)
- [ ] `date_test.go` — TestNewDate, TestParseDate, TestDate_MarshalJSON, TestDate_UnmarshalJSON, TestDate_String, TestDate_Equal, TestDate_Before, TestDate_After, TestDate_Compare, TestDate_DaysUntil, FuzzDateUnmarshal (TYPES-02, D-12)
- [ ] `types_test.go` — TestHoliday_JSON (full fixture), TestLocalizedText_JSON, TestSubdivisionRef_JSON, TestGroupRef_JSON, TestCountry_NameFor, TestLanguage_NameFor, TestSubdivision_NameFor, TestHolidayType_String (TYPES-01, TYPES-03, TYPES-04, TYPES-05)
- [ ] `validate_test.go` — TestValidateCountry, TestValidateLanguage, TestValidateDateRange, TestValidators_NoSensitiveData (VALID-01 .. VALID-04, ERR-04)
- [ ] `internal_test.go` — TestNoInitOrGlobalState (CLIENT-10) using `go/parser` + `go/ast` to walk `*.go` files and assert no `init()` declarations and no package-level `var` outside the locked sentinel list

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Key Decisions table entries for CL-01..CL-05 added to PROJECT.md | CONTEXT.md scope clarifications | Documentation-level change; verified by reading PROJECT.md | Verify PROJECT.md `## Key Decisions` table contains rows for CL-01 (5 sentinels), CL-02 (case-insensitive country), CL-03 (fuzz in Phase 1), CL-04 (6 HolidayType values), CL-05 (`NameFor` rename) |
| `gofmt -d ./...` produces no diff | Code style (CLAUDE.md) | Style check, runs via CI in later phases | `gofmt -d ./... && echo OK` exits 0 with no output |
| `go vet ./...` clean | CLAUDE.md lint baseline | Static analysis check | `go vet ./...` returns no findings |

---

## Validation Sign-Off

- [ ] All requirement IDs have automated verify (`go test -race -run ...`) commands
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING test files (`errors_test.go`, `date_test.go`, `types_test.go`, `validate_test.go`, `internal_test.go`)
- [ ] No watch-mode flags (Go's `testing` runs once per invocation)
- [ ] Feedback latency < 5s for quick suite, < 30s for fuzz
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
