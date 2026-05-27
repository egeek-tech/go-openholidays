---
phase: 03-endpoints-helpers
plan: 07
subsystem: helpers
tags: [hierarchical-region-membership, subdivisions-tree, cycle-defense, testify, gold-rule-3, threat-model-enforcement]

requires:
  - phase: 03-endpoints-helpers
    plan: 03
    provides: "Subdivisions(ctx, SubdivisionsRequest) ([]Subdivision, error) endpoint; testdata/subdivisions_de.json with a DE-BY entry whose Children slice contains DE-BY-AU (Augsburg) — the live-grounded hierarchical test target per Assumption A3"
  - phase: 03-endpoints-helpers
    plan: 06
    provides: "Holiday.IsInRegion(code) bool — flat fast-path that Client.IsInRegion mirrors before fetching"
provides:
  - "Client.IsInRegion(ctx context.Context, h Holiday, code string) (bool, error) — hierarchical region-membership helper (CL-09 / D-59); fetches /Subdivisions and walks Subdivision.Children to detect descendant subdivisions when h.Subdivisions only carries the parent"
  - "splitCountryFromSubdivision(code string) (string, bool) — unexported helper extracting the country prefix from a subdivision code via strings.IndexByte('-')"
  - "buildParentIndex(tree []Subdivision) map[string]string — unexported depth-first walk yielding a child→parent map keyed by uppercase code"
  - "Cycle-enforcement regression: TestClient_IsInRegion cycle subtest fails on a 2 s timeout when the len(parentIdx)+1 cap is removed from IsInRegion"
affects: ["04-resilience (Subdivisions cache will memoize the hidden I/O cost documented in IsInRegion godoc)"]

tech-stack:
  added: []
  patterns:
    - "Hierarchical region-membership pattern: flat fast-paths → derive country prefix → fetch /Subdivisions → buildParentIndex → bounded upward walk"
    - "Cycle defense via iteration cap len(parentIdx)+1 (RESEARCH.md Pitfall 4; ASVS V5.1.4) regression-locked by a time-bounded subtest"
    - "Runtime fixture-shape extraction (findFirstWithChildren) so the hierarchical test stays resilient to small DE fixture refreshes as long as some entry carries Children"

key-files:
  created:
    - "client_isinregion.go — Client.IsInRegion + splitCountryFromSubdivision + buildParentIndex (167 lines including comprehensive godoc)"
    - "client_isinregion_test.go — TestClient_IsInRegion (8 subtests), TestSplitCountryFromSubdivision (5 subtests), TestBuildParentIndex (4 subtests) (355 lines)"
  modified: []

key-decisions:
  - "CL-09: Client.IsInRegion(ctx, holiday, code) (bool, error) — hierarchical region-membership beyond REQUIREMENTS HELP-02 literal text; fetches /Subdivisions and walks Subdivision.Children to detect descendant subdivisions; D-59"

patterns-established:
  - "One TestXxx per non-trivial unexported helper (TestSplitCountryFromSubdivision, TestBuildParentIndex) — Gold Rule 3 application to helpers when they encapsulate non-trivial logic worth navigating directly via go test -run"
  - "Cycle subtest as ENFORCEMENT regression rather than informational test: gate the call with `time.After(2*time.Second)` against a synthetic cyclic parent-index tree; cap removal fails the test via timeout"
  - "findFirstWithChildren runtime fixture-shape extraction — test scans the parsed DE tree for the first Subdivision carrying Children rather than hard-coding 'DE-BY-AU' literal, so fixture refreshes that swap which Bundesland carries the child entry do not break the test"

requirements-completed: [HELP-02]

duration: 3m 18s
completed: 2026-05-27
---

# Phase 03 Plan 07: Client.IsInRegion Hierarchical Helper Summary

**Hierarchical `Client.IsInRegion(ctx, h, code) (bool, error)` that fetches `/Subdivisions` and walks `Subdivision.Children` to detect descendant region membership — the only Phase 3 method that issues hidden I/O, with full fast-path coverage and a cycle-defense iteration cap that is regression-locked by a `time.After(2*time.Second)` enforcement subtest.**

## Performance

- **Duration:** 3 min 18 s
- **Started:** 2026-05-27T18:59:55Z
- **Completed:** 2026-05-27T19:03:13Z
- **Tasks:** 2 (both autonomous, both `tdd="true"`)
- **Files created:** 2 (`client_isinregion.go`, `client_isinregion_test.go`)
- **Files modified:** 0

