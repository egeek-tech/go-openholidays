---
phase: 01-foundation
plan: 02
subsystem: errors
tags: [go, errors, sentinel-errors, errors-is, errors-as, testify, apierror]

# Dependency graph
requires:
  - phase: 01-foundation
    provides: "Go module identity (github.com/egeek-tech/go-openholidays, go 1.23); testify v1.11.1 declared in go.mod (Plan 01-01)"
provides:
  - "5 exported sentinel errors (ErrInvalidCountry, ErrInvalidLanguage, ErrDateRangeTooLarge, ErrInvalidDateRange, ErrEmptyResponse)"
  - "*APIError leaf type with StatusCode/Path/Body/Message fields, Error() and Is() methods (no Unwrap per D-16)"
  - "errors.Is wildcard (zero StatusCode matches any *APIError) + status-code matching"
  - "errors.As round-trip contract verified end-to-end on a hand-constructed *APIError"
  - "testify promoted from indirect to direct dependency in go.mod (first *_test.go import)"
affects:
  - "01-05-validators (validate.go wraps the 5 sentinels with fmt.Errorf %w)"
  - "02-transport (Phase 2 endpoint methods construct *APIError from *http.Response)"
  - "03-endpoints (endpoint tests assert via errors.Is(err, &APIError{StatusCode: N}) and errors.Is(err, ErrSentinel))"

# Tech tracking
tech-stack:
  added:
    - "testify v1.11.1 transitive deps recorded in go.sum: davecgh/go-spew v1.1.1, pmezard/go-difflib v1.0.0, gopkg.in/yaml.v3 v3.0.1 (all // indirect, test-only)"
  patterns:
    - "Sentinel errors as package-level `var ... = errors.New(\"openholidays: ...\")` inside a single var block"
    - "Leaf error type with custom `Is(target error) bool` for status-code matching (no Unwrap)"
    - "Test convention: one TestXxx per exported production function/symbol-group; every case inside t.Run; require for preconditions, assert for verifications (Gold Rule 3)"
    - "Information-disclosure mitigation: APIError.Body is NEVER serialized into Error() output"

key-files:
  created:
    - "errors.go — 5 sentinels + *APIError type + Error() + Is() (no Unwrap)"
    - "errors_test.go — 5 TestXxx functions covering ERR-01/ERR-02/ERR-03, D-15, D-18, Pitfall 5"
  modified:
    - "go.mod — testify promoted from `// indirect` to direct; 3 indirect testify-transitive entries added"
    - "go.sum — module sums for davecgh/go-spew, pmezard/go-difflib, gopkg.in/yaml.v3 added"

key-decisions:
  - "Task 1 carried `tdd=\"true\"` in the plan but Task 2 owned errors_test.go in a separate commit. Honoring each task's own <verify> block (Task 1 = build/grep only, Task 2 = run tests) split the work into a feat→test commit pair instead of a within-Task-1 red/green pair. Functionally equivalent because Task 1's contract is structural (sentinels exist, type exists, Is matches by status) and Task 2 owns the runtime assertions."
  - "Honoring D-16 strictly: *APIError has no Unwrap method. Verified via grep before commit."
  - "Honoring Pitfall 5 strictly: Is() matches on StatusCode only; Path/Body/Message on the target are ignored. Three explicit test cases (path-ignored-on-target, message-ignored-on-target, body-ignored-on-target) lock this against future regression."

patterns-established:
  - "Sentinel block: `var ( ErrX = errors.New(\"openholidays: ...\") ; ... )` — single block, godoc per sentinel, all messages share the project prefix."
  - "Leaf-error matching protocol: when an error type has a finite predicate (here: status code), implement `Is(target error) bool` with the wildcard-on-zero convention; do NOT add Unwrap unless the type actually wraps another error."
  - "Test scaffolding: type-driven table case structs (`type tc struct { ... }`) declared inside the TestXxx body, range loop emits one t.Run per case, every case calls t.Parallel() — established as the default shape for Phase 1 test files."

requirements-completed:
  - ERR-01
  - ERR-02
  - ERR-03
  - ERR-04

# Metrics
duration: ~6min
completed: 2026-05-27
---

# Phase 01 Plan 02: Errors Summary

**Five exported sentinel errors and the `*APIError` leaf type (StatusCode/Path/Body/Message) with `errors.Is` status-code matching and `errors.As` round-trip — fully covered by testify, zero runtime deps.**

## Performance

- **Duration:** ~6 minutes
- **Started:** 2026-05-27T08:20:00Z (approximate)
- **Completed:** 2026-05-27T08:26:16Z
- **Tasks:** 2 completed
- **Files modified:** 4 (2 created — `errors.go`, `errors_test.go`; 2 updated by `go mod tidy` — `go.mod`, `go.sum`)

## Accomplishments

