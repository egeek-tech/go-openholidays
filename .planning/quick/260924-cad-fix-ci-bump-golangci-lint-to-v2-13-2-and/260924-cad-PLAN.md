---
phase: quick-260924-cad
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - .github/workflows/ci.yml
autonomous: true
requirements:
  - CI-LINT-TOOLCHAIN-SYNC
  - CI-VULN-PIN-CURRENT

estimate:
  tokens: 25000
  raw_tokens: 25000
  tasks: 2
  confidence: low

must_haves:
  truths:
    - "Both golangci-lint jobs (`library — lint`, `ohcli — lint`) pin a linter release built with a Go at least as new as the `stable` toolchain they lint, so the linter's vendored go/types can typecheck the stdlib sources it is handed"
    - "The `vulnerabilities (govulncheck)` job pins an exact Go patch release in which the four stdlib advisories the gate currently flags are already fixed"
    - "Each of the three pins carries YAML comment prose recording WHY it sits at that value, so the next person who has to move it knows the governing constraint rather than re-deriving it from a red CI run"
    - "No job other than the two lint jobs and the vuln job has its toolchain or tool version changed"
    - "`.github/workflows/ci.yml` is the only file in the working-tree diff"
  artifacts:
    - path: ".github/workflows/ci.yml"
      provides: "library-lint + ohcli-lint golangci-lint pins at v2.13.2; vuln job Go pin at an exact patched release; refreshed rationale comments above all three"
      contains: "version: v2.13.2"
  key_links:
    - from: ".github/workflows/ci.yml library-lint step `with.version`"
      to: "the golangci-lint release binary the SHA-pinned golangci-lint-action downloads at job time"
      via: "exact immutable release tag (not a range, not `latest`)"
      pattern: "version: v2.13.2"
    - from: ".github/workflows/ci.yml ohcli-lint step `with.version`"
      to: "the golangci-lint release binary the SHA-pinned golangci-lint-action downloads at job time"
      via: "exact immutable release tag (not a range, not `latest`)"
      pattern: "version: v2.13.2"
    - from: ".github/workflows/ci.yml vuln job setup-go `with.go-version`"
      to: "the exact Go toolchain setup-go fetches directly from go.dev, bypassing the lagging actions/go-versions manifest"
      via: "exact quoted patch version"
      pattern: "go-version: \"1.27.1\""
---

<objective>
Unblock CI on `master` by moving three version pins in `.github/workflows/ci.yml`: the golangci-lint release pin in both lint jobs, and the exact Go toolchain pin in the govulncheck job.