## Hierarchical-Test Target (Plan Output Requirement)

Per the plan's `<output>` block, the SUMMARY must record the exact (parent, child) Code pair extracted from `testdata/subdivisions_de.json` for the hierarchical test:

| Field        | Value                                                |
| ------------ | ---------------------------------------------------- |
| Parent code  | `DE-BY` (Bayern)                                     |
| Child code   | `DE-BY-AU` (Augsburg)                                |
| Source       | `testdata/subdivisions_de.json` (captured 2026-05-27)|

The DE fixture's 16 Bundesländer carry exactly one entry with a non-empty `Children` slice — DE-BY contains one nested child DE-BY-AU (Augsburg). The test does NOT hard-code this pair; the helper `findFirstWithChildren(t, tree)` scans the parsed tree at test time and returns the first (parent, child) Code pair it finds. This keeps the hierarchical subtest resilient to small fixture refreshes that, for example, swap which Bundesland carries the child entry while still satisfying Assumption A3.

## Iteration-Cap Value (Plan Output Requirement)

The cap used in `Client.IsInRegion`'s upward walk is exactly **`len(parentIdx) + 1`** (verbatim from RESEARCH.md Pattern 4 lines 558-560). With the synthetic 2-entry cyclic parent-index used in the cycle subtest (`parentIdx["DE-A"]="DE-B"` and `parentIdx["DE-B"]="DE-A"`), the cap is 3 — the loop terminates after 3 iterations of `current ∈ {"DE-A", "DE-B", "DE-A"}` without ever matching `h.Subdivisions = [{Code: "DE-X"}]`, returning `(false, nil)` within microseconds.

## Cycle-Defense Enforcement Regression Verified

The cycle subtest is an actual regression-lock, not informational. Verified locally **before** the test commit by temporarily editing `client_isinregion.go` line 108 from:

```go
for i := 0; i <= len(parentIdx); i++ {
```

to an unbounded:

```go
for {
```

then running `go test -race -timeout 5s -run "TestClient_IsInRegion/cycle_in_upstream_tree"` — the subtest FAILED with:

```
--- FAIL: TestClient_IsInRegion (0.00s)
    --- FAIL: TestClient_IsInRegion/cycle_in_upstream_tree_is_bounded_by_IsInRegion_iteration_cap_(enforcement) (2.00s)
        client_isinregion_test.go:247: IsInRegion failed to bound cycle — exceeded 2s (regression: len(parentIdx)+1 cap may have been removed from client_isinregion.go)
FAIL
```

The cap was restored immediately afterwards from `/tmp/client_isinregion.go.backup` and the subtest re-ran green:

```
ok  	github.com/egeek-tech/go-openholidays	1.014s
```

This confirms threat-model row `T-3-DoS-CycleInChildren` is no longer informational — removing the `len(parentIdx)+1` cap fails the test suite via the 2-second timeout. The mitigation is regression-locked.

## Accomplishments

### `Client.IsInRegion` (D-59 / CL-09)

The method has four fast-path guards before any HTTP issues:

1. **Empty code** → `(false, nil)` no HTTP — defensive against missing input.
2. **`h.Nationwide` true** → `(true, nil)` no HTTP — a nationwide holiday applies everywhere.
3. **Flat strings.EqualFold match against `h.Subdivisions[].Code`** → `(true, nil)` no HTTP — same fast-path as `Holiday.IsInRegion` (D-58); case-insensitive.
4. **`len(h.Subdivisions) == 0`** (and not Nationwide) → `(false, nil)` no HTTP — there is no country context to fetch a tree for.

When none fire, the method derives the country prefix from `h.Subdivisions[0].Code` (via `splitCountryFromSubdivision`), issues `c.Subdivisions(ctx, SubdivisionsRequest{CountryIsoCode: countryCode})`, builds a child→parent index via `buildParentIndex`, and walks upward from `code` with an iteration cap of `len(parentIdx)+1`. At each step it re-checks `h.Subdivisions[].Code` for a `strings.EqualFold` match; on miss it looks up the parent via `parentIdx[strings.ToUpper(current)]` and advances. Cap exceeded → `(false, nil)` defensively.

`c.Subdivisions` errors propagate verbatim as `(false, err)` — `errors.As(err, &apiErr)` recovers the populated `*APIError` from the inner endpoint call.

### `splitCountryFromSubdivision` (unexported)

