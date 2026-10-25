---
phase: quick-261025-vbj
plan: 01
subsystem: infra
tags: [ci, github-actions, golangci-lint, govulncheck, go-toolchain, export-data, version-pinning]

# Dependency graph
requires:
  - phase: quick-260924-cad
    provides: "the floor-1 pin rationale (built-with-Go floor) in both lint steps and the vuln-job manifest-lag record that this task extends"
provides:
  - "`library — lint` and `ohcli — lint` pinned to golangci-lint v2.15.0 in lockstep (built with go1.27.0, vendors golang.org/x/tools v0.52.0), clearing the export-data V5 typecheck failure under `go-version: stable` (go1.27.3)"
  - '`vulnerabilities (govulncheck)` pinned to Go "1.27.3", clearing nine reachable stdlib advisories (go1.27.2 alone still flags GO-2026-6809)'
  - "Two-floor pin record plus check recipe in the library-lint comment, short lockstep cross-reference in the ohcli-lint comment, dated 2026-10-25 paragraph appended to the vuln record (earlier 26-line record byte-identical)"
affects: [ci, dependabot, release]

# Actuals (#2632) — same estimateTokens scale (chars/4) as the plan's estimate.
actuals:
  tokens: 1681        # chars/4 over the realized diff vs 451900a (6726 bytes); the whole changed file is 4621
  tasks: 3
  commits: 1          # MEASURED: git rev-list --count 3b2fe92..HEAD (ledger) = 1; git rev-list --count 451900a..HEAD -- .github/workflows/ci.yml = 1

# Commit-claim reconciliation (#3968, #4670) — measured from the on-disk plan ledger
commits: 1
plan_head_before: 3b2fe92d16e42f39c72a81d98c601a5e95c9d982
plan_head_after: 8a67031fc628ff74ffb2c2027cae54b4bb8041b4

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Lint-job pin has two floors: (1) the golangci-lint release is built with a Go at least as new as the linted toolchain; (2) its vendored golang.org/x/tools export-data reader decodes the export-data version the linted compiler writes — a Go PATCH release can raise it"
    - "Lint-pin check recipe: `golangci-lint version` (floor 1), `go version -m <binary>` filtered for golang.org/x/tools (floor 2), then a run against the toolchain `stable` resolves to"

key-files:
  created: []
  modified:
    - '.github/workflows/ci.yml — two golangci-lint pins v2.13.2 -> v2.15.0 (lockstep), vuln-job Go pin "1.27.1" -> "1.27.3", refreshed rationale comments for all three pins'

key-decisions:
  - "golangci-lint v2.15.0 over v2.14.0: both are built with go1.27.0 and both clear floor 2 (x/tools v0.52.0 and v0.50.0) with 0 issues on both scopes against go1.27.1/2/3; v2.15.0 is the newest release (GitHub releases/latest = v2.15.0, published 2026-10-16)"
  - "Vuln-job Go pin 1.27.3, not 1.27.2: go1.27.2 clears eight of the nine advisories but still exits 3 on GO-2026-6809 (Fixed in: crypto/x509@go1.27.3); go1.27.3 is the newest patched stable and reports No vulnerabilities found."
  - "Both lint jobs moved in lockstep; golangci-lint-action stays at ba0d7d2ec06a0ea1cb5fa41b2e4a3ab91d21278a, which the v9.3.0 annotated tag dereferences to (v9.3.0 is the latest action release)"
  - 'Commit typed `ci:` (non-releasing: release-please-config.json line 23 is `{ "type": "ci", "section": "CI", "hidden": true }`) per Gold Rule 6 — internal tooling, no consumer-facing effect; never `docs:` (releasing here) or `fix:`'
  - "`.golangci.yml` left untouched — `golangci-lint config verify` exits 0 under v2.15.0 on every toolchain tested"

patterns-established:
  - "An incident record states which floor a failure violated and what the failure looked like in the job log, because both floors fail as toolchain skew wearing a lint failure's clothes (floor 1: panic, exit 2; floor 2: typecheck issues, exit 1)"

requirements-completed: [CI-LINT-TOOLCHAIN-SYNC, CI-VULN-PIN-CURRENT]

