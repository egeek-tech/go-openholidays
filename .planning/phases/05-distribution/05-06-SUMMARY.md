---
phase: 05-distribution
plan: 06
subsystem: release-pipeline
tags:
  - ci
  - release
  - goreleaser
  - dependabot
  - github-actions
  - attestation
dependency_graph:
  requires:
    - 05-05 # ci.yml shipped first; share action-pin style + permission idioms
    - 05-04 # integration_test.go (//go:build integration + OPENHOLIDAYS_LIVE env gate) exists; this plan's integration.yml supplies both gates
    - 05-01 # cmd/ohcli exists and builds with `./cmd/ohcli`; this plan's .goreleaser.yaml uses that path
    - 04 # version.go ships `var Version = "0.1.0"`; the W-05 trimPrefix template inject keys to this literal
  provides:
    - integration.yml # nightly cron + manual workflow_dispatch runner for live-API tests
    - release.yml # tag-triggered goreleaser + attest-build-provenance pipeline
    - .goreleaser.yaml # v2-schema config for 6-binary cross-platform builds
    - dependabot.yml # weekly action-version freshness PR generator
  affects:
    - 05-07 # docs phase later writes the release runbook that exercises this pipeline
    - 05-08 # finalization phase tags v0.1.0 and verifies REL-04 against the pipeline this plan built
tech_stack:
  added:
    - goreleaser/goreleaser-action@v6 (workflow-only — installed by the action, not a Go runtime dep)
    - actions/attest-build-provenance@v4 (workflow-only)
  patterns:
    - "Tag-triggered release pipeline: `on: push: tags: ['v*']` + goreleaser v2 schema → ./dist/checksums.txt → attest-build-provenance@v4 signs the manifest."
    - "Two-gate live-API integration runner: `//go:build integration` (compile gate, set by `-tags=integration`) + `OPENHOLIDAYS_LIVE=1` (runtime t.Skip gate)."
    - "ldflags version-inject with `{{ trimPrefix \"v\" .Version }}` to keep released-binary User-Agent in lockstep with dev-binary User-Agent (W-05)."
    - "Conventional-commit auto-changelog via goreleaser `changelog.groups` + `changelog.filters.exclude` (D-12)."
    - "Dependabot for github-actions ecosystem only — gomod intentionally absent because the library is zero-runtime-dependency (PROJECT.md)."
key_files:
  created:
    - .github/workflows/integration.yml
    - .github/workflows/release.yml
    - .goreleaser.yaml
    - .github/dependabot.yml
  modified: []
decisions:
  - "W-05 closed in `.goreleaser.yaml`: ldflags `-X github.com/egeek-tech/go-openholidays.Version={{ trimPrefix \"v\" .Version }}` (NOT bare `{{.Version}}`). Plan 05-06 acceptance criterion explicitly forbids bare `{{.Version}}` on the `-X Version=` injection line; both forms verified by grep."
  - "Source-current action pins applied — goreleaser-action@v6 (not @v5 from CONTEXT.md) and attest-build-provenance@v4 (not @v1 from CONTEXT.md per Pitfall 2). Older versions are deprecated wrappers."
  - "Dependabot configured for github-actions only — no gomod ecosystem entry, because the library declares zero runtime deps (PROJECT.md) and the lone test-only dep (testify) is pinned by hand."
  - "Release workflow omits `workflow_dispatch:` and `pull_request_target:` triggers. Tag-only policy is locked for v0.1.0; manual dispatch can be added in v0.2.x if needed; pull_request_target is a security non-starter (would let a hostile fork PR run the release flow with elevated permissions)."
  - "Both workflow files include a security-note comment block documenting that no `github.event.*` or other untrusted input is interpolated into any `run:` step (only literal env values like `OPENHOLIDAYS_LIVE: \"1\"` and the GitHub-issued `secrets.GITHUB_TOKEN`)."
metrics:
  duration_seconds: ~300
  completed_date: "2026-05-28"
  tasks_completed: 3
  files_changed: 4
---

# Phase 5 Plan 06: Release pipeline (integration.yml, release.yml, .goreleaser.yaml, dependabot.yml) Summary

