# Release Runbook — go-openholidays

Single source of truth for cutting a tagged release of `go-openholidays`. Every step
below is anchored to the workflow, config, or source file it depends on; follow them
in order. This document is consumed by the human release operator — every section
maps to one or more Phase 5 acceptance criteria (REL-01..04, DOC-07, CI-04).

Release Please drives the actual tag/Release/CHANGELOG; the operator's role is to
review the Release PR it opens, merge it, and verify the post-merge artifacts.

Pipeline overview:

```
Conventional-Commit PRs merged to master
                  │
                  ▼
   release-please.yml fires on push:master
                  │
                  ├─ no release-worthy commits since last tag → no-op
                  │
                  └─ release-worthy commits AND no open Release PR
                              │
                              ▼
                  Release Please opens / updates a Release PR
                  ("chore(master): release X.Y.Z")
                              │
       human reviews and merges the Release PR
                              │
                              ▼
   release-please.yml fires again on push:master
                              │
                              ├─ Phase 1: tag vX.Y.Z + GitHub Release cut via API
                              │
                              └─ Phase 2 (gated on release_created):
                                  ├─ checkout @ vX.Y.Z (fresh fetch — tag was just born)
                                  ├─ goreleaser builds 6 binaries + checksums.txt
                                  └─ actions/attest-build-provenance signs checksums.txt
                              │
              post-tag verification (manual reads of pkg.go.dev,
              Go Report Card, gh attestation verify)
```

The two phases live in the **same** job deliberately. A tag pushed by `GITHUB_TOKEN`
does NOT trigger a downstream workflow keyed on `push:tags:v*` (GitHub's anti-loop
guard). Putting goreleaser in a separate workflow silently drops; v0.2.0 hit this
exact failure mode before the consolidation in PR #11 (commit 3b23874).

---

## 1. Pre-merge checklist (before merging a Release Please PR)

Run every command from the repo root on the `master` branch. None of these steps
mutate the remote — they are local readiness gates only.

- [ ] On `master`, fully synced with origin (`git status` clean, `git pull` is a no-op).
- [ ] CI is green on the most recent `master` commit (visit Actions tab; the `CI`
      workflow matrix legs all pass on Go 1.24.x, 1.25.x, and `stable`).
- [ ] The Release Please PR diff matches expectations: `version.go` bump,
      `CHANGELOG.md` entries grouped by Conventional-Commit prefix, and
      `.release-please-manifest.json` updated. No production source changes
      should appear in a Release PR.
- [ ] `version.go` (post-merge) will satisfy the W-05 invariant — the literal value
      matches the intended tag minus the `v` prefix (e.g. for tag `v0.3.0` the
      source has `var Version = "0.3.0"`). Release Please writes this via the
      `// x-release-please-version` marker; verify the diff manually.
- [ ] `go.mod` first line is `module github.com/egeek-tech/go-openholidays` (REL-04 —
      verified once at 2026-05-28; no follow-up edits expected).
- [ ] `go test -race -coverprofile=coverage.out ./...` exits 0 locally; coverage ≥ 85 %.
      The exact gate CI enforces on the `stable` matrix leg.
- [ ] `golangci-lint run --max-issues-per-linter=0 --max-same-issues=0 ./...` exits 0.
      Then re-run with `--build-tags=integration` to cover the integration build.
- [ ] `govulncheck ./...` exits 0.
- [ ] `go test -run Example .` exits 0 (TEST-09 — every `// Output:` block still
      matches). Includes `Example_quickstart`, which must stay byte-for-byte
      equivalent to the runnable `package main` snippet in `README.md` (DOC-01).
- [ ] (Optional) `goreleaser check` exits 0 if `goreleaser` v2 is installed
      locally — catches `.goreleaser.yaml` syntax errors before the workflow runs.
      Note: `goreleaser check` does NOT validate template-function references in
      ldflags; see Section 6 "Known issues" for the v0.2.1 incident.

---

## 2. Cutting the release

The operator's action is to **merge the Release Please PR**. Release Please then
tags `vX.Y.Z`, drafts the GitHub Release, and triggers Phase 2 of
`release-please.yml` (goreleaser + attest-build-provenance) in the same workflow
run.

```bash
# From the GitHub UI on the Release PR, or via the CLI:
gh pr merge <release-pr-number> --merge --repo egeek-tech/go-openholidays
```

A merge commit is required (do not squash) — Release Please's automation reads
the merge commit's metadata to confirm the release boundary. The repo's branch
protection rules should already disallow squash on Release PRs.

Watch `release-please.yml` in the Actions tab. ETA ~3-5 min for Phase 2
(cross-compile + attestation). The run is the SAME workflow that fired when the
Release PR was first opened; the second invocation runs Phase 2 because
`steps.release.outputs.release_created` is now `"true"`.

