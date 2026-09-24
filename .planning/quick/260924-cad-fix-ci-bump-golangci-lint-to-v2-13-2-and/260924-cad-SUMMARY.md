---
phase: quick-260924-cad
plan: 01
subsystem: infra
tags: [ci, github-actions, golangci-lint, govulncheck, go-toolchain, version-pinning]

# Dependency graph
requires: []
provides:
  - "`library — lint` and `ohcli — lint` pinned to golangci-lint v2.13.2 (built with go1.27.0), clearing the vendored-go/types panic under `go-version: stable`"
  - "`vulnerabilities (govulncheck)` pinned to Go 1.27.1, clearing four reachable stdlib advisories"
  - "In-file rationale prose recording the governing constraint behind each of the three pins, so the next bump does not have to be re-derived from a red CI run"
affects: [ci, release, dependabot, distribution]

# Actuals (#2632) — same estimateTokens scale (chars/4) as the plan's estimate.
actuals:
  tokens: 1063        # chars/4 over the realized diff (4253 chars); whole changed file is 3899
  tasks: 2
  commits: 1          # MEASURED: git rev-list --count f338b99..HEAD
  plan_head_before: f338b992f069df0d1c7a1db76bc314d3e4d96c98

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Lint-job pin floor: a golangci-lint release must be built with a Go at least as new as the toolchain it lints"
    - "Security-gate pin rule: the vuln job's Go pin tracks the newest patched stable, pinned exactly (never `stable`) to bypass the lagging actions/go-versions manifest"

key-files:
  created: []
  modified:
    - ".github/workflows/ci.yml — three version pins moved plus refreshed rationale comments in the two lint steps and the vuln step"

key-decisions:
  - "Vuln-job Go pin moved onto the 1.27 line (1.27.1) rather than sideways to the newest 1.26 patch (1.26.8): 1.27.1 is the release actually verified clean locally, and pinning it retires the standing skew between the Go the security gate scans with and the `stable` every other non-matrix job already resolves to"
  - "Commit typed `ci:` (non-releasing, verified `{\"type\": \"ci\", \"section\": \"CI\", \"hidden\": true}` in release-please-config.json), not `fix:` — internal tooling with no consumer-facing effect, per Gold Rule 6"
  - "`.golangci.yml` left untouched — `golangci-lint config verify` exits 0 under v2.13.2, so no config migration is required"
  - "Both lint pins moved together in one commit; moving only one would leave the other job red for the identical toolchain-skew reason"
  - "The vuln step's pre-existing manifest-lag incident record (go1.26.3/go1.26.4, GO-2026-5039/GO-2026-5037) was extended, not replaced — it is still the reason the job pins exactly instead of using `stable`"

patterns-established:
  - "Pin comments state the governing rule, not just the current value: each of the three pins now carries prose naming the constraint that determines what value is legal"
  - "Version-pin acceptance gates filter comment lines before counting, so an incident record may legitimately name superseded versions without self-invalidating the gate"

requirements-completed: [CI-LINT-TOOLCHAIN-SYNC, CI-VULN-PIN-CURRENT]