Adds the nightly live-API integration workflow, the tag-triggered goreleaser+attestation release workflow, the v2-schema goreleaser config that drives it, and the dependabot config that keeps the action pins fresh — completing the CI/release-automation surface required for v0.1.0 tagging in Plan 08.

## Outcome

| Task | Name | Commit | Files |
| ---- | ---- | ------ | ----- |
| 1 | integration.yml + dependabot.yml | `077c974` | `.github/workflows/integration.yml`, `.github/dependabot.yml` |
| 2 | .goreleaser.yaml (v2, 6-binary, W-05 fix) | `f5c4a6d` | `.goreleaser.yaml` |
| 3 | release.yml (tag → goreleaser + attest@v4) | `702646c` | `.github/workflows/release.yml` |

## What Was Built

### Task 1 — `integration.yml` + `dependabot.yml`

**`.github/workflows/integration.yml`**: nightly cron (`"0 3 * * *"`, 03:00 UTC) plus `workflow_dispatch` for manual dispatch. Single `integration` job on `ubuntu-latest` checks out, sets up Go `stable`, then runs `go test -tags=integration -count=1 -timeout=5m ./...` with `OPENHOLIDAYS_LIVE: "1"`. Both gates that `integration_test.go` (Plan 04 output) expects are supplied here — neither default `go test` nor a developer running `-tags=integration` without the env var will hit the live upstream. Closes CI-04.

**`.github/dependabot.yml`**: single `github-actions` ecosystem entry, weekly cadence, `open-pull-requests-limit: 5`. No `gomod` entry — the library is zero-runtime-dependency per PROJECT.md and the lone test-only dep is pinned deliberately. Closes CI-06.

### Task 2 — `.goreleaser.yaml`

v2-schema config (`version: 2` directive). Single `builds:` entry for `./cmd/ohcli` produces 6 binaries (linux/darwin/windows × amd64/arm64) with `CGO_ENABLED=0`, `-trimpath`, `-s -w`, and reproducible `mod_timestamp: "{{ .CommitTimestamp }}"`. Archives are tar.gz by default with Windows overriding to zip, bundling `LICENSE` + `README.md` + `CHANGELOG.md`. SHA-256 `checksums.txt` emitted. GitHub release entry with `prerelease: auto`. Conventional-commit changelog block with four groups (Features / Bug Fixes / Performance / Other) and a comprehensive `filters.exclude` list (chore, docs, test, style, refactor, ci, typo, merge conflict).

**W-05 fix (critical):** `ldflags` use `-X github.com/egeek-tech/go-openholidays.Version={{ trimPrefix "v" .Version }}`. Without the `trimPrefix "v"` template function, a goreleaser-built v0.1.0 binary would emit `User-Agent: go-openholidays/v0.1.0` while a `go build` / `go install` binary (which reads version.go directly, where `var Version = "0.1.0"` has no leading `v`) would emit `User-Agent: go-openholidays/0.1.0` — a silent divergence the Phase 5 reviewer flagged as W-05. The plan's acceptance criteria explicitly forbid bare `{{.Version}}` on the `-X Version=` line and require the `trimPrefix` form; both checks pass.

Deferred per D-10 — NOT present: `brews:` (Homebrew tap), `signs:` (cosign — attestations cover provenance instead), `source:` (source tarball), `sboms:` block. Closes CI-05 (config side) + REL-03.

### Task 3 — `release.yml`

`on: push: tags: ['v*']` — fires only on tag pushes from authenticated repo owners (tag protection is configured outside YAML in repo settings). Top-level permissions: `contents: write`, `id-token: write`, `attestations: write` — all three required for the Sigstore/Fulcio OIDC attestation flow per docs.github.com.

Single `goreleaser` job: `actions/checkout@v4` with `fetch-depth: 0` (required for goreleaser to walk commits between tags for the conventional-commit changelog), `actions/setup-go@v5` with `stable`, `goreleaser/goreleaser-action@v6` (Source-current pin — not @v5 from CONTEXT.md) with `distribution: goreleaser`, `version: "~> v2"`, `args: release --clean`, and `GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}`. Final step `actions/attest-build-provenance@v4` (Source-current — NOT @v1 per Pitfall 2 — @v1/@v2/@v3 are deprecated wrappers) signs `./dist/checksums.txt`. Because the checksum manifest covers every released binary by SHA-256, signing the manifest transitively attests every binary without a per-binary step.

