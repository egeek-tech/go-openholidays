# Phase 5: Distribution - Research

**Researched:** 2026-05-28
**Domain:** Go OSS library shipping — CLI dogfooding, fuzz/integration/benchmark tests, GitHub Actions CI matrix, golangci-lint v2, govulncheck, goreleaser v2 + Artifact Attestations, pkg.go.dev documentation, semver tagging.
**Confidence:** HIGH

## Summary

Phase 5 ships `v0.1.0`. Every external dependency in this phase has a single canonical answer that's already widely deployed across Go OSS in 2026 — there are no exotic choices to research. The bulk of the planning work is **wiring** (file layout, workflow YAML, doc-comment style, godoc Example placement) rather than **choosing**. CONTEXT.md already locked all of the user-facing decisions (CLI surface, output modes, exit-code policy, signing approach, Codecov, no Homebrew/cosign for v0.1.0).

The research surfaces three things the planner did not yet know from CONTEXT.md alone:
1. **Action version drift.** CONTEXT.md cites `actions/attest-build-provenance@v1` but the current major is `v4` (it became a thin wrapper around `actions/attest@v4` in version 4). `codecov/codecov-action@v4` is also superseded — `v5` is current and `v5+` is the supported version for the OIDC/tokenless path on public repos. The planner must pin to current majors, not the strings written in CONTEXT.md.
2. **`.golangci.yml_backup` is cross-project debris.** The file contains exemption rules for `internal/gdpr/*`, `internal/web/handlers/*`, `*_templ.go`, and `web/templates/layout/*` — none of which exist in this repo. Most of the file must be deleted, not "restored." Only the linters-default list and the `tagliatelle.case.rules` block survive; every `exclusions.rules.path:` entry is stale.
3. **`t.Context()` is Go 1.24+.** The CI matrix runs 1.23 / 1.24 / stable. Existing test code uses `context.Background()` exclusively — Phase 5 CLI tests must continue that pattern or the matrix breaks on 1.23.

**Primary recommendation:** Plan Phase 5 as 6–8 small plans (CLI core, CLI integration tests, fuzz + benchmarks, ci.yml + lint config, integration.yml, release.yml + .goreleaser.yaml, docs/README/examples, dependabot + finalization). The work is shallow and parallel-friendly — sub-plans can land in any order once Wave 0 lands `.golangci.yml` and validates the existing tree against it.

## User Constraints (from CONTEXT.md)

### Locked Decisions

- **D-01:** Module path stays `github.com/egeek-tech/go-openholidays` for v0.1.0. No vanity import path.
- **D-02:** Repo is already public — `pkg.go.dev` will index automatically after the `v0.1.0` tag is pushed from `main`. Plan must verify visibility as a pre-tag step in the release runbook.
- **D-03:** Three output modes: default text table, `--json` (typed-struct marshal), `--csv` (spreadsheet-friendly). Format is a single `--format=text|json|csv` flag with default `text`; or short forms `--json` / `--csv` that set it. Planner picks the exact spelling — both are acceptable.
- **D-04:** `--lang` defaults to `"en"` and falls back to first-available `LocalizedText` entry for missing translations. Matches Phase 3 `Holiday.NameFor("xx")` semantics — `Client.lang(country)` resolver is **not** in scope for v0.1.0.
- **D-05:** Errors print to **stderr** with the `ohcli: <message>` prefix. No ANSI color (plain text only).
- **D-06:** Exit code policy is **3-tier POSIX**:
  - `0` — success (including empty result sets)
  - `1` — runtime error (network failure, API error, validation rejection, decode error)
  - `2` — usage error (unknown subcommand, missing required arg, bad flag value)
- **D-07:** Empty results print a single line to **stderr** of the form `no <thing> found for <args>` (e.g. `ohcli: no school holidays found for PL 2025 (region PL-XX)`) and exit `0`. Stdout stays empty so pipes don't break.
- **D-08:** Unit tests use `httptest.NewServer` per case + fixtures from `testdata/`. **Zero live API calls in `go test`** (live API is integration-only, gated by `//go:build integration` + `OPENHOLIDAYS_LIVE=1`).
- **D-09:** Use **Codecov** (`codecov/codecov-action@v4` per CONTEXT.md, but see Source-current note below — pin v5). Token-free for public repos via the GitHub App. README badge format unchanged.
- **D-10:** Release artifacts emitted by goreleaser: linux/darwin/windows × amd64/arm64 (6 binaries) + `checksums.txt`. **No** source tarball, **no** Homebrew tap, **no** cosign signature.
- **D-11:** **GitHub Artifact Attestations** for SLSA-Level-3 provenance via `actions/attest-build-provenance@v1` (per CONTEXT.md, but current major is `v4` — see Source-current note below) after the goreleaser upload step in `release.yml`. Free for public repos, signed by Sigstore/Fulcio, no key management. Downstream verification: `gh attestation verify <binary> --repo egeek-tech/go-openholidays`.
- **D-12:** **CHANGELOG.md auto-generated from conventional commits** by goreleaser's release-notes block. Hand-curated `CHANGELOG.md` is **not** maintained. DOC-05 satisfied via the generated GitHub Release notes; if a top-level `CHANGELOG.md` is still desired, it's a thin pointer file.

**Source-current notes (action versions drift since CONTEXT.md was written):**
- `codecov/codecov-action@v4` → pin to **`@v5`** (v5 is the supported version with tokenless OIDC for public repos).
- `actions/attest-build-provenance@v1` → pin to **`@v4`** (v4 wraps `actions/attest@v4`; v1/v2/v3 are deprecated).
- `actions/setup-go@v5` → pin to **`@v5`** (current major; built-in module + build cache enabled by default).
- `actions/checkout@v4` → pin to **`@v4`** (current major).
- `goreleaser/goreleaser-action@v6` → pin to **`@v6`** (current; v7 also exists but v6 is the conservative pin).

This is a minor mechanical fix; the planner should record it as a clarification (e.g. CL-18) and proceed.

### Claude's Discretion

- **README badge set:** which badges (CI status, Codecov, Go Report Card, godoc, license) and their order is editorial — planner picks a sensible default.
- **`docs/design.md` shape:** ASCII diagrams vs Mermaid vs prose-only is open. Recommend ASCII for grep-ability and rendering on `pkg.go.dev`.
- **`CONTRIBUTING.md` depth:** whether to add issue/PR templates in `.github/`, code-of-conduct, or DCO sign-off is open. Recommend minimal v0.1.0 (just the dev loop + how to run unit/integration/fuzz tests).
- **`Example_*` strategy:** doctest-style with `// Output:` blocks (executable) vs prose-only. Recommend doctest where deterministic; `// Output:` omitted (compiled-only) where output depends on the live API.
- **Fuzz seed corpus:** hand-curated seeds in `testdata/fuzz/` vs auto-seeded from existing `testdata/*.json` fixtures vs both. Recommend "both" — load existing fixtures via `F.Add` in the fuzz harness + add 2–3 adversarial seeds per fuzz target.

### Deferred Ideas (OUT OF SCOPE)

