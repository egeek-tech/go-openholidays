---
phase: 01-foundation
created: 2026-05-27
status: issues
depth: standard
findings_total: 11
critical_count: 0
warning_count: 4
info_count: 7
---

# Phase 01 — Code Review

## Summary

Phase 1 ships six production files (`doc.go`, `version.go`, `errors.go`, `date.go`, `types.go`, `validate.go`) plus five test files implementing domain types, the `Date` wrapper, the sentinel surface, and client-side validators. Overall quality is high: every exported symbol carries a godoc string starting with its name, the test suite follows the testify + `t.Run` + one-test-per-prod-function convention, and the AST audit (`internal_test.go`) correctly enforces CLIENT-10 invariants for files at the repo root. The headline finding is a **client-side validation bypass via Unicode case-folding**: `strings.ToLower("KK")` (two Kelvin signs) produces ASCII `"kk"` that then passes `isTwoASCIILowers`, so `validateLanguage` accepts non-ASCII input that its godoc explicitly rejects. The same shape affects `validateCountry` via `strings.ToUpper`. Three additional warning-tier issues round out the issues_found status: a documentation example in `version.go` that does not work as written, the CLIENT-10 AST audit silently skipping the `internal/` subtree (defeating the very invariant for the directory PROJECT.md says is in scope), and the `func TestValidators_NoSensitiveData` cross-cutting test that quietly violates Gold Rule 3's one-test-per-prod-function shape.

## Findings

### Critical (0)

None.

### Warning (4)

#### W-01: Validators bypass ASCII check via Unicode case-folding to ASCII (correctness/security)

- **File:** `validate.go:28-34, 49-55, 102-116`
- **Issue:** `validateCountry` calls `strings.ToUpper(code)` *before* the ASCII-shape check, and `validateLanguage` calls `strings.ToLower(code)` first. Both functions then ask `isTwoASCIIUppers`/`isTwoASCIILowers` of the canonicalized form. Four Unicode code points fold to ASCII via Go's `strings.To{Upper,Lower}`:
  - U+0130 LATIN CAPITAL LETTER I WITH DOT ABOVE → ToLower → `"i"` (ASCII)
  - U+0131 LATIN SMALL LETTER DOTLESS I → ToUpper → `"I"` (ASCII)
  - U+017F LATIN SMALL LETTER LONG S → ToUpper → `"S"` (ASCII)
  - U+212A KELVIN SIGN → ToLower → `"k"` (ASCII)

  Empirically verified with `go run`: input `"KK"` (two Kelvin signs, 6 UTF-8 bytes) → `strings.ToLower` → `"kk"` → `isTwoASCIILowers("kk")` → true. The validator returns `("kk", nil)`, accepting non-ASCII input that the godoc on line 23-24 explicitly promises to reject: *"not exactly two ASCII letters"*.
- **Why it matters:** The validators are positioned as the ASVS V5.1.3 input-validation control (their package comment names that control). The promise documented in godoc — *"non-ASCII bytes ... are rejected"* — is not actually delivered. While the upstream API would likely return 4xx for these malformed codes, the client-side guarantee is broken, and the test suite at `validate_test.go:59-61` and `validate_test.go:120-122` claims to lock the non-ASCII rejection path but only exercises characters that do *not* fold to ASCII (`Ż`, `ż`).
- **Fix:** Reorder the operation: ASCII-shape-check first, then canonicalize. The simplest fix is to add an ASCII pre-check before `ToUpper`/`ToLower`, or to scan the raw input byte-by-byte:
  ```go
  func validateCountry(code string) (string, error) {
      if !isTwoASCIILettersAnyCase(code) {
          return "", fmt.Errorf("%w: %q", ErrInvalidCountry, code)
      }
      return strings.ToUpper(code), nil
  }

  func isTwoASCIILettersAnyCase(s string) bool {
      if len(s) != 2 {
          return false
      }
      isLetter := func(b byte) bool {
          return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
      }
      return isLetter(s[0]) && isLetter(s[1])
  }
  ```
  Mirror for `validateLanguage`. Add regression tests using the four code points above (`"KK"`, `"İI"`, `"ıı"`, `"ſſ"`) to lock the fix.