---

## 3. Post-tag verification matrix

When `release-please.yml` is green, run every check in this table. Each row maps
to a Phase 5 requirement that `.planning/phases/05-distribution/05-VALIDATION.md`
flagged as manual-only because the underlying surface is an external service or a
CLI that CI cannot reliably poll.

| Check | Command / URL | Expected | When |
|-------|---------------|----------|------|
| REL-03: GitHub Release artifacts present | `gh release view vX.Y.Z --repo egeek-tech/go-openholidays --json assets \| jq '.assets \| length'` | `7` (6 binary archives + 1 `checksums.txt`); asset names match `ohcli_X.Y.Z_*` | Immediately after `release-please.yml` green |
| REL-03: Attestation verifiable from CLI | Download one binary, then `gh attestation verify <archive> --repo egeek-tech/go-openholidays` | Exit 0; output reports the archive verified against this repo's workflow | Immediately after `release-please.yml` green |
| REL-01: `pkg.go.dev` renders the package | Visit `https://pkg.go.dev/github.com/egeek-tech/go-openholidays@vX.Y.Z` | HTTP 200; no "no documentation" banner; every exported symbol renders; `Example_*` functions attached to their target symbols; `[pkg.Type]` doclinks render as clickable | Within 30 min of merge (proxy lag — see Section 4) |
| REL-02: Go Report Card grade A | Visit `https://goreportcard.com/report/github.com/egeek-tech/go-openholidays`; for badge only: `curl -fsS https://goreportcard.com/badge/github.com/egeek-tech/go-openholidays` | Grade A; green badge | Within 24 h of merge (first scan can be slow) |
| DOC-07: pkg.go.dev godoc audit | On the pkg.go.dev page above, spot-check at least three exported symbols (e.g. `NewClient`, `Holiday.NameFor`, `HolidayType.IsKnown`) | Each section renders; no "Documentation: no" warnings; opening godoc line starts with the symbol name | Within 30 min of merge |

---

## 4. pkg.go.dev index trigger (mitigation for Pitfall 6)

The Go module proxy fetches new tagged versions lazily. After a tag is created,
`pkg.go.dev` may report "no module found" for up to 30 min until the proxy is
poked. The repository is public (D-02 / 05-CONTEXT.md), so this is the only
manual nudge required.

If the page returns "no module found" 5 min after the merge:

```bash
curl -fsS https://pkg.go.dev/github.com/egeek-tech/go-openholidays@vX.Y.Z -o /dev/null
# Repeat with @latest if needed:
curl -fsS https://pkg.go.dev/github.com/egeek-tech/go-openholidays@latest -o /dev/null
```

If after 30 min the page still does not render, escalate — the most common root
cause is a `go.mod` first-line mismatch with the actual module-import path users
would use (verified clean in Section 1).

Reference: 05-RESEARCH.md §"Common Pitfalls → Pitfall 6" — pkg.go.dev 30-min
index lag.

---

## 5. Rollback

`release-please.yml` is idempotent on re-run: a transient failure (network blip,
proxy hiccup, GitHub API 5xx) can be recovered by hitting "Re-run workflow" in
the Actions tab. The goreleaser step uses `--clean`, which removes `./dist/`
before rebuilding.

If the tagged release contains binaries you cannot ship (wrong version pin,
attestation regression, accidental inclusion of a file, …), the only safe
recovery is to **bump to the next patch** rather than reuse the tag — the Go
module proxy caches versions, and any caller who already fetched `vX.Y.Z` via
`go get` will see the bad artifact until their local module cache evicts.

```bash
# Make whatever fix is needed on master via a regular PR with a Conventional-
# Commit prefix that bumps version (fix: → patch, feat: → minor, ! suffix or
# BREAKING CHANGE footer → major). Release Please will pick it up automatically
# on the next merge to master and open a new Release PR.
```

Reusing a tag (`gh release delete` + `git push origin :refs/tags/vX.Y.Z`) is the
absolute last-resort path and only viable in the first ~5 minutes after a botched
merge, before any consumer has fetched. Do not use it after a tag has been
public for any meaningful duration.

---

## 6. Known issues (must read before cutting v0.3.0+)

### 6.1 `trimPrefix` goreleaser template — broke v0.2.1 binaries

`.goreleaser.yaml` line 55 currently reads:

```yaml
ldflags:
  - -X github.com/egeek-tech/go-openholidays.Version={{ trimPrefix "v" .Version }}
```

Goreleaser's template engine does **not** define a `trimPrefix` function;
attempting to invoke it produces `template: failed to apply ...: function
"trimPrefix" not defined` and aborts the build. This fired on the v0.2.1 release
(workflow run `26605628217`) and on v0.2.0 (which also hit the standalone-tag
cascade gap, so two distinct bugs masked each other). Net effect: **v0.2.0 and
v0.2.1 GitHub Releases were created but carry zero binary assets and zero
attestations**.

