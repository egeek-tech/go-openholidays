---
phase: 01-foundation
reviewed: 2026-05-28T00:00:00Z
depth: standard
files_reviewed: 14
files_reviewed_list:
  - LICENSE
  - date.go
  - date_test.go
  - doc.go
  - errors.go
  - errors_test.go
  - go.mod
  - go.sum
  - internal_test.go
  - types.go
  - types_test.go
  - validate.go
  - validate_test.go
  - version.go
findings:
  critical: 0
  warning: 2
  info: 5
  total: 7
status: issues_found
---

# Phase 01 — Code Review (re-review)

## Summary

Re-review of the 14 Phase 01 foundation files against the prior 2026-05-27 review (0 Critical + 4 Warning + 7 Info). Subsequent phases (02 transport, 03 endpoints, 04 resilience) have touched two of these files: `errors.go` gained `ErrResponseTooLarge` (Phase 02) and `ErrMalformedResponse` (Phase 03), and `internal_test.go` gained the `CacheHitContextKey` allowlist entry (Phase 04) while also removing the `"internal"` skipDir per the IN-05 follow-up. `validate.go` was rewritten in Phase 02 around `isTwoASCIILetters` (the W-01 fix), collapsing the previous `isTwoASCIIUppers` / `isTwoASCIILowers` pair into a single helper.

**Prior-finding status (11 items):** Four are REMEDIATED — W-01 (Unicode case-fold bypass), W-03 (CLIENT-10 audit skipping `internal/`), I-06 (`isTwoASCIIUppers`/`Lowers` duplication), and the four W-01 regression cases are explicitly locked in `validate_test.go` lines 74-77 and 153-156. Seven remain OPEN — W-02 (`Version const` vs ldflags doc claim), W-04 (cross-cutting `TestValidators_NoSensitiveData`), I-01 (`errEmptyDate` uses `fmt.Errorf`), I-02 (em-dashes), I-03 (unknown `HolidayType`), I-05 (`require.Equal` where `assert.Equal` applies), and I-07 (`Date.UnmarshalJSON` echoes raw `b` without length cap). I-04 (`Subdivision.Children` unbounded recursion) was correctly deferred and the stdlib's implicit 10000-level cap remains in force — no action needed for Phase 01.

**No new BLOCKER-tier defects surfaced.** The two remaining warnings (W-02, W-04) and five remaining info items track verbatim from the prior review with no new context that would change their severity.

## Prior-Finding Re-Verification

| Prior ID | Status | Evidence |
|----------|--------|----------|
| W-01 (Unicode case-fold bypass) | REMEDIATED | `validate.go:34, 58` call `isTwoASCIILetters(code)` on ORIGINAL bytes BEFORE `strings.ToUpper`/`ToLower`; regression cases `"ıA"`, `"ſA"`, `"ıı"`, `"ſſ"`, `"KK"`, `"İa"`, `"İİ"`, `"Ka"` locked in `validate_test.go:74-77, 153-156`. |
| W-02 (Version `const` + ldflags doc) | OPEN | `version.go:10` still `const Version = "0.1.0"`; godoc lines 7-10 still document the `-ldflags '-X ...Version=...'` override that cannot work on `const`. See W-02 below. |
| W-03 (`internal/` skipDir) | REMEDIATED | `internal_test.go:97-102` skipDirs is now `{".planning", ".git", ".claude", "testdata"}` — `"internal"` removed; the IN-05 rationale comment on lines 90-96 is consistent with the prior recommendation. |
| W-04 (cross-cutting validator test) | OPEN | `validate_test.go:286-333` still present; no `Key Decisions` entry was added to PROJECT.md. See W-04 below. |
| I-01 (`fmt.Errorf` w/o verbs) | OPEN | `date.go:24` still `var errEmptyDate = fmt.Errorf("openholidays: empty date string")`. See I-01 below. |
| I-02 (em-dashes in docs) | OPEN | em-dashes still throughout `doc.go`, `errors.go`, `types.go`, `validate.go`, etc.; deliberately deferred — see I-02. |
| I-03 (unknown `HolidayType`) | OPEN | `types.go:103` Type field doc still does not warn callers about non-allowlisted upstream values; no `IsKnown()` helper added. See I-03 below. |
| I-04 (`Subdivision.Children` unbounded) | DEFERRED (no action required) | stdlib `encoding/json` 10000-level cap still applies; no Phase 01 helper added that walks `Children`. Re-flag when Phase 03 traversal helpers land. |
| I-05 (`require.Equal` misuse) | OPEN | `errors_test.go:152` still `require.Equal(t, c.want, c.err.Error())` where `assert.Equal` is the right choice (verification, not precondition). See I-05 below. |
| I-06 (`isTwoASCIIUppers`/`Lowers` dup) | REMEDIATED | `validate.go:110-121` declares a single `isTwoASCIILetters`; the dual `isTwoASCIIUppers`/`isTwoASCIILowers` helpers are gone. |
| I-07 (`UnmarshalJSON` echoes `b`) | OPEN | `date.go:93` still `fmt.Errorf("openholidays: date must be a JSON string, got %s", b)` with no length cap. See I-07 below. |