- Shipped the five exported sentinel error vars (`ErrInvalidCountry`, `ErrInvalidLanguage`, `ErrDateRangeTooLarge`, `ErrInvalidDateRange`, `ErrEmptyResponse`) — each carrying the project-standard `"openholidays: "` prefix, each with a godoc comment, all declared inside a single `var ( ... )` block.
- Shipped the `*APIError` leaf type with fields exactly `StatusCode int, Path string, Body []byte, Message string` (D-14) plus `Error() string` (D-18 two-format rule) and `Is(target error) bool` (D-15 wildcard semantics; Pitfall 5 ignores Path/Body/Message on the target). No `Unwrap()` (D-16) — verified via grep before commit.
- Locked ROADMAP success criterion #2 (`errors.Is` works through `%w` wrappers for every sentinel) and #3 (`errors.As` extracts `*APIError` with populated fields) — 100% statement coverage on `errors.go` via `go test -race -cover ./...`.
- Test file follows Gold Rule 3 strictly: exactly five TestXxx functions, every assertion is inside a `t.Run` block, top-level `t.Parallel()` plus per-case `t.Parallel()`, `require` for preconditions and `assert` for verifications.

## Task Commits

1. **Task 1 — Create `errors.go` with 5 sentinels and `*APIError` type** — `f89e3bf` (feat)
2. **Task 2 — Create `errors_test.go` with full testify coverage** — `cd16161` (test)

## Files Created/Modified

- `errors.go` (created, 105 lines) — Five sentinel errors in a single `var` block plus the `*APIError` type with `Error()` and `Is()`. Imports only `errors` and `fmt`. No `Unwrap()`. Godoc on every exported symbol; godoc on `*APIError` documents the `errors.Is(err, &APIError{StatusCode: 404})` pattern and the wildcard rule for zero `StatusCode`. Godoc on `Is` explicitly documents that `Path`/`Body`/`Message` on the target are ignored (Pitfall 5).
- `errors_test.go` (created, ~250 lines, in-package `package openholidays`) — Five test functions:
  - `TestSentinelErrors` — ERR-01: per-sentinel non-nil, prefix, distinct-identity checks.
  - `TestSentinels_ErrorsIs` — ERR-03 / ROADMAP #2: `errors.Is(fmt.Errorf("...: %w", sentinel), sentinel) == true` for every sentinel; identity does not bleed across sentinels through the wrap.
  - `TestAPIError_Error` — D-18: three cases locking the two output formats and the Body-not-in-Error invariant (ERR-04 / T-01-02-IL).
  - `TestAPIError_Is` — D-15 / Pitfall 5: eight cases including status-match, two wildcards (`StatusCode: 0` and zero-value `&APIError{}`), mismatch, path/message/body each ignored on the target, and non-`*APIError` target.
  - `TestAPIError_ErrorsAs` — ERR-02 / ROADMAP #3: all four fields preserved through a `fmt.Errorf("transport failure: %w", apiErr)` wrap.
- `go.mod` (modified by `go mod tidy`) — `github.com/stretchr/testify v1.11.1` no longer marked `// indirect`; three test-transitive deps recorded as `// indirect`.
- `go.sum` (modified by `go mod tidy`) — module sums added for `davecgh/go-spew`, `pmezard/go-difflib`, `gopkg.in/yaml.v3`.

## Decisions Made

- **TDD-flag interpretation:** Task 1's `tdd="true"` was treated as advisory because Task 2 owns the entire `errors_test.go` in a separate commit. Splitting into `feat(...)` then `test(...)` honors each task's `<verify>` block literally (Task 1 verifies via build+grep; Task 2 verifies via `go test`). The plan's overall contract (one TestXxx per exported function/method per Gold Rule 3) is fully met.
- **Body field never in `Error()`:** locked by an explicit test case (`TestAPIError_Error/body-never-in-error-string`) that sets `Body: []byte("ignored by Error")` and asserts the formatted string does not contain it. This is the ERR-04 / T-01-02-IL mitigation contract in test form.
- **Wildcard semantics test cases:** added both `&APIError{StatusCode: 0}` and `&APIError{}` (zero value) as separate `t.Run` cases — both must match. This codifies that callers can write either `errors.Is(err, &APIError{})` or `errors.Is(err, &APIError{StatusCode: 0})` and get the same "any APIError" semantics.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 — Blocking] `go.sum` missing entries for testify's transitive deps**