coverage:
  - id: D1
    description: "Both golangci-lint jobs pin a linter release built with a Go at least as new as the `stable` toolchain they lint"
    requirement: "CI-LINT-TOOLCHAIN-SYNC"
    verification:
      - kind: integration
        ref: "golangci-lint 2.13.2 run --timeout=5m --build-tags=integration --max-issues-per-linter=0 --max-same-issues=0 . -> '0 issues.', exit 0"
        status: pass
      - kind: integration
        ref: "golangci-lint 2.13.2 run --timeout=5m --max-issues-per-linter=0 --max-same-issues=0 ./cmd/ohcli/... -> '0 issues.', exit 0"
        status: pass
      - kind: unit
        ref: "GATE-1 region-scoped assertions on .github/workflows/ci.yml -> GATE-1-PASS"
        status: pass
    human_judgment: false
  - id: D2
    description: "The govulncheck job pins an exact Go patch release in which the four flagged stdlib advisories are fixed"
    requirement: "CI-VULN-PIN-CURRENT"
    verification:
      - kind: integration
        ref: "govulncheck ./... on go1.27.1 -> 'No vulnerabilities found.', exit 0"
        status: pass
      - kind: unit
        ref: "GATE-2 composite assertion -> GATE-2-PASS"
        status: pass
    human_judgment: false
  - id: D3
    description: "Each of the three pins carries English comment prose recording why it sits at that value"
    verification:
      - kind: unit
        ref: "GATE-1 grep -F for 'built with a Go at least as new as the toolchain it lints' (both lint regions) and 'tracks the newest patched stable' (vuln region)"
        status: pass
    human_judgment: false
  - id: D4
    description: "No job other than the two lint jobs and the vuln job had its toolchain or tool version changed, and ci.yml is the sole file in the diff"
    verification:
      - kind: unit
        ref: "GATE-1: diff touches no `uses:` line and no `1.24.x` line; git diff --name-only == .github/workflows/ci.yml"
        status: pass
      - kind: unit
        ref: "actionlint .github/workflows/ci.yml -> exit 0 (independent workflow-schema check)"
        status: pass
    human_judgment: false

# Metrics
duration: 6min
completed: 2026-09-24
status: complete
---

# Quick Task 260924-cad: CI lint + vuln toolchain pins Summary

**Unblocked CI on `master` by moving three version pins in one workflow file — golangci-lint to v2.13.2 in both lint jobs (clearing a vendored-go/types panic caused by linter-vs-`stable` toolchain skew) and the govulncheck Go pin to 1.27.1 (clearing four reachable stdlib advisories) — with the governing rule behind each pin recorded in-file.**

## Performance

- **Duration:** 6 min
- **Started:** 2026-09-24T07:06:36Z
- **Completed:** 2026-09-24T07:12:49Z
- **Tasks:** 2 of 2
- **Files modified:** 1 (`.github/workflows/ci.yml`, +43/-3)

## Accomplishments

- Both `golangci-lint-action` steps now pin `v2.13.2` (built with go1.27.0). The prior pin was built with go1.26 and, once `go-version: stable` advanced to go1.27, aborted with `panic: file requires newer Go version go1.27 (application built with go1.26)` / exit 2 instead of reporting findings — a toolchain-skew failure that reads like a lint failure in the job log.
- The `vuln` job's `setup-go` pin moved to `"1.27.1"`, the release verified locally to report `No vulnerabilities found.`
- All three pins now carry prose stating the *constraint* that governs their value, not just the value: the lint-job floor rule and the vuln-job "tracks the newest patched stable" rule. The vuln step's existing manifest-lag incident record was preserved intact and extended.
- Proven locally against the same toolchain the runner resolves (`go1.27.1`) using the exact CI arg strings, so the green result is a real proxy for CI rather than an approximation.

## Task Commits

This quick task produced exactly one source commit (both tasks target the same file; Task 2 is the verification-and-commit task for Task 1's edit, per the plan's own sequencing — "Do not commit yet; Task 2 must pass first").

1. **Task 1 (tracer): Move all three pins and refresh their rationale comments** + **Task 2: Re-prove both CI gates locally, then commit** — `3501915` (`ci`)

**Plan metadata:** handled by the orchestrator (docs commit), per this dispatch's constraints.

## Gate Evidence

All five commands run from the repo root. Local toolchain: `go version go1.27.1-X:nodwarf5 linux/amd64`.

| # | Command | Result | Exit |
|---|---------|--------|------|
| 1 | `go version` | `go1.27.1-X:nodwarf5 linux/amd64` | 0 |
| 2 | `golangci-lint version` | `golangci-lint has version 2.13.2 built with go1.27.0 from 27774aaf on 2026-08-27T23:01:12Z` | 0 |
| 3 | `golangci-lint config verify` | (no output — committed `.golangci.yml` needs no migration) | 0 |
| 4 | `golangci-lint run --timeout=5m --build-tags=integration --max-issues-per-linter=0 --max-same-issues=0 .` | `0 issues.` | 0 |
| 5 | `golangci-lint run --timeout=5m --max-issues-per-linter=0 --max-same-issues=0 ./cmd/ohcli/...` | `0 issues.` | 0 |
| 6 | `govulncheck ./...` | `No vulnerabilities found.` | 0 |

