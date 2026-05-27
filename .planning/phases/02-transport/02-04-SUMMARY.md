---
phase: 02-transport
plan: 04
subsystem: validation
tags: [validators, security, unicode, w-01, case-fold, iso-3166, iso-639]

# Dependency graph
requires:
  - phase: 01-foundation
    provides: "validateCountry, validateLanguage, ErrInvalidCountry, ErrInvalidLanguage, validate_test.go test infrastructure"
provides:
  - "validateCountry/validateLanguage that ASCII-shape-check ORIGINAL bytes BEFORE strings.ToUpper/ToLower canonicalization"
  - "isTwoASCIILetters byte-level case-agnostic shape predicate"
  - "8 empirically-verified W-01 regression cases (4 per validator) locked into validate_test.go"
affects: [02-transport endpoint plans that call validateCountry/validateLanguage, Phase 3 PublicHolidays/SchoolHolidays/Subdivisions methods]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Shape-before-canonicalize: byte-level ASCII predicate runs on original input BEFORE Unicode case folding so fold-to-ASCII characters (U+0130, U+0131, U+017F, U+212A) cannot bypass the validator"
    - "Plan-time empirical verification of bypass inputs: each W-01 regression case was verified to fold under strings.ToUpper/ToLower at plan time and locked into the plan; executor does not re-derive"

key-files:
  created: []
  modified:
    - "validate.go - reorder shape check before case canonicalization; add isTwoASCIILetters helper; mark isTwoASCIIUppers/isTwoASCIILowers as currently unreachable"
    - "validate_test.go - extend TestValidateCountry/TestValidateLanguage rejectCases with 4 W-01 regression cases each"

key-decisions:
  - "D-32: ASCII-shape check runs on ORIGINAL bytes BEFORE strings.ToUpper/ToLower; byte-level predicate (no unicode package import) — closes Phase 1 W-01 follow-up"
  - "D-33: No PROJECT.md Key Decisions row needed — defect fix against locked VALID-01/VALID-04 contract; godoc already said 'non-ASCII rejected'"
  - "D-34: Plan touches W-01 only — W-02, W-03, W-04 remain on the Phase 1 follow-up backlog"

patterns-established:
  - "W-01 defense pattern: shape-then-canonicalize for any two-letter code validator; byte arithmetic over strings.ToUpper/ToLower for the gate predicate"
  - "Defense-in-depth retention: isTwoASCIIUppers/isTwoASCIILowers kept in validate.go even though unreachable from validators, both for direct testability and for future re-introduction if a separate strictly-cased call-site appears"

requirements-completed: [VALID-01, VALID-04]

# Metrics
duration: ~5min
completed: 2026-05-27
---

# Phase 02-transport Plan 04: W-01 Unicode case-fold bypass fix Summary

**Reordered validateCountry/validateLanguage to run a byte-level ASCII shape check (isTwoASCIILetters) BEFORE strings.ToUpper/ToLower, closing the W-01 bypass where U+0130, U+0131, U+017F, and U+212A folded to 2-byte ASCII letter strings and slipped through the over-permissive post-fold check.**

## Performance

- **Duration:** ~5 min
- **Started:** 2026-05-27T14:11:29Z (approx, Task 1 commit time)
- **Completed:** 2026-05-27T14:14:30Z
- **Tasks:** 2 (both auto/tdd, both green on first verify run for Task 1; Task 2 surfaced an encoding sub-issue that was Rule-1 fixed inline — see Deviations)
- **Files modified:** 2 (validate.go, validate_test.go)

## Accomplishments

- `validate.go` now runs `isTwoASCIILetters(code)` on the ORIGINAL input BEFORE any `strings.ToUpper` / `strings.ToLower` canonicalization. The W-01 fold-to-ASCII bypass class is now mechanically closed at the validator gate.
- 8 empirically-verified W-01 bypass inputs are locked as regression cases in `validate_test.go` (4 country, 4 language). Each case is named with the `W-01` tag for traceability and accompanied by a leading comment block documenting the exact ToUpper/ToLower fold proving it was a real pre-fix bypass.
- Phase 1 invariants preserved: error messages still quote ORIGINAL input via `%q` (ERR-04 / D-23); existing successCases (`"PL"`, `"pl"`, `"Pl"`, `"en"`, `"EN"`, etc.) still pass; `validateDateRange` untouched; CLIENT-10 audit (`TestNoInitOrGlobalState`) still green; no new package-level vars; no new imports.
- Defense-in-depth helpers `isTwoASCIIUppers` / `isTwoASCIILowers` retained with explicit "currently unreachable from validateCountry/validateLanguage" comments, so a future contributor can grep their way to the rationale before deleting them.

## Task Commits

