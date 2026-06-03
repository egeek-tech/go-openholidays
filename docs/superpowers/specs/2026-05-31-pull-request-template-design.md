# Pull Request Template — Design

**Date:** 2026-05-31
**Status:** Approved (brainstorming)
**Scope:** Add a GitHub pull request template for `egeek-tech/go-openholidays`.

## Goal

Give contributors a default PR description that captures the change's intent,
its conventional-commit type, breaking-change status, and a verification
checklist drawn from the repo's dev loop and Gold Rules. Because the repo
**squash-merges**, the PR title becomes the conventional commit that drives
Release Please / goreleaser — so the template explicitly steers the title and
type.

## Decisions

- **Format:** a single `.github/PULL_REQUEST_TEMPLATE.md`. GitHub PR templates
  are Markdown-only (no YAML-forms equivalent to issue forms). A single
  auto-applied template is chosen over a `PULL_REQUEST_TEMPLATE/` directory of
  selectable templates, which is not auto-applied and would leave most PRs
  blank.
- **Checklist depth:** comprehensive — dev loop plus the Gold Rules that are
  easy to forget.
- **Release impact of this change:** repo tooling only, no consumer-facing
  effect → committed as `chore:`, does not cut a release (Rule 6).

## Sections

1. **Summary** — what and why, with `Closes #`.
2. **Type of change** — checkboxes labelling each conventional type with whether
   it cuts a release. Releasing types verified against `release-please-config.json`
   on 2026-05-31: `feat`, `fix`, `perf`, `deps`, `docs`. Non-releasing:
   `chore`, `refactor`, `test`, `ci`, `build`, `style`. Reinforces Rule 6 and the
   squash-merge title convention.
3. **Breaking changes** — prompts for a `BREAKING CHANGE:` footer; notes v1.0+
   strict SemVer. Defaults to "None".
4. **Checklist** (10 items):
   - PR title is a valid Conventional Commit with an honest type (Rule 6).
   - `go test -race ./...` passes.
   - `golangci-lint run` clean.
   - `govulncheck ./...` clean.
   - `gofmt`-clean.
   - New / changed exported symbols have doc comments.
   - Tests: one-test-per-prod-function + `t.Run` per case (Rule 3).
   - Modified functions had their `// audit:ok` line removed (Rule 5).
   - No new runtime dependency; non-stdlib imports confined to `*_test.go`.
   - Integration tests considered for HTTP changes
     (`OPENHOLIDAYS_LIVE=1 go test -tags=integration ./...`).

## Out of scope

- Multiple/selectable PR templates.
- Any CI automation that parses the template.

## Verification

- No PR template exists before this change (greenfield).
- GitHub auto-populates the PR body from this file on new PRs against the repo.
