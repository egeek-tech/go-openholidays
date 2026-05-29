---
phase: 05-distribution
plan: 08
subsystem: release-runbook
tags: [release, runbook, project-md, key-decisions, go-floor, godoclint]

# Dependency graph
requires:
  - phase: 05-distribution
    provides: release-please workflow (consolidated), goreleaser config, attest-build-provenance, godoclint enforcement
provides:
  - docs/release-runbook.md — pre-merge checklist, Release-Please merge step, post-tag verification matrix, pkg.go.dev index trigger, rollback policy, known-issues section, release history audit trail
  - .planning/PROJECT.md Key Decisions appended with CL-19 (Go 1.24 floor + t.Context() migration + Phase 5 lint additions, superseding CL-18 part c)
  - 05-08-SUMMARY.md (this file)
affects: [release-pipeline, milestone-close, future-tag-cuts]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Consolidated release workflow (release-please.yml): Phase 1 release-please-action + Phase 2 goreleaser/attest in same job, gated on steps.release.outputs.release_created — sidesteps GITHUB_TOKEN cascade gap that masked v0.2.0's missing binaries"
    - "Runbook documents reality (Release Please merge → automatic tag) rather than the originally-planned manual git push origin v* flow"
    - "Release history table in runbook §8 audit-trails v0.1.0 (skipped), v0.2.0 (cascade gap), v0.2.1 (trimPrefix bug) — known-issue section §6.1 carries the one-line fix forward"

key-files:
  created:
    - .planning/phases/05-distribution/05-08-SUMMARY.md
  modified:
    - docs/release-runbook.md (full rewrite to reflect consolidated Release Please flow; replaced the hypothetical manual tag-push narrative with the actual merge-Release-PR flow; added §6 Known Issues documenting the goreleaser trimPrefix bug; added §8 release-history audit trail)
    - .planning/PROJECT.md (Key Decisions: appended CL-19 documenting Go 1.24 floor bump + t.Context() migration + bodyclose/noctx/tparallel/godox/godoclint additions; CL-18 left in place as the original Phase-5-planning-time decision)
---

# Plan 05-08 — Release runbook + PROJECT.md decisions log

## What landed

Two deliverables, both retrospective documentation of work that had already
happened under PRs #11 (consolidated workflow), #13 (CI lint scope + tparallel),
and #14 (godoclint + doclink cleanup):

- **`docs/release-runbook.md`** — fully rewritten. The previous draft described
  a manual `git tag && git push origin v*` flow that was never actually used.
  The rewrite documents the consolidated Release-Please workflow that has been
  the actual release mechanism since PR #11 (commit 3b23874): merge to master
  → Release Please opens a Release PR → operator merges the Release PR →
  release-please.yml's second invocation runs goreleaser + attest in the same
  job, gated on `steps.release.outputs.release_created`. New material includes
  §6 Known Issues (documenting the goreleaser `trimPrefix` template bug that
  zero'd v0.2.1's binaries) and §8 Release History (audit trail of v0.1.0
  skipped, v0.2.0 cascade-gap, v0.2.1 trimPrefix-bug).

- **`.planning/PROJECT.md`** — appended CL-19 to Key Decisions. CL-19
  supersedes CL-18 part (c) which said "Phase 5 tests use context.Background(),
  NOT t.Context()" — that decision held at Phase-5-planning time when the Go
  floor was 1.23. Commit 2123f97 bumped the floor to 1.24 and migrated all
  tests to t.Context(); CL-19 captures the supersession plus the companion
  Phase-5 lint additions (bodyclose, noctx, tparallel, godox, godoclint with
  basic + require-stdlib-doclink + no-unused-link). CL-18 itself is left
  in place as the historically-correct Phase-5-planning-time entry — the
  CL-19 entry's "Supersedes CL-18 part (c)" lead sentence carries the
  forward reference.

## What did NOT land (and why)

