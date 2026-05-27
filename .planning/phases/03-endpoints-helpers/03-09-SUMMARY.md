---
phase: 03-endpoints-helpers
plan: 09
subsystem: testing
tags: [fixtures, drift-detection, json-indent, integration-test, lint-cleanup]

requires:
  - phase: 03-endpoints-helpers
    provides: "Phase 3 testdata/*.json fixtures + integration-tagged update_fixtures_test.go harness (plans 03-02 through 03-08); 03-VERIFICATION.md identified CR-01, CR-02, WR-05 as gaps."
provides:
  - "update_fixtures_test.go writer output that matches the on-disk fixture format byte-for-byte (4-space indent + trailing newline)"
  - "uniform 4-space + trailing-newline format across all six committed testdata/*.json fixtures"
  - "removal of the CLAUDE.md-banned `cap := cap` builtin-shadow in the for-range loop"
affects:
  - "04-* and later phases that rely on drift-detection mode of TestUpdateFixtures reporting OK in CI"
  - "phase-05 CI lint enforcement (golangci-lint with revive/gocritic) — the cap-shadow violation is gone"

tech-stack:
  added: []
  patterns:
    - "writer/on-disk byte-equivalence: the integration harness's json.Indent settings and trailing-newline byte must match the committed fixture format exactly so the no-flag mode is a true drift-detector (not a false-positive generator)"

key-files:
  created: []
  modified:
    - update_fixtures_test.go
    - testdata/countries.json
    - testdata/school_holidays_pl_2025.json

key-decisions:
  - "Convergence direction = 4-space (not 2-space): 4 of 6 fixtures already used 4-space, so re-indenting the two 2-space ones is the smaller diff and matches the most common existing convention. Reverse direction would have churned 4 fixtures instead of 2."
  - "Loop variable rename = `c` (not `cap_`, not `capture`): `c` is the canonical Go single-letter for a struct ranged-over in a tight loop; `capture` would collide with the local type name; underscored names are non-idiomatic in Go."
  - "Re-indent path = Go's encoding/json (not Python json.dumps): using the same code path the writer uses guarantees byte-equivalence with the post-fix writer output. Python would have required separate equivalence verification."

patterns-established:
  - "Fixture-refresh writer parity: the bytes produced by update_fixtures_test.go's pretty-print pipeline (json.Indent + trailing '\\n') MUST equal the bytes committed in testdata/*.json. Any future fixture added to the harness must be in this same shape, and any future change to the writer must be paralleled by re-indenting committed fixtures."

requirements-completed: [TEST-03]

duration: 12min
completed: 2026-05-27
---

# Phase 03 Plan 09: Fixture-refresh gap closure (CR-01, CR-02, WR-05) Summary

**Repaired the TEST-03 drift-detection harness: writer now emits 4-space indent + trailing newline matching the on-disk format, all six fixtures share that uniform format, and the CLAUDE.md-banned `cap := cap` builtin shadow is gone — drift-detection mode is now a true regression signal instead of a false-positive generator.**

## Performance

- **Duration:** 12 min
- **Started:** 2026-05-27T20:00:42Z (approx; matches first read of plan)
- **Completed:** 2026-05-27T20:13:25Z
- **Tasks:** 2
- **Files modified:** 3 (1 source + 2 fixtures)

## Accomplishments