coverage:
  - id: D1
    description: "Both golangci-lint jobs pin v2.15.0, a release that clears both floors for the go1.27.3 toolchain `stable` resolves to"
    requirement: "CI-LINT-TOOLCHAIN-SYNC"
    verification:
      - kind: integration
        ref: "official go1.27.3, golangci-lint 2.15.0 run --timeout=5m --build-tags=integration --max-issues-per-linter=0 --max-same-issues=0 . -> '0 issues.', exit 0"
        status: pass
      - kind: integration
        ref: "official go1.27.3, golangci-lint 2.15.0 run --timeout=5m --max-issues-per-linter=0 --max-same-issues=0 ./cmd/ohcli/... -> '0 issues.', exit 0"
        status: pass
      - kind: integration
        ref: "negative control: official go1.27.3, golangci-lint 2.13.2 (pin read from 451900a), library args -> exit 1, 'export data version 5 is greater than maximum supported version 4'"
        status: pass
      - kind: other
        ref: "GATE-T1 (twice, incl. tracer feedback gate), GATE-T2, GATE-T3-TOOLS (pre- and post-commit) -> PASS"
        status: pass
    human_judgment: false
  - id: D2
    description: 'The vuln job pins exactly "1.27.3", the newest patched stable, on which govulncheck finds no vulnerabilities'
    requirement: "CI-VULN-PIN-CURRENT"
    verification:
      - kind: integration
        ref: "official go1.27.3 (govulncheck -version -> 'Go: go1.27.3'), govulncheck v1.9.0 ./... -> 'No vulnerabilities found.', exit 0"
        status: pass
      - kind: integration
        ref: "negative control: official go1.27.2, govulncheck ./... -> exit 3, GO-2026-6809, 'Fixed in: crypto/x509@go1.27.3'"
        status: pass
    human_judgment: false
  - id: D3
    description: "Each pin's comment records the constraint that governs its value (library-lint long form, ohcli-lint short lockstep cross-reference, vuln dated 2026-10-25 paragraph with the earlier record intact)"
    verification:
      - kind: other
        ref: "GATE-T1 / GATE-T2 literal-on-one-comment-line checks, stale-sentence rejection, 26-line contiguous vuln-record check -> PASS"
        status: pass
    human_judgment: true
    rationale: "The gates prove the required tokens are present and the old record is intact; whether the prose reads clearly is reviewer judgment"
  - id: D4
    description: "Exactly one `ci:` commit touching only .github/workflows/ci.yml; the non-comment diff vs 451900a is exactly the six pin lines; nothing pushed"
    verification:
      - kind: other
        ref: "GATE-T3-STATIC (pre- and post-commit) and GATE-T3-COMMIT -> PASS"
        status: pass
    human_judgment: false

# Metrics
duration: 12min
completed: 2026-10-25
status: complete
---

# Phase quick-261025-vbj Plan 01: CI toolchain pin bump (golangci-lint v2.15.0, govulncheck Go 1.27.3) Summary

**Both lint jobs now pin golangci-lint v2.15.0, whose vendored golang.org/x/tools v0.52.0 can read the pkgbits.V5 export data that go1.27.2 and later compilers write. The vuln job now pins Go "1.27.3", which clears all nine reachable stdlib advisories. Each pin's comment now records the constraint that decides its value.**

## Performance

- **Duration:** about 12 min
- **Started:** 2026-10-25T22:09:02Z
- **Completed:** 2026-10-25T22:21:17Z (code commit and all code gates done)
- **Tasks:** 3 of 3
- **Files modified:** 1 (`.github/workflows/ci.yml`, +61/-14)

## Accomplishments

- `library — lint` and `ohcli — lint` move from v2.13.2 to v2.15.0 together. On official go1.27.3 with the exact CI args, both scopes report `0 issues.` and exit 0. The old pin still fails the same run with the export-data V5 error, which shows the gate can tell the two pins apart.
- `vulnerabilities (govulncheck)` moves from "1.27.1" to "1.27.3". On go1.27.3, govulncheck v1.9.0 prints `No vulnerabilities found.` and exits 0. On go1.27.2 it still exits 3 on GO-2026-6809.
- The library-lint comment now records both floors: floor 1 keeps the go1.27 panic record, and floor 2 is dated 2026-10-25 and carries the go1.27.2 pkgbits V4-to-V5 evidence. It also gives the current pin's facts and the check recipe for the next bump. The ohcli-lint comment carries the short two-floor lockstep cross-reference. The vuln record gains a dated paragraph that names all nine advisories, and its earlier 26 lines are unchanged.