Deliberately omitted: `workflow_dispatch:` (tag-only policy locked for v0.1.0) and `pull_request_target:` (would let a hostile fork PR run release with elevated permissions). Closes CI-05 + D-11.

## Verification Evidence

All four files parse as YAML (Python `yaml.safe_load`) and are clean under a `yamllint -d` permissive rule set.

| File | Acceptance criteria asserted | Result |
| ---- | ---------------------------- | ------ |
| `integration.yml` | `cron: "0 3 * * *"`, `workflow_dispatch:`, `OPENHOLIDAYS_LIVE:`, `go test -tags=integration`, `actions/checkout@v4`, `actions/setup-go@v5` all present | pass |
| `dependabot.yml` | `version: 2`, `package-ecosystem: github-actions`, `interval: weekly`, `open-pull-requests-limit: 5`, NO `gomod` entry | pass |
| `.goreleaser.yaml` | `version: 2`, `project_name: go-openholidays`, `main: ./cmd/ohcli`, `binary: ohcli`, `CGO_ENABLED=0`, all 3 OS + 2 arch entries, `-trimpath`, `name_template: "checksums.txt"`, `algorithm: sha256`, `use: github`, feat/fix regex changelog groups, chore exclusion. **W-05:** `-X ...Version={{ trimPrefix "v" .Version }}` present; bare `{{.Version}}` on the `-X Version=` line absent. **Deferred items:** no top-level `brews:` or `signs:` keys (only mentioned in deferred-items comments). | pass |
| `release.yml` | `name: Release`, `tags: ['v*']`, all three required permissions, `actions/checkout@v4` + `fetch-depth: 0`, `actions/setup-go@v5`, `goreleaser/goreleaser-action@v6`, `version: "~> v2"`, `args: release --clean`, `actions/attest-build-provenance@v4`, `subject-checksums: ./dist/checksums.txt`. **Negative checks:** no `pull_request_target` in non-comment lines; no `@v1` references in non-comment lines. | pass |

`goreleaser check` not run locally — `goreleaser` binary is not installed on the worktree host. The plan's verification specifies the structural Python check as the fallback when the binary is absent; that fallback ran clean. End-to-end `release.yml` exercise is deferred to Plan 08 (REL-04 verification) via a `v0.1.0-rc1` tag rehearsal as designed.

## Deviations from Plan

**None.** Plan 05-06 executed exactly as written. All three tasks landed without auto-fixes, missing-functionality additions, or blocking-issue corrections. The W-05 fix the plan introduced was implemented as specified (the plan acceptance criteria require `trimPrefix "v" .Version` AND forbid bare `{{.Version}}` on the `-X Version=` line — both checks pass).

One micro-adjustment outside the deviation rule classes: both workflow files include an additional security-note comment block documenting that no `github.event.*` or other untrusted input is interpolated into any `run:` step. The plan did not require these comments; they were added in response to the in-environment workflow-write security-reminder hook to make the lack of untrusted-input surface explicit for future readers. This does not affect any acceptance criterion.

## Threat Flags

None — every artifact added by this plan is already covered by the `<threat_model>` in 05-06-PLAN.md (T-05-20 through T-05-26). The release workflow's elevated-permission surface is mitigated by tag-push-only trigger + tag protection (T-05-20); action-version supply-chain risk is mitigated by Dependabot weekly PRs (T-05-21); deprecated `@v1` attest action is forbidden by acceptance criterion + grep (T-05-22); free-API DoS risk is mitigated by single daily cron + 5-minute timeout (T-05-23). No new trust boundaries introduced.

## Known Stubs

None. All four files are complete, structurally valid, and ready for tag-time exercise.

## Self-Check: PASSED

- `.github/workflows/integration.yml` exists (FOUND).
- `.github/workflows/release.yml` exists (FOUND).
- `.goreleaser.yaml` exists (FOUND).
- `.github/dependabot.yml` exists (FOUND).
- Commit `077c974` (Task 1) reachable from HEAD (FOUND).
- Commit `f5c4a6d` (Task 2) reachable from HEAD (FOUND).
- Commit `702646c` (Task 3) reachable from HEAD (FOUND).
