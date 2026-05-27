---
phase: 01-foundation
plan: 03
subsystem: types
tags: [go, date, json, fuzz, testify, tdd, utc-midnight]

# Dependency graph
requires:
  - phase: 01-foundation
    plan: 01
    provides: Go module identity (module github.com/egeek-tech/go-openholidays, go 1.23) + testify v1.11.1 declared in go.mod
provides:
  - "Date wrapper type (type Date struct { time.Time }) normalized to UTC midnight (D-05)"
  - "MarshalJSON / UnmarshalJSON locking ROADMAP success criterion #1 (D-06, D-07)"
  - "NewDate / ParseDate constructors at UTC midnight (D-11)"
  - "Equal / Before / After / Compare with defensive UTC normalization (D-09)"
  - "DaysUntil inclusive day count, DST-safe (D-10)"
  - "errEmptyDate unexported sentinel — keeps public surface at 5 sentinels (D-06)"
  - "FuzzDateUnmarshal — panic-free invariant for (*Date).UnmarshalJSON (D-12, CL-03)"
affects: [01-04-types, 01-05-validators, 02-transport, 03-endpoints]

# Tech tracking
tech-stack:
  added:
    - "github.com/stretchr/testify v1.11.1 promoted from // indirect to direct require (first test import landed)"
    - "Transitive test-only sums: github.com/davecgh/go-spew v1.1.1, github.com/pmezard/go-difflib v1.0.0, gopkg.in/yaml.v3 v3.0.1"
  patterns:
    - "Wrapper struct embedding time.Time (Pattern 1 from 01-RESEARCH.md)"
    - "Defensive UTC normalization via unexported toUTCMidnight helper used by every comparison method"
    - "%w-wrapped unexported sentinel (errEmptyDate) for null + empty rejection, kept off the public sentinel list"
    - "Custom JSON unmarshaler with explicit branch order: null check → JSON-string-shape check → empty check → time.Parse"
    - "Go 1.18+ fuzz target (testing.F) with explicit f.Add seed corpus per JSON-3 mandate"
    - "Table-driven tests with t.Parallel() at top-level and per t.Run case (Gold Rule 3 + Go 1.22+ loop-scoping)"

key-files:
  created:
    - "date.go (173 lines, 13 exported + 1 unexported identifiers, all docstrings)"
    - "date_test.go (~310 lines, 10 TestXxx + 1 FuzzDateUnmarshal, 100% coverage on date.go)"
  modified:
    - "go.mod (testify directive promoted from // indirect to direct + 3 transitive indirect lines)"
    - "go.sum (4 new sum entries for testify's transitive test-only graph)"

key-decisions:
  - "errEmptyDate kept lowercase-first (unexported) per D-06 — public sentinel surface stays at the locked 5 (CL-01)"
  - "MarshalJSON uses AppendFormat with a 12-byte preallocated buffer (canonical efficient form from RESEARCH.md Pattern 1)"
  - "UnmarshalJSON branch order: null → JSON-string shape → empty → time.Parse — explicit and complete per Pitfall JSON-3"
  - "DaysUntil semantics: inclusive count, returns 1 for same-day; negative for d > other (D-10)"
  - "Defensive toUTCMidnight invoked by every comparison method — handles struct-literal Date constructions outside NewDate/ParseDate (Pitfall TZ-1)"
  - "FuzzDateUnmarshal ships in Phase 1 not Phase 5 (CL-03 — must land in PROJECT.md Key Decisions by Plan 06)"
  - "go mod tidy promoted testify to direct require — confirms test-only invariant (only *_test.go files import testify; go mod why will report test-only)"

metrics:
  start: "2026-05-27T08:21:00Z"
  end: "2026-05-27T08:27:08Z"
  duration_minutes: 6
  tasks_completed: 2
  files_created: 2
  files_modified: 2
  commits: 2
  coverage_pct: 100
---

# Phase 01 Plan 03: Date Type Summary

Shipped the canonical `Date` wrapper type (struct embedding `time.Time` with UTC-midnight invariants) plus a comprehensive testify+fuzz test suite. ROADMAP success criterion #1 is now locked: `"2025-12-24"` JSON round-trips byte-for-byte, while `null` and empty JSON strings produce `errors.Is(err, errEmptyDate)` matches instead of silent zero values.

## What Shipped

**`date.go`** declares `package openholidays` with imports `bytes`, `fmt`, `time` (stdlib only — zero runtime deps). It exports:

| Identifier | Signature | Behavior |
|------------|-----------|----------|
| `Date` | `type Date struct { time.Time }` | Wrapper struct; UTC-midnight invariant for all construction paths |
| `NewDate` | `func NewDate(year int, month time.Month, day int) Date` | Builds Date at UTC midnight |
| `ParseDate` | `func ParseDate(s string) (Date, error)` | Empty → `errEmptyDate`; malformed → wrapped `time.Parse` error |
| `Date.MarshalJSON` | `func (d Date) MarshalJSON() ([]byte, error)` | Emits `"YYYY-MM-DD"`; zero `Date{}` → `"0001-01-01"` |
| `Date.UnmarshalJSON` | `func (d *Date) UnmarshalJSON(b []byte) error` | Rejects `null` + `""` via `%w`-wrapped `errEmptyDate`; non-string → "must be JSON string" |
| `Date.String` | `func (d Date) String() string` | Shadows embedded `time.Time.String()` to emit `YYYY-MM-DD` |
| `Date.Equal` | `func (d Date) Equal(other Date) bool` | Both operands normalized to UTC midnight first |
| `Date.Before` | `func (d Date) Before(other Date) bool` | Both operands normalized to UTC midnight first |
| `Date.After` | `func (d Date) After(other Date) bool` | Both operands normalized to UTC midnight first |
| `Date.Compare` | `func (d Date) Compare(other Date) int` | Returns -1/0/+1; UTC-midnight normalized |
| `Date.DaysUntil` | `func (d Date) DaysUntil(other Date) int` | Inclusive count; 1 for same day; negative for backwards; DST-safe |

Unexported helpers also shipped:

- `const dateLayout = "2006-01-02"` — wire-format layout
- `var errEmptyDate` — internal sentinel (D-06: kept off the public 5-sentinel surface)
- `func (d Date) toUTCMidnight() time.Time` — defensive normalization (called by every comparison method)

**`date_test.go`** declares `package openholidays` (in-package, so it can reference the unexported `errEmptyDate` for `errors.Is` assertions) and ships:

- 10 `func TestXxx(t *testing.T)` — one per exported production function per Gold Rule 3
- 1 `func FuzzDateUnmarshal(f *testing.F)` with 7 explicit `f.Add` seed entries
- Every case wrapped in `t.Run(name, func(t *testing.T){...})`
- `t.Parallel()` at top-level and per subtest
- `require` for preconditions, `assert` for verifications

## Verification Evidence

| Command | Result |
|---------|--------|
| `go build ./...` | exits 0 |
| `go vet ./...` | exits 0, no findings |
| `gofmt -l date.go date_test.go` | empty output (clean) |
| `go test -race -cover ./...` | `ok` — coverage 100.0% of statements |
| `go test -race -run 'TestNewDate\|TestParseDate\|TestDate_*'` | 10 tests + 35 subtests all PASS |
| `go test -fuzz=FuzzDateUnmarshal -fuzztime=10s ./...` | 1,271,227 execs in 11s, 0 panics, 0 crashers, 149 new interesting inputs |
| `grep -c '^func Test' date_test.go` | 10 |
| `grep -c '^func FuzzDateUnmarshal' date_test.go` | 1 |
| `grep -c '^type Date struct {$' date.go` | 1 |
| `! grep -q '^var ErrEmptyDate' date.go` | exits 0 — unexported sentinel preserved |

## Success Criteria — Mapped

| # | Criterion | Status |
|---|-----------|--------|
| 1 | `date.go` ships every identifier from `<interfaces>` with exact signatures and canonical Pattern 1 body | DONE |
| 2 | `date_test.go` declares exactly 10 TestXxx + 1 FuzzDateUnmarshal; Gold Rule 3 followed | DONE |
| 3 | `go test -race ./...` exits 0; `go test -fuzz=FuzzDateUnmarshal -fuzztime=10s ./...` exits 0 (no panics in 10s smoke) | DONE |
| 4 | ROADMAP success criterion #1 verified: round-trip works; `null` and `""` produce errors not silent zero values | DONE (see `TestDate_MarshalJSON/roundtrip_locks_roadmap_criterion_1`, `TestDate_UnmarshalJSON/null_returns_errEmptyDate`, `TestDate_UnmarshalJSON/empty_json_string_returns_errEmptyDate`) |
| 5 | `gofmt -l date.go date_test.go` produces no output; `go vet ./...` exits 0 | DONE |

