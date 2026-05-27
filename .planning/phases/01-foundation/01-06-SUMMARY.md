---
phase: 01-foundation
plan: 06
subsystem: phase-closure
tags: [go, ast-audit, client-10, project-md, key-decisions, phase-closeout]

# Dependency graph
requires:
  - phase: 01-foundation
    provides: the 5 exported sentinels + unexported errEmptyDate (errors.go, date.go from Plans 02 and 03); production source files to audit (Plans 02-05); D-22 leap-year deviation finding (Plan 05 SUMMARY)
provides:
  - "TestNoInitOrGlobalState (AST audit locking CLIENT-10): no `func init()` declared in any production *.go in the repo; only six allowlisted package-level vars permitted (the five exported sentinels plus the unexported errEmptyDate)"
  - "PROJECT.md Key Decisions table extended with 6 rows (CL-01..CL-06) recording every Phase 1 scope clarification + the Plan 05 leap-year formula correction"
affects: [phase-01-verification, phase-02-transport, all-future-phases]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "AST-based source-tree audit using go/parser + go/ast + go/token (Validation Architecture Wave 0 Gap)"
    - "Closed allowlist for package-level vars enforced mechanically at test time"
    - "filepath.WalkDir with skip-list for .git/.planning/.claude/testdata/internal and explicit /cmd/* exclusion"

key-files:
  created:
    - internal_test.go
  modified:
    - .planning/PROJECT.md

key-decisions:
  - "Used closed-allowlist AST audit (whitelist 6 var names, reject everything else) rather than blacklist of forbidden patterns. Adding a new package-level var to the codebase therefore requires a deliberate edit to allowedVars in internal_test.go — and that edit shows up in code review as the canonical place where the CLIENT-10 exception is granted."
  - "Skipped the /cmd/* subtree from the audit so Phase 5's demo CLI can use idiomatic flag.FlagSet package-level vars without false-positive failures. CLIENT-10 'no global mutable state' applies to library code; the cmd CLI is an external consumer."
  - "Recorded one ADDITIONAL Key Decisions row beyond the 5 the plan called for: CL-06 captures the Plan 05 leap-year formula correction (backward-anchored to.AddDate(-3, 0, 0) replacing the plan-prescribed forward formula). CL-06 was directed by the executor brief's critical_addendum so phase verification matches the implementation."

patterns-established:
  - "Mechanical invariant audits: when an architectural rule must hold for every future commit (CLIENT-10), write a test that walks the AST and fails CI on violation. Cheaper and more reliable than relying on code review alone."
  - "Negative testing of AST audits: verify the auditor catches violations by planting a violator file in the worktree, running the test (must fail), then removing the violator (must pass). Documented in this plan's deviations as a verification step, not a regression."

requirements-completed: [CLIENT-10]

# Metrics
duration: ~4min
completed: 2026-05-27
---

# Phase 1 Plan 6: Closure Summary

**AST-based CLIENT-10 audit (TestNoInitOrGlobalState) ships in internal_test.go and locks the no-init / no-global-mutable-state invariant for every future commit, plus PROJECT.md Key Decisions table records six scope-clarification decisions (CL-01..CL-06) closing the Phase 1 deviation paperwork.**

## Performance

- **Duration:** ~4 min
- **Completed:** 2026-05-27T08:46:43Z
- **Tasks:** 2 (both committed atomically)
- **Files modified:** 2 (1 created, 1 edited)

## Accomplishments

- `internal_test.go` ships `TestNoInitOrGlobalState`, a `t.Parallel()` AST audit that walks every production `*.go` file in the repo via `go/parser` + `go/ast` and asserts:
  - Zero `func init()` declarations exist in production code.
  - The only package-level `var` identifiers are the locked six: `ErrInvalidCountry`, `ErrInvalidLanguage`, `ErrDateRangeTooLarge`, `ErrInvalidDateRange`, `ErrEmptyResponse`, and the unexported `errEmptyDate`.
  - The walk visited at least 4 production `.go` files (sanity guard against silent-skip regressions; the current codebase has 6).
