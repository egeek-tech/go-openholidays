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
  warning: 1
  info: 2
  total: 3
status: issues_found
---

# Phase 01: Code Review Report (Round 3)

**Reviewed:** 2026-05-28T00:00:00Z
**Depth:** standard
**Files Reviewed:** 14
**Status:** issues_found

## Summary

Round 3 confirmation re-review of all 14 phase-01-scope source files. The
review re-verifies the seven round-2 fixes (WR-01, WR-02, IN-01..IN-05) and
scans for new defects introduced by those fixes and by the subsequent
phase 02/03/04 edits that touched phase-01 files (`ErrMalformedResponse`
addition, `internal_test.go` allowlist updates, `validate.go` helper
collapse).

**All seven round-2 fixes are correctly applied.** Build is clean
(`go build ./...` succeeds with no output); `go vet ./...` is clean.
The phase-01 invariants (exported sentinel surface kept to the documented
set, no `init()`, allowlist-only package-level vars, English-only
identifiers, testify + one-test-per-prod-function + `t.Run`) all hold.

This round-3 pass identifies one new Warning that was missed in earlier
rounds (a typed-nil panic in `(*APIError).Is`) plus two Info-level
observations.

### Round-2 fix verification

| ID | Fix | Verified |
|----|-----|----------|
| WR-01 | `version.go::Version` is `var Version = "0.1.0"` (not `const`) | YES — line 10 |
| WR-01 | `internal_test.go` allowlist includes `"Version"` | YES — line 92 |
| WR-02 | `// Key Decision CL-17` annotation on `TestValidators_NoSensitiveData` | YES — `validate_test.go:287` |
| IN-01 | `errEmptyDate` uses `errors.New(...)` | YES — `date.go:25` |
| IN-02 | (CONVENTIONS.md em-dash ratification — out of scope for this code review, trusted) | n/a |
| IN-03 | `HolidayType.IsKnown()` exists; covered by 10-case test | YES — `types.go:60-71`, `types_test.go:56-85` |
| IN-04 | `errors_test.go` no spurious `require.Equal`; only load-bearing `require.NotNil` (l.41) and `require.True(errors.As)` (l.247) remain | YES |
| IN-05 | `Date.UnmarshalJSON` non-string-token branch uses `truncateForError(b, 64)` + `sanitizeForError`; non-printable bytes masked; oversized labeled `(truncated, N total bytes)` | YES — `date.go:89-106, 192-220`; `date_test.go:176-235` |

Subsequent phase 02/03/04 touches on phase-01 files are clean:

- `errors.go::ErrMalformedResponse` (D-65/D-66/CL-12) — well-documented,
  same sentinel pattern, listed alongside the other six.
- `internal_test.go::allowedVars` — adds `ErrResponseTooLarge`,
  `ErrMalformedResponse`, `CacheHitContextKey`, `Version` in
  chronological-append order; `"internal"` removed from `skipDirs`.
- `validate.go::isTwoASCIILetters` — single helper replaces the original
  pair `isTwoASCIIUppers/Lowers`; logic still rejects on ORIGINAL bytes
  before canonicalization (closes the W-01 Unicode fold bypass).

## Narrative Findings (AI reviewer)

### Warnings

#### WR-01: `(*APIError).Is` panics on typed-nil target

**File:** `errors.go:122-131`

**Issue:** When a caller invokes `errors.Is(err, target)` with a typed-nil
`*APIError` target — for example:

```go
var target *openholidays.APIError // typed nil
errors.Is(err, target)            // PANIC: nil pointer dereference
```

— the type assertion `target.(*APIError)` succeeds with `t == nil`, and
the very next line `t.StatusCode != 0` dereferences the nil pointer,
causing a runtime panic.

This was reproduced against the current `master` build of `errors.go`:
`errors.Is(&APIError{StatusCode: 404}, (*APIError)(nil))` panics with
`runtime error: invalid memory address or nil pointer dereference`. The
other branch of the type assertion (`!ok`, e.g. a non-`*APIError`
target) is already handled correctly and is covered by the
`non-apierror-target-never-matches` subtest in `TestAPIError_Is`.

Typed-nil pointers are an unusual but legal Go pattern — defensive
public-API code in the standard library (`*fs.PathError`, `*url.Error`,
etc.) consistently guards against them. The library's adversarial test
matrix in `TestAPIError_Is` does not include this case, so the regression
ships silently.

Severity: Warning because (1) the panic is reachable from any caller
that holds a typed-nil sentinel and uses it as a target, and (2) the
library is consumed by external Go code where the calling convention
is not under the maintainer's control; but the typed-nil-target idiom
is uncommon in practice, so this is hardening rather than active data
loss.

**Fix:** Treat a nil typed-target as a wildcard (matches any `*APIError`,
consistent with the existing `&APIError{}` zero-value wildcard branch).
This preserves the documented semantics and eliminates the panic:

```go
func (e *APIError) Is(target error) bool {
    t, ok := target.(*APIError)
    if !ok {
        return false
    }
    if t == nil {
        // Typed-nil *APIError target — treat as wildcard match, same
        // as &APIError{} per the documented zero-StatusCode contract.
        return true
    }
    if t.StatusCode != 0 && t.StatusCode != e.StatusCode {
        return false
    }
    return true
}
```

Add a `typed-nil-target-treated-as-wildcard` subtest to `TestAPIError_Is`
asserting that `errors.Is(base, (*APIError)(nil))` returns `true` without
panic. An alternative — returning `false` for typed-nil — would also be
defensible, but the wildcard interpretation aligns with the existing
"zero target StatusCode is a wildcard" semantics.

### Info

#### IN-01: Use `for i := range 2 { ... }` instead of C-style loop in `isTwoASCIILetters`

**File:** `validate.go:114-119`

**Issue:** The body uses a classic C-style three-clause loop:

```go
for i := 0; i < 2; i++ {
    b := s[i]
    if !((b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')) {
        return false
    }
}
```

The module is on `go 1.23` (`go.mod` line 3). Since Go 1.22, the
range-over-integer form `for i := range 2 { ... }` is idiomatic. The
function checks exactly two bytes and even reads more naturally as
two unrolled checks. Not a correctness issue — purely a style/idiom
nudge.

**Fix:** Either switch to `for i := range 2` or unroll explicitly:

```go
func isTwoASCIILetters(s string) bool {
    if len(s) != 2 {
        return false
    }
    return isASCIILetter(s[0]) && isASCIILetter(s[1])
}

func isASCIILetter(b byte) bool {
    return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}
```

The unrolled form is two lines shorter and surfaces a reusable
`isASCIILetter` helper if future validators need ASCII-letter checks
on single bytes (e.g. ISO 3166-2 subdivision codes have a hyphen and
1-3 alphanumeric chars).

#### IN-02: `truncateForError(b, maxBytes)` with `maxBytes <= 0` silently returns `""`

**File:** `date.go:192-201`

**Issue:** `truncateForError(b, 0)` and `truncateForError(b, -1)` both
return the empty string, even when `len(b) > 0`. The intended single
caller in `Date.UnmarshalJSON` passes the constant `64`, so the
defensive branch is unreachable from the production call site, but
the function is package-internal and could be reused. The
zero/negative-cap path silently drops the entire input rather than
returning, say, "(suppressed, N bytes)" or panicking.

This is defensive code that the production call path never triggers
(`maxBytes` is the literal `64`), so it does not affect observable
behaviour today. It is flagged only because (a) the function is
package-internal and could grow new callers, and (b) the silent
zero-cap fallback masks a misuse rather than surfacing it.

**Fix:** Either tighten the contract (panic on `maxBytes <= 0`, since
that is a programmer error) or document the silent-empty fallback in
the docstring. The current behavior is acceptable for a defense-in-
depth helper but should not be a load-bearing primitive without a
doc note. Suggested doc addition:

```go
// When maxBytes <= 0, returns "" — silently dropping the input. This
// branch is defensive; production callers pass a positive constant.
```

### Adversarial trace summary

This section captures the adversarial scan as a narrative for future
reviewers comparing rounds. **Round 3 found 1 Warning + 2 Info.** No
new BLOCKER-class defects.

The `Date` JSON contract is sound under direct UnmarshalJSON calls with
adversarial inputs (empty slice, single byte, lone double quote, NUL
plus binary bytes, oversized 200-byte non-string token, raw UTF-8). The
validator family rejects every W-01 fold-to-ASCII bypass and includes a
documented regression test for each. The CLIENT-10 AST audit allowlist
correctly tracks the seven exported sentinels plus `errEmptyDate`,
`CacheHitContextKey`, and `Version` in chronological-append order. The
10-case `IsKnown` table covers the six documented constants plus
empty/drift/case/free-form negatives.

Test discipline is excellent across all 14 files: exactly one
`TestXxx` per exported production function (or one per typed-string
constant block where the prod surface is just constants); every test
case lives inside a `t.Run`; `require` is used only for load-bearing
preconditions (the remaining `require.X` calls in `errors_test.go` are
provably load-bearing precondition guards — Gold Rule 3 satisfied).
The fuzz target in `date_test.go::FuzzDateUnmarshal` is panic-only
(correct — it asserts the JSON-3 invariant from CONTEXT.md D-12).

The `LICENSE` file is verbatim MIT, dated 2026, attributed to
"go-openholidays contributors" — standard. `go.mod` declares `go 1.23`
with a single test-only dep (testify v1.11.1 plus its three transitive
indirects). `go.sum` matches `go.mod` exactly.

The em-dash style choice in code comments (CONVENTIONS.md ratification
per IN-02) is honored consistently across `date.go`, `errors.go`,
`types.go`, and `validate.go` package-doc and field comments. English-
only per Gold Rule 1 (non-English strings appear only in test fixtures,
which is the documented `testdata`-fixture exception in CONVENTIONS.md
Rule 1).

---

_Reviewed: 2026-05-28T00:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