- Homebrew tap, cosign signatures, source tarballs from goreleaser, `--bom` for CSV-with-BOM, `--format=ical`, issue/PR templates, DCO/sign-off, Code of Conduct, vanity import path, `--lang` env-var fallback (LANG / LC_ALL).

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| CLI-01 | `cmd/ohcli public <country> <year> [--lang xx]` text table | §"CLI Surface Decomposition" |
| CLI-02 | `cmd/ohcli school <country> <year> [--region CC-RR]` with subdivision filter | §"CLI Surface Decomposition" |
| CLI-03 | Stdlib `flag` only; library imported at module path | §"CLI Surface Decomposition" (stdlib `flag` subcommand pattern) |
| CLI-04 | `go install` builds clean on Linux + macOS in CI | §"CI Matrix" — added to `build` matrix legs |
| TEST-07 | `FuzzParseLocalizedText` + `FuzzUnmarshalHoliday` | §"Fuzz Tests" |
| TEST-08 | Integration tests (`//go:build integration` + `OPENHOLIDAYS_LIVE=1`) | §"Integration Tests" |
| TEST-09 | `Example_*` per public method | §"Documentation — pkg.go.dev rendering" |
| TEST-10 | Coverage ≥ 85% enforced in CI | §"Coverage Gate" |
| TEST-11 | Benchmarks for cold + cached `PublicHolidays(PL, 2025)` | §"Benchmarks" |
| CI-01 | GitHub Actions matrix Go 1.23/1.24/stable × ubuntu-latest | §"CI Matrix" |
| CI-02 | vet/build/test-race/lint/govulncheck steps | §"CI Matrix" + §"Lint Config" + §"govulncheck" |
| CI-03 | `go mod tidy && git diff --exit-code` | §"CI Matrix" (tidy-clean step) |
| CI-04 | Nightly `integration.yml` against live API | §"Nightly Integration Workflow" |
| CI-05 | `release.yml` on `v*` tag runs goreleaser | §"Release Workflow + goreleaser v2" |
| CI-06 | Dependabot for GitHub Actions versions | §"Dependabot" |
| CI-07 | Coverage badge wired (Codecov) | §"Codecov v5 wiring" |
| DOC-01 | `README.md` with badges + ≤20-line quickstart | §"Documentation — README.md" |
| DOC-02 | `doc.go` package overview with one runnable example | §"Documentation — doc.go" (current file exists; expand with Example) |
| DOC-03 | `example_test.go` one `Example_*` per public method | §"Documentation — pkg.go.dev rendering" |
| DOC-04 | `docs/design.md` short architecture doc | §"Documentation — docs/design.md" |
| DOC-05 | `CHANGELOG.md` keep-a-changelog (D-12 reroutes via goreleaser) | §"CHANGELOG strategy" |
| DOC-06 | `CONTRIBUTING.md` dev loop | §"Documentation — CONTRIBUTING.md" |
| DOC-07 | Every exported symbol has Go-style doc comment | §"Documentation — godoc coverage audit" |
| REL-01 | `pkg.go.dev` renders cleanly + examples runnable | §"REL-01 verification" |
| REL-02 | Go Report Card grade A | §"Go Report Card" |
| REL-03 | `v0.1.0` tag → goreleaser produces binaries on the Release | §"Release Workflow + goreleaser v2" |
| REL-04 | Module path owner confirmed in `go.mod` | §"REL-04 verification" (already satisfied — go.mod confirms) |

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|--------------|----------------|-----------|
| CLI dispatch + flag parsing | `cmd/ohcli` (separate `package main`) | — | Lives outside the library to dogfood the public surface (CLI-03). Stdlib `flag` only. |
| CLI text/JSON/CSV formatting | `cmd/ohcli` | — | Output rendering is a CLI concern, never library concern. Library returns `[]Holiday`; the CLI marshals. |
| HTTP transport (httptest) for CLI tests | `cmd/ohcli/*_test.go` | library `httptest.NewServer` pattern | CLI imports library at module path, plumbs `WithBaseURL(srv.URL)` into the test server (D-08). |
| Fuzz harness | `*_test.go` at library root | — | Both fuzz targets exercise library internals (`pickLocalized` and `Holiday.UnmarshalJSON`/`*Holiday` decode); they ship in the root package. |
| Integration tests | `*_integration_test.go` at library root | `//go:build integration` tag | Hits the live API; gated by env var so CI can run them on schedule (CI-04). |
| Benchmarks | `*_test.go` (named `Benchmark*`) at library root | `httptest.NewServer` | Benchmark uses an in-memory server to remove network variability from the measurement; not a live-API benchmark. |
| CI pipeline orchestration | `.github/workflows/ci.yml` | — | Single source of truth for matrix + lint + vuln + coverage + tidy steps. |
| Nightly integration | `.github/workflows/integration.yml` | — | Separate workflow so a flaky upstream does not red-X every push. |
| Release pipeline | `.github/workflows/release.yml` + `.goreleaser.yaml` | `actions/attest-build-provenance@v4` | goreleaser produces binaries + checksums; attestation step signs the checksum manifest after upload. |
| Lint configuration | `.golangci.yml` | — | Single file; project-scoped exclusions only (no inherited debris from other projects). |
| Documentation surface | `README.md` + `doc.go` + `example_test.go` + `docs/design.md` + `CONTRIBUTING.md` | godoc on every exported symbol | Multi-file split — `pkg.go.dev` reads `doc.go` + `example_test.go`; humans read `README.md` + `docs/design.md`. |

## Standard Stack

### Core (already in repo or stdlib-only)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go toolchain | 1.23 (module floor), 1.24, stable (CI matrix) | Compile / test runtime | Module floor pinned to 1.23 for `iter.Seq` per CONTEXT.md / `go.mod` line 3. Matrix tests forward-compat. [VERIFIED: `go.mod`] |
| stdlib `flag` | n/a | CLI argv parsing | Zero-dep policy mandates stdlib-only for CLI (CLI-03). [VERIFIED: CLAUDE.md] |
| stdlib `text/tabwriter` | n/a | Aligned text-table output | Stdlib answer for ohcli's text format. Documented since Go 1.0; no third-party challenger. [VERIFIED: stdlib] |
| stdlib `encoding/json` | n/a | `--json` output | Already the library's serializer; CLI uses `json.NewEncoder(os.Stdout).Encode(v)` for `--json`. [VERIFIED: stdlib] |
| stdlib `encoding/csv` | n/a | `--csv` output | RFC 4180–compliant; header row first, then one row per holiday. UTF-8 without BOM (CONTEXT.md specifics). [VERIFIED: stdlib] |
| `github.com/stretchr/testify` | v1.11.1 | Test assertions | Already in `go.sum`; primary per Gold Rule 3. [VERIFIED: `go.mod`] |

### Supporting (CI / release tooling, no Go imports added)

| Tool | Version | Purpose | When to Use |
|------|---------|---------|-------------|
| `golangci-lint` | **v2.12.2** locally; pin **`v2.12.x`** in CI via `golangci/golangci-lint-action@v7` | Aggregated linting | All lint runs (`ci.yml`). [VERIFIED: local `golangci-lint version`] |
| `govulncheck` | latest via `golang/govulncheck-action@v1` | Vulnerability scan | CI step + nightly integration job. [CITED: github.com/golang/govulncheck-action] |
| `goreleaser` | **v2.x** (latest stable) via `goreleaser/goreleaser-action@v6` | Release binary builder | `release.yml` on `v*` tag. [CITED: goreleaser.com/customization/builds] |
| `actions/checkout` | **`@v4`** | Code checkout | Every workflow. [VERIFIED: current major] |
| `actions/setup-go` | **`@v5`** | Go toolchain install + module cache | Every workflow. [CITED: github.com/actions/setup-go] |
| `codecov/codecov-action` | **`@v5`** | Coverage upload | Final step of `ci.yml`. [CITED: github.com/codecov/codecov-action] |
| `actions/attest-build-provenance` | **`@v4`** | SLSA-Level-3 provenance | After goreleaser in `release.yml`. [CITED: github.com/actions/attest-build-provenance] |
| `dependabot` | n/a (GitHub-native) | Action version updates | `.github/dependabot.yml`. [CITED: docs.github.com] |

### Alternatives Considered (and rejected per CONTEXT.md + zero-dep policy)

| Instead of | Could Use | Tradeoff / Why Not |
|------------|-----------|--------------------|
| Stdlib `flag` | `cobra`, `urfave/cli` | CLI is 2 subcommands; adds dep tree of 6–8 modules. CONTEXT.md / CLAUDE.md mandate stdlib. |
| `actions/attest-build-provenance` | `cosign` + own key | CONTEXT.md D-10 explicitly defers cosign to v0.2.x. Attestations are free, keyless, SLSA-L3. |
| `keep-a-changelog` hand-curated | goreleaser auto-changelog from conventional commits | CONTEXT.md D-12 picks auto. Existing commit log already follows Conventional Commits (`feat(04):`, `fix(04):`, `docs(03):`, `chore(04):`). |
| `--bom` for CSV | UTF-8 plain | Deferred per CONTEXT.md. Modern Excel reads UTF-8 without BOM. |
| Homebrew tap | Plain goreleaser archive | Deferred to v0.2.x. |

**Installation (developer setup):**
```bash
# All tools already installed locally:
go install github.com/golangci/golangci-lint/cmd/golangci-lint@v2.12.2
go install golang.org/x/vuln/cmd/govulncheck@latest
# goreleaser only needed in CI; local devs don't need it. If wanted:
go install github.com/goreleaser/goreleaser/v2@latest
```

**Version verification (run before plan):**
```bash
go version                                             # confirms 1.23+ module floor matches local toolchain
golangci-lint version                                  # confirms v2.x
govulncheck -version 2>/dev/null || which govulncheck  # confirms installed
```
[VERIFIED via local Bash, 2026-05-28: go 1.26.3, golangci-lint v2.12.2, govulncheck present at /home/rtkocz/go/bin/govulncheck.]

## Package Legitimacy Audit

This phase introduces **zero new Go imports** in production code (CLI uses stdlib only; library code does not gain any dependency). The only new "packages" are GitHub Actions used in workflow YAML; each is an official `actions/*`, `golang/*`, `goreleaser/*`, or `codecov/*` action with millions of users and signed by the publishing org.

| Item | Source | Disposition | Notes |
|------|--------|-------------|-------|
| `actions/checkout@v4` | github.com/actions/checkout | Approved | First-party GitHub action. |
| `actions/setup-go@v5` | github.com/actions/setup-go | Approved | First-party GitHub action. |
| `actions/attest-build-provenance@v4` | github.com/actions/attest-build-provenance | Approved | First-party GitHub action; wraps `actions/attest@v4`. |
| `golangci/golangci-lint-action@v7` | github.com/golangci/golangci-lint-action | Approved | Maintained by golangci-lint authors. Verify SHA-pin in `dependabot.yml` config. |
| `golang/govulncheck-action@v1` | github.com/golang/govulncheck-action | Approved | First-party Go team action. |
| `goreleaser/goreleaser-action@v6` | github.com/goreleaser/goreleaser-action | Approved | First-party goreleaser action. |
| `codecov/codecov-action@v5` | github.com/codecov/codecov-action | Approved | First-party Codecov action; tokenless OIDC supported. |

**Packages removed due to slopcheck `[SLOP]` verdict:** none — no Go packages added.
**Packages flagged as suspicious `[SUS]`:** none.

slopcheck was not run because **no new Go module dependencies are added in this phase**. All third-party items are GitHub Actions, which slopcheck does not scan; they are individually verified as first-party publishers above.

## Architecture Patterns

### System Architecture Diagram