- The audit was verified end-to-end with a planted violator file (added a `func init()` + a `var temporaryGlobal`): the test failed with the expected detailed message identifying both violations, then passed after removing the file.
- PROJECT.md `## Key Decisions` table now contains 6 new rows (CL-01..CL-06) each marked `Adopted in Phase 1`, each citing its source CONTEXT.md / RESEARCH.md / SUMMARY.md location. The 11 existing rows remain untouched.
- The PROJECT.md edit is a pure 6-line insertion (`git diff --stat` shows `1 file changed, 6 insertions(+)` and no deletions).
- Full Phase 1 suite stays green at 100.0% statement coverage with `-race` clean; `go vet`, `go build`, `gofmt -l` all clean; 30s fuzz smoke on `FuzzDateUnmarshal` clean.

## Task Commits

Each task was committed atomically on `worktree-agent-ace2ce43c27b4ddad`:

1. **Task 1 — CLIENT-10 AST audit** — `34278db` (test): internal_test.go (+217 lines, new file).
2. **Task 2 — PROJECT.md CL-01..CL-06 rows** — `6521ad9` (docs): .planning/PROJECT.md (+6 lines).

## Files Created/Modified

- `internal_test.go` (217 lines) — single `TestNoInitOrGlobalState` test function with two `t.Run` subtests. Imports the stdlib AST trio (`go/ast`, `go/parser`, `go/token`), `os`, `path/filepath`, `sort`, `strconv`, `strings`, `testing`, plus testify's `assert`/`require`. Two package-level vars in the test file itself (`allowedVars`, `skipDirs`) — both are immutable lookup maps used only by the AST walker; they live in the test file, so they are exempt from CLIENT-10 (which applies to production code only — the audit explicitly skips `*_test.go` files).
- `.planning/PROJECT.md` (+6 lines) — Key Decisions table rows CL-01 through CL-06.

## Decisions Made

- **Closed-allowlist audit, not blacklist.** The test maintains an explicit allowlist of 6 var identifiers. Adding a 7th package-level var anywhere in production code is a CI failure until the contributor edits `allowedVars` in `internal_test.go` and gets that edit reviewed. This forces every CLIENT-10 exception through the same gate.
- **Skip `/cmd/*` from the audit.** Phase 5's `cmd/ohcli` will use stdlib `flag` with package-level `flag.FlagSet` values per Go CLI convention. CLIENT-10 ("no global mutable state") applies to the library; demo binaries are external consumers. Documented inline in the test source.
- **Walk-up `findRepoRoot()` helper.** The test discovers the repo root by walking up from `os.Getwd()` looking for `go.mod`. For Phase 1's single-package layout this returns the same directory as `os.Getwd()`, but the walk-up makes the test resilient if Phase 2+ moves it under a subpackage.
- **Include CL-06 (not in original plan).** The executor brief's `<critical_addendum>` directed adding a 6th Key Decisions row capturing Plan 01-05's leap-year deviation: `validateDateRange` uses backward-anchored `to.AddDate(-3, 0, 0)` rather than the plan-prescribed forward `from.AddDate(3, 0, 1)`. Plan 05's SUMMARY documents the asymmetry; without recording it in PROJECT.md, the implementation and the documented decision (D-22 in CONTEXT.md) would diverge. Treated as a Rule 2 critical functionality add (the brief explicitly required it), tracked here as a deliberate scope expansion of Plan 06.

## Deviations from Plan

### Auto-added critical content

**1. [Rule 2 — Missing critical functionality] CL-06 row added to PROJECT.md per executor brief addendum**

- **Found during:** Pre-execution context review (executor brief `<critical_addendum>` block).
- **Issue:** Plan 06 as written calls for 5 Key Decisions rows (CL-01..CL-05). Plan 01-05's SUMMARY documents that `validateDateRange` ships with the backward-anchored formula `to.AddDate(-3, 0, 0)` rather than the plan-prescribed forward `from.AddDate(3, 0, 1)` (forward formula fails the leap-year boundary because Go's `time.AddDate` normalizes `2024-02-29 + 3yr + 1d` to `2027-03-02`, accepting the case the ROADMAP requires to reject). Without recording this in PROJECT.md, the documented decision (D-22 in CONTEXT.md, which still references the forward formula in its current text) and the live implementation diverge.
- **Fix:** Added a 6th row CL-06 to PROJECT.md's Key Decisions table citing `.planning/phases/01-foundation/01-05-SUMMARY.md` "Deviations from Plan" Rule 1 as the source. Outcome marked `Adopted in Phase 1`.
- **Files modified:** .planning/PROJECT.md (one extra row beyond the 5 the original plan called for).
- **Committed in:** 6521ad9 (same commit as CL-01..CL-05).
- **Plan-level follow-up needed:** CONTEXT.md D-22 still describes the forward formula. Updating it is out of scope for this plan (CONTEXT.md is the locked discuss-phase artifact for Phase 1) — leave it as a phase-verifier note: the discrepancy between D-22 text and the implementation is now traceable via CL-06's source citation.