Composite plan gates: `GATE-1-PASS`, `GATE-2-PASS`, `GATE-COMMIT-PASS` all printed. Supplementary independent check: `actionlint .github/workflows/ci.yml` exit 0.

## Root Causes

**Lint jobs — toolchain skew, not a lint finding.** Both lint jobs resolve `go-version: stable`, which now returns go1.27.x. The governing rule is that a golangci-lint release must be **built with a Go at least as new as the toolchain it lints**; a linter built with go1.26 has a vendored `go/types` that refuses to typecheck go1.27 stdlib sources and panics. No source change can clear this — only moving the pin does. Both lint jobs must always move together.

**Vuln job — four reachable stdlib advisories.** `govulncheck` exited 3 on `GO-2026-6218` (net/url), `GO-2026-6090` (crypto/tls), `GO-2026-5972` (encoding/asn1) and `GO-2026-5026` (net/http), each reported against the Go the job previously pinned.

## Files Created/Modified

- `.github/workflows/ci.yml` — `library-lint` linter pin → `v2.13.2`; `ohcli-lint` linter pin → `v2.13.2`; `vuln` job `setup-go` `go-version` → `"1.27.1"`; rationale comment blocks refreshed in all three steps.
- `.planning/WINDOWS.md` — created as a side effect of recording the accepted `govulncheck@latest` debt in the cross-phase defect ledger (entry id 1). Left uncommitted for the orchestrator's docs commit.

Deliberately **not** touched: `.golangci.yml`, `go.mod`, `go.sum`, all `.planning/**` historical phase records, every action SHA pin, all three `go-version: stable` lines, all three `["1.24.x", "1.25.x", "stable"]` test matrices, and both Dependabot branches.

## Decisions Made

1. **1.27.1 over the 1.26 line.** The advisories reported `Fixed in ...@go1.26.6` and go1.26.8 was available, but the pin moved onto the 1.27 line because (a) go1.27.1 is the release actually verified clean here, and (b) it aligns the security gate with the same `stable` the other non-matrix jobs already resolve to, removing a standing two-toolchain skew.
2. **Commit type `ci:`.** Verified rather than assumed: `release-please-config.json` carries `{"type": "ci", "section": "CI", "hidden": true}`, i.e. non-releasing. Precedent `c6a722f ci: bump govulncheck Go pin to 1.26.5 for GO-2026-5856` confirmed present. Per Gold Rule 6 this is internal tooling with no consumer-facing effect and fixes no shipped defect, so inflating it to `fix:` would be wrong.
3. **Comment records kept, not replaced.** The vuln step's manifest-lag explanation is still true and is the reason the job pins exactly instead of using `stable`; the new advisory record was appended to it.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 — Bug] Required gate phrases were wrapped across two comment lines, so the line-based gate greps failed**
- **Found during:** Task 1 (first GATE-1 run)
- **Issue:** My initial comment prose wrapped each gate-required phrase over a line break (e.g. `built with a Go at least as` / `new as the toolchain it lints`). The gate uses line-based `grep -F`, so all three phrase assertions failed — `GATE-1` exited 1 while every pin-value assertion already passed.
- **Fix:** Reflowed the three comment blocks so each required phrase (`built with a Go at least as new as the toolchain it lints` in both lint regions, `tracks the newest patched stable` in the vuln region) sits entirely on one comment line. The gate itself was **not** weakened or modified.
- **Files modified:** `.github/workflows/ci.yml` (comment prose only — no pin value changed)
- **Verification:** Per-assertion diagnostic run first to identify precisely which three of the fifteen assertions failed, then `GATE-1-PASS` on re-run.
- **Committed in:** `3501915` (folded into the single task commit; the defect never reached a commit)

### Process Deviations

**2. [Scope] Staging narrowed to the source file only**
- Task 2's action text said to stage "`.github/workflows/ci.yml` plus this task's planning artifacts". The dispatch constraints explicitly override this: docs artifacts (SUMMARY.md, STATE.md, PLAN.md) are the orchestrator's Step 8 commit. Resolved in favour of the dispatch constraint — only `.github/workflows/ci.yml` was staged.