#### W-02: `version.go` godoc shows an ldflags override example that cannot work — `Version` is `const`, not `var`

- **File:** `version.go:7-10`
- **Issue:** The godoc claims `Version` "can be overridden at link time, for example: `go build -ldflags '-X github.com/egeek-tech/go-openholidays.Version=0.1.1-rc1'`". The `-X` ldflag only works on package-level `var`s of type `string`; it silently has no effect on `const`. Verified locally: a small program with `const Version = "0.0.0"` built with `-ldflags '-X main.Version=1.2.3'` still prints `"0.0.0"`.
- **Why it matters:** This violates Gold Rule 2 ("never guess; verify or ask") in committed documentation. A consumer who follows the example will silently get the compiled-in default — exactly the failure mode the rule was written to prevent. The CLI in `cmd/ohcli` (Phase 5) is also planned to read this value for `--version`, so any release pipeline that tries to inject the version string via the documented command will produce a binary that lies about its version.
- **Fix:** Either (a) drop the ldflags paragraph from the godoc entirely if Phase 1's contract is "Version is compiled in", or (b) change the declaration to `var Version = "0.1.0"` so the example actually works. Option (b) preserves the documented capability and keeps the constant-folding benefit minor (Version appears only in a User-Agent string built once per Client). Either fix is a one-line change; the *important* part is that committed docs match committed behavior.

#### W-03: CLIENT-10 AST audit silently skips the `internal/` subtree, defeating its purpose there

- **File:** `internal_test.go:64-70`
- **Issue:** `skipDirs` contains `"internal": {}`, and `filepath.WalkDir` returns `filepath.SkipDir` for any directory whose `d.Name()` matches. The header comment on lines 67-70 justifies this as "Phase 1 has none of these subdirectories, but later phases will add internal/, cmd/, testdata/" — but for `internal/` this is incorrect: PROJECT.md says *"Internal helpers live under internal/"*, meaning `internal/` is library code subject to the same CLIENT-10 invariant ("no init() side effects, no global mutable state"). Skipping it from the audit means a future contributor can introduce `func init()` or an unauthorized package-level `var` inside `internal/foo/` and the audit will not catch it.
- **Why it matters:** The whole point of the `internal_test.go` AST audit is that it is the *mechanical* CI guard for CLIENT-10. Excluding the directory where most of the library's code will live in later phases is a self-inflicted blind spot. The comment claims this is intentional but conflates `internal/` (library private code, CLIENT-10 applies) with `testdata/` (fixtures, no Go code) and `cmd/` (external CLI, separately argued).
- **Fix:** Remove `"internal": {}` from `skipDirs`. Add a comment explaining that `internal/` is library code subject to CLIENT-10. If a specific `internal/` package needs an exception in a future phase, add the symbol to `allowedVars` with a justification — the closed-allowlist design of `allowedVars` already gives that escape hatch the deliberate-review property the rest of the audit relies on.

#### W-04: `TestValidators_NoSensitiveData` is a cross-cutting test that the project's stated convention does not authorize