### Other notes

- The plan's `<verify><automated>` block for Task 1 included a `gofmt -l internal_test.go | grep -c '.' | grep -q '^0$'` clause. Ran `gofmt -l internal_test.go` directly — empty output, gate satisfied.
- The plan's success criterion specified asserting "only 5 sentinels (ErrInvalidCountry, ErrInvalidLanguage, ErrDateRangeTooLarge, ErrInvalidDateRange, ErrEmptyResponse) AND the unexported errEmptyDate from date.go" — the allowedVars set in internal_test.go matches exactly. Plan body's must_haves item phrased it as "6 sentinels" total; both phrasings mean the same set.

---

**Total deviations:** 1 auto-added (Rule 2 — Missing critical functionality). No bugs encountered, no architectural changes proposed.

**Impact on plan:** CL-06 was a directed scope expansion from the executor brief's critical_addendum, not a regression. Without it, PROJECT.md Key Decisions would document a formula that does not match the shipped code, leaving Phase 1 verification in an inconsistent state.

## Issues Encountered

- None.

## Stub tracking

No stubs added in this plan. Both deliverables are production-quality:
- `internal_test.go` ships the full AST walker, not a placeholder.
- PROJECT.md rows cite verifiable upstream sources (CONTEXT.md, RESEARCH.md, 01-05-SUMMARY.md).

## Threat Flags

None. The AST audit operates entirely against the project's own source tree (test-time only, no network or external IO). No new trust boundaries introduced.

## User Setup Required

None — no external service configuration required.

## Next Phase Readiness

Phase 1 is now ready for `/gsd:verify-phase 1`:

- **ROADMAP success criterion #1** (locked sentinels + APIError) — satisfied by Plan 02 + this plan's audit locking the set.
- **ROADMAP success criterion #2** (Date type with all methods) — satisfied by Plan 03.
- **ROADMAP success criterion #3** (errors.As round-trip + Is matching) — satisfied by Plan 02.
- **ROADMAP success criterion #4** (validators with correct leap-year behavior) — satisfied by Plan 05 (now documented via CL-06).
- **ROADMAP success criterion #5** (CLIENT-10 mechanically locked) — satisfied by this plan.

Phase 2 (transport) can begin once the verifier signs off. The audit in this plan will catch any package-level var that future phases try to add — Phase 2's `Client` struct must therefore live as a type with fields, not as package-level state.

## Self-Check

Verified before writing this section.

**Created files exist:**
```bash
[ -f /data/git/private/holidays/.claude/worktrees/agent-ace2ce43c27b4ddad/internal_test.go ] && echo FOUND
```
Result: FOUND.

**Modified files have the expected changes:**
- `.planning/PROJECT.md` has 6 rows matching `^| CL-0[1-6]:` (grep returned 6); 6 rows containing `Adopted in Phase 1` (grep returned 6).

**Commits exist on the worktree-agent branch:**
- `34278db` — `test(01-06): add CLIENT-10 AST audit (TestNoInitOrGlobalState)` — FOUND.
- `6521ad9` — `docs(01-06): record CL-01..CL-06 in PROJECT.md Key Decisions` — FOUND.

**Plan verification gates:**
- `go test -race -cover ./...` → ok, 100.0% coverage.
- `go test -race -run TestNoInitOrGlobalState ./...` → ok.
- `grep -rn 'func init' --include='*.go' . | grep -v _test.go | grep -v '.planning/' | wc -l` → 0.
- `go build ./...` → exit 0.
- `go vet ./...` → exit 0.
- `gofmt -l doc.go version.go errors.go date.go types.go validate.go internal_test.go errors_test.go date_test.go types_test.go validate_test.go` → empty output.
- `go test -fuzz=FuzzDateUnmarshal -fuzztime=30s ./...` → PASS, 4,855,054 execs in 30 s.

## Self-Check: PASSED

---
*Phase: 01-foundation*
*Completed: 2026-05-27*