- **Task 3 (Human checkpoint — push v0.1.0 tag + verify external services)**:
  obsolete as-written. The project skipped v0.1.0 entirely (bump-minor-pre-major
  + Release Please cut v0.2.0 as the first release). The post-tag verification
  matrix in the rewritten runbook §3 stands in for the original checkpoint and
  will execute against the first release that actually carries binaries — see
  next bullet.

- **Verification that the release pipeline ships signed binaries**: blocked
  on the `.goreleaser.yaml` `trimPrefix` fix (runbook §6.1). The Phase 5
  exit criterion "binaries shipped + attestations verifiable" therefore
  remains structurally satisfied (consolidated workflow + attest config in
  place) but empirically unverified (the next release will be the first to
  exercise it end-to-end). Captured as the lone milestone-close gap.

## Deviations from plan

- **Plan files_modified included `.planning/PROJECT.md`** — that file was
  modified, but a prior session had already populated CL-18 with broadly
  Dependabot-up-to-date content. The plan's exact CL-18 text (referencing
  codecov@v5, goreleaser@v6) is out-of-date because Dependabot subsequently
  bumped to codecov@v6 / goreleaser@v7. The existing CL-18 entry already
  pivots to "Dependabot is the authoritative source for current pin majors"
  rather than freezing version literals, so no edit to CL-18 was needed.
  The CL-19 entry was added as the cleanest record of the Go-floor bump and
  lint additions that have happened since CL-18 was authored.

- **Plan acceptance criteria included literal greps for `codecov/codecov-action@v5`
  and `goreleaser/goreleaser-action@v6`** — neither string is in PROJECT.md
  any longer because CL-18 was rewritten to be version-agnostic. The intent
  (action-version policy recorded) is satisfied; the literal grep would fail.
  Treating this as a stale acceptance-criterion artifact, not a real gap.

## Open follow-ups

| Item | Owner | Notes |
|------|-------|-------|
| `.goreleaser.yaml` line 55: drop `trimPrefix "v" .Version` — use `{{ .Version }}` directly | next release prep | One-line fix; runbook §6.1 carries the reproduction + remediation; merging this before the next Release PR is the precondition for the first release to ship verified binaries |
| First-signed-binary release verification | next release operator | Treat the post-trimPrefix-fix release as a v0.1.0-equivalent dry run; every row in runbook §3 verification matrix must pass before declaring the milestone closed |
| Untracked planning artifacts (`01-foundation/01-UAT.md`, `04-resilience/04-PATTERNS.md`, `GSD-PROJECT-BRIEF.md`) | next milestone-close step | Triage commit/delete/ignore — not part of 05-08 scope |

## Acceptance criteria status

| Criterion | Status | Evidence |
|-----------|--------|----------|
| `.planning/PROJECT.md` updated | ✅ | CL-19 row appended after CL-18; existing rows preserved |
| `grep -F 'CL-19' .planning/PROJECT.md` exits 0 | ✅ | new row contains `**CL-19: Supersedes CL-18 part (c)...` |
| `grep -F 'CL-18' .planning/PROJECT.md` still exits 0 | ✅ | CL-18 left in place verbatim |
| Existing CL-01..CL-17 rows preserved | ✅ | Edit only appended a new row; no other modifications |
| docs/release-runbook.md exists | ✅ | full rewrite, ~9 sections |
| Runbook references real release pipeline (`release-please.yml`) | ✅ | §0 overview + §9 references both anchor on the consolidated workflow |
| Runbook documents pkg.go.dev / Go Report Card / attestation verification | ✅ | §3 verification matrix rows REL-01..03 + DOC-07 |
| Runbook documents rollback policy | ✅ | §5 — bump-to-next-patch is the only safe path, tag reuse is last resort |
| Phase 5 binaries shipped + verified | ❌ | Blocked on `trimPrefix` fix (§6.1) — see Open Follow-ups |

## Phase-close posture

This plan closes the documentation work for Phase 5. The empirical "first
release with verified binaries" milestone is one one-line goreleaser-config
fix away. The milestone may be declared complete once the next release cuts
green end-to-end with assets attached.