- **File:** `validate_test.go:241-299`
- **Issue:** Gold Rule 3 (Project Rules in `CLAUDE.md`): *"Exactly one TestXxx function per exported production function."* The Rule does not explicitly cover unexported functions, but `validate.go` already has three direct one-to-one tests (`TestValidateCountry`, `TestValidateLanguage`, `TestValidateDateRange`) for its three unexported validators, so the convention is being honored for that file. `TestValidators_NoSensitiveData` then adds a fourth test that probes a *cross-cutting invariant* (ERR-04 transport-leak guard) across all three validators. The body of `TestValidators_NoSensitiveData` re-invokes the validators and asserts only on error-message substrings — it is functionally a duplicate-execution of paths already covered by the per-validator tests with one extra assertion. The "one TestXxx per prod function" shape is broken.
- **Why it matters:** Gold rules are the project's non-negotiable surface. The earlier files in the same phase (`date_test.go`, `errors_test.go`, `types_test.go`) follow the shape rigidly; this one file departs from it without an explicit `Key Decisions` entry. Future contributors will look at `validate_test.go` and reasonably believe the shape is loose, normalizing further drift.
- **Fix:** Two reasonable paths:
  1. **Merge:** Fold the leak-guard assertion into each per-validator test as an additional `t.Run("error_message_has_no_transport_leak", ...)` subtest. This preserves the one-test-per-prod-function shape strictly.
  2. **Document the exception:** If the cross-cutting shape is intentional (it has merit — a single failing test surfaces a single conceptual leak), add an entry to PROJECT.md's `Key Decisions` log stating that ERR-04 regression guards are admitted as cross-cutting tests, and update Gold Rule 3 to permit this category. The current code does neither, so the file is silently inconsistent with the rule.

  Pick one. Option 1 is the lowest-friction choice and keeps the rule's enforceability sharp.

### Info (7)

#### I-01: `errEmptyDate` uses `fmt.Errorf` with no verbs where `errors.New` is canonical