## Task Commits

1. **Task 1 (tracer): library-lint pin + two-floor record** — no commit, per `<commit_contract>`. GATE-T1 passed twice: once as the task verify and once as the tracer feedback gate (auto mode).
2. **Task 2: ohcli-lint lockstep + vuln pin 1.27.3** — no commit, per `<commit_contract>`. GATE-T2 passed.
3. **Task 3: full end-state re-proof + single commit** — `8a67031` (`ci: bump golangci-lint pin to v2.15.0 and govulncheck Go pin to 1.27.3`)

**Plan metadata:** owned by the orchestrator's `chore(quick-261025-vbj): ...` docs commit. This SUMMARY is deliberately left uncommitted.

## Gate Evidence

All runs used official toolchains from `$(go env GOMODCACHE)/golang.org/toolchain@v0.0.1-go1.27.N.linux-amd64/bin`, placed first on PATH with `GOTOOLCHAIN=local`, never the Arch system `go` (`go1.27.3-X:nodwarf5`). Each lint run used a fresh `GOLANGCI_LINT_CACHE`. All six gate scripts were checked byte-identical to the plan's `<automated>` blocks before they ran.

| # | Command | Key output line | Exit |
|---|---------|-----------------|------|
| 1 | Precondition: `git diff --quiet 451900a -- .github/workflows/ci.yml` | (no diff) | 0 |
| 2 | Precondition: go.dev `?mode=json`, first stable | `go1.27.3` | 0 |
| 3 | Precondition: actions/go-versions `versions-manifest.json`, first stable | `1.27.3` | 0 |
| 4 | `sha256sum -c` on the v2.13.2 / v2.14.0 / v2.15.0 tarballs (release `checksums.txt`) | `OK` x3; each extracted binary byte-equal to its tarball member | 0 |
| 5 | `golangci-lint version` (2.15.0 binary) | `golangci-lint has version 2.15.0 built with go1.27.0 from 25c5369a on 2026-10-16T11:03:09Z` | 0 |
| 6 | `go version -m` on binaries, filtered for golang.org/x/tools | v2.13.2 `v0.49.0`; v2.14.0 `v0.50.0`; v2.15.0 `v0.52.0` | 0 |
| 7 | Toolchain source `src/cmd/compile/internal/noder/unified.go:29` | go1.27.1 `const uirVersion = pkgbits.V4`; go1.27.2 and go1.27.3 `pkgbits.V5` (V5 doc: go.dev/issue/81188) | 0 |
| 8 | Toolchain identity | `go version go1.27.3 linux/amd64` (also go1.27.1 and go1.27.2 for the controls) | 0 |
| 9 | GATE-T1 (task verify) | `GATE-T1-PASS` | 0 |
| 10 | GATE-T1 again (tracer feedback gate) | `GATE-T1-PASS`; lib.log `0 issues.`; neg.log `55 issues: / * typecheck: 55`, 11 lines with the V5 error | 0 |
| 11 | GATE-T2 | `GATE-T2-PASS`; ohcli.log `0 issues.`; vuln.log `No vulnerabilities found.`; vuln-neg.log `Vulnerability #1: GO-2026-6809` / `Fixed in: crypto/x509@go1.27.3` (exit 3) | 0 |
| 12 | GATE-T3-STATIC (pre-commit and post-commit) | `GATE-T3-STATIC-PASS`; non-comment diff = exactly the six pin lines | 0 |
| 13 | GATE-T3-TOOLS (pre-commit and post-commit) | `GATE-T3-TOOLS-PASS` | 0 |
| 14 | GATE-T3-COMMIT | `GATE-T3-COMMIT-PASS` | 0 |
| 15 | `actionlint .github/workflows/ci.yml` (v1.7.12, inside every gate) | (clean) | 0 |
| 16 | Extra evidence: lint matrix, 3 releases x go1.27.1/2/3 x both scopes, plus `config verify` | v2.13.2: go1.27.1 `0 issues.` (exit 0); go1.27.2 and go1.27.3 library `55 issues:` and ohcli `5 issues:` (exit 1). v2.14.0 and v2.15.0: `0 issues.` on all six combinations each (exit 0). `config verify` exit 0 in all nine | as stated |
| 17 | Extra evidence: `govulncheck ./...` on go1.27.1 | nine advisories; eight `Fixed in: ...@go1.27.2`, GO-2026-6809 `Fixed in: crypto/x509@go1.27.3` | 3 |
| 18 | Extra evidence: `govulncheck -version` on go1.27.3 | `Go: go1.27.3` / `Scanner: govulncheck@v1.9.0` / `DB updated: 2026-10-21 00:27:29 +0000 UTC` | 0 |
| 19 | Extra evidence: GitHub API (read-only) | golangci-lint releases/latest `v2.15.0` (2026-10-16); golangci-lint-action latest `v9.3.0`, whose tag dereferences to commit `ba0d7d2ec06a0ea1cb5fa41b2e4a3ab91d21278a` | 0 |
| 20 | Extra evidence: `gh pr checks 70` / `gh pr checks 71` (read-only) | on each PR exactly `library — lint`, `ohcli — lint`, `vulnerabilities (govulncheck)` fail; the other 10 checks pass | — |
| 21 | Extra evidence: vuln.go.dev `GO-2026-6809.json` | `published=2026-10-21T00:27:29Z`, after the 2026-10-15 CI run, which listed eight advisories | 0 |