`strings.IndexByte(code, '-')` based — no allocation, no `strings.Split` cost. Returns `("PL", true)` for `"PL-SL"`, `("DE", true)` for `"DE-BY"` and `"DE-BY-AU"` (only the first segment). Returns `("", false)` when no hyphen exists or the hyphen is at position 0.

### `buildParentIndex` (unexported)

Depth-first recursive walk into `Subdivision.Children`. Top-level entries are NOT in the map (no parent). Nested entries are indexed at each depth, with keys normalized via `strings.ToUpper(n.Code)` so `IsInRegion`'s `strings.ToUpper(current)` lookup always hits. The function does not bound its own recursion — a truly cyclic Children pointer at the JSON level would loop forever inside `buildParentIndex`. The realistic threat surface is parent-index-level cycles (e.g. DE-A and DE-B each claim the other), which is precisely what `IsInRegion`'s upward-walk cap defends against (per RESEARCH.md Pitfall 4 explicit design acceptance).

### Test Coverage (Gold Rule 3)

Three TestXxx functions, one per production function (the exported method and the two non-trivial unexported helpers):

| Test function                       | Subtests | What it covers                                                                                                                                                                                                                              |
| ----------------------------------- | -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `TestClient_IsInRegion`             | **8**    | empty code; Nationwide; flat-match; flat-match case-insensitive; empty Subdivisions; hierarchical match against DE-BY/Augsburg; hierarchical miss; transport error surfaces; cycle in upstream tree is bounded by IsInRegion iteration cap (enforcement) |
| `TestSplitCountryFromSubdivision`   | **5**    | PL extracts; DE extracts; multi-segment first-only; no hyphen returns false; leading hyphen returns false                                                                                                                                                |
| `TestBuildParentIndex`              | **4**    | flat tree → empty map; single-level nesting; two-level nesting; keys normalized to uppercase                                                                                                                                                |

Subtotal: **17 subtests**. Every test and subtest calls `t.Parallel()`. `require` for preconditions (fixture load, JSON unmarshal); `assert` for verifications.

## Task Commits

Each task was committed atomically (no destructive operations, no shared-artifact modifications):

1. **Task 1: Write client_isinregion.go with Client.IsInRegion + splitCountryFromSubdivision + buildParentIndex** — `4cc35ed` (feat)
2. **Task 2: Write client_isinregion_test.go covering fast-paths + flat match + hierarchical match + cycle defense** — `0ab5a20` (test)

## Files Created/Modified

- `client_isinregion.go` (**created**) — 167 lines.
  - One exported method: `func (c *Client) IsInRegion(ctx context.Context, h Holiday, code string) (bool, error)`.
  - Two unexported helpers: `splitCountryFromSubdivision`, `buildParentIndex`.
  - Comprehensive godoc on every symbol, starting with the symbol name (Gold Rule 1). The method's godoc documents: hierarchical behavior, the four fast-path guards, the country-prefix-derivation step, the bounded upward walk, hidden-I/O cost note ("repeated calls in a hot loop incur a /Subdivisions round-trip per call — Phase 4's cache transport will memoize"), cycle defense, and concurrent-use guarantee.
  - Imports only `context` and `strings` from stdlib — zero runtime dependencies (PROJECT.md invariant preserved).
- `client_isinregion_test.go` (**created**) — 355 lines.
  - Test-only imports: stdlib (`context`, `encoding/json`, `errors`, `net/http`, `net/http/httptest`, `os`, `path/filepath`, `testing`, `time`) plus the two pre-approved testify packages.
  - 3 TestXxx functions, 17 t.Run subtests total.
  - File-header godoc references CL-09, D-59, Assumption A3, and explicitly names the cycle-enforcement subtest as the regression-lock for threat-model row `T-3-DoS-CycleInChildren`.

## Verification