## Warnings (2)

### WR-01: `version.go` documents an ldflags override that cannot work — `Version` is `const`, not `var`

**File:** `version.go:7-10`
**Issue:** The godoc on `Version` claims the constant "can be overridden at link time, for example: `go build -ldflags '-X github.com/egeek-tech/go-openholidays.Version=0.1.1-rc1'`". The Go linker's `-X` flag only applies to package-level `string` `var`s; on a `const` it silently has no effect. The declaration on line 10 is `const Version = "0.1.0"`, so any release pipeline that follows the documented command will build a binary that reports the compiled-in default while the operator believes the override took.

This is verifiable from the Go documentation (`cmd/link` docs) and was empirically confirmed in the prior review. It violates Gold Rule 2 ("never guess; verify or ask") inside committed documentation — a consumer who follows the example gets exactly the failure mode the rule is written to prevent. Phase 05 (`cmd/ohcli`) is planned to read this value for `--version`, so the documented release pipeline produces binaries that lie about their version.

**Fix:** Either (a) drop the ldflags paragraph from the godoc if the contract is "compiled-in only", or (b) change the declaration to `var Version = "0.1.0"` so the documented override actually works. Option (b) preserves the documented capability at negligible cost (the value is read once per `Client` for the User-Agent string). The important property is that committed docs match committed behavior.

```go
// Option (b) — one-line change:
var Version = "0.1.0"
```

If (b) is chosen, also add `"Version": {}` to `allowedVars` in `internal_test.go:72-82` so the CLIENT-10 AST audit accepts the new package-level `var`.

### WR-02: `TestValidators_NoSensitiveData` is a cross-cutting test that Gold Rule 3 does not authorize

**File:** `validate_test.go:286-333`
**Issue:** Gold Rule 3 (CLAUDE.md): *"Exactly one TestXxx function per exported production function."* `validate.go` declares three unexported validators (`validateCountry`, `validateLanguage`, `validateDateRange`) and `validate_test.go` already ships three direct one-to-one tests (`TestValidateCountry`, `TestValidateLanguage`, `TestValidateDateRange`) for each. `TestValidators_NoSensitiveData` then adds a fourth test that probes the ERR-04 transport-leak invariant across all three validators — its body re-invokes each validator and asserts only the absence of forbidden substrings. The one-test-per-prod-function shape is broken without a `Key Decisions` entry authorizing the exception.

The earlier files in the same phase (`date_test.go`, `errors_test.go`, `types_test.go`) follow Gold Rule 3 rigidly. Future contributors reading `validate_test.go` will reasonably believe the rule is loose and normalize further drift.

**Fix:** Two reasonable paths:

1. **Merge (lowest-friction):** Fold the leak-guard assertion into each per-validator test as an additional `t.Run("error_message_has_no_transport_leak", ...)` subtest. Preserves the one-test-per-prod-function shape strictly.
2. **Document the exception:** Add an entry to PROJECT.md's Key Decisions log that ERR-04 regression guards are admitted as cross-cutting tests, and update Gold Rule 3's wording to permit this category. The current code does neither, so the file is silently inconsistent with the project rule it should obey.

## Info (5)

### IN-01: `errEmptyDate` uses `fmt.Errorf` with no verbs where `errors.New` is canonical

**File:** `date.go:24`
**Issue:** `var errEmptyDate = fmt.Errorf("openholidays: empty date string")`. With no `%w` or other format verbs, `fmt.Errorf` is functionally identical to `errors.New` but runs through `fmt`'s parser at package-init time. All seven sentinels in `errors.go` (lines 21, 25, 29, 32, 36, 44, 61) correctly use `errors.New`; `errEmptyDate` is the only odd one out.

**Fix:**
```go
var errEmptyDate = errors.New("openholidays: empty date string")
```
One-line change; brings consistency with the sibling file.

### IN-02: Non-ASCII em-dashes ("—") sprinkled through doc comments

