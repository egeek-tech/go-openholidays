# GitHub Issue Templates — Design

**Date:** 2026-05-31
**Status:** Approved (brainstorming)
**Scope:** Add GitHub Issue Forms for `egeek-tech/go-openholidays`.

## Goal

Give issue reporters a structured path that captures the information maintainers
actually need for a Go client library, and route non-bug traffic (questions, docs,
upstream-API problems) away from the bug tracker. The structured YAML bodies also
feed the repo's `gsd-inbox` triage workflow, which parses issues against project
templates.

## Decisions

- **Format:** GitHub **Issue Forms** (YAML, `.github/ISSUE_TEMPLATE/*.yml`). Chosen
  over classic Markdown templates for required-field enforcement, dropdowns, and
  machine-parseable bodies.
- **Artifacts:** three forms + one chooser config.
- **Labels:** reuse GitHub's default repo labels (`bug`, `enhancement`,
  `documentation`) — no label provisioning required.
- **Blank issues:** disabled, so every issue starts from a template.
- **Discussions:** enabled on the repo, so usage questions are routed there.

## Files

All under `.github/ISSUE_TEMPLATE/`:

| File | Title prefix | Labels |
|------|--------------|--------|
| `bug_report.yml` | `[Bug]: ` | `bug` |
| `feature_request.yml` | `[Feature]: ` | `enhancement` |
| `documentation.yml` | `[Docs]: ` | `documentation` |
| `config.yml` | — | — |

## Field design

Dropdown values are derived from the live public surface (verified against source on
2026-05-31), not guessed.

### `bug_report.yml`

- **Pre-submission checklist** (checkboxes, required): searched existing issues;
  reproduced on the latest release; this concerns the client library, not the
  upstream OpenHolidays API.
- **`go-openholidays` version** (input, required) — hint:
  `go list -m github.com/egeek-tech/go-openholidays`.
- **Go version** (input, required) — hint: `go version`.
- **OS / architecture** (input, optional) — `GOOS/GOARCH`.
- **Affected endpoint / area** (dropdown): `PublicHolidays`, `SchoolHolidays`,
  `Countries`, `Subdivisions`, `Languages`, `IsInRegion`,
  `Client construction / options`, `Retry & backoff`, `Cache`, `Request hook`,
  `cmd/ohcli`, `Other / not sure`.
- **Error sentinel** (dropdown, optional): `ErrInvalidCountry`, `ErrInvalidLanguage`,
  `ErrDateRangeTooLarge`, `ErrInvalidDateRange`, `ErrEmptyResponse`,
  `ErrResponseTooLarge`, `ErrMalformedResponse`, `None / not an error`, `Other`.
- **What happened** (textarea, required).
- **Expected behavior** (textarea, required).
- **Minimal reproduction** (textarea, `render: go`).
- **Relevant logs / output** (textarea, `render: shell`) — note: redact anything
  sensitive; never paste full response bodies.

### `feature_request.yml`

- **Pre-submission checklist** (checkbox, required): searched existing issues.
- **Problem / use case** (textarea, required).
- **Proposed API** (textarea, `render: go`, optional).
- **Alternatives considered** (textarea, optional).
- **Area** (dropdown): `New endpoint coverage`, `New option / helper`,
  `CLI (ohcli)`, `Performance`, `Docs`, `Other`.
- **Zero-dependency acknowledgment** (checkbox, optional, non-blocking): aware the
  library has a zero-runtime-dependency policy.

### `documentation.yml`

- **Where** (dropdown): `README`, `godoc / pkg.go.dev`, `CONTRIBUTING`,
  `example_test.go`, `CLI help`, `Other`.
- **Location / link** (input, optional).
- **What's wrong or missing** (textarea, required).
- **Suggested fix** (textarea, optional).

### `config.yml`

- `blank_issues_enabled: false`.
- Contact links:
  - **Questions & usage help** → GitHub Discussions.
  - **API reference** → pkg.go.dev for the module.
  - **Upstream OpenHolidays API** → openholidaysapi.org (for "is this an upstream
    data problem, not the client?").
  - **Contributing guide** → `CONTRIBUTING.md`.

## Out of scope

- Pull-request template (not requested).
- Custom/new labels beyond GitHub defaults.
- Any CI automation that consumes the forms.

## Verification

- `.github/ISSUE_TEMPLATE/` does not exist before this change (greenfield).
- After creation, YAML is valid and GitHub renders each form in the issue chooser.
