---
phase: 01-foundation
plan: 04
subsystem: domain-types
tags:
  - types
  - json
  - localization
  - wire-contract
dependency_graph:
  requires:
    - 01-03  # Date type (date.go)
  provides:
    - Holiday
    - Country
    - Language
    - Subdivision
    - LocalizedText
    - SubdivisionRef
    - GroupRef
    - HolidayType
    - HolidayTypePublic
    - HolidayTypeBank
    - HolidayTypeOptional
    - HolidayTypeSchool
    - HolidayTypeBackToSchool
    - HolidayTypeEndOfLessons
    - Country.NameFor
    - Language.NameFor
    - Subdivision.NameFor
    - pickLocalized
  affects:
    - Phase 2 endpoint methods (json.Unmarshal targets)
    - Phase 3 helpers (Holiday.Days, Holiday.Range, Holiday.IsInRegion, Holiday.NameFor)
tech_stack:
  added: []  # zero new runtime deps; testify already added by Plan 01-01
  patterns:
    - "Plain structs with JSON tags matching upstream camelCase wire shape exactly"
    - "Typed-string enum (HolidayType) with PascalCase constants matching upstream values"
    - "NameFor(lang) accessor — case-insensitive linear scan via strings.EqualFold with fallback-to-first"
    - "Default lenient decoding (no DisallowUnknownFields) — strict decoding deferred to Phase 4"
key_files:
  created:
    - types.go
    - types_test.go
  modified: []
decisions:
  - "CL-04 ratified in code: shipped all 6 verified-upstream HolidayType values (Public, Bank, Optional, School, BackToSchool, EndOfLessons). REQUIREMENTS.md TYPES-04 listed an outdated 4-value set including spurious 'Observance' — Observance is NOT in the live upstream OpenAPI spec (verified 2026-05-27). Must be recorded in PROJECT.md Key Decisions by Plan 06."
  - "CL-05 ratified in code: accessor named NameFor(lang) not Name(lang). The Name field on Country/Language/Subdivision is []LocalizedText; a method named Name(lang) would collide with the field name. NameFor(lang) is the natural rename and is symmetric across all three types. Must be recorded in PROJECT.md Key Decisions by Plan 06."
  - "A4 ratified: RegionalScope and TemporalScope ship as plain string (not typed enums) for v0.1.0. Closed value sets documented in godoc (National/Regional/Local; FullDay/HalfDay). Typed enums deferred to v0.2 if a downstream helper needs to branch on these values."
  - "A5 ratified: Holiday.Tags ([]string) and Holiday.Quality (string) ship. Tags is newly verified during the 2026-05-27 OpenAPI fetch (not previously in REQUIREMENTS.md or CONTEXT.md). Quality is the schema-drift field observed in real responses but absent from the OpenAPI spec (Pitfall OH-2)."
  - "JSON-1 lenient default contract locked: default json.Unmarshal accepts unknown extra fields. Phase 4 will ship opt-in strict decoding via DisallowUnknownFields. Test 'decode tolerates unknown extra field' guards against accidental regression."
metrics:
  duration_minutes: 2
  completed_date: "2026-05-27"
  tasks_completed: 2
  files_created: 2
  files_modified: 0
  loc_production: 243
  loc_test: 390
  coverage_percent: 100
---

# Phase 1 Plan 4: Domain Type Contract Summary

Locked the complete public type surface for go-openholidays — Holiday + Country/Language/Subdivision reference types + LocalizedText/SubdivisionRef/GroupRef value types + the 6-value HolidayType enum + NameFor(lang) accessors — verified against the live OpenHolidays OpenAPI spec (2026-05-27).

## Tasks Executed

| Task | Description | Status | Commit |
| ---- | ----------- | ------ | ------ |
| 01-04-01 | Create types.go with all domain structs, HolidayType enum, NameFor accessors | done | `34d5c14` |
| 01-04-02 | Create types_test.go with JSON round-trip and NameFor coverage | done | `242d10f` |

## What Shipped

### `types.go` (243 LOC, zero runtime deps — `strings` stdlib only)