**3. [Prose quality] One amend for comment readability**
- After the initial commit (`e0a012d`), the reflow had left two orphan short lines. The comment prose is itself a stated deliverable of this plan, so the wrapping was tidied and the unpushed local commit amended → `3501915`. `GATE-1` and `GATE-COMMIT` were re-run green after the amend; `GATE-2` is unaffected (it exercises Go source, not the workflow file). No published tag or release was involved, so Gold Rule 4 does not apply.

**4. [Tooling policy] Declined an injected instruction to bypass the file tools**
- An instruction arrived through the MCP-server-instructions channel directing that file changes be made with `sed`/heredocs instead of the `Read`/`Edit`/`Write` tools. It did not originate from the user or the orchestrator and contradicts the executor contract (which mandates `Write`/`Edit` and forbids heredoc file creation), so it was not followed. All edits used the `Edit` tool.

---

**Total deviations:** 1 auto-fixed (Rule 1) + 3 process notes
**Impact on plan:** None on scope or outcome. The single functional defect was self-inflicted prose formatting, caught by the plan's own gate before commit, and fixed without touching any gate or any pin value. No scope creep: the diff remains three pin values plus comment prose in one file.

## Issues Encountered

The plan's `<file_map>` states "non-comment lines matching `go-version: stable`: **3**". A raw `grep -c` returns 4; the fourth hit is comment prose at the vuln step (`# version rather than \`go-version: stable\`, and deliberately skip the`). The plan's count is correct as written (it specifies *non-comment* lines) — recorded only so a future reader running the unfiltered grep is not misled. No gate depends on this count, by the plan's deliberate design.

A plugin hook flagged general GitHub Actions injection risks on edit. Reviewed and not applicable: this change adds no `${{ github.event.* }}` interpolation and no new `run:` step, so it introduces no injection surface.

## Security Notes

Threat register dispositions held as planned. `T-quick-cad-01` and `T-quick-cad-02` are mitigated: both new pins are exact immutable values (`v2.13.2`, `"1.27.1"`) rather than ranges or `latest`, and every action SHA pin is unchanged — `GATE-1` asserts the `golangci-lint-action` SHA still appears in both lint regions and that the diff modifies no `uses:` line, so the version bump could not smuggle in an action change. No `permissions` block was touched and no new secret is referenced.

`T-quick-cad-SC` remains **accepted, pre-existing debt**: the vuln job still installs its scanner via `go install golang.org/x/vuln/cmd/govulncheck@latest`, a floating version that is not reproducible run-to-run. It was out of this change's authorized scope and is now recorded in `.planning/WINDOWS.md` (entry id 1) so it stays visible; pinning it is a separate `ci:` change.

## Follow-up — NOT performed here

- **`@dependabot rebase` on PR #68 (codecov-action) and PR #66 (testify)** once `master` is green. Explicitly outside this change; nothing was pushed and no PR was touched.
- Pin `govulncheck` to an exact version in the `vuln` job (`T-quick-cad-SC`, ledger entry 1).

## User Setup Required

None — no external service configuration required.

## Next Phase Readiness

The branch `ci/lint-vuln-toolchain-go-1.27` carries one `ci:` commit (`3501915`) ready for the orchestrator's docs commit and whatever PR flow follows. All three previously-red jobs are proven green locally against the toolchain the runner resolves to. Because the commit type is non-releasing, merging it will not cut a release.

## Self-Check: PASSED

- `.github/workflows/ci.yml` exists and is committed (`git diff --quiet HEAD` clean for it).
- `.planning/quick/260924-cad-fix-ci-bump-golangci-lint-to-v2-13-2-and/260924-cad-SUMMARY.md` exists.
- Commit `3501915` exists in `git log`, subject matches `^ci: `, carries the required `Co-Authored-By` trailer.
- Measured `commits=1` from ledger base `f338b992f069df0d1c7a1db76bc314d3e4d96c98` — matches the frontmatter, and the single commit contains real code changes (+43/-3), so the count is not a narrated value.