```
                                       ┌─────────────────────────────────┐
                                       │       USER (terminal / CI)      │
                                       └─────────────┬───────────────────┘
                                                     │
                                          argv: ohcli public PL 2025 --json
                                                     │
                                                     ▼
                            ┌────────────────────────────────────────────┐
                            │              cmd/ohcli/main.go             │
                            │                                            │
                            │  1. Dispatch:   os.Args[1] → switch        │
                            │  2. Per-sub:    flag.NewFlagSet(...)       │
                            │  3. Validate:   positional args + flags    │
                            │     ├─ bad usage  → stderr + exit 2        │
                            │     └─ good       ↓                        │
                            └────────────────────────────────────────────┘
                                                     │
                                       ctx := context.Background()
                                                     │
                                                     ▼
                            ┌────────────────────────────────────────────┐
                            │         openholidays.NewClient(...)        │
                            │                                            │
                            │  WithUserAgent("ohcli/" + Version)         │
                            │  WithTimeout(15 * time.Second)             │
                            │  (no retry, no cache, no hook in v0.1.0)   │
                            └────────────────────────────────────────────┘
                                                     │
                                                     ▼
                            ┌────────────────────────────────────────────┐
                            │ c.PublicHolidays / c.SchoolHolidays /      │
                            │ c.Countries / c.Languages / c.Subdivisions │
                            │                                            │
                            │  RoundTripper chain (Phase 2 + 4):         │
                            │  hook → cache → logging → header → base    │
                            └────────────────────────────────────────────┘
                                                     │
                                  HTTP GET https://openholidaysapi.org/...
                                                     │
                                                     ▼
                            ┌────────────────────────────────────────────┐
                            │         decode → []Holiday (or err)        │
                            └────────────────────────────────────────────┘
                                                     │
                       ┌─────────────────────────────┴───────────────────────┐
                       │                                                     │
                  err  │                                                     │  result
                       ▼                                                     ▼
        ┌─────────────────────────┐                           ┌───────────────────────────────┐
        │  fmt.Fprintf(stderr,    │                           │  switch *formatFlag {         │
        │    "ohcli: %v\n", err)  │                           │    "text" → tabwriter.NewW... │
        │  exit 1                 │                           │    "json" → json.NewEncoder.. │
        └─────────────────────────┘                           │    "csv"  → csv.NewWriter ... │
                                                              │  }                            │
                                                              │  exit 0                       │
                                                              └───────────────────────────────┘

    If len(result) == 0:
       fmt.Fprintf(stderr, "ohcli: no public holidays found for %s %d\n", country, year)
       (stdout stays empty; exit 0)
```

### Recommended Project Structure (additions only; existing root layout preserved)

```
go-openholidays/
├── cmd/
│   └── ohcli/
│       ├── main.go                  # entrypoint, subcommand dispatch
│       ├── public.go                # `ohcli public` subcommand
│       ├── school.go                # `ohcli school` subcommand
│       ├── countries.go             # `ohcli countries` (optional — see §"CLI Surface")
│       ├── format.go                # text/json/csv renderers (shared)
│       └── main_test.go             # CLI tests against httptest.NewServer
├── docs/
│   └── design.md                    # short architecture doc (ASCII diagrams)
├── testdata/
│   └── fuzz/
│       ├── FuzzParseLocalizedText/  # auto-discovered by `go test -fuzz`
│       │   └── <seed-files>
│       └── FuzzUnmarshalHoliday/
│           └── <seed-files>
├── example_test.go                  # Example_* per public method (new)
├── fuzz_test.go                     # FuzzParseLocalizedText + FuzzUnmarshalHoliday (new)
├── integration_test.go              # //go:build integration (new)
├── bench_test.go                    # BenchmarkPublicHolidays_PL_2025 (new)
├── .github/
│   ├── workflows/
│   │   ├── ci.yml                   # matrix CI (new)
│   │   ├── integration.yml          # nightly live API (new)
│   │   └── release.yml              # tag → goreleaser (new)
│   └── dependabot.yml               # GH Actions version updates (new)
├── .goreleaser.yaml                 # goreleaser v2 config (new)
├── .golangci.yml                    # finalized lint config (new — replaces _backup)
├── README.md                        # quickstart + badges (new)
├── CONTRIBUTING.md                  # dev loop (new)
├── CHANGELOG.md                     # thin pointer file per D-12 (new)
└── (existing root *.go files unchanged)
```

### Pattern 1: Stdlib `flag` subcommand dispatch (CLI-03)

**What:** A small `os.Args` switch dispatches to per-subcommand `flag.FlagSet` instances. The library `cobra` is NOT used — zero-dep policy.

**When to use:** Always, for v0.1.0. Revisit if subcommand count grows past ~4 (CLAUDE.md §"Stack Patterns by Variant").

**Example:**
```go
// Source: stdlib pattern documented at pkg.go.dev/flag and used by `go` itself.
// File: cmd/ohcli/main.go

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
)

const usage = `usage: ohcli <command> [flags]

Commands:
  public   <country> <year> [--lang xx] [--format text|json|csv]
  school   <country> <year> [--region CC-RR] [--lang xx] [--format text|json|csv]
  countries [--lang xx] [--format text|json|csv]
  version
`

func main() {
	os.Exit(run(os.Args, os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr *os.File) int {
	if len(args) < 2 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	ctx := context.Background()
	switch args[1] {
	case "public":
		return cmdPublic(ctx, args[2:], stdout, stderr)
	case "school":
		return cmdSchool(ctx, args[2:], stdout, stderr)
	case "countries":
		return cmdCountries(ctx, args[2:], stdout, stderr)
	case "version":
		fmt.Fprintln(stdout, openholidays.Version)
		return 0
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "ohcli: unknown command %q\n%s", args[1], usage)
		return 2
	}
}
```
Each subcommand handler owns its own `flag.NewFlagSet("public", flag.ContinueOnError)`, sets `fs.SetOutput(stderr)` so flag errors flow to stderr per D-05, and returns an exit code. `run` is exported as an unexported function so tests can invoke it with a custom `args` slice and capture `stdout`/`stderr` without `os.Exit`.

### Pattern 2: Output rendering (text via `tabwriter`, JSON via `encoding/json`, CSV via `encoding/csv`)

**What:** A `format.go` file ships three renderers behind a single interface; the chosen renderer is selected by `--format` (or short flags `--json`/`--csv`).

**Example (text rendering with tabwriter):**
```go
// Source: stdlib pattern at pkg.go.dev/text/tabwriter
// File: cmd/ohcli/format.go

func renderText(w io.Writer, hs []openholidays.Holiday, lang string) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "DATE\tEND\tNAME\tNATIONWIDE\tTYPE")
	for _, h := range hs {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%t\t%s\n",
			h.StartDate, h.EndDate, h.NameFor(lang), h.Nationwide, h.Type)
	}
	return tw.Flush()
}
```

**Example (CSV with RFC 4180 + header row, no BOM):**
```go
// Source: stdlib pattern at pkg.go.dev/encoding/csv
func renderCSV(w io.Writer, hs []openholidays.Holiday, lang string) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"start_date", "end_date", "name", "nationwide", "type", "subdivision_codes"}); err != nil {
		return err
	}
	for _, h := range hs {
		codes := make([]string, 0, len(h.Subdivisions))
		for _, s := range h.Subdivisions {
			codes = append(codes, s.Code)
		}
		if err := cw.Write([]string{
			h.StartDate.String(),
			h.EndDate.String(),
			h.NameFor(lang),
			strconv.FormatBool(h.Nationwide),
			string(h.Type),
			strings.Join(codes, ";"),
		}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
```

**Example (JSON: stdlib `encoding/json` direct re-marshal):**
```go
func renderJSON(w io.Writer, hs []openholidays.Holiday) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(hs)
}
```

### Pattern 3: Fuzz target with seed corpus from existing fixtures

**What:** Fuzz tests use `F.Add` to register library fixtures as seeds plus 2–3 hand-curated adversarial seeds; the runtime fuzzer plus the auto-discovered `testdata/fuzz/<name>/` directory cover everything else.

**When to use:** Both fuzz targets (`FuzzParseLocalizedText`, `FuzzUnmarshalHoliday`) follow this pattern.

**Example:**
```go
// Source: go.dev/doc/security/fuzz/
// File: fuzz_test.go

func FuzzUnmarshalHoliday(f *testing.F) {
	// Load Phase 3 fixtures as initial seeds.
	for _, name := range []string{
		"testdata/public_holidays_pl_2025.json",
		"testdata/school_holidays_pl_2025.json",
	} {
		b, err := os.ReadFile(name)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}
	// Adversarial seeds: bytes that exercise known JSON edge cases.
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"id":"","startDate":"2025-01-01","endDate":"2024-12-31","type":"Public","name":[]}`)) // EndDate before StartDate
	f.Add([]byte(`[{"id":"x","startDate":null,"endDate":"2025-01-01","type":"","name":null}]`))

	f.Fuzz(func(t *testing.T, data []byte) {
		var hs []openholidays.Holiday
		// Decode must never panic, regardless of input. Errors are OK.
		_ = json.Unmarshal(data, &hs)
	})
}
```

### Pattern 4: Integration test with build tag + env-var guard

**What:** Integration tests live in files tagged `//go:build integration` and additionally check `os.Getenv("OPENHOLIDAYS_LIVE") == "1"`, so even running `go test -tags=integration` without the env var does the right thing in local development.

**Example:**
```go
// File: integration_test.go
//go:build integration

package openholidays

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestIntegration_PublicHolidays(t *testing.T) {
	if os.Getenv("OPENHOLIDAYS_LIVE") != "1" {
		t.Skip("OPENHOLIDAYS_LIVE not set; skipping live-API integration test")
	}
	c := NewClient(WithTimeout(15 * time.Second))
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	t.Run("14 public holidays for PL 2025", func(t *testing.T) {
		hs, err := c.PublicHolidays(ctx, PublicHolidaysRequest{
			CountryIsoCode: "PL",
			ValidFrom:      NewDate(2025, time.January, 1),
			ValidTo:        NewDate(2025, time.December, 31),
		})
		require.NoError(t, err)
		require.Len(t, hs, 14, "PL 2025 has 14 public holidays per Phase 3 golden fixture")
	})
}
```

### Pattern 5: Benchmark with in-memory HTTP server

**What:** Benchmarks use `httptest.NewServer` serving a captured fixture, not the live API. The point is to measure decode + transport overhead, not internet latency. Cold-vs-cached measured as separate sub-benchmarks.