Purpose: Two open Dependabot PRs (#68 codecov-action, #66 testify) are blocked by exactly three red jobs — `library — lint`, `ohcli — lint`, `vulnerabilities (govulncheck)`. Neither failure is caused by the dependency bumps; both reproduce on `master` at f338b99, so the fix belongs here rather than in the Dependabot branches. Every test job already passes on both PRs.

Output: An edited `.github/workflows/ci.yml` (three pin values + refreshed rationale comments) committed as a `ci:` change. No source code, no `go.mod`/`go.sum`, no `.golangci.yml`, no `.planning/**` history, no release or tag touched (Gold Rule 4).
</objective>

<execution_context>
@~/.claude/gsd-core/workflows/execute-plan.md
@~/.claude/gsd-core/templates/summary.md
</execution_context>

<context>
@.planning/STATE.md
@CLAUDE.md

<!-- Every fact below was VERIFIED LIVE by the orchestrator this session. Treat as ground truth: do NOT re-investigate, do NOT hedge. Gold Rule 2 is already satisfied by these verifications; the executor's job is to apply the change and re-prove the two gates locally, not to re-diagnose. -->

<verified_facts>

**Root cause 1 — both lint jobs panic (not a lint finding).**
- CI log: `panic: file requires newer Go version go1.27 (application built with go1.26)`, then `golangci-lint exit with code 2`.
- Both lint jobs run `go-version: stable`, which now resolves to go1.27.1. The currently pinned linter release is built with go1.26.2, so its vendored `go/types` refuses to typecheck go1.27 stdlib sources. The governing constraint: **the linter binary must be built with a Go at least as new as the toolchain it lints.**
- Verified locally (this machine's toolchain is `go1.27.1`): the currently pinned linter reproduces the identical panic + exit 2 under the exact CI args. `golangci-lint has version 2.13.2 built with go1.27.0 from 27774aaf on 2026-08-27T23:01:12Z` returns `0 issues`, exit 0, on BOTH scopes under the exact CI args:
  - library: `run --timeout=5m --build-tags=integration --max-issues-per-linter=0 --max-same-issues=0 .`
  - ohcli:   `run --timeout=5m --max-issues-per-linter=0 --max-same-issues=0 ./cmd/ohcli/...`
- `golangci-lint config verify` passes (exit 0) under v2.13.2 against the committed `.golangci.yml`. **No config migration is needed — do not touch `.golangci.yml`.**
- v2.13.2 (released 2026-08-27) is the latest golangci-lint release.

**Root cause 2 — govulncheck exits 3 on four reachable stdlib advisories.**
- Flagged: `GO-2026-6218` (net/url), `GO-2026-6090` (crypto/tls), `GO-2026-5972` (encoding/asn1), `GO-2026-5026` (net/http). All reported as `Found in: ...@go1.26.5` / `Fixed in: ...@go1.26.6`. The job pins that exact 1.26-series patch.
- Verified locally: `govulncheck ./...` on go1.27.1 reports `No vulnerabilities found.`, exit 0.
- Current releases: go1.26.8 is the newest 1.26.x patch; go1.27.1 is the newest stable overall.

**Decision (record this in the SUMMARY).** The vuln pin moves to `1.27.1` rather than to a 1.26.x patch, because: (a) 1.27.1 is the newest stable and is the version actually verified clean locally, and (b) it aligns the security gate with the same `stable` the other three non-matrix jobs already resolve to, removing a standing skew between what the gate scans and what everything else builds with.

**The existing vuln-job comment already prescribes this change.** It explains at length why the job pins an EXACT version instead of `go-version: stable` (both `stable` and the shared SDK cache resolve through the actions/go-versions manifest, which lags go.dev by days), and it closes with "Bump this when a newer patch fixes an advisory the gate flags." This change is that instruction being followed — extend the record, do not delete it.

</verified_facts>

<file_map>
`.github/workflows/ci.yml` is 331 lines. Verified anchors:
- L160 `  library-lint:` — job start. L180 `go-version: stable`. L183-L195 the step's rationale comment block. **L196 `          version: v2.12.x` ← change target 1.**
- L232 `  ohcli-lint:` — job start. L252 `go-version: stable`. L257-L259 the step's rationale comment block. **L261 `          version: v2.12.x` ← change target 2.**
- L293 `  vuln:` — job start. L300-L317 the manifest-lag rationale comment block (mentions the go1.26.3/go1.26.4 + GO-2026-5039/GO-2026-5037 incident — that history stays). **L319 `          go-version: "1.26.x"` ← change target 3.**

Verified baseline counts in the file (used by the acceptance gates):
- non-comment lines matching the superseded linter pin: **2**
- non-comment lines matching the superseded vuln Go pin: **1**
- non-comment lines matching `go-version: stable`: **3** (L180, L252, L284) — this count MUST stay 3; the vuln job is the only job that carries an exact pin.
- matrix jobs pin `["1.24.x", "1.25.x", "stable"]` at L81, L110, L205 — all three MUST stay untouched.
</file_map>

<out_of_scope>
DO NOT touch any of the following:
- `.planning/**` — the other repo-wide grep hits for the superseded versions are historical phase records (05-distribution RESEARCH/PLAN/SUMMARY, an older quick-task summary). They document what was true then and must stay accurate to their own moment.
- `.golangci.yml` — verified to need no migration (`config verify` exit 0 under v2.13.2).
- `go.mod` / `go.sum` — the module declares `go 1.24` and is not implicated in either failure.
- Any Dependabot branch — the follow-up is `@dependabot rebase` on #68 and #66 once `master` is green, handled outside this plan.
- The untracked `.gsd/dispatch-isolation-sentinel.json` — agent runtime scratch.
</out_of_scope>

<commit_contract>
Commit type MUST be `ci:` — internal tooling, zero consumer-facing effect, fixes no defect in shipped behavior.
- `release-please-config.json` L23 marks `{ "type": "ci", ..., "hidden": true }` — non-releasing. Verified, not assumed.
- Precedent for exactly this class of change: `c6a722f ci: bump govulncheck Go pin to 1.26.5 for GO-2026-5856` (no scope in the type).
- Do NOT use `fix:` — `fix` cuts a release in this repo, and inflating a CI-only change into a releasing type is precisely what Gold Rule 6 forbids.

Suggested subject: `ci: bump golangci-lint pin to v2.13.2 and govulncheck Go pin to 1.27.1`

End the commit message with:
`Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>`
</commit_contract>

<gate_design_note>
Every version-pin acceptance gate below filters comment lines (`grep -v '^[[:space:]]*#'`) before counting. This is deliberate and required: the vuln job's comment block is an incident record that legitimately names superseded Go versions, and Task 1 extends that record. An unfiltered whole-file negative grep would be self-invalidating — the executor's own (correct) documentation prose would trip it. Filtering the comments lets the historical narrative stay complete while the gate still proves no *active* pin sits at a superseded value.
</gate_design_note>
</context>

<tasks>

<task type="tracer">
  <name>Task 1: Move all three pins and refresh their rationale comments — one file, end to end</name>
  <files>.github/workflows/ci.yml</files>
  <reversibility rating="reversible">Three scalar values in one CI workflow file; a single `git revert` restores the prior state with no data migration and no consumer impact.</reversibility>
  <action>
Edit `.github/workflows/ci.yml` only. Make exactly three value changes plus their comment refresh.

1. `library-lint` job, the `golangci/golangci-lint-action` step's `with.version` at L196 → `v2.13.2`.
2. `ohcli-lint` job, the `golangci/golangci-lint-action` step's `with.version` at L261 → `v2.13.2`.
3. `vuln` job, the `actions/setup-go` step's `with.go-version` at L319 → `"1.27.1"` (keep the double quotes — the surrounding comment explains that this job pins an exact version on purpose).

Leave the two `golangci-lint-action` SHA pins (`ba0d7d2ec06a0ea1cb5fa41b2e4a3ab91d21278a  # v9.3.0`) and the `actions/setup-go` SHA pin untouched. Leave all three `go-version: stable` lines and all three matrix `["1.24.x", "1.25.x", "stable"]` lines untouched. Do not reflow, reorder, or reindent anything else in the file.

Comment refresh — record the governing constraint, not just the new number:

- In BOTH lint-job step comment blocks (L183-L195 and L257-L259), add a short paragraph recording why the pin has a floor. It MUST contain this phrase verbatim, because the acceptance gate greps for it and because it is the actual rule a future maintainer needs: `built with a Go at least as new as the toolchain it lints`. Say what happens when the rule is violated: these jobs run `go-version: stable`, and a linter built with an older Go panics in its vendored go/types rather than reporting findings — the failure looks like a lint error but is a toolchain-skew error. Note that both lint jobs must move together.
- In the `vuln` job comment block (L300-L317), keep the entire existing manifest-lag explanation and the go1.26.3/go1.26.4 incident record intact — it is why this job pins exactly instead of using `stable`, and it is still true. Append to it: the advisories that forced this move (`GO-2026-6218` net/url, `GO-2026-6090` crypto/tls, `GO-2026-5972` encoding/asn1, `GO-2026-5026` net/http) and the standing rule for the pin. That rule sentence MUST contain this phrase verbatim, because the acceptance gate greps for it: `tracks the newest patched stable`. Also record the deliberate choice of the 1.27 line over a newer 1.26 patch: it is the version verified clean, and it aligns this gate with the `stable` the other non-matrix jobs already resolve to.

All comment prose in English (Gold Rule 1). Comments may name superseded versions where that serves the incident record — the acceptance gates filter comment lines precisely so this stays safe.

Do not commit yet; Task 2 must pass first.
  </action>
  <verify>
    <automated>cd /data/git/private/egeek.tech/go-openholidays && F=.github/workflows/ci.yml && LIB=$(sed -n '/^  library-lint:/,/^  ohcli-test:/p' $F) && OH=$(sed -n '/^  ohcli-lint:/,/^  ohcli-build:/p' $F) && VN=$(sed -n '/^  vuln:/,$p' $F) && python3 -c "import yaml;yaml.safe_load(open('$F'));print('yaml-ok')" && printf '%s' "$LIB" | grep -q 'version: v2\.13\.2' && printf '%s' "$OH" | grep -q 'version: v2\.13\.2' && printf '%s' "$VN" | grep -q 'go-version: "1\.27\.1"' && [ "$(grep -v '^[[:space:]]*#' $F | grep -c 'v2\.12\.2')" -eq 0 ] && [ "$(grep -v '^[[:space:]]*#' $F | grep -c '1\.26\.5')" -eq 0 ] && printf '%s' "$LIB" | grep -q 'go-version: stable' && printf '%s' "$OH" | grep -q 'go-version: stable' && printf '%s' "$LIB" | grep -q 'ba0d7d2ec06a0ea1cb5fa41b2e4a3ab91d21278a' && printf '%s' "$OH" | grep -q 'ba0d7d2ec06a0ea1cb5fa41b2e4a3ab91d21278a' && printf '%s' "$LIB" | grep -F -q 'built with a Go at least as new as the toolchain it lints' && printf '%s' "$OH" | grep -F -q 'built with a Go at least as new as the toolchain it lints' && printf '%s' "$VN" | grep -F -q 'tracks the newest patched stable' && D=$(git diff -U0 -- $F) && [ "$(printf '%s' "$D" | grep -c '^[+-][^+-].*uses:')" -eq 0 ] && [ "$(printf '%s' "$D" | grep -c '^[+-][^+-].*1\.24\.x')" -eq 0 ] && N=$(git diff --name-only) && [ "$N" = ".github/workflows/ci.yml" ] && echo GATE-1-PASS</automated>
  </verify>
  <done>
`GATE-1-PASS` prints. Concretely, proven region by region rather than by whole-file counts (so each gate names WHICH job it is asserting about):

- The file still parses as YAML.
- The `library-lint` region and the `ohcli-lint` region each contain the new linter pin, each still resolve their toolchain the same floating way they did before, each still contain the `golangci-lint-action` SHA, and each contain the toolchain-floor comment phrase.
- The `vuln` region contains the new exact Go pin and carries the pin-rule comment phrase.
- Zero active (non-comment) lines anywhere in the file still carry either superseded pin.
- The diff changes no `uses:` line (so no action SHA pin moved) and no `1.24.x` line (so no test matrix moved). Together with the single-file diff gate, this is what proves no job other than the three in scope had its toolchain touched — there is deliberately no whole-file count gate on the floating-toolchain lines, because the same literal is load-bearing prose in Task 1's instructions and a negative grep on it would be self-invalidating.
- `.github/workflows/ci.yml` is the sole file in the working-tree diff.
  </done>
</task>

<task type="auto">
  <name>Task 2: Re-prove both CI gates locally against the new pins, then commit</name>
  <files>.github/workflows/ci.yml</files>
  <precondition>A golangci-lint v2.13.2 binary is present at `$GCL` below and `go version` reports a go1.27.x toolchain — the local proof is only a valid proxy for CI because this machine's toolchain matches what `go-version: stable` resolves to on the runner. If the scratchpad binary is gone (session-scoped), reinstall the exact release with `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2` and use that path instead; if `go version` is not a 1.27.x, halt and report rather than drawing a conclusion from a mismatched toolchain.</precondition>
  <action>
Do not edit any file in this task. Run the two gates that are red in CI, locally, against the versions Task 1 just pinned, and capture their real output as the evidence for the SUMMARY.

Linter binary (already downloaded this session):
`GCL=/tmp/claude-1000/-data-git-private-egeek-tech-go-openholidays/c4241ea1-1131-4e22-b62e-9db2044ba1e4/scratchpad/gcl/golangci-lint-2.13.2-linux-amd64/golangci-lint`

Run, from the repo root, using the EXACT arg strings the workflow passes (copy them from the `args:` lines, do not paraphrase — a different arg set proves nothing about CI):
- `$GCL version` — confirm it reports 2.13.2 and the Go it was built with.
- `$GCL config verify` — confirms the committed `.golangci.yml` needs no migration.
- `$GCL run --timeout=5m --build-tags=integration --max-issues-per-linter=0 --max-same-issues=0 .` — the `library — lint` job.
- `$GCL run --timeout=5m --max-issues-per-linter=0 --max-same-issues=0 ./cmd/ohcli/...` — the `ohcli — lint` job.
- `govulncheck ./...` (binary at `$(go env GOPATH)/bin/govulncheck`) — the `vulnerabilities (govulncheck)` job.

All five must exit 0. If any does not, STOP and report the actual output — do not widen scope to make it pass, and do not edit source files, `.golangci.yml`, or `go.mod` to appease a finding. A real finding here means the diagnosis needs revisiting, which is a decision for the user, not a fix for this plan.

Once all five are green, commit per the `<commit_contract>` in `<context>`: type `ci:`, English subject naming both pins, the `Co-Authored-By` trailer. Stage only `.github/workflows/ci.yml` plus this task's planning artifacts.

In the SUMMARY, record: the five command exit statuses and their key output lines; the toolchain-skew rule as the root cause of the lint panics; the four advisory IDs as the root cause of the vuln failure; the 1.27-line-over-1.26-patch decision with its two reasons; and the follow-up that is explicitly NOT part of this change — `@dependabot rebase` on #68 and #66 once `master` is green.
  </action>
  <verify>
    <automated>cd /data/git/private/egeek.tech/go-openholidays && GCL=/tmp/claude-1000/-data-git-private-egeek-tech-go-openholidays/c4241ea1-1131-4e22-b62e-9db2044ba1e4/scratchpad/gcl/golangci-lint-2.13.2-linux-amd64/golangci-lint; [ -x "$GCL" ] || GCL="$(go env GOPATH)/bin/golangci-lint"; GOV="$(go env GOPATH)/bin/govulncheck" && GV=$(go version) && printf '%s' "$GV" | grep -q 'go1\.27\.' && LV=$("$GCL" version 2>&1) && printf '%s' "$LV" | grep -F -q 'version 2.13.2' && "$GCL" config verify && "$GCL" run --timeout=5m --build-tags=integration --max-issues-per-linter=0 --max-same-issues=0 . && "$GCL" run --timeout=5m --max-issues-per-linter=0 --max-same-issues=0 ./cmd/ohcli/... && VOUT=$("$GOV" ./... 2>&1) && printf '%s' "$VOUT" | grep -F -q 'No vulnerabilities found' && echo GATE-2-PASS</automated>
    <automated>cd /data/git/private/egeek.tech/go-openholidays && SUBJ=$(git log -1 --pretty=%s) && BODY=$(git log -1 --pretty=%B) && printf '%s' "$SUBJ" | grep -Eq '^ci: ' && printf '%s' "$BODY" | grep -F -q 'Co-Authored-By: Claude Opus 5 (1M context)' && git diff --quiet HEAD -- .github/workflows/ci.yml && echo GATE-COMMIT-PASS</automated>
  </verify>
  <done>
`GATE-2-PASS` and `GATE-COMMIT-PASS` both print. Concretely: the linter binary used for the proof self-reports 2.13.2 (so the proof and the pin are the same version); `config verify` exits 0 (no `.golangci.yml` migration needed); both lint scopes exit 0 with zero issues under the exact CI arg strings; `govulncheck ./...` reports no vulnerabilities; and the change is committed with a `ci:` subject, the required trailer, and no uncommitted residue in the workflow file.
  </done>
</task>

</tasks>

<threat_model>
Security enforcement active — OWASP ASVS level 1, blocking threshold `high`.

## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| GitHub Actions runner → go.dev release CDN | The `vuln` job's `setup-go` step fetches an exact Go toolchain tarball directly from go.dev, deliberately bypassing the actions/go-versions manifest. Untrusted bytes become the toolchain that runs the security gate. |
| GitHub Actions runner → golangci-lint GitHub Releases | The SHA-pinned `golangci-lint-action` downloads the release binary named by `with.version`. Untrusted bytes become the process that lints the repo. |
| GitHub Actions runner → Go module proxy | The `vuln` job's pre-existing `go install golang.org/x/vuln/cmd/govulncheck@latest` step resolves a floating version at job time. Not modified by this change. |

## STRIDE Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-quick-cad-01 | Tampering | `library-lint` / `ohcli-lint` step `with.version` | medium | mitigate | Pin an exact immutable release tag (`v2.13.2`), never a range and never `latest`, so the resolved artifact is deterministic across reruns. The action that performs the download stays SHA-pinned at `ba0d7d2ec06a0ea1cb5fa41b2e4a3ab91d21278a` — Task 1's gate asserts that SHA still appears exactly twice, so the version bump cannot smuggle in an action change. |
| T-quick-cad-02 | Tampering | `vuln` job `setup-go` `with.go-version` | medium | mitigate | Pin an exact quoted patch version (`"1.27.1"`) rather than `stable`, so the security gate's own toolchain is reproducible and auditable from the file. The `setup-go` SHA pin is unchanged. The bumped version is the one verified clean locally, not an aspirational value. |
| T-quick-cad-03 | Denial of Service | `vuln` job availability | low | accept | If a pinned version is later withdrawn or temporarily unavailable, the job hard-fails and blocks merge. Accepted: a security gate that fails closed on an unresolvable toolchain is the correct failure direction, and the failure is loud and immediately diagnosable from the job log. |
| T-quick-cad-SC | Tampering | `go install golang.org/x/vuln/cmd/govulncheck@latest` (pre-existing, L~325) | medium | accept | A floating `@latest` resolves a new version on every run, so the gate's own scanner is not reproducible. Explicitly OUT of this change's authorized scope (only three pin values were authorized) and NOT introduced by it. Recorded here as known accepted debt; pinning it is a separate `ci:` change. Note: the GSD package-legitimacy checkpoint covers npm/pip/cargo installs and does not apply to this Go module install; no `[ASSUMED]`/`[SUS]` package is being added by this plan. |
| T-quick-cad-04 | Elevation of Privilege | job `permissions` blocks | low | accept | Both lint jobs and the `vuln` job declare `permissions: contents: read`. This change does not alter any `permissions` block, so no privilege surface moves. No new secret is referenced. |

No threat is rated `high` or `critical`, so nothing here blocks the change at the configured `security_block_on: high` threshold.
</threat_model>

<verification>
1. `python3 -c "import yaml;yaml.safe_load(open('.github/workflows/ci.yml'))"` — the workflow still parses.
2. Region-scoped positive gates (each names the job it asserts about): the new linter pin is present in the `library-lint` region AND in the `ohcli-lint` region; the new exact Go pin is present in the `vuln` region.
3. Whole-file negative gates on active (non-comment) lines only: zero occurrences of either superseded pin.
4. Untouched invariants: the diff touches no `uses:` line and no `1.24.x` line; the `golangci-lint-action` SHA is still present in both lint regions; both lint regions still resolve their toolchain the same way they did before.
5. Rationale comments present per region: the toolchain-floor phrase in both lint regions, the vuln-pin rule phrase in the `vuln` region.
6. Both lint scopes exit 0 under the exact CI arg strings, using a binary that self-reports 2.13.2, on a local toolchain that self-reports go1.27.x.
7. `golangci-lint config verify` exits 0 — no `.golangci.yml` migration.
8. `govulncheck ./...` reports no vulnerabilities.
9. Scope containment: `.github/workflows/ci.yml` is the only file in the working-tree diff; no `.planning/**` history record, `.golangci.yml`, `go.mod`, or `go.sum` change.
10. Commit subject starts `ci: ` and carries the `Co-Authored-By` trailer.
</verification>

<success_criteria>
- The three pins in `.github/workflows/ci.yml` sit at `v2.13.2` (twice) and `"1.27.1"` (once, in the `vuln` job), with every other job's toolchain untouched.
- Each pin carries English comment prose stating the constraint that governs its value, so the next bump does not require re-deriving the rule from a red CI run.
- All five local gate commands exit 0, proving `library — lint`, `ohcli — lint`, and `vulnerabilities (govulncheck)` will go green on the runner — the local toolchain is go1.27.1, the same release `go-version: stable` resolves to.
- Exactly one commit, type `ci:` (non-releasing per `release-please-config.json`), touching one source file.
- Follow-up recorded but NOT performed: `@dependabot rebase` on #68 and #66 after `master` is green.
</success_criteria>

<output>
Create `.planning/quick/260924-cad-fix-ci-bump-golangci-lint-to-v2-13-2-and/260924-cad-SUMMARY.md` when done.
</output>