## Root Causes

1. **Lint jobs: a second, previously undocumented floor.** `go-version: stable` resolved to go1.27.2 in the red 2026-10-15 run and resolves to go1.27.3 today. go1.27.2 changed the compiler's unified-IR export-data version from `pkgbits.V4` to `pkgbits.V5` in a patch release (go.dev/issue/81188). v2.13.2 was built with go1.27.0, so it cleared floor 1, but it vendors golang.org/x/tools v0.49.0, whose reader supports at most V4. Stdlib imports therefore failed typecheck with `export data version 5 is greater than maximum supported version 4`, cascading into `undefined: rand` and `type Date has no field or method ...`. golangci-lint exited 1 ("issues found"), so the failure looked like real findings even though it was toolchain skew.
2. **Vuln job: nine reachable stdlib advisories against go1.27.1.** Eight are fixed in go1.27.2: GO-2026-6617, GO-2026-6611, GO-2026-6610 and GO-2026-6603 (net/http/internal/http2); GO-2026-6613 and GO-2026-6605 (net/http); GO-2026-6608 (net/textproto, mime/multipart); and GO-2026-6607 (crypto/tls). GO-2026-6809 (crypto/x509, distrust of compromised intermediate certificates) is fixed only in go1.27.3.

## Files Created/Modified

- `.github/workflows/ci.yml`
  - library-lint, L160+: pin at L245 is `v2.15.0`; the comment has an opening statement, floor 1 (the panic record), floor 2 (dated 2026-10-25, from L211), the current pin's facts, the check recipe, and the unchanged lockstep paragraph.
  - ohcli-lint, L281+: pin at L323 is `v2.15.0`; the comment carries the short two-floor lockstep cross-reference.
  - vuln, L355+: the 2026-10-25 paragraph starts at L392, and the pin at L406 is `"1.27.3"`.

## Decisions Made

- v2.15.0 rather than v2.14.0, 1.27.3 rather than 1.27.2, lockstep pins, the action SHA left unchanged, and the `ci:` type all follow the plan; the evidence is in the frontmatter `key-decisions`.
- Wording choices inside the plan's latitude:
  - The floor-1 lead-in changed from "The governing rule is that a golangci-lint release must be" to "Floor 1: a golangci-lint release must be". The required line `built with a Go at least as new as the toolchain it lints.` and the rest of the panic record are unchanged.
  - A bare `#` separator now sits before the closing lockstep paragraph, whose three lines are unchanged.
- The floor-2 log excerpt quotes the real `could not import os (... internal/cpu ...)` line from the CI log, using the same `(...` elision as the plan, and keeps its real `)))` closing.

## Deviations from Plan

### Auto-fixed Issues

**1. [CLAUDE.md Gold Rule 2 - accuracy] "every stdlib import" narrowed to "stdlib imports"**
- **Found during:** Task 1, while checking the claims before writing the comment.
- **Issue:** The plan's prose asks for "every stdlib import failed typecheck" in library-lint and "reports every stdlib import as (typecheck) issues" in ohcli-lint, and the commit-body bullet says the same. The output does not prove "every":
  - v2.13.2 on go1.27.3 reported 11 distinct `could not import` issues (sync, runtime, os, sync/atomic, errors, go/token, encoding/json, math, net, syscall, flag), each once per import path.
  - Other failed imports, such as math/rand/v2 and time, show up only as cascades (`undefined: rand`, `type Date has no field or method ...`).