**Example:**
```go
// File: bench_test.go
package openholidays

func BenchmarkClient_PublicHolidays(b *testing.B) {
	body, err := os.ReadFile("testdata/public_holidays_pl_2025.json")
	require.NoError(b, err)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	b.Cleanup(srv.Close)

	req := PublicHolidaysRequest{
		CountryIsoCode: "PL",
		ValidFrom:      NewDate(2025, time.January, 1),
		ValidTo:        NewDate(2025, time.December, 31),
	}

	b.Run("cold (no cache)", func(b *testing.B) {
		c := NewClient(WithBaseURL(srv.URL))
		b.Cleanup(func() { _ = c.Close() })
		ctx := context.Background()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := c.PublicHolidays(ctx, req)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("cached", func(b *testing.B) {
		c := NewClient(WithBaseURL(srv.URL), WithCache(time.Hour))
		b.Cleanup(func() { _ = c.Close() })
		ctx := context.Background()
		// Note: PublicHolidays is NOT cached by default (RESIL-07).
		// This sub-benchmark measures the cached path for Countries/Languages,
		// which the perf budget actually applies to. See §"Benchmarks" below
		// for resolved interpretation of the < 5 ms cached budget.
		_, _ = c.Countries(ctx, CountriesRequest{}) // warm
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := c.Countries(ctx, CountriesRequest{})
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
```

### Anti-Patterns to Avoid

- **`cobra` / `urfave/cli` for the demo CLI.** Pulls 6–8 transitive deps; the CLI is two subcommands. CONTEXT.md mandates stdlib-only.
- **`os.Exit` inside non-`main` packages.** Use exit codes returned up to `main`. The CLI tests must be able to assert exit codes without forking.
- **Logging response bodies in the CLI.** PROJECT.md says response bodies must never be logged above `Debug`. The CLI doesn't add `WithLogger` at all; library decides logging.
- **`fmt.Println` to stdout for errors.** D-05 sends errors to stderr; mixing them breaks CSV pipelines (`ohcli public PL 2025 --csv > out.csv` must contain only CSV in `out.csv`, no chatter).
- **Hand-curating `CHANGELOG.md` alongside goreleaser.** D-12 picks one. Two sources of truth diverge.
- **Skipping `--version`.** A CLI without `ohcli version` is universally annoying. Tiny addition; exists in `version.go`.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Aligned column output | Custom column-width math | `text/tabwriter` (stdlib) | Stdlib handles tabs and dynamic widths correctly. |
| CSV escaping | Custom string concatenation | `encoding/csv` (stdlib) | Stdlib quotes embedded commas/newlines per RFC 4180. |
| JSON marshaling | Custom string builder | `encoding/json` (stdlib) | Already the library's serializer. |
| Subcommand routing | Custom parser DSL | `flag.NewFlagSet` per subcommand + `os.Args[1]` switch | Standard pattern used by `go` itself; sufficient for two subcommands. |
| Release artifact production | Custom shell loop with `GOOS=… GOARCH=… go build` | `goreleaser` v2 | Handles cross-compile, checksums, archives, release-notes generation. |
| Signing release binaries | Custom keypair + `cosign` | `actions/attest-build-provenance@v4` | Free, keyless, SLSA-L3, no key management — exactly the v0.1.0 ask. |
| Conventional-commit changelog parsing | Custom regex pass | goreleaser `changelog.groups` + `filters.exclude` | Built-in regex grouping matches `feat:` / `fix:` / etc. |
| Fuzz seed corpus management | Custom file loader | `F.Add` + `testdata/fuzz/<FuzzName>/` directory | Auto-discovered by `go test -fuzz`. |
| GitHub Actions version bumps | Manual updates | Dependabot for `github-actions` ecosystem | Free, zero maintenance. |
| Coverage threshold gate | Custom awk pipeline | Codecov v5 + `codecov.yml` threshold | One source of truth; PR comments built-in. (Fallback: stdlib `go tool cover -func` + awk if Codecov is offline.) |

**Key insight:** Phase 5 is almost entirely "wire the right standard tool the right way." Every problem in this phase has been solved a thousand times by the Go OSS ecosystem; the planner's job is to specify the exact wiring, not invent.

## Runtime State Inventory

*Omitted: Phase 5 is greenfield additions (new files, new workflows, new tag). No existing strings, IDs, schedules, or stored state are being renamed or migrated.*

**Single exception worth noting:** the `Version` value in `version.go` is referenced by `User-Agent: go-openholidays/<version>` (Phase 2 transport) and by `ohcli version` (this phase). The tag-shipping flow assumes the source-of-truth value in `version.go` matches the git tag. Plan should include a CI guard (or release-runbook step) that asserts `version.go`'s `Version` equals the tag being released minus the `v` prefix.

## Common Pitfalls

### Pitfall 1: `t.Context()` breaks the Go 1.23 leg of the matrix

**What goes wrong:** `*testing.T.Context()` was added in Go 1.24. A test using `t.Context()` compiles on 1.24 / stable but fails on 1.23 with `t.Context undefined`.

**Why it happens:** Convenience method introduced post–module-floor.

**How to avoid:** Phase 5 tests use `context.Background()` or `context.WithCancel(context.Background())`, matching the existing codebase. The example in CONVENTIONS.md (`c.PublicHolidays(t.Context(), ...)`) is aspirational and only safe when the module floor bumps to 1.24.

**Warning signs:** A test compiles locally on Go 1.26.3 but the `go-version: 1.23.x` CI leg goes red with `undefined: t.Context`.

[VERIFIED: Go 1.24 release notes — `testing.T.Context` added in 1.24]

### Pitfall 2: `actions/attest-build-provenance@v1` is end-of-life

**What goes wrong:** Pinning `@v1` works today but receives no security updates; eventually GitHub deprecates retired action versions.

**Why it happens:** CONTEXT.md was written when v1 was current. The action has since shipped v2, v3, v4 — and starting at v4 it's a thin wrapper over `actions/attest@v4`.

**How to avoid:** Pin **`@v4`** (or `@v5` if released by the time the plan executes). Use `subject-checksums: ./dist/checksums.txt` input pointing at goreleaser's checksum manifest.

**Warning signs:** GitHub deprecation notice in the workflow run logs.

[CITED: github.com/actions/attest-build-provenance README — "As of version 4, `actions/attest-build-provenance` is simply a wrapper on top of `actions/attest`."]

### Pitfall 3: `.golangci.yml_backup` references nonexistent paths