The fix is to drop the `trimPrefix` call entirely — goreleaser's `.Version` is
already the tag without the leading `v` per the official template docs, so the
correct ldflag is:

```yaml
ldflags:
  - -X github.com/egeek-tech/go-openholidays.Version={{ .Version }}
```

This is a one-line fix that should land in a `fix(release): drop unsupported
trimPrefix template call` PR before the next release attempt. The next release
will then be the first to ship signed binaries — treat that release like a v0.1.0
dry-run (Section 7).

### 6.2 v0.2.0 / v0.2.1 are tag-only releases

For external consumers using `go get github.com/egeek-tech/go-openholidays@v0.2.1`
the library code is intact and complete — `go get` reads the module from
`proxy.golang.org` and does not touch the GitHub Release assets. Only the
**CLI** ohcli binaries and their attestations are missing.

If a user asks for an ohcli build for v0.2.x: instruct them to build from source
(`go install github.com/egeek-tech/go-openholidays/cmd/ohcli@v0.2.1`) until the
next release ships verified binaries.

### 6.3 First-time binary release after the fix

When the `trimPrefix` fix lands and the next Release PR is merged, the resulting
release is effectively the project's first signed-binary release. Treat the
post-tag verification matrix (Section 3) as a hard checkpoint — every row must
pass before the milestone closes.

---

## 7. Post-release housekeeping

After the human verifier in Section 3 has confirmed all rows pass:

- Release Please will have already updated `version.go`, `CHANGELOG.md`, and
  `.release-please-manifest.json` via the merged Release PR. No manual bump is
  required. The next merged Conventional-Commit-prefixed PR (`feat:`, `fix:`,
  etc.) triggers the next Release PR.

- Update `.planning/STATE.md` if the release closed a milestone — set
  `progress.completed_phases` accordingly and mark the milestone `completed`.

- Optional post-release retrospective: note in `STATE.md` Session Continuity
  what worked, what surprised you, and what remained gated on manual checks.
  Especially valuable on the first signed-binary release after the §6.1 fix.

- `CHANGELOG.md` is auto-managed by Release Please using the
  `release-please-config.json` `release-as` and `bump-minor-pre-major` settings.
  No manual `CHANGELOG.md` edit is required or accepted.

---

## 8. Release history (audit trail)

| Tag | Date | Flow | Binaries | Notes |
|-----|------|------|----------|-------|
| v0.1.0 | — | (never cut) | — | Project decided to use Release Please bump-minor-pre-major; the first auto-cut was v0.2.0 |
| v0.2.0 | 2026-05-28 | Standalone tag-trigger workflow | 0 | GITHUB_TOKEN cascade gap — separate goreleaser workflow keyed on `push:tags:v*` never fired |
| v0.2.1 | 2026-05-28 | Consolidated workflow (PR #11) | 0 | Goreleaser failed: `template: function "trimPrefix" not defined` (§6.1) |
| v0.3.0 (next) | TBD | Consolidated workflow + `trimPrefix` fix | (target: 7 assets) | Will be the first release to actually ship signed binaries |

---

## 9. References

- `.github/workflows/release-please.yml` — the consolidated release workflow.
  Phase 1 is googleapis/release-please-action; Phase 2 is gated on
  `steps.release.outputs.release_created` and runs goreleaser +
  attest-build-provenance.
- `.github/workflows/ci.yml` — Go matrix tests + lint + govulncheck on every PR
  and push to master.
- `.github/workflows/integration.yml` — nightly live-API integration suite
  (independent of releases; listed here only for completeness).
- `.goreleaser.yaml` — drives the 6-binary build matrix, archive layout, and
  `checksums.txt`. See §6.1 for the open `trimPrefix` issue.
- `release-please-config.json` + `.release-please-manifest.json` — Release Please
  configuration (`bump-minor-pre-major: true` pre-1.0).
- `version.go` — single source of truth for the `Version` literal; written by
  Release Please via the `// x-release-please-version` marker.
- `go.mod` first line — `module github.com/egeek-tech/go-openholidays` (REL-04).
- `.planning/phases/05-distribution/05-VALIDATION.md` §"Manual-Only
  Verifications" — canonical list of post-tag checks for REL-01..03 and DOC-07.
- `.planning/phases/05-distribution/05-RESEARCH.md` §"Common Pitfalls →
  Pitfall 6" — pkg.go.dev 30-min index lag mitigation.
- `.planning/PROJECT.md` Key Decisions CL-18 — action-version policy.
- `.planning/PROJECT.md` Key Decisions CL-19 — Go 1.24 floor + t.Context()
  migration + Phase 5 lint additions.