- **CR-01 closed** (writer indent): `json.Indent(&pretty, body, "", "  ")` (2-space) → `json.Indent(&pretty, body, "", "    ")` (4-space) at the one call site in `update_fixtures_test.go`. The comment referencing `testdata/countries.json` as the 2-space reference fixture was updated to point at `testdata/languages.json` (the 4-space reference).
- **CR-02 closed** (trailing newline): `pretty.WriteByte('\n')` appended immediately after `json.Indent` and before the `if !*updateFixtures {` branch. `(*bytes.Buffer).WriteByte` never returns an error per stdlib docs, so no error wrap is needed. The Operating-modes godoc block was rewritten to document the trailing-newline appendix.
- **WR-05 closed** (cap shadow): the `for _, cap := range captures` loop was renamed to `for _, c := range captures`; the `cap := cap // pin loop variable…` shadow plus its two-line justifying comment were deleted (Go 1.22+ per-iteration scoping makes the pin a no-op; the variable name additionally shadowed the Go builtin `cap`). All in-loop references (`cap.path`, `cap.query`, `cap.fixture`, `cap.validate`) were rewritten to `c.path` etc. The `type capture struct {…}` type declaration was deliberately left untouched (it is a type name in declaration position, not a variable, and is not flagged by either CLAUDE.md's `tc := tc` rule or the `cap` builtin-shadow rule).
- **Fixture convergence**: `testdata/countries.json` and `testdata/school_holidays_pl_2025.json` (the only two of the six committed fixtures that were on 2-space indent) re-indented to 4-space + trailing newline via Go's `encoding/json` round-trip (same `json.Indent(..., "    ")` code path the post-Task-1 writer uses, so the bytes are guaranteed to match what the writer emits). All six fixtures (`countries`, `languages`, `public_holidays_pl_2025`, `school_holidays_pl_2025`, `subdivisions_de`, `subdivisions_pl`) now share a single uniform format.

## Task Commits

1. **Task 1: Fix CR-01 + CR-02 + WR-05 in `update_fixtures_test.go`** — `8a79659` (fix)
2. **Task 2: Re-indent `testdata/countries.json` + `testdata/school_holidays_pl_2025.json` to 4-space + trailing newline** — `2157589` (chore)

_(SUMMARY.md commit follows below — committed by execute-plan.md's final git_commit_metadata step in worktree mode.)_

## Files Created/Modified

- `update_fixtures_test.go` — Three surgical edits in the integration harness: indent string flipped to 4-space, trailing `\n` appended to the pretty buffer, loop variable renamed away from the Go builtin `cap` (shadow + justifying comment deleted). Godoc Operating-modes block updated to document the trailing-newline appendix; reference fixture in the inline indent comment switched from `testdata/countries.json` (formerly 2-space) to `testdata/languages.json` (4-space).
- `testdata/countries.json` — Re-indented 2-space → 4-space; trailing newline preserved (was already present). Semantic content unchanged (2 country entries; verified by SHA-256 of `json.dumps(json.load(…), sort_keys=True)` before == after: `d77ca20821352d71576b494f96eac44d43e0cd5ba820c96960d20f25503fda11`).
- `testdata/school_holidays_pl_2025.json` — Re-indented 2-space → 4-space; trailing newline preserved. Semantic content unchanged (7 school-holiday entries; SHA-256 before == after: `b82894877e6442a2c57dcbfe77cb158e8de624dd4067c504dd8592d0908ee6e9`).

## Before / After — exact lines in `update_fixtures_test.go`

### CR-01 indent fix
- **Before** (line 226 of the original file): `require.NoError(t, json.Indent(&pretty, body, "", "  "))`
- **After** (now at line 230): `require.NoError(t, json.Indent(&pretty, body, "", "    "))`

### CR-02 trailing-newline fix
- **Before** (no such line): –
- **After** (line 231, immediately after the `json.Indent` call, before the `if !*updateFixtures {` branch on line 233): `pretty.WriteByte('\n')`

### WR-05 cap-shadow fix
- **Before** (lines 191–194 of the original file):
  ```go
  for _, cap := range captures {
      cap := cap // pin loop variable for the closure even though Go 1.22+
      // per-iteration scoping makes this strictly redundant — kept for
      // clarity at the cost of one linter rule that we tolerate here.
      t.Run(cap.fixture, func(t *testing.T) {
  ```
- **After** (lines 194–195):
  ```go
  for _, c := range captures {
      t.Run(c.fixture, func(t *testing.T) {
  ```
- Every in-loop `cap.path` / `cap.query` / `cap.fixture` / `cap.validate` rewritten to `c.path` etc. The struct-type declaration `type capture struct { … }` (around line 154) was deliberately left untouched.

## Decisions Made

- **4-space, not 2-space**, because 4 of 6 fixtures were already on 4-space — converging on the majority is a 2-file change instead of a 4-file change.
- **Loop variable renamed to `c`**, not `cap_` or `capture`: `c` is the canonical Go single-letter for the loop variable in a struct range; `capture` collides with the local type name; trailing-underscore names violate Go style.
- **Re-indent via Go's `encoding/json`**, not Python's `json.dumps(indent=4)`: using the same code path the writer uses guarantees byte-equivalence with the post-Task-1 writer output. The Python alternative would have required a separate byte-diff to verify equivalence.

## Deviations from Plan

None — plan executed exactly as written.

## Issues Encountered

**Tool-side write-persistence anomaly during Task 1.** The Edit and Write tools each reported success but the underlying file on disk was not actually modified (verified by `md5sum`, `git status`, and `sed -n` reading the original content from disk after each successful-looking tool call). The Read tool subsequently returned a phantom post-edit view of the file that diverged from disk reality. Shell-level writes (verified via `echo … >>` round-trip on the same file) worked normally. Workaround: applied the three Task 1 edits via a `python3` script running in `Bash`, which writes through shell I/O. Post-workaround `md5sum` confirmed the on-disk content matched the intended edits, all grep gates passed, and the commit succeeded normally. This is reported as a tool-environment observation, not a plan/code defect — the plan was executed exactly as written; only the mechanism for landing the edit differed.

## User Setup Required

None — no external service configuration required.

## Manual follow-up (not blocking this plan)

Because the drift-detection harness requires live network access to the upstream OpenHolidays API, the final end-to-end proof (the writer's output bytes equal the on-disk fixture bytes for every fixture) must be run manually once on a network-connected developer machine:

```
OPENHOLIDAYS_LIVE=1 go test -tags=integration -run TestUpdateFixtures ./...
```

Expected: every subtest reports OK (drift-detection mode reports no DRIFT) because:
- the writer now emits `<4-space-indent JSON>\n`, and
- every committed fixture in `testdata/` is now `<4-space-indent JSON>\n`.

This run is gated behind a developer's network access by design (the live HTTP path is double-gated via `//go:build integration` and `OPENHOLIDAYS_LIVE=1`). The automated `go test -race -count=1 ./...` continues to pass — no production code was touched.

## Next Phase Readiness

- All three Phase-3 verification gaps (CR-01, CR-02, WR-05) are closed at the source level.
- Six fixtures share one uniform format; no future fixture-refresh `-update` run will silently churn whitespace.
- Phase 05 (CI / lint enforcement) will not flag the `cap` builtin shadow because it is gone.
- The remaining Phase-3 verification gaps from `03-VERIFICATION.md` (`SC2-COMBINED`, `WR-01-RANGE-FIRST-YIELD`) are tracked separately and are out of scope for plan 03-09 per the plan's `gap_ids:` frontmatter.

## Verification gates

All automated gates from the plan's `<verify>` blocks passed:

| Gate | Result |
|------|--------|
| `grep -nE '^\s*cap := cap' update_fixtures_test.go` returns 0 lines | PASS |
| `grep -nE 'for _, cap := range captures' update_fixtures_test.go` returns 0 lines | PASS |
| `grep -nE 'for _, c := range captures' update_fixtures_test.go` returns 1 line | PASS |
| `grep -cE '\bcap\.(path\|query\|fixture\|validate)\b' update_fixtures_test.go` returns 0 | PASS |
| `grep -nF 'json.Indent(&pretty, body, "", "    ")' update_fixtures_test.go` returns 1 line | PASS |
| `grep -nF 'json.Indent(&pretty, body, "", "  ")' update_fixtures_test.go` returns 0 lines | PASS |
| `grep -nF "pretty.WriteByte('\n')" update_fixtures_test.go` returns 1 line | PASS |
| Ordering: `WriteByte` line (231) > `json.Indent` line (230) and < `if !*updateFixtures` line (233) | PASS |
| `type capture struct {` declaration still present (exactly 1 occurrence) | PASS |
| All six fixtures' line 2 starts with 4-space + non-space | PASS |
| All six fixtures' final byte is `0x0a` | PASS |
| `git diff --stat` in testdata/ shows only the two re-indented files | PASS |
| Semantic content of both re-indented fixtures unchanged (SHA-256 of sort_keys JSON dump identical before/after) | PASS |
| `go vet ./...` | PASS |
| `go vet -tags=integration ./...` | PASS |
| `go build ./...` | PASS |
| `go test -race -count=1 ./...` | PASS |

## Self-Check: PASSED

- `update_fixtures_test.go`: FOUND on disk; contains the 4-space `json.Indent` call, the `pretty.WriteByte('\n')` append, and the `for _, c := range captures` rename. `md5sum` post-edit: `d6571ee6f94faedeba948adccee26fc5`.
- `testdata/countries.json`: FOUND on disk; line 2 begins with 4 spaces; final byte is `0a`.
- `testdata/school_holidays_pl_2025.json`: FOUND on disk; line 2 begins with 4 spaces; final byte is `0a`.
- Task 1 commit `8a79659`: FOUND in `git log --oneline` (`fix(03-09): correct fixture-refresh writer output to match on-disk format`).
- Task 2 commit `2157589`: FOUND in `git log --oneline` (`chore(03-09): re-indent two fixtures to 4-space so all six share one format`).

---
*Phase: 03-endpoints-helpers, Plan: 09*
*Completed: 2026-05-27*