**What goes wrong:** Restoring the backup verbatim makes `golangci-lint run` either complain about unknown paths or — worse — silently apply nothing (paths don't match).

**Why it happens:** The backup file was imported from another project. It exempts `internal/gdpr/*/db.go`, `internal/web/handlers/*`, `web/templates/layout/*`, `internal/app/app.go`, `internal/web/middleware/recover.go`, `*_templ.go` — none of these paths exist in this repo.

**How to avoid:** Strip every `exclusions.rules.path:` entry from `.golangci.yml_backup` when promoting it to `.golangci.yml`. Keep only:
- `version: "2"`
- `run.modules-download-mode: readonly`
- `linters.default: none`
- `linters.enable: [...]` — the curated list from the backup (verify each linter still works in v2.12)
- `linters.settings.gosec.excludes: [G101]`
- `linters.settings.revive.rules: - name: exported, disabled: true` (the project has comprehensive godoc; the `exported` linter is fine to leave on, but the backup disables it — defer to current behavior)
- `linters.settings.tagliatelle.case.rules: {json: snake, yaml: snake, toml: lower}` — but verify this matches actual JSON tags (the project uses **camelCase** wire tags like `isoCode`, `startDate`, not snake_case; **`tagliatelle` should be removed from the enable list entirely** or its case rules changed to `camel`)
- `exclusions.presets: [comments, common-false-positives, legacy, std-error-handling]`
- The `_test.go` G124 + G122 exemptions (those make sense and apply to this repo's tests)
- The `// unparam — disabled 2026-05-11` comment (preserve rationale for the historical decision)

**This rewrite is the single largest deliverable in the lint plan.** Treat it as "author a new `.golangci.yml`," not "copy and rename."

**Warning signs:** First `golangci-lint run` produces a wall of `unknown linter` or `path not found` errors.

[VERIFIED: read of `.golangci.yml_backup` 2026-05-28 — confirmed cross-project paths.]

### Pitfall 4: `tagliatelle` enabled with `json: snake` against camelCase wire format

**What goes wrong:** `tagliatelle` enforces a configured case for JSON tags. The current backup sets `json: snake`, but every Go struct in the repo (`Holiday`, `Country`, `Language`, `Subdivision`, etc.) uses upstream's **camelCase** JSON tags (e.g., `json:"isoCode"`, `json:"startDate"`). Enabling tagliatelle as-is would flag every existing type.

**Why it happens:** The backup's settings target a different project (the comments mention `internal/gdpr/export*.go` and OIDC ID-token claims which are snake_case).

**How to avoid:** Either (a) remove `tagliatelle` from `linters.enable` for v0.1.0, or (b) change `case.rules.json` to `camel`. Recommend (a) — there's no value in enforcing case style when upstream picks the case; the team's invariant is "match upstream verbatim," not "match a style guide."

**Warning signs:** `golangci-lint run` reports `tagliatelle` violations on every type.

[VERIFIED: cross-reference `types.go` tags (`json:"isoCode"`) against backup settings (`json: snake`) 2026-05-28.]

### Pitfall 5: Cached perf budget (< 5 ms) cannot be measured against `PublicHolidays`

**What goes wrong:** ROADMAP success criterion #3 reads "PublicHolidays(PL, 2025) < 500 ms cold and < 5 ms cached." But RESIL-07 (Phase 4 decision D-83) caches **only** `/Countries`, `/Languages`, `/Subdivisions` — holiday endpoints are never cached by default. So the "< 5 ms cached" target as literally written has no code path that hits it.

**Why it happens:** A bedrock decision (don't cache holiday endpoints to avoid stale-data risk on a domain whose entries change with government policy) collides with a perf-budget metric that assumed they would be cached.

**How to avoid:** Pick one interpretation in the benchmark plan:
- **(a)** Measure `Countries` cold and `Countries` cached as the canonical "< 500 ms / < 5 ms" pair, because that's the only endpoint pair the cache actually accelerates. Update success criterion #3 in CL-18.
- **(b)** Measure `PublicHolidays` cold (against in-memory httptest) and a hypothetical cached path by injecting `WithCache` (which silently no-ops for holiday endpoints). The "cached" measurement is then effectively the cold path again, which makes the test pointless.
- **Recommend (a).** Document the wording change as part of the same CL-18 that updates action versions.

**Warning signs:** Benchmark output shows "cached" run at roughly the same ns/op as "cold" run.

[VERIFIED: cross-reference Phase 4 D-83 (cacheable paths) against ROADMAP success criterion #3 2026-05-28.]

### Pitfall 6: `pkg.go.dev` index lag after first tag

**What goes wrong:** First-time tag push doesn't appear on `pkg.go.dev` for up to 30 minutes.

**Why it happens:** `pkg.go.dev` polls the module proxy; the proxy fetches on-demand when first requested.

**How to avoid:** After tagging, manually trigger the index by hitting `https://pkg.go.dev/github.com/egeek-tech/go-openholidays@v0.1.0` once in a browser. The proxy fetches the module within seconds.

**Warning signs:** Browsing `pkg.go.dev/github.com/egeek-tech/go-openholidays` returns "no results" immediately post-tag.

[CITED: pkg.go.dev/about — proxy-based indexing model.]

### Pitfall 7: Example output drift breaks `go test -run Example`

**What goes wrong:** An `Example_*` test with a `// Output:` block fails when the expected output drifts.

**Why it happens:** `Example_*` runs in `go test` and compares captured stdout against the `// Output:` block byte-for-byte. Any whitespace/order change breaks it.

**How to avoid:** Use `// Unordered output:` for maps/slices whose order varies; use commented-out `// Output:` block ("verifies compilation only, not output") for examples that depend on the live API or non-deterministic data. Per Claude's Discretion recommendation: doctest where deterministic, compile-only otherwise.

**Warning signs:** `go test -run Example` red, but the example "looks right" visually.

[CITED: go.dev/blog/examples — Testable Examples in Go.]

### Pitfall 8: `go install` for `cmd/ohcli` doesn't get a version-stamped binary

**What goes wrong:** A user running `go install github.com/egeek-tech/go-openholidays/cmd/ohcli@latest` gets a binary whose `ohcli version` prints `(devel)` instead of the module's tagged version.

**Why it happens:** Go's module download path uses `runtime/debug.ReadBuildInfo()` for version unless ldflags inject a value. The `version.go` constant is only set by goreleaser (which uses ldflags); `go install` bypasses goreleaser.

**How to avoid:** Implement `ohcli version` to read `debug.ReadBuildInfo().Main.Version` first, fall back to the `version.go` constant. That way `go install ...@v0.1.0` shows `v0.1.0`, and the goreleaser-built binary shows the ldflags value.

**Warning signs:** A `go install`-installed binary prints `(devel)` for `ohcli version`.

[VERIFIED: stdlib `runtime/debug.ReadBuildInfo` behavior — pkg.go.dev/runtime/debug.]

## Code Examples

### CLI subcommand handler (full pattern for `ohcli public`)

```go
// File: cmd/ohcli/public.go

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/egeek-tech/go-openholidays"
)

// cmdPublic implements `ohcli public <country> <year> [--lang xx] [--format ...]`.
// Returns the process exit code (0 success, 1 runtime error, 2 usage error).
func cmdPublic(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("public", flag.ContinueOnError)
	fs.SetOutput(stderr)
	lang := fs.String("lang", "en", "ISO 639-1 language code for localized names")
	format := fs.String("format", "text", "output format: text, json, or csv")
	asJSON := fs.Bool("json", false, "shortcut for --format=json")
	asCSV := fs.Bool("csv", false, "shortcut for --format=csv")

	if err := fs.Parse(args); err != nil {
		// fs already printed the error.
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(stderr, "ohcli: public requires <country> and <year>")
		fs.Usage()
		return 2
	}
	country := fs.Arg(0)
	year, err := strconv.Atoi(fs.Arg(1))
	if err != nil || year < 1900 || year > 2100 {
		fmt.Fprintf(stderr, "ohcli: invalid year %q\n", fs.Arg(1))
		return 2
	}
	switch {
	case *asJSON:
		*format = "json"
	case *asCSV:
		*format = "csv"
	}
	if *format != "text" && *format != "json" && *format != "csv" {
		fmt.Fprintf(stderr, "ohcli: invalid format %q (want text|json|csv)\n", *format)
		return 2
	}

	c := openholidays.NewClient(
		openholidays.WithUserAgent("ohcli/" + openholidays.Version),
		openholidays.WithTimeout(15*time.Second),
	)
	defer c.Close()

	req := openholidays.PublicHolidaysRequest{
		CountryIsoCode: country,
		ValidFrom:      openholidays.NewDate(year, time.January, 1),
		ValidTo:        openholidays.NewDate(year, time.December, 31),
		LanguageIsoCode: *lang,
	}
	hs, err := c.PublicHolidays(ctx, req)
	if err != nil {
		fmt.Fprintf(stderr, "ohcli: %v\n", err)
		return 1
	}
	if len(hs) == 0 {
		fmt.Fprintf(stderr, "ohcli: no public holidays found for %s %d\n", country, year)
		return 0
	}
	if err := render(stdout, hs, *lang, *format); err != nil {
		fmt.Fprintf(stderr, "ohcli: %v\n", err)
		return 1
	}
	return 0
}
```

### Example_* (the canonical pkg.go.dev runnable doctest)

```go
// File: example_test.go (lives in the openholidays package).
// pkg.go.dev attaches this to the Client.PublicHolidays method documentation.

package openholidays_test

import (
	"context"
	"fmt"
	"time"

	"github.com/egeek-tech/go-openholidays"
)

// ExampleClient_PublicHolidays demonstrates the canonical
// "fetch one year of public holidays" call against a Client.
//
// The // Output: block is intentionally omitted because the live API would
// be hit at `go test -run Example` time; the example is therefore compile-only
// (the test still verifies that the symbols and types match the public surface).
func ExampleClient_PublicHolidays() {
	c := openholidays.NewClient()
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	hs, err := c.PublicHolidays(ctx, openholidays.PublicHolidaysRequest{
		CountryIsoCode: "PL",
		ValidFrom:      openholidays.NewDate(2025, time.January, 1),
		ValidTo:        openholidays.NewDate(2025, time.December, 31),
	})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("got %d Polish public holidays\n", len(hs))
}

// ExampleHoliday_NameFor demonstrates language fallback when the requested
// language is not present in the LocalizedText slice.
func ExampleHoliday_NameFor() {
	h := openholidays.Holiday{
		Name: []openholidays.LocalizedText{
			{Language: "pl", Text: "Boże Narodzenie"},
			{Language: "en", Text: "Christmas Day"},
		},
	}
	fmt.Println(h.NameFor("xx")) // language not found — falls back to first entry
	// Output: Boże Narodzenie
}
```

### `.golangci.yml` (finalized — clean cut of `.golangci.yml_backup`)

```yaml
# File: .golangci.yml — golangci-lint v2 configuration for go-openholidays.
# This is the canonical lint config; .golangci.yml_backup may be deleted once
# this file passes a clean `golangci-lint run` against the existing tree.
version: "2"
run:
  modules-download-mode: readonly
linters:
  default: none
  enable:
    # PROJECT.md / CLAUDE.md required (the 6 mandated linters):
    - govet
    - errcheck
    - staticcheck
    - gosec
    - revive
    - gocritic
    # Curated additions retained from .golangci.yml_backup that proved
    # useful during Phases 1-4:
    - asciicheck
    - copyloopvar
    - durationcheck
    - contextcheck
    - errchkjson
    - errname
    - fatcontext
    - gomoddirectives
    - ineffassign
    - intrange
    - mirror
    - musttag
    - nilerr
    - sloglint
    - testifylint
    - thelper
    - unconvert
    # unparam: disabled 2026-05-11. 13 findings were split between real code
    # smells and test-helper future-flex parameters. Needs case-by-case
    # judgment, not a blanket fix.
    - unused
    - usestdlibvars
    - usetesting
    - wastedassign
    # tagliatelle: NOT enabled. Upstream OpenHolidays wire format is camelCase
    # (isoCode, startDate, etc.); enforcing a case scheme on JSON tags would
    # require either the wrong setting (snake) or a setting that matches
    # upstream (camel) — and the latter adds no value over "match upstream
    # verbatim." See Phase 5 RESEARCH.md Pitfall 4.
  settings:
    gosec:
      excludes:
        - G101  # hard-coded credentials false-positive on testdata fixture paths
  exclusions:
    generated: lax
    presets:
      - comments
      - common-false-positives
      - legacy
      - std-error-handling
    rules:
      # G124: tests construct request cookies via httptest. G124 warns about
      # missing Secure/HttpOnly/SameSite, which are response-side flags;
      # request cookies don't carry them.
      - linters:
          - gosec
        text: "G124"
        path: _test\.go
      # G122: integration guardrail tests walk a known repo subtree on a
      # fresh CI checkout (no untrusted symlinks possible). The TOCTOU
      # surface gosec warns about doesn't apply here.
      - linters:
          - gosec
        text: "G122"
        path: _test\.go
formatters:
  exclusions:
    generated: lax
```

### `.goreleaser.yaml` (v2 schema, drop-in)

```yaml
# File: .goreleaser.yaml
version: 2

project_name: go-openholidays

before:
  hooks:
    - go mod tidy

builds:
  - id: ohcli
    main: ./cmd/ohcli
    binary: ohcli
    env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
      - windows
    goarch:
      - amd64
      - arm64
    flags:
      - -trimpath
    ldflags:
      - -s -w
      - -X github.com/egeek-tech/go-openholidays.Version={{.Version}}
    mod_timestamp: "{{ .CommitTimestamp }}"

archives:
  - id: ohcli-archive
    name_template: "ohcli_{{.Version}}_{{.Os}}_{{.Arch}}"
    format_overrides:
      - goos: windows
        format: zip
    files:
      - LICENSE
      - README.md
      - CHANGELOG.md

checksum:
  name_template: "checksums.txt"
  algorithm: sha256

release:
  github:
    owner: egeek-tech
    name: go-openholidays
  draft: false
  prerelease: auto

changelog:
  sort: asc
  use: github
  groups:
    - title: Features
      regexp: '^.*?feat(\([[:word:]]+\))??!?:.+$'
      order: 0
    - title: Bug Fixes
      regexp: '^.*?fix(\([[:word:]]+\))??!?:.+$'
      order: 1
    - title: Performance
      regexp: '^.*?perf(\([[:word:]]+\))??!?:.+$'
      order: 2
    - title: Other
      order: 999
  filters:
    exclude:
      - '^chore:'
      - '^chore\('
      - '^docs:'
      - '^docs\('
      - '^test:'
      - '^test\('
      - '^style:'
      - '^style\('
      - '^refactor:'
      - '^refactor\('
      - '^ci:'
      - '^ci\('
      - typo
      - merge conflict
```
[CITED: goreleaser.com/customization/builds/go/, goreleaser.com/customization/changelog/]

### `.github/workflows/ci.yml`

```yaml
# File: .github/workflows/ci.yml
name: CI

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

permissions:
  contents: read

jobs:
  test:
    strategy:
      fail-fast: false
      matrix:
        go-version: ["1.23.x", "1.24.x", "stable"]
        os: [ubuntu-latest]
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: ${{ matrix.go-version }}
          check-latest: true

      - name: Verify go.mod tidy
        run: |
          go mod tidy
          git diff --exit-code -- go.mod go.sum
        # Only run tidy verification on the canonical Go version to avoid
        # tidy-output drift between matrix legs.
        if: matrix.go-version == 'stable'

      - name: go vet
        run: go vet ./...

      - name: go build
        run: go build ./...

      - name: go test (race + cover)
        run: go test -race -coverprofile=coverage.out -covermode=atomic ./...

      - name: Coverage gate (>= 85%)
        run: |
          pct=$(go tool cover -func=coverage.out | awk '/^total:/ {gsub("%",""); print $3}')
          echo "coverage: $pct%"
          awk -v p="$pct" 'BEGIN { if (p+0 < 85.0) { print "coverage below 85%"; exit 1 } }'
        if: matrix.go-version == 'stable'

      - name: Upload to Codecov
        if: matrix.go-version == 'stable'
        uses: codecov/codecov-action@v5
        with:
          files: ./coverage.out
          fail_ci_if_error: false  # CI does not depend on Codecov uptime
          slug: egeek-tech/go-openholidays
          use_oidc: true  # tokenless OIDC for public repos

  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: stable
      - uses: golangci/golangci-lint-action@v7
        with:
          version: v2.12.2

  vuln:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: stable
      - uses: golang/govulncheck-action@v1
        with:
          go-package: ./...

permissions:
  id-token: write  # required for codecov-action use_oidc: true
  contents: read
```

### `.github/workflows/integration.yml`

```yaml
# File: .github/workflows/integration.yml
name: Integration (nightly)

on:
  schedule:
    - cron: "0 3 * * *"  # 03:00 UTC daily
  workflow_dispatch:

permissions:
  contents: read

jobs:
  integration:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: stable

      - name: Run integration tests
        env:
          OPENHOLIDAYS_LIVE: "1"
        run: go test -tags=integration -count=1 -timeout=5m ./...
```

### `.github/workflows/release.yml`

```yaml
# File: .github/workflows/release.yml
name: Release

on:
  push:
    tags:
      - "v*"

permissions:
  contents: write       # write release notes
  id-token: write       # sigstore attestation
  attestations: write   # attach attestations to artifacts

jobs:
  goreleaser:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0  # required for goreleaser changelog generation

      - uses: actions/setup-go@v5
        with:
          go-version: stable

      - name: Run goreleaser
        uses: goreleaser/goreleaser-action@v6
        with:
          distribution: goreleaser
          version: "~> v2"
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}

      - name: Attest binaries via subject-checksums
        uses: actions/attest-build-provenance@v4
        with:
          subject-checksums: ./dist/checksums.txt
```

### `.github/dependabot.yml`

```yaml
# File: .github/dependabot.yml
version: 2
updates:
  - package-ecosystem: github-actions
    directory: /
    schedule:
      interval: weekly
    open-pull-requests-limit: 5
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `actions/attest-build-provenance@v1` | `@v4` (thin wrapper around `actions/attest@v4`) | early 2026 | Same UX; supported version. |
| `codecov/codecov-action@v4` | `@v5` (OIDC + tokenless on public repos) | 2025 | OIDC removes the need for `secrets.CODECOV_TOKEN` on public repos. |
| `actions/setup-go@v4` | `@v5` (built-in module + build cache) | 2024 | Drops the need for explicit `actions/cache@v3` step. |
| `golangci-lint v1.x` with `disable-all: true` | `v2.x` with `linters.default: none` | March 2025 | Config schema rewrite; v1 keys silently ignored. Migration: `golangci-lint migrate`. |
| `goreleaser` v1 schema | v2 schema (`version: 2` directive required) | Sept 2024 | v1 configs without `version: 2` produce a deprecation warning. |
| Hand-rolled `CHANGELOG.md` | goreleaser-generated from conventional commits | trend 2024-2025 | Single source of truth; release notes attached to GitHub Release. |
| `cosign` for OSS Go binary signing | GitHub Artifact Attestations (`actions/attest-*@v4`) | mid-2024 | Keyless; no per-project Sigstore wiring; verifiable via `gh attestation verify`. |
| `t.Parallel()` + per-iter `tc := tc` shadow | `t.Parallel()` only (Go 1.22+ loop scoping) | Go 1.22 | The shadow is now redundant and `copyloopvar` flags it. |

**Deprecated/outdated:**
- `actions/attest-build-provenance@v1`, `@v2`, `@v3` — replaced by `@v4` (wrapper).
- `codecov-action@v3` and earlier — token-only; `v4`/`v5` add OIDC.
- `golangci-lint v1.x` configurations — schema rewrite in v2.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `cmd/ohcli countries` (3rd subcommand) is in scope | §"CLI Surface Decomposition" | If user wanted only `public` + `school` per ROADMAP literal text, the `countries` subcommand is wasted work. Recommend keeping it (10 LoC) — it dogfoods the reference endpoints and helps debug `--lang` resolution. |
| A2 | The `< 5 ms cached` perf budget is reinterpreted to apply to `Countries`, not `PublicHolidays` | §"Common Pitfalls" → Pitfall 5 | If user insists on literal text, the test cannot pass — `PublicHolidays` is uncached. Recorded as CL-18 candidate. |
| A3 | `actions/attest-build-provenance@v4` is the right pin (not `@v1` per CONTEXT.md) | §"User Constraints" → Source-current notes | If a year passes and `@v5` is released, the planner should rebump. Low-risk: action versioning is well-behaved. |
| A4 | `codecov/codecov-action@v5` is the right pin (not `@v4` per CONTEXT.md), with `use_oidc: true` | §"User Constraints" → Source-current notes | `@v4` still works for public repos via the GitHub App; `@v5` is just safer / has more features. |
| A5 | The `.golangci.yml_backup` rewrite is "delete most of it, keep linters list + tagliatelle removal" | §"Common Pitfalls" → Pitfalls 3, 4 | If the team wants tagliatelle re-enabled with `camel` rules, that's a 1-line config change. Recommend leaving disabled — adds noise without value. |
| A6 | `go install ./cmd/ohcli` reads `runtime/debug.ReadBuildInfo` for `version` output, then falls back to `version.go`'s `Version` const | §"Common Pitfalls" → Pitfall 8 | If the team only cares about goreleaser-built binaries showing version, this whole concern is moot. Minimal extra code (5 lines). |
| A7 | `pkg.go.dev` index lag after first tag (up to 30 min) is the user's problem, not a CI concern | §"Common Pitfalls" → Pitfall 6 | If the release-runbook must "verify pkg.go.dev visibility" within minutes of tagging, add a manual curl/poll step. |
| A8 | The fuzz seed corpus loads existing `testdata/*.json` fixtures via `F.Add` + 2-3 adversarial seeds | §"Pattern 3" | If the team wants only hand-curated seeds, drop the `F.Add` loop. The corpus discovery via `testdata/fuzz/<name>/` directory still works. |

**No claim is `[ASSUMED]` in the strong sense of "I made it up";** every item above is either a recommendation pending user confirmation or a recent ecosystem fact that may want re-verification by the planner. The planner should record A2, A3, A4 as a single CL-18 in PROJECT.md ("Phase 5 action-version pinning + cached perf budget rewording").

## Open Questions

1. **`commitlint` workflow vs CONTRIBUTING.md note for conventional-commit enforcement.**
   - What we know: existing commit history already follows the convention; goreleaser parses it cleanly.
   - What's unclear: whether to add `.github/workflows/commitlint.yml` (would gate PR titles) or just document the convention in `CONTRIBUTING.md`.
   - Recommendation: **CONTRIBUTING.md note for v0.1.0**. Adding a commitlint workflow is two more YAML files and a Node.js dep — feels heavy for a 1-person OSS lib. Revisit in v0.2.x.

2. **Whether `.golangci.yml_backup` should be deleted after `.golangci.yml` lands.**
   - What we know: the backup contains no useful project-specific knowledge (all carve-outs are cross-project debris); the surviving content is the linters list which is already in the recommended `.golangci.yml`.
   - What's unclear: whether to delete it as part of this phase or leave as historical artifact.
   - Recommendation: **delete in this phase**. The `.planning/codebase/CONVENTIONS.md` already records the "unparam disabled 2026-05-11" rationale; the comment in `.golangci.yml` carries forward.

3. **CHANGELOG.md placeholder file content.**
   - What we know: D-12 routes the actual change log through goreleaser-generated GitHub Release notes.
   - What's unclear: whether the placeholder `CHANGELOG.md` file should exist at all (vs no file) and, if it exists, what it contains.
   - Recommendation: **ship a one-line `CHANGELOG.md`** pointing to GitHub Releases. Existence satisfies DOC-05 literally; a missing file would cause routine "where's the changelog?" issues from users.

   ```markdown
   # Changelog

   Release notes for go-openholidays are generated from conventional commits
   and published on each GitHub Release.

   See: https://github.com/egeek-tech/go-openholidays/releases
   ```

4. **README badge order.**
   - What we know: user said this is editorial.
   - Recommendation order: `[CI badge] [coverage badge] [go report card badge] [godoc badge] [license badge]`. Industry-standard order seen on every major Go OSS README. Skip the "made-with-Go" badge — clutter.

5. **`Example_*` placement: in `example_test.go` (root) vs split per-file (e.g., `client_example_test.go`).**
   - What we know: `pkg.go.dev` doesn't care about file split.
   - Recommendation: **single `example_test.go` at root** for v0.1.0. ~10 examples in one file is easier to read than 6 example files. Split when count > 15 (won't happen in v0.1.0).

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| `go` toolchain | Build, test, vet | ✓ | 1.26.3 (local); CI provides 1.23/1.24/stable | — |
| `golangci-lint` | Lint step | ✓ | v2.12.2 | CI installs via `golangci/golangci-lint-action@v7` |
| `govulncheck` | Vuln scan | ✓ | latest on `$PATH` | CI installs via `golang/govulncheck-action@v1` |
| `goreleaser` | Release pipeline | ✗ | — | CI provides via `goreleaser/goreleaser-action@v6`. Local devs don't need it. |
| `gh` (GitHub CLI) | Attestation verification | not checked | — | Documented for end-users; not a dev/CI requirement. |
| Live `openholidaysapi.org` | Integration tests only | ✓ (presumed; not pinged in this research) | — | Integration test skips when `OPENHOLIDAYS_LIVE != "1"`. |

**Missing dependencies with no fallback:** none. The phase is buildable and testable on the current machine; `goreleaser` is the only tool absent locally, and only CI uses it.

**Missing dependencies with fallback:** none active.

[VERIFIED via Bash 2026-05-28: `go version`, `golangci-lint version`, `which govulncheck`, `which goreleaser` (not present).]

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` + `github.com/stretchr/testify` v1.11.1 |
| Config file | none required (`go test` is configuration-free) |
| Quick run command | `go test ./...` |
| Full suite command | `go test -race -count=1 -coverprofile=coverage.out ./...` |
| Fuzz quick run | `go test -run Fuzz -fuzz=FuzzParseLocalizedText -fuzztime=30s` |
| Integration run | `OPENHOLIDAYS_LIVE=1 go test -tags=integration -count=1 ./...` |
| Bench run | `go test -bench=. -benchmem -run=^$ ./...` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| CLI-01 | `ohcli public PL 2025` text-table output | unit (httptest) | `go test ./cmd/ohcli/ -run TestCmdPublic` | ❌ Wave 0 |
| CLI-02 | `ohcli school PL 2025 --region PL-SL` subdivision filter | unit (httptest) | `go test ./cmd/ohcli/ -run TestCmdSchool` | ❌ Wave 0 |
| CLI-03 | `go list -m -f '{{.Path}}' ./cmd/ohcli` is the module path; zero non-stdlib imports outside library | static check (`go vet`, `go list -deps`) | `go list -deps ./cmd/ohcli/ \| grep -v '^github.com/egeek-tech/go-openholidays' \| grep -v '^[a-z]'` (heuristic) | ❌ Wave 0 |
| CLI-04 | `go install ./cmd/ohcli` builds clean on Linux + macOS | CI matrix step | added to `ci.yml` `go build ./...` legs | ❌ Wave 0 (ci.yml plan) |
| TEST-07 | `FuzzParseLocalizedText` runs 60s without panic | fuzz | `go test -run Fuzz -fuzz=FuzzParseLocalizedText -fuzztime=60s ./...` | ❌ Wave 0 |
| TEST-07 | `FuzzUnmarshalHoliday` runs 60s without panic | fuzz | `go test -run Fuzz -fuzz=FuzzUnmarshalHoliday -fuzztime=60s ./...` | ❌ Wave 0 |
| TEST-08 | Integration tests against live API | integration (gated) | `OPENHOLIDAYS_LIVE=1 go test -tags=integration ./...` | ❌ Wave 0 |
| TEST-09 | Every public method has at least one `Example_*` | doc-test | `go test -run Example ./...` | ❌ Wave 0 |
| TEST-10 | Coverage ≥ 85% | coverage gate | `go tool cover -func=coverage.out \| awk '/^total:/'` then awk threshold | ❌ Wave 0 (ci.yml step) |
| TEST-11 | `PublicHolidays(PL, 2025) < 500 ms cold` | bench (in-memory httptest) | `go test -bench=BenchmarkClient_PublicHolidays -benchtime=10x ./...` | ❌ Wave 0 |
| TEST-11 | `Countries cached < 5 ms` (A2 reinterpretation) | bench | `go test -bench=BenchmarkClient_Countries -benchtime=10x ./...` | ❌ Wave 0 |
| CI-01 | Matrix Go 1.23/1.24/stable × ubuntu-latest | workflow execution | actual `ci.yml` run on PR | ❌ Wave 0 (ci.yml plan) |
| CI-02 | All steps green (vet, build, test-race, lint, vuln) | workflow execution | actual `ci.yml` run | ❌ Wave 0 |
| CI-03 | `go mod tidy` clean | workflow step | `go mod tidy && git diff --exit-code` | ❌ Wave 0 |
| CI-04 | Nightly integration runs against live API | workflow execution (manual + scheduled) | `gh workflow run integration.yml` | ❌ Wave 0 |
| CI-05 | `release.yml` on `v*` tag produces 6 binaries + checksums + attestation | workflow execution (real tag) | tag `v0.1.0-rc.1` to a test branch first to dry-run | ❌ Wave 0 |
| CI-06 | Dependabot configured for actions ecosystem | static check | file exists at `.github/dependabot.yml` with correct content | ❌ Wave 0 |
| CI-07 | Codecov badge resolves to a coverage page | runtime check | `curl -fsSL https://codecov.io/gh/egeek-tech/go-openholidays/branch/main/graph/badge.svg` | ❌ Wave 0 (post-first-PR) |
| DOC-01 | README quickstart compiles | doc-test | manually copy quickstart into a temp main.go, `go build` it | ❌ Wave 0 |
| DOC-02 | `doc.go` has runnable example for `Client.PublicHolidays` | doc-test | `go test -run ExampleClient_PublicHolidays` | partial (`doc.go` exists, no Example yet) |
| DOC-03 | One `Example_*` per public method | doc-test | per-Example test under `go test -run Example` | ❌ Wave 0 |
| DOC-04 | `docs/design.md` exists and renders | static check | file exists at `docs/design.md` | ❌ Wave 0 |
| DOC-05 | `CHANGELOG.md` exists | static check | file exists at `CHANGELOG.md` | ❌ Wave 0 |
| DOC-06 | `CONTRIBUTING.md` documents dev loop | static check + manual review | file exists | ❌ Wave 0 |
| DOC-07 | Every exported symbol has Go-style doc comment | lint (`revive` rule `exported`) | `golangci-lint run` with `revive.rules.exported` enabled (currently disabled per backup) | partial (godoc coverage is high; need audit) |
| REL-01 | `pkg.go.dev` renders the package cleanly | runtime check (post-tag) | `curl -fsSL https://pkg.go.dev/github.com/egeek-tech/go-openholidays@v0.1.0` returns 200 + no "no documentation" banner | ❌ Wave 0 (post-tag) |
| REL-02 | Go Report Card grade A on first scan | runtime check (post-tag) | `curl -fsSL https://goreportcard.com/badge/github.com/egeek-tech/go-openholidays` returns a green badge | ❌ Wave 0 (post-tag) |
| REL-03 | `v0.1.0` tag pushed; release artifacts attached | runtime check | `gh release view v0.1.0 --json assets` lists 6 binaries + `checksums.txt` | ❌ Wave 0 (post-tag) |
| REL-04 | `go.mod` first line is the confirmed module path | static check | `head -1 go.mod` equals `module github.com/egeek-tech/go-openholidays` | ✅ already satisfied (verified 2026-05-28) |

### Sampling Rate

- **Per task commit:** `go test ./...` (no `-race`, no fuzz, no integration) — ~5-10s.
- **Per wave merge:** `go test -race -count=1 -coverprofile=coverage.out ./...` + `golangci-lint run ./...` + `govulncheck ./...` + `go test -run Example ./...` — full local CI parity.
- **Phase gate:** full suite green; coverage ≥ 85%; one tag-shipping dry-run via `git tag -d v0.1.0-rc.1` + `git push --tags` to a side branch to exercise `release.yml` end-to-end before tagging `v0.1.0` on main.

### Wave 0 Gaps

- [ ] `cmd/ohcli/main.go`, `cmd/ohcli/main_test.go`, `cmd/ohcli/public.go`, `cmd/ohcli/school.go`, `cmd/ohcli/format.go` (+ per-file test files) — covers CLI-01..04.
- [ ] `fuzz_test.go` at repo root — covers TEST-07.
- [ ] `integration_test.go` at repo root — covers TEST-08.
- [ ] `example_test.go` at repo root — covers TEST-09 and DOC-03.
- [ ] `bench_test.go` at repo root — covers TEST-11.
- [ ] `.github/workflows/ci.yml` — covers CI-01..03, CI-07, TEST-10.
- [ ] `.github/workflows/integration.yml` — covers CI-04.
- [ ] `.github/workflows/release.yml` + `.goreleaser.yaml` — covers CI-05, REL-03.
- [ ] `.github/dependabot.yml` — covers CI-06.
- [ ] `.golangci.yml` (replacing `_backup`) — covers CI-02 lint leg.
- [ ] `README.md` — covers DOC-01.
- [ ] `docs/design.md` — covers DOC-04.
- [ ] `CHANGELOG.md` (thin pointer file) — covers DOC-05.
- [ ] `CONTRIBUTING.md` — covers DOC-06.
- [ ] Godoc audit + completion (most types already documented from Phases 1–4) — covers DOC-07.

## Security Domain

`security_enforcement` is not explicitly set to `false` in `.planning/config.json`, so this section is included.

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|------------------|
| V2 Authentication | no | OpenHolidays API has no auth; library has no credential handling. |
| V3 Session Management | no | Stateless API client. |
| V4 Access Control | no | No multi-tenant logic. |
| V5 Input Validation | yes | Phase 1 validators (`validateCountry`, `validateLanguage`, `validateDateRange`) + CLI flag parsing (positional + flag values), all of which reject malformed input client-side. |
| V6 Cryptography | partial | No application-level crypto; release artifacts signed via GitHub Artifact Attestations (Sigstore/Fulcio, transparency-log-backed). |
| V7 Error Handling and Logging | yes | Per ERR-04, error messages and logs never contain credentials or full response bodies. Phase 5 CLI inherits this; `ohcli` only emits `err.Error()` strings, which by Phase 1–4 design carry no sensitive data. |
| V11 Business Logic | n/a | Library, not an application with business logic. |
| V12 Files & Resources | yes | CLI reads no local files at runtime (output is stdout); fixtures and test data are read-only. No path traversal risk. |
| V14 Configuration | yes | `.github/dependabot.yml` for action version freshness; `govulncheck` for stdlib vulnerability surface. |

### Known Threat Patterns for the Phase 5 Stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Compromised GitHub Action (slopsquatted or stolen) | Tampering | Pin to major version + Dependabot updates; `actions/*`, `goreleaser/*`, `codecov/*`, `golang/*` are first-party. Consider SHA-pinning in v0.2.x. |
| Supply-chain injection via Go module proxy | Tampering | Zero runtime deps eliminates the surface for runtime code; test-only deps (testify) are pre-approved and pinned. `go mod tidy` clean step in CI catches drift. |
| Hostile API returning > 10 MiB body | DoS | Already mitigated by `io.LimitReader` cap (Phase 2 TRANS-02); CLI inherits the cap. |
| Crafted JSON that panics decoder | DoS | `FuzzParseLocalizedText` + `FuzzUnmarshalHoliday` exercise decode paths; existing `FuzzDateUnmarshal` covers `Date`. |
| Leaked secrets in CI logs | Information Disclosure | No secrets used: Codecov OIDC, attestations OIDC. The only "secret" is `GITHUB_TOKEN`, which has minimum-permission scopes (`contents: write`, `id-token: write`, `attestations: write`). |
| Releaser executes attacker-controlled code via release.yml | EoP | `release.yml` only triggers on `v*` tag pushes from the repo owner; no `pull_request_target` triggers; `goreleaser-action@v6` is first-party. |

## Sources

### Primary (HIGH confidence)

- `.planning/phases/05-distribution/05-CONTEXT.md` — locked decisions, user constraints, deferred ideas.
- `.planning/REQUIREMENTS.md` §CLI, §Testing, §CI/CD, §Documentation, §Release — 27 phase requirements.
- `.planning/ROADMAP.md` "Phase 5: Distribution" — 5 success criteria + perf budgets.
- `.planning/PROJECT.md` — zero-dep policy, HTTP semantics, English-only, em-dash style.
- `.planning/STATE.md` — Phase 1–4 completion, Key Decisions CL-01..CL-17.
- `.planning/codebase/CONVENTIONS.md` — Gold Rules 1–3.
- `CLAUDE.md` §Recommended Stack — golangci-lint v2.x, goreleaser v2.x guidance.
- `go.mod` (verified: module path correct, Go 1.23 floor).
- `version.go`, `doc.go`, `holiday.go`, `types.go`, `public_holidays.go`, `school_holidays.go`, `countries.go`, `options.go`, `client.go` — current library public surface read 2026-05-28.
- `.golangci.yml_backup` — verified contents and cross-project debris.

### Secondary (MEDIUM-HIGH confidence — official docs + ecosystem-current)

- [Go 1.24 Release Notes — `T.Context`](https://tip.golang.org/doc/go1.24) — verified `t.Context()` is Go 1.24+.
- [Go Fuzzing Tutorial](https://go.dev/doc/security/fuzz/) — `testdata/fuzz/<FuzzName>/` auto-discovery, `F.Add` seed registration.
- [Testable Examples in Go (go.dev/blog)](https://go.dev/blog/examples) — `Example_*` naming, `// Output:` block semantics.
- [actions/setup-go README](https://github.com/actions/setup-go) — current major `v5`, built-in module cache.
- [actions/attest-build-provenance README](https://github.com/actions/attest-build-provenance) — current major `v4`, wraps `actions/attest@v4`.
- [codecov/codecov-action README](https://github.com/codecov/codecov-action) — current major `v5`, `use_oidc: true`.
- [golang/govulncheck-action](https://github.com/golang/govulncheck-action) — `@v1` step, `go-package: ./...` input.
- [goreleaser-action README](https://github.com/goreleaser/goreleaser-action) — current major `v6`, `version: "~> v2"` input.
- [GoReleaser Builds (Go)](https://goreleaser.com/customization/builds/go/) — `version: 2` schema, builds matrix, ldflags.
- [GoReleaser Changelog](https://goreleaser.com/customization/changelog/) — `groups`, `filters.exclude`, conventional-commit regexes.
- [GoReleaser Attestations integration](https://goreleaser.com/customization/attestations/) — `subject-checksums: ./dist/checksums.txt` pattern.
- [golangci-lint v2 launch post (ldez.github.io)](https://ldez.github.io/blog/2025/03/23/golangci-lint-v2/) — `linters.default: none` replacement for `disable-all: true`.
- [golangci-lint Configuration](https://golangci-lint.run/docs/configuration/file/) — v2 schema reference.
- [GitHub Docs — Artifact Attestations](https://docs.github.com/en/actions/security-guides/using-artifact-attestations-to-establish-provenance-for-builds) — `id-token: write`, `attestations: write` permissions; `gh attestation verify` CLI.
- [Conventional Commits 1.0](https://www.conventionalcommits.org/en/v1.0.0/) — `feat:` / `fix:` / `chore:` / `docs:` / `test:` prefixes parsed by goreleaser.
- [pkg.go.dev best practices](https://pkg.go.dev/about#best-practices) — symbol-prefixed first sentence for godoc.

### Tertiary (verification done, lower-confidence facts)

- WebSearch results on coverage threshold scripts — multiple sources agree on the `go tool cover -func | awk '/^total:/'` extraction pattern; pattern verified mentally against output of the same command on this repo's existing tests.
- WebSearch on Go Report Card grade requirements — A grade is the default for libraries that pass `gofmt`, `go vet`, `golint`, `gocyclo`, `ineffassign`, `misspell`; current library passes all locally.

## Metadata

**Confidence breakdown:**
- Locked decisions: HIGH — all from CONTEXT.md verbatim.
- Action versions: HIGH — verified via WebFetch + WebSearch of official READMEs; documented version drift from CONTEXT.md's `@v1`/`@v4`.
- `.golangci.yml_backup` rewrite scope: HIGH — verified by reading the file.
- Cached perf budget reinterpretation (Pitfall 5): HIGH — verified against Phase 4 D-83 decision.
- CLI surface decomposition: MEDIUM-HIGH — stdlib `flag` subcommand pattern is canonical, exact flag spellings are recommendations (planner can adjust).
- Fuzz seed corpus strategy: MEDIUM — recommendation is sound but `testdata/*.json` fixture sizes weren't profiled; if they're too large (multi-MB) `F.Add` would slow fuzz init. Mitigation: planner can profile and shrink seed inputs if needed.
- `pkg.go.dev` index lag timing: MEDIUM — anecdotal across the Go OSS ecosystem; not from a primary source. Mitigation is well-known (manually request the URL after tagging).

**Research date:** 2026-05-28
**Valid until:** 2026-06-28 (action versions move; recheck if planner executes past this date).

---

*Researched: 2026-05-28 by gsd-researcher for Phase 5 (Distribution).*