## Commits

| Hash | Type | Files | Subject |
|------|------|-------|---------|
| `0fc7350` | feat | `date.go` | `feat(01-03): add Date type with UTC-midnight normalization` |
| `13cd276` | test | `date_test.go`, `go.mod`, `go.sum` | `test(01-03): add Date tests + FuzzDateUnmarshal` |

Both commits are atomic per task. Each commit is independently runnable: after `0fc7350`, `go build ./...` and `go vet ./...` pass; after `13cd276`, the full suite + fuzz smoke pass under `-race`.

## TDD Gate Compliance

Plan 01-03 marked Task 1 with `tdd="true"` but split production code (Task 1) and test code (Task 2) into separate atomic commits. As a result, the gate sequence in git log is:

1. `feat(01-03)` — production code first (Task 1)
2. `test(01-03)` — comprehensive tests second (Task 2)

This is the inverse of strict RED-then-GREEN. The plan's structure made this unavoidable: Task 1's `<files>` is `date.go` only and its commit was self-contained. The tests (including the JSON-3 fuzz target) all pass against the production code, so the GREEN-equivalent gate (all assertions hold) is satisfied. No RED commit was emitted, but ROADMAP success criterion #1 is locked behind the GREEN-equivalent suite.

If strict RED gating is required later, the planner can split Task 1 to commit failing tests first against a stub implementation, then commit the canonical body.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 — Blocking] Ran `go mod tidy` to refresh `go.sum` for testify's transitive graph**
- **Found during:** Task 2 (first test execution attempt)
- **Issue:** `go test` failed with `missing go.sum entry for module providing package github.com/davecgh/go-spew/spew (imported by github.com/stretchr/testify/assert)`. testify v1.11.1 was already declared in `go.mod` (from Plan 01-01) but no test code had imported it yet, so transitive sums hadn't been recorded.
- **Fix:** `go mod tidy`. This is NOT a Rule 3 package install — testify is pre-approved, already declared, and the sum-file refresh is a mechanical consequence of the first test import. It also promoted the `// indirect` directive on testify to a direct require, which now correctly reflects that `date_test.go` imports it.
- **Files modified:** `go.mod`, `go.sum`
- **Commit:** `13cd276` (rolled into the Task 2 commit since the tidy was directly caused by the new test imports)

**2. [Style] Reformatted FuzzDateUnmarshal seeds from a `for range` loop to 7 explicit `f.Add(...)` calls**
- **Found during:** Task 2 acceptance criteria check
- **Issue:** Initial draft used a `for _, s := range seeds { f.Add(s) }` loop. The plan's acceptance criterion reads "at least 7 `f.Add(...)` seed corpus entries" — semantically equivalent (the fuzz runtime confirms `7/7 baseline coverage`), but a literal grep for `f.Add(` returned only 1 occurrence.
- **Fix:** Rewrote to 7 explicit `f.Add` calls so a literal `grep -c 'f\.Add('` returns 7. Behavior identical; readability improved (each seed is annotated with its purpose).
- **Files modified:** `date_test.go` (Task 2 commit, pre-stage)
- **Commit:** `13cd276`

### Architectural Decisions

None.

### Authentication Gates

None.

## Known Stubs

None — every behavior committed is wired and tested.

## Threat Flags

None — the plan's `<threat_model>` covers the surface introduced (`(*Date).UnmarshalJSON`). No new ingress beyond what was modeled.

## Outstanding Follow-ups

- **PROJECT.md Key Decisions entry for CL-03** (fuzz in Phase 1) — Plan 01-06 owns the consolidated Key Decisions update for all Phase 1 deviations. Tracked in STATE.md "Open Todos".
- **Plan 04 (types.go)** can now embed `Date` in `Holiday.StartDate` / `Holiday.EndDate`.
- **Plan 05 (validate.go)** can now call `Date.After` and `Date.AddDate` in `validateDateRange`.

## Self-Check: PASSED

- `[ -f date.go ]` → FOUND
- `[ -f date_test.go ]` → FOUND
- `[ -f .planning/phases/01-foundation/01-03-SUMMARY.md ]` → FOUND (this file)
- `git log --all | grep 0fc7350` → FOUND
- `git log --all | grep 13cd276` → FOUND
- Coverage 100% verified by `go test -race -cover ./...`
- Fuzz smoke green (1.27M execs, 0 panics)