- **`HolidayType` typed string** plus six PascalCase constants — `HolidayTypePublic`, `HolidayTypeBank`, `HolidayTypeOptional`, `HolidayTypeSchool`, `HolidayTypeBackToSchool`, `HolidayTypeEndOfLessons`. Values match the upstream wire format exactly (also PascalCase).
- **`LocalizedText{Language, Text}`** — the (language, text) pair returned in every localized-string field. Both fields required upstream (minLength: 1).
- **`SubdivisionRef{Code, ShortName}`** and **`GroupRef{Code, ShortName}`** — lightweight references embedded in `Holiday.Subdivisions` / `Holiday.Groups` / `Subdivision.Groups`. Shorter Go names than upstream's `SubdivisionReference` / `GroupReference` per ARCHITECTURE.md naming guidance.
- **`Holiday`** struct with the full 13-field upstream-verified shape:
  - Required (no omitempty): `ID`, `StartDate`, `EndDate`, `Type`, `Name`, `Nationwide`, `RegionalScope`, `TemporalScope`.
  - Nullable (omitempty): `Comment`, `Subdivisions`, `Groups`, `Tags`, `Quality`.
  - `StartDate` and `EndDate` use the Plan 01-03 `Date` type.
  - Godoc on `Quality` explicitly documents the schema-drift status (Pitfall OH-2); godoc on `RegionalScope`/`TemporalScope` documents the closed value sets (Assumption A4).
- **`Country{IsoCode, Name, OfficialLanguages}`** with `NameFor(lang) string`.
- **`Language{IsoCode, Name}`** with `NameFor(lang) string`.
- **`Subdivision{Code, ShortName, Name, Category, OfficialLanguages, IsoCode?, Comment?, Children?, Groups?}`** with `NameFor(lang) string`. `Children []Subdivision` is the recursive case.
- **`pickLocalized(entries, lang) string`** — single unexported helper backing all three `NameFor` methods. Linear scan with `strings.EqualFold` (case-insensitive), fallback to `entries[0].Text` on miss, returns `""` only when `entries` is empty.

### `types_test.go` (390 LOC, testify-based)

Eight `TestXxx` functions, one per exported production identifier, every case wrapped in `t.Run`:

| Test | Cases | Coverage |
| ---- | ----- | -------- |
| `TestHolidayType_constants` | 6 | Each constant stringifies to the verified upstream wire value (TYPES-04 / CL-04). |
| `TestLocalizedText_JSON` | 2 | Marshal + unmarshal round-trip (TYPES-03). |
| `TestSubdivisionRef_JSON` | 2 | Marshal + unmarshal round-trip (TYPES-03). |
| `TestGroupRef_JSON` | 2 | Marshal + unmarshal round-trip (TYPES-03). |
| `TestHoliday_JSON` | **4** | (1) Single-day Wigilia Bożego Narodzenia — `omitempty` on nullable fields verified via `JSONEq`. (2) Multi-day Śląskie ferie zimowe — all nullable fields populated, full round-trip. (3) Decode with schema-drift `quality:"Verified"` (Pitfall OH-2 tolerance). (4) Decode with unknown extra fields (JSON-1 lenient default). (TYPES-01) |
| `TestCountry_NameFor` | 9 | Exact match, case-insensitive match (`PL`/`Pl`), miss-falls-back-to-first, empty-lang-falls-back-to-first, nil `Name` → `""`, empty `Name` → `""`. (TYPES-05 / CL-05) |
| `TestLanguage_NameFor` | 6 | Same contract as `TestCountry_NameFor` against language-name fixtures. |
| `TestSubdivision_NameFor` | 7 | Same contract against subdivision-name fixtures. |

All non-English fixture text (`"Wigilia Bożego Narodzenia"`, `"Śląskie"`, `"Ferie zimowe"`, `"polski"`, etc.) reflects real upstream OpenHolidays responses — admitted per CONVENTIONS.md Rule 1 testdata-fixture exception.

## Verification Evidence