1. **Task 1: Reorder validate.go to run ASCII shape check before case canonicalization; add isTwoASCIILetters helper** — `3fb8fb4` (fix)
2. **Task 2: Extend validate_test.go rejectCases with 8 empirically-verified W-01 regression cases** — `5d9cb4c` (test)

## Files Created/Modified

- `validate.go` — `validateCountry` and `validateLanguage` now invoke `isTwoASCIILetters(code)` on the ORIGINAL string before any canonicalization. The new helper performs byte-level `[A-Za-z]{2}` matching (no `unicode` package, no case folding). The two existing case-sensitive helpers are kept under a "currently unreachable" comment.
- `validate_test.go` — `TestValidateCountry.rejectCases` and `TestValidateLanguage.rejectCases` each gain 4 W-01 regression entries (8 cases total). Each block is prefixed with a comment documenting the ToUpper/ToLower fold proof.

## Eight Empirically-Verified W-01 Inputs (locked at plan time, verified again at execution start)

| # | Validator | Input (visible) | UTF-8 bytes | Fold target | Fold byte count | Pre-fix outcome | Post-fix outcome |
|---|-----------|------------------|-------------|-------------|------------------|------------------|-------------------|
| 1 | country  | `ıA`  (U+0131 + 'A')          | `c4 b1 41`       | ToUpper → `IA` | 2 | ACCEPTED (`"IA"`) | REJECTED (ErrInvalidCountry) |
| 2 | country  | `ſA`  (U+017F + 'A')          | `c5 bf 41`       | ToUpper → `SA` | 2 | ACCEPTED (`"SA"`) | REJECTED (ErrInvalidCountry) |
| 3 | country  | `ıı`  (U+0131 ×2)              | `c4 b1 c4 b1`    | ToUpper → `II` | 2 | ACCEPTED (`"II"`) | REJECTED (ErrInvalidCountry) |
| 4 | country  | `ſſ`  (U+017F ×2)              | `c5 bf c5 bf`    | ToUpper → `SS` | 2 | ACCEPTED (`"SS"`) | REJECTED (ErrInvalidCountry) |
| 5 | language | `KK`  (U+212A Kelvin ×2)       | `e2 84 aa e2 84 aa` | ToLower → `kk` | 2 | ACCEPTED (`"kk"`) | REJECTED (ErrInvalidLanguage) |
| 6 | language | `İa`  (U+0130 + 'a')          | `c4 b0 61`       | ToLower → `ia` | 2 | ACCEPTED (`"ia"`) | REJECTED (ErrInvalidLanguage) |
| 7 | language | `İİ`  (U+0130 ×2)              | `c4 b0 c4 b0`    | ToLower → `ii` | 2 | ACCEPTED (`"ii"`) | REJECTED (ErrInvalidLanguage) |
| 8 | language | `Ka`  (U+212A + 'a')          | `e2 84 aa 61`    | ToLower → `ka` | 2 | ACCEPTED (`"ka"`) | REJECTED (ErrInvalidLanguage) |

The pre-fix → post-fix delta in column 7 vs. column 8 is the proof the W-01 fix lands.

## Decisions Made

- **D-32 honored:** byte-level `isTwoASCIILetters` predicate, no `unicode` package import. The word `unicode` appears in three explanatory comments within `validate.go` (the helper's docstring explicitly references `unicode.IsLetter` to explain why byte arithmetic is preferred); these are deliberate documentation, not imports, and the file's import block remains `("fmt", "strings")`. The Phase 2 plan's acceptance criterion `grep -c 'unicode' validate.go returns 0` is satisfied in spirit (no `unicode` package usage) but not in literal letter; this is documented below under Issues Encountered.
- **D-33 honored:** no `Key Decisions` row added to PROJECT.md — this is a defect fix against the existing VALID-01/VALID-04 contract.
- **D-34 honored:** W-02 (Date.String UTC anchoring), W-03 (CLI flag parsing edge), W-04 (test parallelism nuance) untouched — they remain on the Phase 1 follow-up backlog per CONTEXT.md.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 — Bug] Initial Edit produced ASCII `K`/`Ka` bytes for the two U+212A Kelvin-sign W-01 cases; tests caught the issue and the next Edit re-wrote the file with the correct `e2 84 aa` bytes**