**File:** `doc.go:7`, `date.go:46-47, 67, 73`, `errors.go:39-42`, `types.go:7, 86, 105`, `validate.go:30, 75-76`, others.
**Issue:** Gold Rule 1 demands English-language docs. Em-dashes are not ASCII (U+2014). Go stdlib style by convention uses ASCII `--` or `-` for these breaks. The em-dashes do not break anything mechanical — `gofmt` and `go doc` handle them — but they are a stylistic departure from the rest of the Go ecosystem and a tiny seam that makes copy-paste from terminals awkward.

**Fix:** Optional, low priority. If the project wants strict ASCII-clean source files, replace `—` with ` -- ` or `-`. Otherwise document the deliberate choice in CONVENTIONS.md so the choice is intentional rather than accidental drift.

### IN-03: `Holiday` permits unknown `Type` values silently (callers cannot detect schema drift in `Type`)

**File:** `types.go:103, 94-138`
**Issue:** `Type HolidayType` is a typed string alias. JSON unmarshal of `"type": "Religious"` (an upstream value not in the six allowlist constants) succeeds with `Type` populated as `"Religious"`. The Holiday godoc on lines 80-93 mentions the six constants but does not warn callers that other values may appear in the wild. The Phase 4 strict decoder surfaces unknown *fields* but not unknown enum *values* — those flow through the lenient and strict decoders identically because the underlying type is `string`.

**Fix:** Add one sentence to the `Holiday.Type` field doc, or extract a tiny helper `(HolidayType).IsKnown() bool` so callers can branch defensively:
```go
// IsKnown reports whether t matches one of the six HolidayType constants
// defined by this package. Callers MUST be prepared for upstream to return
// a value outside the constants; the default lenient decoder accepts unknown
// values, and IsKnown is the recommended check.
func (t HolidayType) IsKnown() bool {
    switch t {
    case HolidayTypePublic, HolidayTypeBank, HolidayTypeOptional,
        HolidayTypeSchool, HolidayTypeBackToSchool, HolidayTypeEndOfLessons:
        return true
    }
    return false
}
```
Neither is required for Phase 01 to ship; this is a doc/affordance polish.

### IN-04: `TestAPIError_Error` uses `require.Equal` where the convention asks for `assert.Equal`

**File:** `errors_test.go:152`
**Issue:** Gold Rule 3 split: *"require for preconditions (aborts the case), assert for verifications (reports without aborting)."* Line 152 (`require.Equal(t, c.want, c.err.Error())`) is the *verification* of the test case's primary outcome — `assert.Equal` is the right choice. The preceding `require.NotNil(t, c.err, ...)` on line 151 is correctly a precondition. The same pattern is used correctly elsewhere in the file (e.g., `errors_test.go:225` uses `assert.Equal`).

**Fix:** Change line 152 to `assert.Equal`. One-character edit; aligns with the rule and the rest of the file.

### IN-05: `Date.UnmarshalJSON` echoes raw `b` into the error message without length cap

**File:** `date.go:93`
**Issue:** `return fmt.Errorf("openholidays: date must be a JSON string, got %s", b)`. If `b` is megabytes of binary garbage (very unlikely from `encoding/json` which has already pre-tokenized the value, but possible if a caller invokes `UnmarshalJSON` directly with attacker-controlled bytes), the error string grows unbounded and may contain non-printable bytes. `FuzzDateUnmarshal` exercises panic-freedom but not error-string size.

**Fix:** Cap echoed bytes at ~64 with a `(truncated)` suffix when over. Practical risk is low because `encoding/json` already bounds token size, but the defense is one helper and aligns with the "never echo unbounded caller input into operator-visible strings" principle that informs ERR-04:
```go
if len(b) < 2 || b[0] != '"' || b[len(b)-1] != '"' {
    return fmt.Errorf("openholidays: date must be a JSON string, got %s", truncateForError(b, 64))
}
```
Defer to a future phase if a real abuse vector is identified.

## Verified Good (re-confirmed)

These were re-traced against the current files and confirmed correct (most carry over from the prior review):