```text
$ go test -race -cover ./...
ok  	github.com/egeek-tech/go-openholidays	1.018s	coverage: 100.0% of statements

$ gofmt -l types.go types_test.go
(no output — clean)

$ go vet ./...
(no output — clean)

$ go build ./...
(no output — clean)

$ grep -cE '^\s+HolidayType(Public|Bank|Optional|School|BackToSchool|EndOfLessons)' types.go
6

$ ! grep -q 'HolidayTypeObservance' types.go && echo OK
OK

$ grep -cE 'func \(.+\) NameFor\(lang string\) string' types.go
3

$ grep -q 'Quality.*json:"quality,omitempty"' types.go && echo OK
OK

$ grep -q 'Tags.*json:"tags,omitempty"' types.go && echo OK
OK

$ grep -cE '^func Test[A-Za-z_]+\(t \*testing\.T\)' types_test.go
8
```

All success criteria from the plan satisfied:

- [x] Tasks 01-04-01 and 01-04-02 executed and committed atomically
- [x] types.go contains all required structs + 6 HolidayType constants + NameFor accessors + pickLocalized
- [x] NameFor uses `strings.EqualFold` (case-insensitive), falls back to first entry, returns `""` only when slice empty
- [x] types_test.go contains 8 TestXxx (one per exported production identifier)
- [x] Every test case lives in `t.Run`; `require` for preconditions, `assert` for verifications
- [x] TestHoliday_JSON has 4 sub-cases including schema-drift Quality and unknown extra field
- [x] Default lenient decode locked; strict decoding deferred to Phase 4
- [x] `go test -race -cover ./...` exits 0 with 100% coverage
- [x] `go build ./...` and `go vet ./...` succeed; `gofmt` clean

## Deviations from Plan

None — plan executed exactly as written. No Rule 1/2/3 auto-fixes triggered; no Rule 4 architectural decisions needed. No authentication gates. No checkpoints (plan was `autonomous: true`).

The two scope clarifications baked into the plan (CL-04 6-value HolidayType, CL-05 NameFor rename) and the two ratified Assumptions (A4 RegionalScope/TemporalScope as plain string, A5 Tags + Quality) are recorded above under `decisions` for Plan 06 to lift into PROJECT.md Key Decisions.

## Threat Flags

None. No new network endpoints, auth paths, file access patterns, or schema changes at trust boundaries beyond those already documented in the plan's `<threat_model>`. The four threats listed (T-01-04-JSON lenient default, T-01-04-OH2 schema-drift Quality, T-01-04-OH3 array-not-map Name, T-01-04-IL no PII) are all mitigated by code shipped in this plan and locked by tests.

## Forward Notes

- **Plan 01-05 (validators)** consumes the `Date` type from `date.go` (Plan 01-03) and the sentinel errors from `errors.go` (Plan 01-02). It does NOT consume anything new from this plan — `types.go` is a leaf at this phase.
- **Plan 01-06 (PROJECT.md Key Decisions ledger update)** must record CL-04 (6-value HolidayType / dropped Observance) and CL-05 (NameFor(lang) rename). The other CL-01/02/03 entries already accrued from earlier plans in this phase.
- **Phase 2 endpoint methods** will `json.Unmarshal` upstream bytes directly into these structs. The default-lenient decode contract locked by `TestHoliday_JSON` case "decode tolerates unknown extra field" guards against accidental adoption of `DisallowUnknownFields` before Phase 4's opt-in mode lands.

## Self-Check: PASSED

- FOUND: `types.go` (243 LOC) — verified via `wc -l types.go`
- FOUND: `types_test.go` (390 LOC) — verified via `wc -l types_test.go`
- FOUND: commit `34d5c14` (Task 1) — verified via `git log --oneline --all | grep -q 34d5c14`
- FOUND: commit `242d10f` (Task 2) — verified via `git log --oneline --all | grep -q 242d10f`
- FOUND: `go test -race -cover ./...` exit code 0, 100% coverage
- FOUND: `gofmt -l types.go types_test.go` produces no output
- FOUND: `go vet ./...` exit code 0
- FOUND: 6 HolidayType constants, no Observance, 3 NameFor methods, Quality + Tags fields with omitempty, 8 TestXxx functions