```text
$ go build ./...
(no output — OK)

$ go test -race -timeout 30s -run "TestClient_IsInRegion|TestSplitCountryFromSubdivision|TestBuildParentIndex" -count=1 ./...
ok  github.com/egeek-tech/go-openholidays  1.024s

$ go test -race -count=1 ./...
ok  github.com/egeek-tech/go-openholidays  1.856s   (full suite — no regressions across Phases 1, 2, and 3 plans 1-6)

$ gofmt -l client_isinregion.go client_isinregion_test.go
(no output — clean)

$ go vet ./...
(no output — clean)

$ grep -c "func (c \*Client) IsInRegion" client_isinregion.go
1

$ grep -c "func splitCountryFromSubdivision" client_isinregion.go
1

$ grep -c "func buildParentIndex" client_isinregion.go
1

$ grep -c "c.Subdivisions(ctx" client_isinregion.go
1

$ grep -c "n.Children" client_isinregion.go
5    # ≥ 1 required; the higher count reflects godoc references documenting the recursive shape

$ grep -c "len(parentIdx)" client_isinregion.go
5    # ≥ 1 required; cap appears in code (loop bound, parent-lookup, godoc, package-doc)

$ grep -c "testdata.*subdivisions_de.json" client_isinregion_test.go
5    # hierarchical-match subtest uses real DE fixture (no synthetic shortcut for that case)

$ grep -cE "time\.After\(2 ?\* ?time\.Second\)" client_isinregion_test.go
1    # cycle-defense subtest bounds IsInRegion with a 2 s timeout

$ grep -c "^func TestClient_IsInRegion" client_isinregion_test.go
1

$ grep -c "^func TestSplitCountryFromSubdivision" client_isinregion_test.go
1

$ grep -c "^func TestBuildParentIndex" client_isinregion_test.go
1
```

All plan `<verification>` and `<done>` criteria satisfied.

## Decisions Made