- **Found during:** Task 2 (post-Edit verification run)
- **Issue:** The plan's locked input list rendered the Kelvin-sign cases as the strings `"KK"` and `"Ka"` (visually). U+212A and ASCII 'K' look nearly identical in many fonts but have completely different UTF-8 byte sequences (`e2 84 aa` vs `4b`). After the first Edit, `go test -race -run TestValidateLanguage -v` showed `An error is expected but got nil` for both Kelvin cases — meaning the test file had been written with ASCII `K` bytes, so the inputs were just valid 2-byte ASCII-uppercase language codes that the post-fix validator legitimately accepted. The pre-fix → post-fix delta the test exists to lock would not have been exercised at all.
- **Fix:** Issued a second Edit on the Kelvin block (added an explanatory comment about the U+212A vs ASCII 'K' visual collision); the Edit's surrounding-context bytes carried the correct `e2 84 aa` codepoints from the Read context, so the file was rewritten with the right encoding. Hexdump of the final `validate_test.go` confirms `e2 84 aa e2 84 aa` for "KK" and `e2 84 aa 61` for "Ka".
- **Verification:** `xxd` on the four language W-01 input literals confirmed correct codepoints; `go test -race -run "TestValidateCountry|TestValidateLanguage" -v ./...` shows all 8 W-01 subtests PASS.
- **Committed in:** `5d9cb4c` (Task 2 commit — final state includes the corrected bytes and an in-file comment warning future contributors about the U+212A-vs-ASCII visual collision)

---

**Total deviations:** 1 auto-fixed (Rule 1 — bug surfaced by the test and corrected before Task 2 commit landed).
**Impact on plan:** No scope creep. The fix preserved plan intent verbatim (U+212A Kelvin sign as the regression input, per CONTEXT.md D-32 and the threat-model T-02-18 mitigation).

## Issues Encountered

- **Plan acceptance-criterion literal vs intent gap (documented, not blocking):** Task 1's acceptance criterion `grep -c 'unicode' validate.go returns 0` (per D-32 "no `unicode` package import") cannot be satisfied literally because the plan's own `<action>` block prescribes that the new helper's docstring read "Byte arithmetic (rather than unicode.IsLetter) is intentional and mandatory…" — the word `unicode` appears verbatim there. The Phase 1 helpers `isTwoASCIIUppers` / `isTwoASCIILowers` also retain their existing docstrings which reference `unicode.IsUpper` / `unicode.IsLetter` as comparison points. Net: `validate.go` has 3 grep hits for `unicode`, all in comments, all required by the plan-prescribed prose. The `unicode` package is NOT imported (`awk '/^import \(/,/^\)/' validate.go | grep -c unicode` = 0). The plan author's intent — "no `unicode` package usage" — is satisfied; only the literal grep check is off by one transitive reference.
- **Plan acceptance-criterion grep encoding gap (documented):** Task 2's `grep -c 'input: "KK"' validate_test.go returns 1` and `'input: "Ka"' returns 1` are written with ASCII `K`/`Ka` in the grep pattern, but the actual stored bytes are `e2 84 aa` (U+212A). A shell pattern with ASCII `K` won't match the U+212A bytes, so a literal `grep -c` returns 0. Equivalent verification: `grep -c $'input: "\xe2\x84\xaa\xe2\x84\xaa"' validate_test.go` and `grep -c $'input: "\xe2\x84\xaaa"' validate_test.go` each return 1. The W-01 case-name count (`grep -cE '^\s*\{name: "W-01' validate_test.go`) returns exactly 8, satisfying the more meaningful structural check.

## User Setup Required

None — pure code change, no external service configuration.

## Verification Evidence

```
$ go vet ./... && go build ./...                                           # both exit 0
$ go test -race -run "TestValidateCountry|TestValidateLanguage" ./...      # ok (W-01 8/8 subtests green)
$ go test -race ./...                                                       # ok (whole module green)
$ go test -race -run TestNoInitOrGlobalState ./...                          # ok (CLIENT-10 audit green)
$ awk '/^import \(/,/^\)/' validate.go | grep -c unicode                    # 0  (no unicode package import)
$ grep -cE 'isTwoASCIILetters\(code\)' validate.go                          # 2  (both validators gated)
$ grep -cE '^func isTwoASCIILetters' validate.go                            # 1  (new helper present)
$ grep -cE '^\s*\{name: "W-01' validate_test.go                             # 8  (exactly 8 regression case names)
$ iconv -f UTF-8 -t UTF-8 validate_test.go > /dev/null && echo OK           # OK (valid UTF-8)
```

## Next Phase Readiness

- Phase 3 endpoint plans (`PublicHolidays`, `SchoolHolidays`, `Subdivisions`) can now call `validateCountry` / `validateLanguage` without the W-01 footgun. The validator gates are sound BEFORE the wiring lands.
- W-02 (Date.String UTC anchoring), W-03 (CLI flag edge), W-04 (test parallelism nuance) are explicitly OUT of scope per D-34 and remain in the Phase 1 follow-ups backlog.
- No blockers.

## Self-Check: PASSED

- `validate.go` modifications present and committed in `3fb8fb4`.
- `validate_test.go` modifications present and committed in `5d9cb4c`.
- Both commit hashes exist on the worktree branch (`git log --oneline -3` confirms).
- 8 W-01 subtests all PASS under `go test -race -v`.
- Full module suite green.
- No new files created; no files deleted; no shared orchestrator state mutated.

---
*Phase: 02-transport*
*Completed: 2026-05-27*
