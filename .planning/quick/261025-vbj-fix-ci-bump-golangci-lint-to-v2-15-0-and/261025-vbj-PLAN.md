---
phase: quick-261025-vbj
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
  tokens: 35000
  raw_tokens: 35000
  tasks: 3
  confidence: low

must_haves:
  truths:
    - "Both golangci-lint jobs (`library — lint`, `ohcli — lint`) pin v2.15.0, a release that clears BOTH floors for the toolchain `stable` resolves to (go1.27.3): it is built with go1.27.0, and its vendored golang.org/x/tools v0.52.0 export-data reader decodes the V5 export data that go1.27.2+ compilers write — so both lint scopes report `0 issues.` instead of export-data typecheck errors"
    - "The lint gate discriminates: on the same official go1.27.3 toolchain and the same CI args, the previous pin still fails with `export data version 5 is greater than maximum supported version 4`"
    - "The `vulnerabilities (govulncheck)` job pins exactly 1.27.3, the newest patched stable, on which govulncheck reports `No vulnerabilities found.` — go1.27.1 flags nine reachable stdlib advisories and go1.27.2 still flags GO-2026-6809"
    - "Each pin's comment records the constraint that governs its value: library-lint holds the long-form two-floor record with the go1.27.2 pkgbits V4-to-V5 evidence and the check recipe; ohcli-lint holds the short cross-reference and stays in lockstep; the vuln step gains a dated 2026-10-25 paragraph while its earlier record stays byte-identical"
    - "The only non-comment lines of `.github/workflows/ci.yml` that differ from 451900a are the three pin values — no action SHA, test matrix, `go-version: stable` line, or govulncheck install line moved"
    - "Exactly one `ci:` commit, touching only `.github/workflows/ci.yml`, carries the change; nothing is pushed and no PR or Dependabot branch is touched"
  artifacts:
    - path: ".github/workflows/ci.yml"
      provides: "library-lint + ohcli-lint golangci-lint pins at v2.15.0 (lockstep); vuln job exact Go pin at 1.27.3; refreshed rationale comments for all three pins"
      contains: "version: v2.15.0"
  key_links:
    - from: ".github/workflows/ci.yml library-lint step `with.version`"
      to: "the golangci-lint release binary the SHA-pinned golangci-lint-action downloads, run against the toolchain setup-go resolves for `go-version: stable`"
      via: "exact release version (never a range, never `latest`)"
      pattern: "version: v2.15.0"
    - from: ".github/workflows/ci.yml ohcli-lint step `with.version`"
      to: "the same golangci-lint release binary, run against the same `stable` toolchain"
      via: "exact release version, kept in lockstep with library-lint"
      pattern: "version: v2.15.0"
    - from: ".github/workflows/ci.yml vuln job setup-go `with.go-version`"
      to: "the exact Go toolchain setup-go fetches from go.dev, whose stdlib govulncheck scans for reachable advisories"
      via: "exact quoted patch version"
      pattern: "go-version: \"1.27.3\""
---

<objective>
Unblock the three red required checks — `library — lint`, `ohcli — lint`, `vulnerabilities (govulncheck)` — by moving three version pins in `.github/workflows/ci.yml` and recording, beside each pin, the constraint that governs its value.

Purpose: Dependabot PRs #70 (actions/create-github-app-token 3.2.0 to 3.3.0) and #71 (goreleaser/goreleaser-action 7.2.3 to 7.3.0) are red on exactly those three checks while every test-matrix and build job is green. Both PRs only move action SHAs; the failures are toolchain drift that reproduces on master at 451900a, so the fix lands on master through its own branch `ci/lint-vuln-toolchain-go-1.27.3` (created from origin/master 451900a, already checked out). Push, PR and the Dependabot rebases are the orchestrator's job AFTER this plan — not part of it.

Output: one `ci:` commit touching only `.github/workflows/ci.yml` — both golangci-lint pins to `v2.15.0` in lockstep, the vuln job's exact Go pin to 1.27.3, a long-form two-floor record in the library-lint comment, a short cross-reference in the ohcli-lint comment, and a dated 2026-10-25 paragraph appended to the vuln record — plus an uncommitted SUMMARY for the orchestrator's docs commit.
</objective>

<execution_context>
@~/.claude/gsd-core/workflows/execute-plan.md
@~/.claude/gsd-core/templates/summary.md
</execution_context>

<context>
@CLAUDE.md
@.github/workflows/ci.yml

<!-- Everything in <verified_facts> was verified by running commands on 2026-10-25 (orchestrator) and re-checked by the planner against the live tree, the release binaries and the toolchain sources. It is ground truth: do not re-diagnose. The executor's job is to apply the change and re-prove the END STATE with the gates. -->

<verified_facts>