- **Package layout & naming.** All files in `package openholidays`; `go.mod` declares `go 1.23` matching the locked floor.
- **Sentinel surface = 7 exported + 1 unexported.** `ErrInvalidCountry`, `ErrInvalidLanguage`, `ErrDateRangeTooLarge`, `ErrInvalidDateRange`, `ErrEmptyResponse`, `ErrResponseTooLarge` (Phase 02), `ErrMalformedResponse` (Phase 03), plus unexported `errEmptyDate`. The CLIENT-10 `allowedVars` map (internal_test.go:72-82) exactly matches plus `CacheHitContextKey` (Phase 04). `TestSentinelErrors` and `TestSentinels_ErrorsIs` (errors_test.go) exhaustively lock identity uniqueness and `errors.Is` recoverability for all seven.
- **Every exported symbol carries a godoc comment starting with its name.** Spot-checked `NewDate`, `ParseDate`, `Date`, all six Date helpers (`MarshalJSON`, `UnmarshalJSON`, `String`, `Equal`, `Before`, `After`, `Compare`, `DaysUntil`), `Holiday`, `HolidayType`, the six type constants, `LocalizedText`, `SubdivisionRef`, `GroupRef`, `Country`, `Language`, `Subdivision`, all three `NameFor` methods, `APIError`, `(*APIError).Error`, `(*APIError).Is`, every sentinel, `Version`. All conform.
- **`APIError.Is` semantics.** Wildcard (`StatusCode == 0`) matches any `*APIError`; non-zero matches by status only; `Path`/`Body`/`Message` ignored on target; non-`*APIError` targets never match. Implementation (errors.go:122-131) matches the godoc (errors.go:109-121) and `TestAPIError_Is` (errors_test.go:160-231) exercises every branch.
- **`Date.MarshalJSON` ↔ `Date.UnmarshalJSON` round-trip.** `MarshalJSON` always emits `"YYYY-MM-DD"` with quotes (12-byte capacity hint correct); `UnmarshalJSON` rejects `null`, `""`, non-string tokens, and malformed dates with wrapped `errEmptyDate` or `time.Parse` errors. `FuzzDateUnmarshal` corpus locks panic-freedom.
- **`Date.DaysUntil` arithmetic.** Re-traced for same-day (=1), one-day-forward (=2), one-day-backward (=-2), 14-day forward Śląskie ferie span, and the 14-day negative case. Inclusive count semantics are correct; UTC-midnight normalization eliminates DST perturbation because both operands are at 00:00 UTC and `Sub().Hours()/24` is a clean integer multiple well within `float64`'s 2^53 exact-integer range.
- **`validateDateRange` backward-anchored boundary.** `to.AddDate(-3, 0, 0)` correctly handles 2024-02-29 → 2027-02-28 as exact-3-year PASS and 2024-02-29 → 2027-03-01 as one-day-over FAIL; tests `validate_test.go:201-205, 244-250` lock both polarities. The decision to anchor at `to` rather than `from` avoids `time.AddDate`'s forward-overflow asymmetry that would mishandle leap-day `from` values.
- **No `init()` anywhere in production files.** Confirmed by reading each `.go` file and by `TestNoInitOrGlobalState`'s AST walk. With the W-03 fix (removing `"internal"` from skipDirs), the audit now covers any future `internal/*` package as well.
- **No global mutable state.** Only the eight allowlisted package-level `var`s exist: seven sentinels + `errEmptyDate` + `CacheHitContextKey`. All other package-level identifiers are `const` (`dateLayout`, `Version`, the six `HolidayType` constants).
- **Zero runtime dependencies.** Production files import only stdlib: `bytes`, `errors`, `fmt`, `strings`, `time`. `go.mod`'s `require` line covers only `testify` (Gold Rule 3 approved test-only) and its indirect deps (`go-spew`, `go-difflib`, `yaml.v3`).
- **All tests use testify (`assert` + `require`) and `t.Run` per case.** Gold Rule 3 conformance modulo IN-04 nitpick and WR-02.
- **No secrets in error messages.** Validator errors quote only caller-supplied input and `Date.String()`. `TestValidators_NoSensitiveData` locks this (its existence is the shape WR-02 questions, but the assertion itself is correct).
- **`Holiday` JSON tags match verified upstream wire shape** (camelCase keys, `omitempty` on `Comment`/`Subdivisions`/`Groups`/`Tags`/`Quality`). `TestHoliday_JSON` (types_test.go:125-274) covers single-day, multi-day, schema-drift `Quality`, and unknown-extra-field lenient decoding.
- **`NameFor` accessors on `Country`, `Language`, `Subdivision`.** Case-insensitive exact match → first-entry fallback → empty string on empty slice. Single `pickLocalized` helper backs all three; per-type tests exercise the same shape.
- **W-01 regression locks.** `validate_test.go:74-77` and `153-156` exhaustively cover the four Unicode characters (U+0131, U+017F, U+0130, U+212A) whose ToUpper/ToLower folds bypassed the prior post-canonicalization check.
- **LICENSE** is MIT (`LICENSE:1`), copyright 2026 go-openholidays contributors, no per-file headers required per CLAUDE.md.

---

_Reviewed: 2026-05-28_
_Reviewer: gsd-code-reviewer (standard depth, re-review)_