- **File:** `date.go:24`
- **Issue:** `var errEmptyDate = fmt.Errorf("openholidays: empty date string")`. With no `%w` or other verbs, `fmt.Errorf` is functionally identical to `errors.New` but slower (it runs through `fmt`'s parser). The five sentinels in `errors.go` (lines 20, 24, 28, 31, 35) correctly use `errors.New`; `errEmptyDate` is the odd one out.
- **Fix:** `var errEmptyDate = errors.New("openholidays: empty date string")`. One-line change; consistency with the sibling file.

#### I-02: Non-ASCII em-dashes ("—") sprinkled through doc comments

- **File:** `doc.go:7`, `date.go:46-47, 73`, `errors.go:73-74, 92-94, 39-40`, `types.go:7, 86, 105`, `validate.go:18, 75-76`, others.
- **Issue:** Gold Rule 1 demands English-language docs. Em-dashes are not ASCII (`U+2014`). Go's stdlib style (and `gofmt`-style doc comments by convention) uses ASCII `--` or `-` for these breaks. The em-dashes don't break anything mechanical — `gofmt` and `go doc` handle them — but they are a stylistic departure from the rest of the Go ecosystem and a tiny seam that makes copy-paste from terminals awkward.
- **Fix:** Optional, low priority. If the project wants strict ASCII-clean source files, replace `—` with ` -- ` or `-`. Otherwise document the deliberate choice in CONVENTIONS.md.

#### I-03: `Holiday` permits unknown `Type` values silently (deferred to Phase 4, worth a doc pointer)

- **File:** `types.go:103, 94-138`
- **Issue:** `Type HolidayType` is a typed string. JSON unmarshal of `"type": "Religious"` (an upstream value not in the six allowlist constants) succeeds with `Type` populated as `"Religious"`. The Phase 4 strict decoder will surface this; Phase 1 lenient decoding tolerates it. The Holiday godoc on lines 80-93 mentions the six values but does not warn callers that other values may appear in the wild.
- **Fix:** Add one sentence to the `Holiday.Type` field doc: *"Callers MUST be prepared for upstream to return a value outside the six HolidayType constants; the default lenient decoder accepts unknown values."* Or extract a tiny helper `(HolidayType).IsKnown() bool` so callers can branch defensively. Neither is required for Phase 1 to ship; this is a doc/affordance polish.

#### I-04: `Subdivision.Children` is unbounded recursion with no depth limit

- **File:** `types.go:212`
- **Issue:** Decoding a pathologically deep upstream payload (`{"children":[{"children":[...]}]}` nested N times) would recurse N levels in `encoding/json`. The stdlib decoder does have a built-in nesting limit (10000), so this is bounded — but the bound is implicit, not chosen by us. For a hostile / buggy upstream this is fine in practice; flagging for future phases that build helpers walking `Children` (e.g., Phase 3 traversals) to add an explicit depth cap.
- **Fix:** No change for Phase 1. When Phase 3 adds traversal helpers, cap recursion depth (e.g., 32 levels) and return a typed error on overflow.

#### I-05: `TestAPIError_Error` uses `require.Equal` where the convention asks for `assert.Equal`

- **File:** `errors_test.go:143`
- **Issue:** Gold Rule 3 split: *"require for preconditions (aborts the case), assert for verifications (reports without aborting)."* Line 143 (`require.Equal(t, c.want, c.err.Error())`) is the *verification* of the test case's primary outcome — it should be `assert.Equal`. The preceding `require.NotNil(t, c.err, ...)` on line 142 is correctly a precondition. Same pattern is used correctly elsewhere in the file (e.g., line 216 uses `assert.Equal`).
- **Fix:** Change line 143 to `assert.Equal`. One-character edit; aligns with the rule and the rest of the file.

#### I-06: `isTwoASCIIUppers` / `isTwoASCIILowers` duplicate structure

- **File:** `validate.go:102-116`
- **Issue:** The two functions are identical apart from the byte-range constants. Parameterization (`isTwoASCIIInRange(s string, lo, hi byte) bool`) would dedupe 14 lines into one helper. Not a bug; mild DRY temptation.
- **Fix:** Optional. Only worth doing if the W-01 fix introduces a third variant (`isTwoASCIILettersAnyCase`), in which case a shared `isTwoASCIIByte(b byte, ...) bool` helper or a single byte-classifier becomes cleaner than three near-copies. Tie the cleanup to the W-01 refactor.

#### I-07: `Date.UnmarshalJSON` echoes raw `b` into the error message without length cap

- **File:** `date.go:93`
- **Issue:** `return fmt.Errorf("openholidays: date must be a JSON string, got %s", b)`. If `b` is megabytes of binary garbage (extremely unlikely from a JSON parser that already pre-tokenized this value, but possible if a caller invokes `UnmarshalJSON` directly with attacker-controlled bytes), the error string is unbounded. The `FuzzDateUnmarshal` corpus does not stress this — the fuzz only checks panic-freedom, not error-string size.
- **Fix:** Cap echoed bytes at, e.g., 64 bytes with `… (truncated)` suffix when over. The fix is one helper; the practical risk is low because callers virtually always pass `b` from `encoding/json`, which has already tokenized the value to a sane size. Defer to a future phase if a real abuse vector is identified.

## Verified Good

These were explicitly traced and confirmed correct:

- **Package layout & naming.** All files in `package openholidays`; file names match the file responsibility (`date.go`, `errors.go`, `types.go`, `validate.go`, `version.go`, `doc.go`). `go.mod` declares `go 1.23` matching CONTEXT.md's locked floor.
- **Five exported sentinels, exactly.** `ErrInvalidCountry`, `ErrInvalidLanguage`, `ErrDateRangeTooLarge`, `ErrEmptyResponse`, `ErrInvalidDateRange` — D-13 honored. `errEmptyDate` is correctly unexported (D-06), and the AST audit's `allowedVars` matches.
- **All exported symbols carry godoc starting with the symbol name.** Spot-checked `NewDate`, `ParseDate`, `Date`, `Holiday`, `HolidayType`, `LocalizedText`, `SubdivisionRef`, `GroupRef`, `Country`, `Language`, `Subdivision`, all three `NameFor` methods, `APIError`, `(*APIError).Error`, `(*APIError).Is`, every sentinel, `Version`. All conform.
- **`APIError.Is` semantics.** Wildcard (`StatusCode == 0`) matches any `*APIError`; non-zero matches by status only; `Path`/`Body`/`Message` are ignored on the target; non-`*APIError` targets never match. Implementation (errors.go:96-105) matches the godoc (errors.go:83-95) and the test (errors_test.go:151-222) exercises every branch including the wildcard.
- **`Date.MarshalJSON` ↔ `Date.UnmarshalJSON` round-trip symmetry.** `MarshalJSON` always emits `"YYYY-MM-DD"` with surrounding quotes (12-byte capacity hint correct); `UnmarshalJSON` rejects `null`, `""`, non-string tokens, and malformed dates with a wrapped `errEmptyDate` or `time.Parse` error. Tests cover all five branches plus a fuzz target for panic-freedom (D-12, CL-03 satisfied).
- **`Date.DaysUntil` arithmetic.** The inclusive count semantics (same day → 1, one day later → 2, negative direction → magnitude + sign) are correct. UTC-midnight normalization defends against DST: with both operands at 00:00 UTC, `Sub().Hours()/24` is a clean integer multiple, well within `float64`'s 2^53 exact-integer range for any plausible calendar span.
- **`validateDateRange` leap-year boundary.** The backward-anchored `to.AddDate(-3, 0, 0)` formulation correctly handles 2024-02-29 → 2027-02-28 as the exact-3-year boundary (PASS) and 2024-02-29 → 2027-03-01 as one day over (FAIL). The forward-anchored `from.AddDate(3, 0, 0)` formulation that CL-06 was decided against would mishandle this pair. Tests on validate_test.go:168-172 and 211-216 lock both polarities.
- **No `init()` anywhere in production files.** Verified by reading each `.go` file and confirmed by the `TestNoInitOrGlobalState` AST audit (the audit's own logic is correct for the repo-root subtree it walks; W-03 concerns coverage of `internal/`, not the per-file logic).
- **No global mutable state.** Only the six allowlisted `var`s (five sentinels + `errEmptyDate`) exist at package scope. All other package-level identifiers are `const` (`dateLayout`, `Version`, the six `HolidayType` constants).
- **Zero runtime dependencies.** No `.go` file outside `*_test.go` imports anything outside `std`: verified that the entire production set imports only `bytes`, `errors`, `fmt`, `strings`, `time`. `go.mod`'s `require` line covers only `testify` (Gold Rule 3 approved test-only) and its indirect deps.
- **All tests use testify (`assert` + `require`) and `t.Run` per case.** Gold Rule 3 conformance (excepting I-05 nitpick).
- **No secrets in error messages.** Validator errors quote only the caller-supplied input and the formatted `Date.String()` form; no HTTP body, URL, or auth-header context leaks into any error string. ERR-04 / T-01-02-IL invariant honored. The defensive regression test in `validate_test.go:241-299` locks this.
- **`Holiday` JSON tags match the verified upstream wire shape** (camelCase keys, `omitempty` on `Comment`/`Subdivisions`/`Groups`/`Tags`/`Quality`). Tested round-trip on `TestHoliday_JSON` including the schema-drift `Quality` field and unknown extra fields under the default lenient decoder.
- **`NameFor` accessor semantics on `Country`, `Language`, `Subdivision`.** Case-insensitive exact match → first entry fallback → empty string on empty slice. Single `pickLocalized` helper backs all three; tests exercise each entry point with the same shape. CL-05 (`NameFor` rename to avoid `Name` field collision) honored.
- **`gofmt`/`go vet` cleanliness assumed and surface-checked.** No unused imports, no shadowing of stdlib names, no `==` vs `===`-class issues (Go doesn't have these), no `interface{}` (uses concrete types throughout), no `panic` calls in production code.

---

_Reviewed: 2026-05-27_
_Reviewer: gsd-code-reviewer (standard depth)_