- **Fix:** Wrote "stdlib imports failed typecheck" (library-lint), "reports stdlib imports as (typecheck) issues" (ohcli-lint), and the same wording in the commit body. No gate depends on the word "every".
- **Files modified:** `.github/workflows/ci.yml` (comment prose only)
- **Verification:** GATE-T1, GATE-T2 and all GATE-T3 gates pass.
- **Committed in:** `8a67031`

---

**Total deviations:** 1, a wording precision fix required by Gold Rule 2.
**Impact on plan:** None on behavior or scope. The non-comment diff is still exactly the six pin lines.

## Issues Encountered

None. No gate ever failed, so no fix attempts were needed. The plan reserves `.planning/STATE.md`, `.planning/WINDOWS.md` and `ROADMAP.md` for the orchestrator, so they were not touched, and no broken-windows entry was appended. This change adds no stubs, skipped tests or unrun verifies.

## Noticed, not changed

- **Stale header prose in `.github/workflows/ci.yml`:** L7 (`prep → matrix Go 1.23/1.24/stable`) and L13 (`library-test → matrix Go 1.23/1.24/stable`) disagree with the actual matrices `["1.24.x", "1.25.x", "stable"]`. L40 (`We pin to "1.23.x" etc.`) is stale in the same way; it was not listed in the plan and was noticed during execution. All three are out of scope here and suit a future `ci:` comment-only change.
- **`.planning/WINDOWS.md` entry 1 is still open** (T-quick-cad-SC): the vuln job still installs `go install golang.org/x/vuln/cmd/govulncheck@latest`, now at ci.yml L415. The local proof used govulncheck v1.9.0, which `@latest` resolves to today; CI will resolve whatever `@latest` is at run time.

## Security Notes

- T-quick-vbj-01 and T-quick-vbj-02 are mitigated: every pin is an exact version (`v2.15.0`, `"1.27.3"`), and both actions stay SHA-pinned. GATE-T3-STATIC proves no `uses:`, `permissions`, `run:` or `${{ }}` line moved.
- T-quick-vbj-03 is mitigated:
  - The three tarballs pass `sha256sum -c` against their release `checksums.txt`, and each extracted binary is byte-equal to its tarball member.
  - The toolchains came from the go command's checksum-verified module download.
  - Every gate checks tool identity before trusting a result.
- No new trust-boundary surface was added, so there are no threat flags.

## Follow-up — NOT performed here (orchestrator's job)

- Push the branch `ci/lint-vuln-toolchain-go-1.27.3` and open a PR to `master`.
- After it merges, run `@dependabot rebase` on #70 (actions/create-github-app-token 3.2.0 -> 3.3.0) and #71 (goreleaser/goreleaser-action 7.2.3 -> 7.3.0).

## User Setup Required

None. No external service configuration is required.

## Next Phase Readiness

All three gates now pass locally, on the same official toolchains and arg strings CI uses. The runner has not confirmed this yet; that happens once the orchestrator pushes the branch. Both lint pins have to move again, together, the next time `stable` advances past what either floor allows. The check recipe in the library-lint comment tests a candidate against both floors.

## Self-Check: PASSED

- FOUND: `.github/workflows/ci.yml` (418 lines; worktree byte-equal to HEAD)
- FOUND: `.planning/quick/261025-vbj-fix-ci-bump-golangci-lint-to-v2-15-0-and/261025-vbj-SUMMARY.md` (untracked, in no commit)
- FOUND: commit `8a67031fc628ff74ffb2c2027cae54b4bb8041b4`, an ancestor of HEAD, with parent `3b2fe92`
- `GATE-T3-SUMMARY-PASS`. Frontmatter parses as YAML. `gsd-tools uat classify-coverage` reports `errors: []` (D1/D2/D4 auto-pass, D3 goes to a reviewer).
- Measured commit count: `git rev-list --count 3b2fe92..HEAD` = 1 = `commits:`; `git rev-list --count 451900a..HEAD -- .github/workflows/ci.yml` = 1
- Tree outside `.planning/` is clean; nothing pushed.

---
*Phase: quick-261025-vbj*
*Completed: 2026-10-25*