- **Found during:** Task 2 (first attempt to compile `errors_test.go`)
- **Issue:** Plan 01-01 added `github.com/stretchr/testify v1.11.1` with `// indirect` because no `*_test.go` file imported it yet. When Task 2's `errors_test.go` introduced the first real testify import, the test compile failed with `missing go.sum entry for module providing package github.com/davecgh/go-spew/spew`. This is the exact promotion-from-indirect-to-direct transition Plan 01-01's SUMMARY anticipated: *"testify recorded but not yet imported (will become direct in Phase 1 Plan 02 or later when first \*\_test.go imports it)"*.
- **Fix:** Ran `go mod tidy`. testify lost its `// indirect` marker (now a direct dep); three test-transitive deps were added as `// indirect`: `github.com/davecgh/go-spew v1.1.1`, `github.com/pmezard/go-difflib v1.0.0`, `gopkg.in/yaml.v3 v3.0.1`. All three are well-known, pre-vetted testify chain members; no new top-level dep was introduced.
- **Files modified:** `go.mod`, `go.sum`
- **Verification:** `go test -race -cover ./...` exits 0 with `coverage: 100.0% of statements`; `go vet ./...` clean.
- **Committed in:** `cd16161` (Task 2 commit — bundled with `errors_test.go` so the test commit lands self-consistent).

---

**Total deviations:** 1 auto-fixed (Rule 3 — blocking compilation).
**Impact on plan:** Necessary for the test file to compile. The deviation was explicitly anticipated by Plan 01-01's SUMMARY (which stated testify would become direct at the first `*_test.go` import). No scope creep — no new packages introduced, only testify's own dependency closure was resolved.

## Issues Encountered

None. The deviation above is a clean "this transition will happen" event already foreshadowed in the prior plan's SUMMARY.

## TDD Gate Compliance

The plan tagged Task 1 with `tdd="true"` but routed all assertion code into Task 2 (a separate file, separate commit). Per the plan's `<verify>` blocks (Task 1: build/grep only; Task 2: `go test`), the resulting commit sequence is `feat(01-02): ...` → `test(01-02): ...`, which inverts the canonical RED→GREEN order. This was treated as advisory because:

1. Task 1's behavior contract is structural (the right symbols exist with the right shape) — verifiable without runtime tests via `grep`, which is exactly what Task 1's `<verify>` does.
2. Task 2 owns the runtime assertions and ran them after each was added.
3. Gold Rule 3's "one TestXxx per exported function" was fully honored in Task 2.

No information was lost — the test suite locks the same invariants whether RED preceded GREEN or both shipped in tandem. Recommend the planner remove `tdd="true"` from Task 1 in future similar plans (or merge the two tasks under one TDD flag) to avoid the surface inconsistency.

## Known Stubs

None. Both `errors.go` and `errors_test.go` are complete relative to their Phase 1 scope. The `*APIError` type's construction logic is intentionally deferred to Phase 2 (D-19), but that is a documented phase boundary — the type itself is fully shipped here.

## Threat Flags

None. This plan ships pure types and methods; no network surface, no goroutines, no file I/O, no schema. The three STRIDE entries from the plan's threat model (T-01-02-IL, T-01-02-SP, T-01-02-IS) are all mitigated as designed:

- **T-01-02-IL (Information Disclosure via Error()):** `TestAPIError_Error/body-never-in-error-string` locks the invariant.
- **T-01-02-SP (Sentinel spoofing via package-level var reassignment):** accepted per the plan; no Phase 1 code mutates these sentinels; `TestSentinelErrors` asserts distinct identities.
- **T-01-02-IS (Is() matching scope creep):** `TestAPIError_Is/path-ignored-on-target`, `message-ignored-on-target`, and `body-ignored-on-target` lock Pitfall 5 against future regression.

## Self-Check: PASSED

Verified before SUMMARY write:

- `errors.go` exists (105 lines, gofmt-clean, vet-clean).
- `errors_test.go` exists (gofmt-clean, vet-clean).
- Commit `f89e3bf` (Task 1) found in `git log`.
- Commit `cd16161` (Task 2) found in `git log`.
- `go test -race -cover ./...` → `ok  github.com/egeek-tech/go-openholidays  coverage: 100.0% of statements`.
- `gofmt -l errors.go errors_test.go` → empty output.
- `go vet ./...` → exits 0.
- Sentinel count: 5 (matches).
- TestXxx count: 5 (matches).
- `grep 'func (e \*APIError) Unwrap' errors.go` → no match (D-16 honored).

## Next Phase Readiness

- The error contract is locked: Phase 2's transport layer can now construct `*APIError` values from `*http.Response` knowing the type shape, `Error()` format, and `Is()` matching rules are stable.
- Plan 01-03 (date.go) can wrap `errEmptyDate` privately (D-06) without colliding with the locked 5-sentinel surface.
- Plan 01-05 (validate.go) can wrap any of these 5 sentinels with `%w` — the test suite in Plan 01-02 has already proven `errors.Is` works through `%w` wrappers for every sentinel.
- Plan 01-06 (FuzzDateUnmarshal + PROJECT.md Key Decisions entries) must still add the CL-01 row to PROJECT.md `Key Decisions` documenting the 5-sentinel deviation from ROADMAP criterion #2's literal "4 sentinels" wording.

---
*Phase: 01-foundation*
*Completed: 2026-05-27*