**Lint jobs — toolchain skew, not lint findings.**
- Both lint jobs use `go-version: stable`, which now resolves to go1.27.3 (planner observed live: the first stable entry of `https://go.dev/dl/?mode=json` is `go1.27.3`; the first stable entry of the actions/go-versions `versions-manifest.json` is `1.27.3`). The red CI run of 2026-10-15 used go1.27.2.
- go1.27.2 raised the compiler's unified-IR export-data version in a PATCH release. In the official toolchain sources, `src/cmd/compile/internal/noder/unified.go` line 29 is `const uirVersion = pkgbits.V4` in go1.27.1 and `const uirVersion = pkgbits.V5` in go1.27.2 and go1.27.3; `src/internal/pkgbits/version.go` documents V5 as encoding "the index of methods to preserve relative order of nongeneric and generic methods (go.dev/issue/81188)".
- Official release binaries (sha256-verified against each release's `checksums.txt`):

  | release | `golangci-lint version` | vendored golang.org/x/tools (`go version -m`) |
  |---------|-------------------------|-----------------------------------------------|
  | v2.13.2 (current pin) | `has version 2.13.2 built with go1.27.0 ... on 2026-08-27` | v0.49.0 |
  | v2.14.0 | `has version 2.14.0 built with go1.27.0 ... on 2026-09-24` | v0.50.0 |
  | v2.15.0 (latest; new pin) | `has version 2.15.0 built with go1.27.0 ... on 2026-10-16` | v0.52.0 |

- Measured matrix — official binaries x official toolchains, exact CI arg strings, fresh `GOLANGCI_LINT_CACHE` per run, `config verify` exit 0 everywhere:
  - v2.13.2: go1.27.1 gives `0 issues.` exit 0; go1.27.2 and go1.27.3 fail on both scopes with `could not import os (... could not load export data: internal error in importing "internal/cpu" (cannot decode "internal/cpu", export data version 5 is greater than maximum supported version 4); please report an issue)) (typecheck)`, cascading into `undefined: rand` and `d.Year undefined (type Date has no field or method Year)`; library scope ends `55 issues: * typecheck: 55`, ohcli scope `5 issues`, exit 1 ("issues found"). It reads like real findings; it is toolchain skew.
  - v2.14.0 and v2.15.0: `0 issues.` exit 0, both scopes, on go1.27.1, go1.27.2 and go1.27.3.
- So the floor the comments document today — a golangci-lint release must be built with a Go at least as new as the toolchain it lints — is necessary but NOT sufficient: v2.13.2 met it and still broke on a patch release. The second floor: the linter's vendored golang.org/x/tools export-data reader must decode the export-data version the linted toolchain's compiler writes. Check a candidate with `go version -m` on its binary (look for the golang.org/x/tools line) and by running it against the toolchain.
- **Decision (orchestrator):** pin BOTH lint jobs to v2.15.0 in lockstep (newest release, verified clean). golangci-lint-action stays at `ba0d7d2ec06a0ea1cb5fa41b2e4a3ab91d21278a  # v9.3.0` (v9.3.0 is the latest action release) — no `uses:` line moves.

**Vuln job — nine reachable stdlib advisories.**
- govulncheck v1.9.0 (golang.org/x/vuln v1.9.0, published 2026-10-11 — what CI's `go install ...@latest` resolves to today) against official go1.27.1: exit 3, nine advisories:
  - fixed in go1.27.2: GO-2026-6617 (HTTP/2 server crash due to HPACK encoder race), GO-2026-6611 (excessive CPU from repeated initial window changes), GO-2026-6610 (HTTP/2 transport accepts malformed framing-related headers), GO-2026-6603 (HTTP/2 server memory exhaustion via Trailer headers) — all net/http/internal/http2; GO-2026-6613 (HTTP/1 server connection desync after 2xx CONNECT response) and GO-2026-6605 (HTTP/1 client connection desync after CONNECT rejection) — net/http; GO-2026-6608 (memory limit bypass parsing MIME headers) — net/textproto, mime/multipart; GO-2026-6607 (malformed ECH outer extension references) — crypto/tls.
  - fixed in go1.27.3: GO-2026-6809 (distrust compromised intermediate certificates) — crypto/x509.
  - The 2026-10-15 CI run reported eight: GO-2026-6809 was not yet published then.
- Against go1.27.2: exit 3, only GO-2026-6809 (`Fixed in: crypto/x509@go1.27.3`). Against go1.27.3: exit 0, `No vulnerabilities found.` — go1.27.2 is not enough.
- **Decision:** the pin goes to 1.27.3. This follows the standing rule already written in the vuln comment ("tracks the newest patched stable"; "Bump this when a newer patch fixes an advisory the gate flags").

**Release type.** `release-please-config.json` line 23 is `{ "type": "ci", "section": "CI", "hidden": true }`, so `ci:` is non-releasing (verified, not assumed). `docs` is a RELEASING type in this repo — never use it in this task.

**Local proof environment.** The system `go` on PATH is an Arch build (`go version go1.27.3-X:nodwarf5 linux/amd64`) — never the linted or scanned toolchain. Official toolchains live under `$(go env GOMODCACHE)/golang.org/toolchain@v0.0.1-go1.27.N.linux-amd64/`. Every gate puts the official `bin/` first on PATH with `GOTOOLCHAIN=local` and asserts its exact identity: `go version` must print `go version go1.27.3 linux/amd64` (no `-X:` suffix), and `govulncheck -version` must print `Go: go1.27.3`. `GOROOT` is unset in this environment, so the official `go` resolves its own GOROOT (verified).

</verified_facts>

<file_map>
`.github/workflows/ci.yml` at 451900a is 371 lines. Anchors are for orientation — locate by content, because Task 1's edit shifts every later line.
- L160 `  library-lint:` job. L180 `go-version: stable` (keep). L183 step `Run golangci-lint (library scope, all build tags, unlimited issues)`. L184-L194 the `--build-tags=integration` and `--max-issues-per-linter=0 --max-same-issues=0` rationale (keep byte-identical). **L195-L210 the pin rationale (Task 1 reworks it).** L211 `uses: golangci/golangci-lint-action@ba0d7d2e...  # v9.3.0` (keep). **L213 the `version:` line (Task 1 sets it to `v2.15.0`).** L214 `args:` (keep).
- L249 `  ohcli-lint:` job. L269 `go-version: stable` (keep). L272 step `Run golangci-lint (ohcli scope, unlimited issues)`. L273-L275 the max-issues cross-reference and the no-`--build-tags` note (keep). **L277-L285 the pin rationale (Task 2 reworks it).** L286 `uses:` (keep). **L288 the `version:` line (Task 2 sets it to `v2.15.0`).** L289 `args:` (keep).
- L320 `  vuln:` job. **L330-L355 the setup-go rationale: the manifest-lag incident record plus the 2026-09-24 paragraph — keep all 26 lines byte-identical and contiguous; Task 2 appends after L355.** L356 `- name: Set up Go`. **L359 the `go-version:` line (Task 2 sets it to `"1.27.3"`, quoted).** L362-L366 the inlined-install comment, L367-L368 `Install govulncheck` (its floating install line is `.planning/WINDOWS.md` entry 1 — keep), L370-L371 `Run govulncheck` (keep).
</file_map>

<gate_baselines>
Every count/match gate below was run against HEAD (451900a) at planning time, and against a scratch clone carrying a simulated correct end state, before it was written into this plan (#4927).
- Non-comment lines, whole file — new lint pin: 0 at HEAD, must become exactly two copies of the expected `version: v2.15.0` line (asserted by exact text equality, not by a line count). New vuln pin: 0 at HEAD, must become exactly the one expected `go-version: "1.27.3"` line. Old lint pin: 2 at HEAD, must become 0. Old vuln pin: 1 at HEAD, must become 0.
- Changed non-comment lines vs 451900a: none at HEAD; must become exactly the six pin lines (the three old pin lines out, the three new pin lines in), asserted as a sorted-multiset equality, so any other non-comment change fails the gate.
- library-lint comment literals that are NEW (0 at HEAD): `export data version 5 is greater than maximum supported version 4`, `golang.org/x/tools`, `v0.49.0`, `v0.52.0`, `pkgbits.V5`, `go1.27.2`, `go version -m`, `v2.15.0`. PRESERVED (1 at HEAD, must stay): `built with a Go at least as new as the toolchain it lints`, `panic: file requires newer Go version go1.27`. The stale present-tense sentence about the old pin's build Go: 1 at HEAD, must become 0.
- ohcli-lint comment: `golang.org/x/tools` 0 at HEAD (new); `built with a Go at least as new as the toolchain it lints` and `lockstep` 1 at HEAD (preserved).
- vuln comment: `2026-10-25`, all nine advisory IDs, `go1.27.2`, `go1.27.3` — 0 each at HEAD (new); the 26-line block L330-L355 — present at HEAD (preserved, checked as one contiguous unit).
- End-to-end gates: the Task 1, Task 2 and Task 3 gates all FAIL at HEAD (the file still pins the old values) and PASS on the simulated end state. The negative control (old pin, official go1.27.3, library args) exited 1 with the export-data error at planning time. `actionlint .github/workflows/ci.yml` (v1.7.12, on PATH) exits 0 at HEAD and must stay 0.
</gate_baselines>

<gate_design>
- **Pins and args are read FROM the edited file**, region by region (an awk job-block extractor keyed on the two-space job keys), then resolved to the matching release binary or official toolchain and executed. The end-to-end gates therefore prove what CI will actually run, not a hard-coded copy of it. Each job's `args:` value is asserted equal to the exact CI string and then passed literally, so no gate depends on shell word-splitting.
- **Every fallible `git` call is captured into a variable first** (`BF=$(git show ...)`, `DF=$(git diff ...)`), so its exit status stops the `&&` chain instead of being swallowed by a pipeline.
- **Comment lines are filtered before counting pins** (`grep -v '^[[:space:]]*#'`), because the incident records legitimately name superseded versions; a whole-file negative grep would be self-invalidating.
- **Phrase gates are line-based.** Every required literal must sit whole on one comment line (the 260924-cad executor lost a gate run to a phrase wrapped across two lines).
- **The old pin is read from 451900a** (`git show`) for the negative control, so the control exercises exactly the value being replaced.
- **The base is the full SHA** `451900a32ea76a4183bc7257cf0a9ba1b46f0a18`, and scope checks exclude `.planning/`, so the gates stay valid whether or not the orchestrator commits this PLAN before dispatch and whether the executor runs on the main tree or in a worktree.
- **Tool locations.** Every gate reads tools from `SP="${VBJ_TOOLS:-/tmp/claude-1000/-data-git-private-egeek-tech-go-openholidays/60bc92a5-6195-410b-aee1-7a5b13689646/scratchpad}"` (outside the checkout): golangci-lint at `$SP/dl/gcl-VER/golangci-lint-VER-linux-amd64/golangci-lint`, govulncheck at `$SP/vuln-bin/govulncheck`. Each gate prints `logs: DIR` — on failure, read the `.log` files in that directory.
</gate_design>

<out_of_scope>
Do NOT change, stage or commit any of these:
- every `uses:` action SHA pin (including golangci-lint-action and setup-go); the three `["1.24.x", "1.25.x", "stable"]` test matrices; every `go-version: stable` line; the `go install golang.org/x/vuln/cmd/govulncheck@latest` line (`.planning/WINDOWS.md` entry 1, `T-quick-cad-SC` — a separate future `ci:` change);
- `.golangci.yml` (`config verify` exits 0 under v2.15.0 — no migration), `go.mod`, `go.sum`, any Go source file, any other workflow;
- `.planning/**` — including `.planning/WINDOWS.md` and `.planning/STATE.md` (the orchestrator owns planning artifacts and commits them as `chore(quick-261025-vbj): ...`);
- the pre-existing stale header prose at ci.yml L7 and L13 ("matrix Go 1.23/1.24/stable", while the matrices are 1.24.x/1.25.x/stable) — noticed at planning time, outside this change; report it in the SUMMARY only;
- git remotes: no push, no PR, no PR comments, no Dependabot branch, no `@dependabot` command.
</out_of_scope>

<source_audit>
Quick task: no ROADMAP phase, REQUIREMENTS IDs, RESEARCH.md or CONTEXT.md exist for it. The orchestrator's verified evidence, decisions and binding rules stand in for them.

| Source | Item | Task | Status |
|--------|------|------|--------|
| GOAL | `library — lint`, `ohcli — lint`, `vulnerabilities (govulncheck)` go green on master | 1, 2, 3 | COVERED |
| REQ CI-LINT-TOOLCHAIN-SYNC | both lint jobs pin a linter that clears both floors for `stable` | 1, 2 | COVERED |
| REQ CI-VULN-PIN-CURRENT | the vuln job pins the exact newest patched stable | 2 | COVERED |
| DECISION | both lint jobs at v2.15.0 in lockstep; golangci-lint-action SHA unchanged | 1, 2, 3 (gate) | COVERED |
| DECISION | vuln pin 1.27.3, not 1.27.2 | 2 | COVERED |
| RULE | replace the stale present-tense sentence; record the x/tools export-data floor with the go1.27.2 V4-to-V5 evidence | 1 | COVERED |
| RULE | ohcli-lint carries the short cross-reference form, in lockstep | 2 | COVERED |
| RULE | vuln: new dated 2026-10-25 paragraph; earlier prose intact | 2 | COVERED |
| RULE | Gold Rule 6: one `ci:` commit, root cause + evidence in the body, Co-Authored-By trailer; never `docs:` | 3 | COVERED |
| RULE | Gold Rules 1 and 2: English everywhere; every written claim backed by evidence or a command run | 1, 2, 3 | COVERED |
| GATE | actionlint; pin greps; v2.15.0 `config verify` + both exact CI scopes on go1.27.3; govulncheck on go1.27.3; diff scope vs 451900a; negative control | 3 (end state), 1 and 2 (per slice) | COVERED |
| EXCLUDED | push, PR, Dependabot, WINDOWS.md, action SHAs, matrices, `stable` lines, `@latest` install, `.golangci.yml`, go.mod/go.sum, Go source | — | out of scope by orchestrator constraint |
</source_audit>

<commit_contract>
- Type `ci:` — internal CI tooling, no consumer-facing effect, fixes no shipped defect (Gold Rule 6; non-releasing per `release-please-config.json` L23). Never `fix:` or `docs:`.
- Subject: `ci: bump golangci-lint pin to v2.15.0 and govulncheck Go pin to 1.27.3` (mirrors precedent 3501915).
- Body: English, ASCII, wrapped near 72 columns, shaped like 3501915's body (read it with `git log -1 --format=%B 3501915`). Content is listed in Task 3.
- The body carries the line `Plan: .planning/quick/261025-vbj-fix-ci-bump-golangci-lint-to-v2-15-0-and/261025-vbj-PLAN.md` — it names the quick-task id that the quick workflow's review-scope resolver greps commit messages for, so code review sees `.github/workflows/ci.yml`.
- Final line, exactly: `Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>`
- Stage `.github/workflows/ci.yml` only. Tasks 1 and 2 do not commit; Task 3 makes the single commit.
</commit_contract>
</context>

<tasks>

<task type="tracer">
  <name>Task 1 (tracer): `library — lint` end to end — pin v2.15.0 plus the two-floor record, proven on official go1.27.3</name>
  <precondition>`.github/workflows/ci.yml` is byte-identical to its 451900a version (`git diff --quiet 451900a32ea76a4183bc7257cf0a9ba1b46f0a18 -- .github/workflows/ci.yml` exits 0) AND `stable` still means go1.27.3 — `curl -fsS 'https://go.dev/dl/?mode=json' | jq -r '[.[] | select(.stable)][0].version'` prints `go1.27.3` and `curl -fsS https://raw.githubusercontent.com/actions/go-versions/main/versions-manifest.json | jq -r '[.[] | select(.stable)][0].version'` prints `1.27.3` (both observed at planning time, 2026-10-25); if a newer stable has shipped, halt and report, because the pins and the local proof would no longer describe what CI runs.</precondition>
  <files>.github/workflows/ci.yml</files>
  <reversibility rating="reversible">One scalar value and comment prose in one CI workflow file; `git revert` restores it with no migration and no consumer impact.</reversibility>
  <action>
Scope: the `library-lint` job of `.github/workflows/ci.yml` only. This is the library half of the orchestrator decision (both lint jobs to v2.15.0 in lockstep); Task 2 moves ohcli-lint.

1. Tooling, once for the whole plan — reuse what exists, never overwrite. With `SP` as defined in `<gate_design>`:
   - golangci-lint `2.15.0` (the new pin) and `2.13.2` (the negative control) must exist at `$SP/dl/gcl-VER/golangci-lint-VER-linux-amd64/golangci-lint`; both were present and sha256-verified at planning time. If either is missing: create the empty directory `$SP/dl/gcl-VER`, download `golangci-lint-VER-linux-amd64.tar.gz` and `golangci-lint-VER-checksums.txt` from `https://github.com/golangci/golangci-lint/releases/download/vVER/` into it, check the tarball's line with `sha256sum -c` (halt on mismatch), then extract the tarball in that directory.
   - Official toolchains go1.27.3 (lint and vuln) and go1.27.2 (vuln negative control in Task 2) must exist under `$(go env GOMODCACHE)/golang.org/toolchain@v0.0.1-goVER.linux-amd64/`; both were present at planning time. If one is missing, `GOTOOLCHAIN=goVER go version` fetches it (the go command checksum-verifies the module).
   - govulncheck must exist at `$SP/vuln-bin/govulncheck` (x/vuln v1.9.0, present at planning time). If missing: `GOBIN="$SP/vuln-bin" go install golang.org/x/vuln/cmd/govulncheck@latest` — the same `@latest` CI uses.
   - If `$SP` is not writable in this session, build the same layout under another writable directory outside the checkout and prefix every verify command in this plan with `export VBJ_TOOLS=THAT_DIR &&`.

2. Pin. In the `Run golangci-lint (library scope, all build tags, unlimited issues)` step, set `with.version` to `v2.15.0`. That `version:` line is the only non-comment line this task changes: the step's `uses:` line (SHA plus `# v9.3.0`), its `args:` line, the job's `go-version: stable`, and every other job stay byte-identical.

3. Comment. In that step's comment block, leave the two opening paragraphs (the `--build-tags=integration` rationale and the `--max-issues-per-linter=0 --max-same-issues=0` rationale) untouched. Rework the pin paragraph after them — from "The `version:` pin below has a floor, not just a value." through the closing lockstep paragraph — into the long-form record of two floors:
   a. Open by stating that the pin has two floors and a candidate release must clear both.
   b. Floor 1 — keep the existing first-floor prose as the record of the go1.27 panic incident: the rule sentence (keep the comment line that reads exactly `built with a Go at least as new as the toolchain it lints.`), the `go-version: stable` explanation, the three-line panic log excerpt, and the "reads like a lint failure ... only moving the pin does" sentence. Delete the stale present-tense sentence that closes that paragraph today (the one stating which Go the old pin is built with): the pin it describes is going away, and Floor 2 restates that fact in the past tense.
   c. Floor 2 — a new paragraph dated 2026-10-25 that records, in this order: the linter's vendored golang.org/x/tools export-data reader must decode the export-data version that the linted toolchain's compiler writes; a Go PATCH release can raise that version — go1.27.2 changed the compiler's unified-IR export-data version from pkgbits.V4 (go1.27.1) to pkgbits.V5 (go.dev/issue/81188); the previous pin v2.13.2 was built with go1.27.0, so it cleared floor 1, but it vendors golang.org/x/tools v0.49.0, whose reader supports at most V4; from go1.27.2 on, every stdlib import failed typecheck — quote the error as a log excerpt styled like the panic excerpt, with `export data version 5 is greater than maximum supported version 4` whole on one line — cascading into errors such as `undefined: rand`, and golangci-lint exited 1 (issues found) rather than 2, so it read like real findings while being toolchain skew again; floor 1 is therefore necessary but not sufficient.
   d. The current pin: v2.15.0 is built with go1.27.0, vendors golang.org/x/tools v0.52.0, and reports 0 issues on both lint scopes against go1.27.1, go1.27.2 and go1.27.3.
   e. The check recipe for the next bump, so it need not be re-derived from a red CI run: `golangci-lint version` names the Go a release was built with (floor 1); `go version -m` run on the release binary, filtered for golang.org/x/tools, names its vendored reader (floor 2); then run the candidate against the toolchain `stable` resolves to.
   f. Keep the closing paragraph ("Both lint jobs must move together: ...") as it is.

   Style: match the block — eight spaces then `# `, prose wrapped near the existing width, log excerpts indented two extra spaces after the `#`, a bare `        #` line between paragraphs, em-dashes as the block already uses them. The verify reads comment lines one at a time, so each of these must sit whole on a single comment line: `built with a Go at least as new as the toolchain it lints`, `panic: file requires newer Go version go1.27`, `export data version 5 is greater than maximum supported version 4`, `golang.org/x/tools`, `v0.49.0`, `v0.52.0`, `pkgbits.V5`, `go1.27.2`, `go version -m`, `v2.15.0`. Whenever the new prose refers to the old pin's build Go, phrase it in the past tense — the verify rejects the old present-tense sentence anywhere in this job's comments. Every claim must come from `<verified_facts>` (Gold Rule 2); all prose in English (Gold Rule 1).

4. Do not commit — Task 3 makes the single commit after the full end-state gate.
  </action>
  <verify>
    <automated>BASE=451900a32ea76a4183bc7257cf0a9ba1b46f0a18 && SP="${VBJ_TOOLS:-/tmp/claude-1000/-data-git-private-egeek-tech-go-openholidays/60bc92a5-6195-410b-aee1-7a5b13689646/scratchpad}" && F=.github/workflows/ci.yml && TC="$(go env GOMODCACHE)/golang.org/toolchain@v0.0.1-go1.27.3.linux-amd64/bin" && R() { awk -v j="  $1:" '$0==j{f=1;print;next} f&&/^  [a-z][a-z0-9_-]*:[[:space:]]*$/{f=0} f' "$2"; } && NC() { grep -v '^[[:space:]]*#'; } && MISS= && W=$(mktemp -d "$SP/vbj-t1.XXXXXX") && echo "logs: $W" && actionlint "$F" && LIB=$(R library-lint "$F") && V=$(printf '%s\n' "$LIB" | NC | sed -n 's/^[[:space:]]*version: v\([0-9][0-9.]*\)$/\1/p') && [ "$V" = 2.15.0 ] && A=$(printf '%s\n' "$LIB" | NC | sed -n 's/^[[:space:]]*args: //p') && [ "$A" = "--timeout=5m --build-tags=integration --max-issues-per-linter=0 --max-same-issues=0 ." ] && B="$SP/dl/gcl-$V/golangci-lint-$V-linux-amd64/golangci-lint" && "$B" version | grep -qF "has version $V built with go1.27.0" && go version -m "$B" | grep -qE '^[[:space:]]+dep[[:space:]]+golang\.org/x/tools[[:space:]]+v0\.52\.0([[:space:]]|$)' && [ "$(env PATH="$TC:$PATH" GOTOOLCHAIN=local go version)" = "go version go1.27.3 linux/amd64" ] && env PATH="$TC:$PATH" GOTOOLCHAIN=local GOLANGCI_LINT_CACHE="$W/c1" "$B" config verify && env PATH="$TC:$PATH" GOTOOLCHAIN=local GOLANGCI_LINT_CACHE="$W/c1" "$B" run --timeout=5m --build-tags=integration --max-issues-per-linter=0 --max-same-issues=0 . > "$W/lib.log" 2>&1 && grep -qxF '0 issues.' "$W/lib.log" && BF=$(git show "$BASE:$F") && V0=$(printf '%s\n' "$BF" | R library-lint - | NC | sed -n 's/^[[:space:]]*version: v\([0-9][0-9.]*\)$/\1/p') && [ "$V0" = 2.13.2 ] && B0="$SP/dl/gcl-$V0/golangci-lint-$V0-linux-amd64/golangci-lint" && [ -x "$B0" ] && ! env PATH="$TC:$PATH" GOTOOLCHAIN=local GOLANGCI_LINT_CACHE="$W/c0" "$B0" run --timeout=5m --build-tags=integration --max-issues-per-linter=0 --max-same-issues=0 . > "$W/neg.log" 2>&1 && grep -qF 'export data version 5 is greater than maximum supported version 4' "$W/neg.log" && [ "$(printf '%s\n' "$LIB" | grep -cF 'v2.13.2 is built with go1.27.0')" -eq 0 ] && for p in 'built with a Go at least as new as the toolchain it lints' 'panic: file requires newer Go version go1.27' 'export data version 5 is greater than maximum supported version 4' 'golang.org/x/tools' 'v0.49.0' 'v0.52.0' 'pkgbits.V5' 'go1.27.2' 'go version -m' 'v2.15.0'; do printf '%s\n' "$LIB" | grep '^[[:space:]]*#' | grep -qF -- "$p" || { echo "missing from library-lint comments: $p"; MISS=1; }; done && [ -z "$MISS" ] && echo GATE-T1-PASS</automated>
  </verify>
  <done>
`GATE-T1-PASS` prints. Concretely: `actionlint` exits 0; the library-lint job's non-comment `version:` value, read from the edited file, is `2.15.0` and its `args:` line is still the exact CI string; the binary that value resolves to self-reports `2.15.0 built with go1.27.0` and vendors golang.org/x/tools v0.52.0; on the official go1.27.3 toolchain (identity asserted) `config verify` exits 0 and the library scope reports `0 issues.` with exit 0; the old pin, read from 451900a, fails on the same toolchain and args with the export-data V5 error, so the gate discriminates; the stale present-tense sentence is gone from the job's comments; all ten required literals sit on comment lines of the job. Nothing is committed.
  </done>
</task>

<task type="auto">
  <name>Task 2: `ohcli — lint` in lockstep plus `vulnerabilities (govulncheck)` pinned to 1.27.3 — each proven end to end</name>
  <files>.github/workflows/ci.yml</files>
  <reversibility rating="reversible">Two scalar values and comment prose in the same CI file; `git revert` restores them.</reversibility>
  <action>
Scope: the `ohcli-lint` and `vuln` jobs of `.github/workflows/ci.yml`. Task 1's library-lint edit stays as it is.

1. ohcli-lint pin. In the `Run golangci-lint (ohcli scope, unlimited issues)` step, set `with.version` to `v2.15.0`, matching library-lint (lockstep, per the orchestrator decision). The step's `uses:` line, its `args:` line and the job's `go-version: stable` stay byte-identical.

2. ohcli-lint comment. Leave the first paragraph (the cross-reference for `--max-issues-per-linter=0 --max-same-issues=0` and the note on why there is no `--build-tags`) untouched. Rework the pin paragraph — from "The `version:` pin carries the same floor library-lint documents" through "un-bumped one stays red on its own." — into the short cross-reference form. It must say: the pin carries the same two floors library-lint documents at length; floor 1 as one sentence, keeping the comment line that reads exactly `built with a Go at least as new as the toolchain it lints.`; floor 2 as one sentence — its vendored golang.org/x/tools export-data reader must decode the export-data version that toolchain's compiler writes, which a Go patch release can raise (go1.27.2 did); the two failure signatures under `go-version: stable` — missing floor 1 panics inside the vendored go/types (exit 2), missing floor 2 reports every stdlib import as (typecheck) issues (exit 1), both toolchain skew wearing a lint failure's clothes; and keep the instruction to move this pin in lockstep with library-lint's, using the word `lockstep`. Do not repeat the long-form evidence; library-lint holds it. Each of `built with a Go at least as new as the toolchain it lints`, `golang.org/x/tools` and `lockstep` must sit whole on one comment line.

3. vuln pin. In the `vuln` job's `Set up Go` step, set `with.go-version` to `"1.27.3"` — keep the double quotes; this job pins an exact version on purpose, as its comment explains. This is the standing rule ("tracks the newest patched stable") and the orchestrator decision. The step's `uses:` line and `cache: true` stay byte-identical.

4. vuln comment. Leave every existing line of the comment block above that step — from "The govulncheck security gate must scan against a Go stable that" through "already resolves to — one toolchain to reason about, not two." — byte-identical and contiguous; the verify checks those 26 lines as one unit. Append directly after the last of them, separated by a bare `      #` line, ONE new paragraph opening with `2026-10-25:` in the same style. It must record: the bump instruction came due again; against go1.27.1 the gate exited 3 on nine reachable stdlib advisories; eight are fixed in go1.27.2 — GO-2026-6617, GO-2026-6611, GO-2026-6610 and GO-2026-6603 (net/http/internal/http2), GO-2026-6613 and GO-2026-6605 (net/http), GO-2026-6608 (net/textproto, mime/multipart) and GO-2026-6607 (crypto/tls); the ninth, GO-2026-6809 (crypto/x509, distrust of compromised intermediate certificates), is fixed only in go1.27.3, and go1.27.2 still exits 3 on it; so the pin moves to go1.27.3 rather than go1.27.2, and on go1.27.3 `govulncheck ./...` reports "No vulnerabilities found."; this is the standing rule above applied as written. Write the toolchains as `go1.27.2` and `go1.27.3` in the prose, and never split an advisory ID across lines (the verify greps each token on one comment line).

5. Leave the inlined-install comment, the `Install govulncheck` step and the `Run govulncheck` step untouched (`.planning/WINDOWS.md` entry 1 is out of scope). Every claim must come from `<verified_facts>` (Gold Rule 2); English only (Gold Rule 1).

6. Do not commit.
  </action>
  <verify>
    <automated>BASE=451900a32ea76a4183bc7257cf0a9ba1b46f0a18 && SP="${VBJ_TOOLS:-/tmp/claude-1000/-data-git-private-egeek-tech-go-openholidays/60bc92a5-6195-410b-aee1-7a5b13689646/scratchpad}" && F=.github/workflows/ci.yml && MC="$(go env GOMODCACHE)" && TC="$MC/golang.org/toolchain@v0.0.1-go1.27.3.linux-amd64/bin" && R() { awk -v j="  $1:" '$0==j{f=1;print;next} f&&/^  [a-z][a-z0-9_-]*:[[:space:]]*$/{f=0} f' "$2"; } && NC() { grep -v '^[[:space:]]*#'; } && PIN() { NC | sed -n 's/^[[:space:]]*version: v\([0-9][0-9.]*\)$/\1/p'; } && MISS= && W=$(mktemp -d "$SP/vbj-t2.XXXXXX") && echo "logs: $W" && actionlint "$F" && OH=$(R ohcli-lint "$F") && VN=$(R vuln "$F") && V=$(printf '%s\n' "$OH" | PIN) && [ "$V" = 2.15.0 ] && [ "$(R library-lint "$F" | PIN)" = "$V" ] && A=$(printf '%s\n' "$OH" | NC | sed -n 's/^[[:space:]]*args: //p') && [ "$A" = "--timeout=5m --max-issues-per-linter=0 --max-same-issues=0 ./cmd/ohcli/..." ] && B="$SP/dl/gcl-$V/golangci-lint-$V-linux-amd64/golangci-lint" && [ "$(env PATH="$TC:$PATH" GOTOOLCHAIN=local go version)" = "go version go1.27.3 linux/amd64" ] && env PATH="$TC:$PATH" GOTOOLCHAIN=local GOLANGCI_LINT_CACHE="$W/c1" "$B" config verify && env PATH="$TC:$PATH" GOTOOLCHAIN=local GOLANGCI_LINT_CACHE="$W/c1" "$B" run --timeout=5m --max-issues-per-linter=0 --max-same-issues=0 ./cmd/ohcli/... > "$W/ohcli.log" 2>&1 && grep -qxF '0 issues.' "$W/ohcli.log" && G=$(printf '%s\n' "$VN" | NC | sed -n 's/^[[:space:]]*go-version: "\([0-9][0-9.]*\)"$/\1/p') && [ "$G" = 1.27.3 ] && TV="$MC/golang.org/toolchain@v0.0.1-go$G.linux-amd64/bin" && [ "$(env PATH="$TV:$PATH" GOTOOLCHAIN=local go version)" = "go version go$G linux/amd64" ] && GV="$SP/vuln-bin/govulncheck" && env PATH="$TV:$PATH" GOTOOLCHAIN=local "$GV" -version | grep -qxF "Go: go$G" && env PATH="$TV:$PATH" GOTOOLCHAIN=local "$GV" ./... > "$W/vuln.log" 2>&1 && grep -qxF 'No vulnerabilities found.' "$W/vuln.log" && T2="$MC/golang.org/toolchain@v0.0.1-go1.27.2.linux-amd64/bin" && [ "$(env PATH="$T2:$PATH" GOTOOLCHAIN=local go version)" = "go version go1.27.2 linux/amd64" ] && { env PATH="$T2:$PATH" GOTOOLCHAIN=local "$GV" ./... > "$W/vuln-neg.log" 2>&1; [ $? -eq 3 ]; } && grep -qF 'GO-2026-6809' "$W/vuln-neg.log" && grep -qF 'Fixed in: crypto/x509@go1.27.3' "$W/vuln-neg.log" && BF=$(git show "$BASE:$F") && BLK=$(printf '%s\n' "$BF" | sed -n '/^      # The govulncheck security gate must scan against a Go stable that$/,/one toolchain to reason about, not two\.$/p') && [ "$(printf '%s\n' "$BLK" | wc -l)" -eq 26 ] && case "$(cat "$F")" in *"$BLK"*) true ;; *) echo "earlier vuln-step comment prose is not intact"; false ;; esac && for p in 'built with a Go at least as new as the toolchain it lints' 'golang.org/x/tools' 'lockstep'; do printf '%s\n' "$OH" | grep '^[[:space:]]*#' | grep -qF -- "$p" || { echo "missing from ohcli-lint comments: $p"; MISS=1; }; done && for p in '2026-10-25' 'GO-2026-6617' 'GO-2026-6613' 'GO-2026-6611' 'GO-2026-6610' 'GO-2026-6608' 'GO-2026-6607' 'GO-2026-6605' 'GO-2026-6603' 'GO-2026-6809' 'go1.27.2' 'go1.27.3'; do printf '%s\n' "$VN" | grep '^[[:space:]]*#' | grep -qF -- "$p" || { echo "missing from vuln comments: $p"; MISS=1; }; done && [ -z "$MISS" ] && echo GATE-T2-PASS</automated>
  </verify>
  <done>
`GATE-T2-PASS` prints. Concretely: `actionlint` exits 0; the ohcli-lint pin read from the file is `2.15.0` and equals library-lint's (lockstep); its `args:` line is the exact CI string; on official go1.27.3 `config verify` exits 0 and the ohcli scope reports `0 issues.` with exit 0; the vuln job's quoted Go pin read from the file is `1.27.3`, the official toolchain for it self-identifies, govulncheck reports `Go: go1.27.3` and `No vulnerabilities found.` with exit 0; the negative control on official go1.27.2 exits 3 on GO-2026-6809 with `Fixed in: crypto/x509@go1.27.3`, which proves go1.27.2 is not enough; the 26-line earlier vuln record is intact and contiguous; all required literals sit on comment lines of their jobs. Nothing is committed.
  </done>
</task>

<task type="auto">
  <name>Task 3: Re-prove the full end state against official go1.27.3, then make the single `ci:` commit</name>
  <files>.github/workflows/ci.yml</files>
  <action>
Make no further edits unless a gate below fails because of a Task 1 or Task 2 edit.

1. Run the first two verify commands (GATE-T3-STATIC, GATE-T3-TOOLS) on the finished file. Both must print their PASS line. If one fails, fix only the Task 1/Task 2 edits in `.github/workflows/ci.yml` and re-run — never weaken, reorder or skip a gate, and never touch another file. If a linter reports real findings or govulncheck reports a vulnerability on the new pins, STOP and report the actual output: that contradicts `<verified_facts>`, and the decision belongs to the orchestrator, not to this plan.

2. Commit once. `git add .github/workflows/ci.yml` — nothing else (no `.planning/**`, no SUMMARY). Write the message to a file outside the checkout (for example in the scratchpad) and run `git commit -F` on it, per `<commit_contract>`. Body content, in this order, mirroring 3501915:
   - Context: Dependabot PRs #70 (actions/create-github-app-token) and #71 (goreleaser/goreleaser-action) are red on exactly `library - lint`, `ohcli - lint` and `vulnerabilities (govulncheck)`; neither failure is caused by their action bumps; both reproduce on master at 451900a, so the fix belongs here.
   - Lint jobs (v2.13.2 -> v2.15.0): `stable` moved to go1.27.2 and then go1.27.3; go1.27.2 raised the compiler's unified-IR export-data version from pkgbits.V4 to pkgbits.V5 (go.dev/issue/81188); v2.13.2 (built with go1.27.0) still vendors golang.org/x/tools v0.49.0, whose reader stops at V4, so every stdlib import failed typecheck with `export data version 5 is greater than maximum supported version 4` and golangci-lint exited 1 — toolchain skew that reads like findings. The old floor (built with a Go at least as new as the linted toolchain) is necessary but not sufficient; the second floor (the vendored x/tools export-data reader must decode what the linted compiler writes) and the check recipe are now recorded in both step comments. v2.15.0 is built with go1.27.0, vendors x/tools v0.52.0, and is the newest release; v2.14.0 (x/tools v0.50.0) was also verified clean. Both jobs moved in lockstep; golangci-lint-action stays at its SHA (v9.3.0).
   - Vuln job (1.27.1 -> 1.27.3): on go1.27.1 govulncheck exited 3 on nine reachable stdlib advisories; eight are fixed in go1.27.2, GO-2026-6809 (crypto/x509) only in go1.27.3, and go1.27.2 still exits 3 on it, so the pin skips go1.27.2. The 2026-10-15 CI run listed eight because GO-2026-6809 was not yet published. The existing comment already prescribed the bump; its record is kept intact and extended with a dated 2026-10-25 paragraph.
   - Verification: the real outputs from this task's gates, on the official go1.27.3 toolchain with the exact CI arg strings — the toolchain identity line, `golangci-lint version`, `config verify` exit 0, both scopes `0 issues.` exit 0, the negative control (old pin, exit 1, export-data V5 error), govulncheck `No vulnerabilities found.` exit 0, and the go1.27.2 control (exit 3, GO-2026-6809).
   - Scope: no action SHA pin, test matrix, `go-version: stable` line or govulncheck install line moved; no Go source, `.golangci.yml`, `go.mod` or `go.sum` touched. Type `ci` per Gold Rule 6 (internal tooling, no consumer-facing effect, hidden and non-releasing in release-please-config.json).
   - The `Plan:` line from `<commit_contract>`, then the exact `Co-Authored-By` trailer as the final line.
   Do not push, do not open or comment on any PR, do not touch the Dependabot branches. If GATE-T3-COMMIT fails only on message content, fix it with `git commit --amend -F` on this unpushed local commit and re-run the gate.

3. Run the third verify command (GATE-T3-COMMIT) after the commit.

4. Write `.planning/quick/261025-vbj-fix-ci-bump-golangci-lint-to-v2-15-0-and/261025-vbj-SUMMARY.md` from the summary template, with the frontmatter line `status: complete`, `requirements-completed: [CI-LINT-TOOLCHAIN-SYNC, CI-VULN-PIN-CURRENT]`, and `actuals` measured, not narrated (`git rev-list --count 451900a32ea76a4183bc7257cf0a9ba1b46f0a18..HEAD -- .github/workflows/ci.yml` for commits). Record: a gate-evidence table (command, key output line, exit) covering every run above; the two root causes (the two lint floors with the V4-to-V5 evidence; the nine advisories); the decisions (v2.15.0 over v2.14.0; 1.27.3 over 1.27.2; lockstep; action SHA unchanged; `ci:` type with the release-please evidence); any deviations; a "Noticed, not changed" section listing the stale header prose at ci.yml L7/L13 and that `.planning/WINDOWS.md` entry 1 (the floating govulncheck install) is still open; and the follow-ups NOT performed here (push, PR, `@dependabot rebase` on #70 and #71 — the orchestrator's job). Do not stage or commit the SUMMARY, and do not edit `.planning/STATE.md` or `.planning/WINDOWS.md`. Then run the fourth verify command.
  </action>
  <verify>
    <automated>BASE=451900a32ea76a4183bc7257cf0a9ba1b46f0a18 && F=.github/workflows/ci.yml && R() { awk -v j="  $1:" '$0==j{f=1;print;next} f&&/^  [a-z][a-z0-9_-]*:[[:space:]]*$/{f=0} f' "$2"; } && NC() { grep -v '^[[:space:]]*#'; } && PIN() { NC | sed -n 's/^[[:space:]]*version: v\([0-9][0-9.]*\)$/\1/p'; } && actionlint "$F" && [ "$(grep -v '^[[:space:]]*#' "$F" | grep -F 'version: v2.15.0')" = "$(printf '%s\n' '          version: v2.15.0' '          version: v2.15.0')" ] && [ "$(grep -v '^[[:space:]]*#' "$F" | grep -F 'go-version: "1.27.3"')" = '          go-version: "1.27.3"' ] && [ "$(grep -v '^[[:space:]]*#' "$F" | grep -cF 'version: v2.13.2')" -eq 0 ] && [ "$(grep -v '^[[:space:]]*#' "$F" | grep -cF 'go-version: "1.27.1"')" -eq 0 ] && [ "$(R library-lint "$F" | PIN)" = 2.15.0 ] && [ "$(R ohcli-lint "$F" | PIN)" = 2.15.0 ] && [ "$(R vuln "$F" | NC | grep -cxF '          go-version: "1.27.3"')" -eq 1 ] && DF=$(git diff -U0 "$BASE" -- "$F") && CH=$(printf '%s\n' "$DF" | grep -E '^[-+]' | grep -vE '^(---|\+\+\+) ' | grep -vE '^[-+][[:space:]]*(#|$)') && [ "$(printf '%s\n' "$CH" | sort)" = "$(printf '%s\n' '-          version: v2.13.2' '-          version: v2.13.2' '-          go-version: "1.27.1"' '+          version: v2.15.0' '+          version: v2.15.0' '+          go-version: "1.27.3"' | sort)" ] && NF=$(git diff --name-only "$BASE" -- . ':(exclude).planning') && [ "$NF" = "$F" ] && echo GATE-T3-STATIC-PASS</automated>
    <automated>BASE=451900a32ea76a4183bc7257cf0a9ba1b46f0a18 && SP="${VBJ_TOOLS:-/tmp/claude-1000/-data-git-private-egeek-tech-go-openholidays/60bc92a5-6195-410b-aee1-7a5b13689646/scratchpad}" && F=.github/workflows/ci.yml && MC="$(go env GOMODCACHE)" && TC="$MC/golang.org/toolchain@v0.0.1-go1.27.3.linux-amd64/bin" && R() { awk -v j="  $1:" '$0==j{f=1;print;next} f&&/^  [a-z][a-z0-9_-]*:[[:space:]]*$/{f=0} f' "$2"; } && NC() { grep -v '^[[:space:]]*#'; } && PIN() { NC | sed -n 's/^[[:space:]]*version: v\([0-9][0-9.]*\)$/\1/p'; } && ARGS() { NC | sed -n 's/^[[:space:]]*args: //p'; } && W=$(mktemp -d "$SP/vbj-t3.XXXXXX") && echo "logs: $W" && V=$(R library-lint "$F" | PIN) && [ "$V" = 2.15.0 ] && [ "$(R ohcli-lint "$F" | PIN)" = "$V" ] && [ "$(R library-lint "$F" | ARGS)" = "--timeout=5m --build-tags=integration --max-issues-per-linter=0 --max-same-issues=0 ." ] && [ "$(R ohcli-lint "$F" | ARGS)" = "--timeout=5m --max-issues-per-linter=0 --max-same-issues=0 ./cmd/ohcli/..." ] && B="$SP/dl/gcl-$V/golangci-lint-$V-linux-amd64/golangci-lint" && "$B" version | grep -qF "has version $V built with go1.27.0" && go version -m "$B" | grep -qE '^[[:space:]]+dep[[:space:]]+golang\.org/x/tools[[:space:]]+v0\.52\.0([[:space:]]|$)' && [ "$(env PATH="$TC:$PATH" GOTOOLCHAIN=local go version)" = "go version go1.27.3 linux/amd64" ] && env PATH="$TC:$PATH" GOTOOLCHAIN=local GOLANGCI_LINT_CACHE="$W/c1" "$B" config verify && env PATH="$TC:$PATH" GOTOOLCHAIN=local GOLANGCI_LINT_CACHE="$W/c1" "$B" run --timeout=5m --build-tags=integration --max-issues-per-linter=0 --max-same-issues=0 . > "$W/lib.log" 2>&1 && grep -qxF '0 issues.' "$W/lib.log" && env PATH="$TC:$PATH" GOTOOLCHAIN=local GOLANGCI_LINT_CACHE="$W/c1" "$B" run --timeout=5m --max-issues-per-linter=0 --max-same-issues=0 ./cmd/ohcli/... > "$W/ohcli.log" 2>&1 && grep -qxF '0 issues.' "$W/ohcli.log" && BF=$(git show "$BASE:$F") && V0=$(printf '%s\n' "$BF" | R library-lint - | PIN) && [ "$V0" = 2.13.2 ] && B0="$SP/dl/gcl-$V0/golangci-lint-$V0-linux-amd64/golangci-lint" && [ -x "$B0" ] && ! env PATH="$TC:$PATH" GOTOOLCHAIN=local GOLANGCI_LINT_CACHE="$W/c0" "$B0" run --timeout=5m --build-tags=integration --max-issues-per-linter=0 --max-same-issues=0 . > "$W/neg.log" 2>&1 && grep -qF 'export data version 5 is greater than maximum supported version 4' "$W/neg.log" && G=$(R vuln "$F" | NC | sed -n 's/^[[:space:]]*go-version: "\([0-9][0-9.]*\)"$/\1/p') && [ "$G" = 1.27.3 ] && TV="$MC/golang.org/toolchain@v0.0.1-go$G.linux-amd64/bin" && GV="$SP/vuln-bin/govulncheck" && env PATH="$TV:$PATH" GOTOOLCHAIN=local "$GV" -version | grep -qxF "Go: go$G" && env PATH="$TV:$PATH" GOTOOLCHAIN=local "$GV" ./... > "$W/vuln.log" 2>&1 && grep -qxF 'No vulnerabilities found.' "$W/vuln.log" && echo GATE-T3-TOOLS-PASS</automated>
    <automated>BASE=451900a32ea76a4183bc7257cf0a9ba1b46f0a18 && F=.github/workflows/ci.yml && S=$(git log -1 --format=%s) && printf '%s\n' "$S" | grep -qE '^ci: .*v2\.15\.0.*1\.27\.3' && M=$(git log -1 --format=%B) && printf '%s\n' "$M" | grep -qxF 'Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>' && printf '%s\n' "$M" | grep -qF '261025-vbj' && PAR=$(git rev-list --parents -n 1 HEAD) && [ "$(printf '%s\n' "$PAR" | wc -w)" -eq 2 ] && HF=$(git diff-tree --no-commit-id --name-only -r HEAD) && [ "$HF" = "$F" ] && [ "$(git rev-list --count "$BASE"..HEAD -- "$F")" -eq 1 ] && git merge-base --is-ancestor "$BASE" HEAD && LF=$(git log --format= --name-only "$BASE"..HEAD -- . ':(exclude).planning') && [ "$(printf '%s\n' "$LF" | grep . | sort -u)" = "$F" ] && git diff --quiet HEAD -- "$F" && git diff --cached --quiet -- . ':(exclude).planning' && ST=$(git status --porcelain --untracked-files=all -- . ':(exclude).planning') && [ -z "$ST" ] && echo GATE-T3-COMMIT-PASS</automated>
    <automated>BASE=451900a32ea76a4183bc7257cf0a9ba1b46f0a18 && S=.planning/quick/261025-vbj-fix-ci-bump-golangci-lint-to-v2-15-0-and/261025-vbj-SUMMARY.md && test -s "$S" && grep -qx 'status: complete' "$S" && [ -z "$(git log --format=%H "$BASE"..HEAD -- "$S")" ] && echo GATE-T3-SUMMARY-PASS</automated>
  </verify>
  <done>
`GATE-T3-STATIC-PASS`, `GATE-T3-TOOLS-PASS`, `GATE-T3-COMMIT-PASS` and `GATE-T3-SUMMARY-PASS` all print. Concretely: `actionlint` exits 0; exactly two non-comment `version: v2.15.0` lines (one per lint job) and one non-comment `go-version: "1.27.3"` line (in the vuln job), with neither old pin left on any non-comment line; the non-comment diff against 451900a is exactly the six pin lines, so no action SHA, matrix, `stable` line or govulncheck install line moved; `.github/workflows/ci.yml` is the only path outside `.planning/` that differs from 451900a; on official go1.27.3, v2.15.0 passes `config verify` and reports `0 issues.` on both exact CI scopes, the old pin still reproduces the export-data V5 error, and govulncheck reports `No vulnerabilities found.`; HEAD is a single non-merge `ci:` commit touching only the workflow file, carrying the quick-task id and the exact trailer, with a clean tree outside `.planning/`; the SUMMARY exists with `status: complete` and is in no commit.
  </done>
</task>

</tasks>

<threat_model>
Security enforcement active — OWASP ASVS level 1, blocking threshold `high`.

## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| GitHub Actions runner -> golangci-lint GitHub Releases | The SHA-pinned golangci-lint-action downloads the release binary named by `with.version`; those bytes become the process that lints the repo with the job's `contents: read` token. |
| GitHub Actions runner -> go.dev | The vuln job's setup-go fetches the exact Go toolchain named by `with.go-version`; those bytes become the toolchain the security gate scans with. |
| Executor workstation -> GitHub Releases / Go module proxy | The local proof runs downloaded release binaries and toolchains; a tampered or mismatched tool would produce a false green. |
| GitHub Actions runner -> Go module proxy | The pre-existing `go install golang.org/x/vuln/cmd/govulncheck@latest` resolves a floating scanner version at job time. Not modified by this change. |

## STRIDE Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-quick-vbj-01 | Tampering | `library-lint` / `ohcli-lint` step `with.version` | medium | mitigate | Pin an exact release version (`v2.15.0`), never a range or `latest`, so the artifact the action fetches is fixed by name across reruns; the downloading action stays SHA-pinned at `ba0d7d2ec06a0ea1cb5fa41b2e4a3ab91d21278a`. GATE-T3-STATIC proves the non-comment diff is exactly the six pin lines, so the bump cannot smuggle in a `uses:` change. |
| T-quick-vbj-02 | Tampering | `vuln` job setup-go `with.go-version` | medium | mitigate | Pin an exact quoted patch version (`"1.27.3"`) instead of `stable`, so the security gate's own toolchain is reproducible and auditable from the file; the setup-go SHA pin is unchanged; the value is the one verified clean, with go1.27.2 shown insufficient by a negative control. |
| T-quick-vbj-03 | Spoofing | local proof tooling (golangci-lint binaries, Go toolchains, govulncheck) | medium | mitigate | golangci-lint binaries are official release tarballs checked with `sha256sum -c` against the release `checksums.txt` (re-downloads repeat that check); toolchains come from the go command's checksum-verified module download; every gate asserts tool identity before trusting a result (`go version` must equal `go version go1.27.3 linux/amd64`, which rejects the Arch `-X:nodwarf5` build; govulncheck must print `Go: go1.27.3`; golangci-lint must self-report its version and vendored x/tools); the Task 1 precondition re-observes that `stable` is still 1.27.3. |
| T-quick-vbj-04 | Denial of Service | lint and vuln job availability | low | accept | If a pinned release or toolchain is later withdrawn or unreachable, the job fails closed and blocks merge. A security and quality gate failing closed is the correct direction, and the job log makes it obvious. |
| T-quick-vbj-05 | Elevation of Privilege | job `permissions` blocks, secrets | low | accept | Both lint jobs and the vuln job keep `permissions: contents: read`; no `permissions` block, secret, `run:` step or `${{ }}` expression is added or changed (the six-line non-comment diff gate proves it), so no privilege or injection surface moves. |
| T-quick-vbj-SC | Tampering | `go install golang.org/x/vuln/cmd/govulncheck@latest` (pre-existing, vuln job) | medium | accept | A floating `@latest` makes the scanner version non-reproducible run to run. Pre-existing, explicitly outside this change's authorized scope, already tracked as `.planning/WINDOWS.md` entry 1 (`T-quick-cad-SC`); pinning it is a separate `ci:` change. The GSD package-legitimacy checkpoint covers npm/pip/cargo installs and does not apply; this plan adds no project dependency. |

No threat is rated `high` or `critical`, so nothing blocks at `security_block_on: high`.
</threat_model>

<verification>
1. `actionlint .github/workflows/ci.yml` exits 0 after every task.
2. Per-slice end-to-end proofs (Tasks 1 and 2): each pin is read from its own job region of the edited file, resolved to the matching official binary or toolchain, and executed — library scope and ohcli scope report `0 issues.` with v2.15.0 on official go1.27.3; govulncheck reports `No vulnerabilities found.` on official go1.27.3.
3. Discrimination: the old lint pin (read from 451900a) fails on the same toolchain and args with `export data version 5 is greater than maximum supported version 4`; govulncheck on official go1.27.2 exits 3 on GO-2026-6809.
4. Pin greps on non-comment lines (Task 3): exactly two `version: v2.15.0`, one `go-version: "1.27.3"`, zero of either old pin.
5. Containment: the non-comment diff against 451900a is exactly the six pin lines; `.github/workflows/ci.yml` is the only changed path outside `.planning/`; the tree outside `.planning/` is clean after the commit.
6. Rationale prose: the required literals sit on comment lines of their own jobs; the stale present-tense sentence is gone; the 26-line earlier vuln record is byte-identical and contiguous.
7. `config verify` exits 0 under v2.15.0 — no `.golangci.yml` migration.
8. Commit: HEAD is one non-merge `ci:` commit touching only the workflow file, with the quick-task id in its body and the exact `Co-Authored-By` trailer; nothing pushed.
9. SUMMARY exists with `status: complete` and is not committed.
</verification>

<success_criteria>
- `.github/workflows/ci.yml` pins golangci-lint `v2.15.0` in both lint jobs (lockstep) and Go `"1.27.3"` in the vuln job; every other job, action SHA, matrix, `stable` line and the govulncheck install line is unchanged.
- Each pin carries English comment prose naming the constraint that decides its legal values: two floors plus a check recipe in library-lint, a short lockstep cross-reference in ohcli-lint, and a dated 2026-10-25 extension of the vuln record.
- All end-state gates pass against the official go1.27.3 toolchain that `stable` resolves to, with negative controls proving the gates discriminate — so `library — lint`, `ohcli — lint` and `vulnerabilities (govulncheck)` will go green on the runner.
- Exactly one commit, type `ci:` (non-releasing), touching one file; no push, PR or Dependabot action taken.
</success_criteria>

<output>
Create `.planning/quick/261025-vbj-fix-ci-bump-golangci-lint-to-v2-15-0-and/261025-vbj-SUMMARY.md` when done (`status: complete` in the frontmatter; do not commit it — the orchestrator's docs commit owns it).
</output>