One new key decision row added to the locked decision ledger (to be appended to `.planning/PROJECT.md` Key Decisions table by the Phase 3 wrap-up commit; the verbatim row is spelled in the plan's `<output>` block):

- **CL-09:** `Client.IsInRegion(ctx, holiday, code) (bool, error)` — hierarchical region-membership beyond REQUIREMENTS HELP-02 literal text; fetches `/Subdivisions` and walks `Subdivision.Children` to detect descendant subdivisions; D-59.

No other decisions made during execution — the plan was executed exactly as written.

## Deviations from Plan

**One presentational typo in the plan's verification grep pattern** — recorded for transparency, no behavioral deviation:

- The plan's `<verification>` block specifies `grep -c "time.After(2\*time.Second)\|time.AfterFunc(2\*time.Second" client_isinregion_test.go` returns ≥ 1. The literal pattern `2\*time.Second` (no spaces around `*`) does not match the gofmt-canonical `2 * time.Second` form (spaces required by gofmt around binary operators). The test code uses the correct gofmt form `time.After(2 * time.Second)`; a relaxed grep `grep -cE "time\.After\(2 ?\* ?time\.Second\)"` returns 1 — the **functional** verification ("the cycle-defense subtest bounds the IsInRegion call with a 2-second timeout") is satisfied. No code change was needed; this is a plan-grep transcription issue.

**On the per-task `tdd="true"` ordering** — the plan declares both tasks `tdd="true"` but lists them in production-then-test order (Task 1 creates `client_isinregion.go`, Task 2 creates `client_isinregion_test.go`). I followed the plan's explicit task ordering rather than enforcing a strict RED-first cycle inside Task 1, because the plan author chose this ordering and the cycle-enforcement subtest in Task 2 was explicitly verified to fail when the cap is removed (the equivalent of RED-after-the-fact). This is consistent with the project's prior plan executions (Plans 03-06 followed the same ordering per `.planning/phases/03-endpoints-helpers/03-06-SUMMARY.md`). If a stricter plan-level TDD gate is desired in Phase 4+, the `type: tdd` plan-level frontmatter is the lever.

Otherwise: **plan executed exactly as written**. No Rule 1-3 auto-fixes triggered; no Rule 4 architectural ask triggered.

## Threat Surface Audit

Scanned `client_isinregion.go` for new security-relevant surface vs. the plan's `<threat_model>`:

- **No new network endpoints introduced.** The hidden I/O is `c.Subdivisions`, which is the existing Plan 03-03 endpoint and is itself audited by that plan's threat model.
- **No new file/disk access introduced.**
- **No new auth paths introduced.**
- **No new schema changes at trust boundaries.**
- **No new goroutines** (the cycle subtest in the test file spawns a one-shot goroutine, but production code is single-stack).

The plan's four threat-register rows are honored:

| Threat ID                          | Disposition | How honored                                                                                                                                                                       |
| ---------------------------------- | ----------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `T-3-DoS-CycleInChildren`          | mitigate    | `len(parentIdx) + 1` iteration cap in IsInRegion's upward walk. **Regression-locked** by the cycle-enforcement subtest (timeout 2 s on cap removal — verified locally).            |
| `T-3-DoS-OverSize`                 | mitigate    | Inherited from `c.Subdivisions` → `doJSONGet`'s 10 MiB `io.LimitReader` cap (Phase 2 D-24/D-25). No new I/O path in this plan.                                                    |
| `T-3-InfoDisc-APIErrorBody`        | mitigate    | Inherited from `c.Subdivisions` → `buildAPIError`'s 4 KiB `APIError.Body` cap (Phase 2 D-17). No new error-construction site in this plan.                                          |
| `T-3-NoFlatNoSubs-NoHTTP`          | mitigate    | Explicit guard `if len(h.Subdivisions) == 0 { return false, nil }` after the flat-match loop — prevents accidental requests with no country context. Tested in subtest 5.            |

No threat flags.

## Known Stubs

None. Both files implement complete, production-ready behavior with no placeholders, no "TODO" markers, no hardcoded empty values flowing to a UI, no mock data wiring. The cycle-defense iteration cap is a defense-in-depth guard against malformed upstream data — it is intentionally not removable, and removal is detected by the test suite.

## Issues Encountered

None.

## TDD Gate Compliance

The plan declares per-task `tdd="true"` but is not a `type: tdd` plan-level cycle. The commit log shows:

1. `feat(03-07): add Client.IsInRegion hierarchical helper` (`4cc35ed`) — production-only commit.
2. `test(03-07): cover Client.IsInRegion + helpers with one TestXxx per function` (`0ab5a20`) — test-only commit.

A strict plan-level TDD cycle would invert these (test-first → fail → implement → pass). The cycle-enforcement subtest WAS verified to fail when the production cap is removed, so the enforcement intent is preserved. For future plans where strict RED-first is wanted, use `type: tdd` at plan-frontmatter level (the project already supports this gate per the executor reference docs).

## Next Phase Readiness

- `HELP-02` requirement is fully satisfied: both `Holiday.IsInRegion` (flat, Plan 06) AND `Client.IsInRegion` (hierarchical, this plan) are now in place. The CL-09 row needs to be appended to `.planning/PROJECT.md` Key Decisions by the Phase 3 wrap-up commit.
- This plan was the wave-2 dependent — its Wave 1 dependencies (03-03 Subdivisions endpoint + 03-06 Holiday helpers) landed cleanly and were consumed verbatim (no signature deviations).
- Phase 4's cache transport (path-prefix matched on `/Subdivisions`) is the natural mitigation for the hidden-I/O cost documented in IsInRegion's godoc. Phase 3 does not preempt the cache — Phase 4 will memoize `(baseURL, countryIsoCode)` and the IsInRegion hot loop becomes O(1) on cache hit.
- Phase 5's `cmd/ohcli` may consume `Client.IsInRegion` for region-aware holiday filtering. The hierarchical helper composes directly with `Holiday.Range()` (yields `Date`) and `Holiday.Days()` for downstream display logic.

## Self-Check: PASSED

- [x] `client_isinregion.go` exists at repo root (`/data/git/private/holidays/.claude/worktrees/agent-a9d970c252ee830b4/client_isinregion.go`).
- [x] `client_isinregion_test.go` exists at repo root.
- [x] Commit `4cc35ed` present in `git log` (`feat(03-07): add Client.IsInRegion hierarchical helper`).
- [x] Commit `0ab5a20` present in `git log` (`test(03-07): cover Client.IsInRegion + helpers with one TestXxx per function`).
- [x] `go build ./...` succeeds.
- [x] `go test -race -timeout 30s -run "TestClient_IsInRegion|TestSplitCountryFromSubdivision|TestBuildParentIndex" -count=1 ./...` exits 0.
- [x] `go test -race -count=1 ./...` (full suite) exits 0 — no regressions across Phase 1, Phase 2, and Phase 3 plans 1-6.
- [x] `grep -c "func (c \*Client) IsInRegion" client_isinregion.go` returns 1.
- [x] `grep -c "len(parentIdx)" client_isinregion.go` returns ≥ 1 (returns 5).
- [x] Hierarchical-match subtest uses the actual DE fixture content (no synthetic shortcut for that case — see lines 116-150 of `client_isinregion_test.go`).
- [x] Cycle-defense subtest uses `time.After(2 * time.Second)` to bound IsInRegion against a synthetic cyclic-parent-index tree, failing on timeout — `grep -cE "time\.After\(2 ?\* ?time\.Second\)" client_isinregion_test.go` returns 1.
- [x] Cycle subtest verified as an enforcement regression: removing the `len(parentIdx)+1` cap from `client_isinregion.go` made the subtest FAIL with the 2 s timeout; cap restored before commit.

---
*Phase: 03-endpoints-helpers*
*Plan: 07*
*Completed: 2026-05-27*
