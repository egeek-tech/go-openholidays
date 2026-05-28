# Phase 5: Distribution - Context

**Gathered:** 2026-05-28
**Status:** Ready for planning

<domain>
## Phase Boundary

Ship the library as `v0.1.0` to `pkg.go.dev` and a public GitHub Release. Scope:

- `cmd/ohcli` demo CLI (text + `--json` + `--csv`, stdlib `flag` only) that dogfoods the public API.
- Fuzz tests (`FuzzParseLocalizedText`, `FuzzUnmarshalHoliday`), integration tests (`//go:build integration` + `OPENHOLIDAYS_LIVE=1`), benchmarks proving cold/cached perf budgets.
- CI matrix (`go: [1.23, 1.24, stable]` × `os: ubuntu-latest`) running vet/build/test-race/golangci-lint/govulncheck/`go mod tidy --check`; nightly `integration.yml` against live API; `release.yml` on `v*` tag.
- Documentation: `README.md` (badges + ≤20-line quickstart), `doc.go` package overview, `example_test.go` (one `Example_*` per public method), `docs/design.md`, `CHANGELOG.md` (auto-generated), `CONTRIBUTING.md`, full godoc coverage.
- `v0.1.0` git tag → goreleaser produces signed cross-platform binaries (linux/darwin/windows × amd64/arm64) + checksums + GitHub Artifact Attestations.

**Out of phase (already locked elsewhere):** new endpoints, retry/cache/hook behavior changes, additional Holiday helpers, vanity import path, code generation, persistent cache.

</domain>

<decisions>
## Implementation Decisions

### Module identity (REL-04 — deferred decision resolved)
- **D-01:** Module path stays `github.com/egeek-tech/go-openholidays` for v0.1.0. No vanity import path.
- **D-02:** Repo is already public — `pkg.go.dev` will index automatically after the `v0.1.0` tag is pushed from `main`. Plan must verify visibility as a pre-tag step in the release runbook.

### CLI surface (cmd/ohcli)
- **D-03:** Three output modes: default text table, `--json` (typed-struct marshal), `--csv` (spreadsheet-friendly). Format is a single `--format=text|json|csv` flag with default `text`; or short forms `--json` / `--csv` that set it. Planner picks the exact spelling — both are acceptable.
- **D-04:** `--lang` defaults to `"en"` and falls back to first-available `LocalizedText` entry for missing translations. Matches Phase 3 `Holiday.NameFor("xx")` semantics — `Client.lang(country)` resolver is **not** in scope for v0.1.0.
- **D-05:** Errors print to **stderr** with the `ohcli: <message>` prefix. No ANSI color (plain text only).
- **D-06:** Exit code policy is **3-tier POSIX**:
  - `0` — success (including empty result sets)
  - `1` — runtime error (network failure, API error, validation rejection, decode error)
  - `2` — usage error (unknown subcommand, missing required arg, bad flag value)
- **D-07:** Empty results print a single line to **stderr** of the form `no <thing> found for <args>` (e.g. `ohcli: no school holidays found for PL 2025 (region PL-XX)`) and exit `0`. Stdout stays empty so pipes don't break.
- **D-08:** Unit tests use `httptest.NewServer` per case + fixtures from `testdata/`. **Zero live API calls in `go test`** (live API is integration-only, gated by `//go:build integration` + `OPENHOLIDAYS_LIVE=1`).

### Coverage badge (CI-07)
- **D-09:** Use **Codecov** (`codecov/codecov-action@v4`). Token-free for public repos via the GitHub App. README badge: `[![codecov](https://codecov.io/gh/egeek-tech/go-openholidays/branch/main/graph/badge.svg)](https://codecov.io/gh/egeek-tech/go-openholidays)`.

### Release pipeline (goreleaser + signing)
- **D-10:** Release artifacts emitted by goreleaser:
  - Cross-platform binaries: `linux/darwin/windows × amd64/arm64` (6 binaries per release).
  - `checksums.txt` SHA-256 file (goreleaser default).
  - **No** source tarball, **no** Homebrew tap, **no** cosign signature. Re-evaluate Homebrew tap + cosign in `v0.2.x` once the release workflow has been exercised on a few tags.
- **D-11:** **GitHub Artifact Attestations** for SLSA-Level-3 provenance via `actions/attest-build-provenance@v1` after the goreleaser upload step in `release.yml`. Free for public repos, signed by Sigstore/Fulcio, no key management. Downstream verification: `gh attestation verify <binary> --repo egeek-tech/go-openholidays`.
- **D-12:** **CHANGELOG.md auto-generated from conventional commits** by goreleaser's release-notes block. Hand-curated `CHANGELOG.md` is **not** maintained — release notes live on GitHub Releases, generated from `feat:` / `fix:` / `chore:` etc. commit prefixes between tags. DOC-05 ("keep-a-changelog format") satisfied via the generated GitHub Release notes; if a top-level `CHANGELOG.md` is still desired, it's a thin pointer file: "See https://github.com/egeek-tech/go-openholidays/releases for changes."

### Claude's Discretion
- **README badge set**: which badges (CI status, Codecov, Go Report Card, godoc, license) and their order is editorial — planner picks a sensible default.
- **`docs/design.md` shape**: ASCII diagrams vs Mermaid vs prose-only is open. Recommend ASCII for grep-ability and rendering on `pkg.go.dev`.
- **`CONTRIBUTING.md` depth**: whether to add issue/PR templates in `.github/`, code-of-conduct, or DCO sign-off is open. Recommend minimal v0.1.0 (just the dev loop + how to run unit/integration/fuzz tests); add templates in v0.2.x.
- **`Example_*` strategy**: doctest-style with `// Output:` blocks (executable) vs prose-only. Recommend doctest where deterministic (constants, simple validators) and `// Demo:` style where the output depends on the live API. One example per public method is the floor; bundling related examples is fine.
- **Fuzz seed corpus**: hand-curated seeds in `testdata/fuzz/` vs auto-seeded from existing `testdata/*.json` fixtures vs both. Recommend "both" — load existing fixtures as seed corpus + add 2-3 adversarial seeds per fuzz target.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project & requirements
- `.planning/PROJECT.md` — Tech-stack rationale, dep policy (zero runtime deps), HTTP semantics (Accept/User-Agent/15s timeout/10 MiB cap), security posture, logging rules, Key Decisions log.
- `.planning/REQUIREMENTS.md` §CLI / §Testing / §CI/CD / §Documentation / §Release — All 27 Phase 5 requirements (CLI-01..04, TEST-07..11, CI-01..07, DOC-01..07, REL-01..04).
- `.planning/ROADMAP.md` "Phase 5: Distribution" section — Goal + 5 success criteria, including the locked CLI signatures, CI matrix shape, perf budgets, and tag-shipping flow.

### Earlier-phase context (decisions that constrain phase 5)
- `.planning/phases/01-foundation/01-CONTEXT.md` — Locks Go 1.23 floor, zero-dep policy, `slog` for logging, English-only.
- `.planning/phases/02-transport/02-CONTEXT.md` — Transport chain, `Client` lifecycle, `Close()` contract.
- `.planning/phases/03-endpoints-helpers/03-CONTEXT.md` — Five endpoint signatures, `Holiday.NameFor`/`IsInRegion`/`Range` semantics that the CLI will exercise.
- `.planning/phases/04-resilience/04-CONTEXT.md` — Retry/cache/hook composition; relevant for CLI examples that demonstrate `WithRetry` + `WithCache`.

### Conventions & gold rules
- `.planning/codebase/CONVENTIONS.md` — Gold Rules 1–3 (English-only, verify-don't-guess, testify + one-test-per-prod-function + `t.Run`). Em-dash style ratified here (no mass-replace).
- `CLAUDE.md` §Recommended Stack — `golangci-lint v2.x` config shape, `goreleaser v2.x` cross-compile + `CGO_ENABLED=0` + checksums + SLSA provenance.
- `CLAUDE.md` §Gold Project Rules — Reaffirmed in code.

### Existing artifacts to integrate (in-repo)
- `.golangci.yml_backup` — Drafted lint config (linters.default: none + custom set). Phase 5 plan must restore + finalize this as `.golangci.yml`. Note the existing carve-out comments (e.g., `unparam` disabled 2026-05-11 — preserve the rationale comment).
- `testdata/*.json` — Real OpenHolidays API responses for PL/DE. Reusable as fuzz seed corpus and as fixtures for `httptest`-backed CLI tests.

### External specs / docs
- Conventional Commits 1.0 (`https://www.conventionalcommits.org/en/v1.0.0/`) — Format the goreleaser release-notes generator parses. Plan must include a `commitlint` or equivalent enforcement decision (recommend a `.github/workflows/commitlint.yml` or just a CONTRIBUTING.md note for v0.1.0).
- GitHub Artifact Attestations (`https://docs.github.com/en/actions/security-guides/using-artifact-attestations-to-establish-provenance-for-builds`) — Exact step name + flags for `actions/attest-build-provenance@v1`.
- goreleaser v2 (`https://goreleaser.com/customization/builds/`) — `.goreleaser.yaml` v2 schema; `version: 2` directive.
- Codecov GitHub Action (`https://github.com/codecov/codecov-action`) — Current v4 step config.
- Go fuzz docs (`https://go.dev/security/fuzz/`) — Seed corpus directory convention (`testdata/fuzz/FuzzXxx/`).
- pkg.go.dev rendering rules (`https://pkg.go.dev/about#best-practices`) — Symbol-prefixed doc comment requirement for godoc.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- **Library public surface** (`client.go`, `request.go`, `holiday.go`, etc.): the CLI imports the library at its module path and treats it as an external consumer (CLI-03). No internal-package poking.
- **`testdata/countries.json` / `languages.json` / `public_holidays_pl_2025.json` / `school_holidays_pl_2025.json` / `subdivisions_de.json` / `subdivisions_pl.json`**: Real upstream responses. Reusable as (a) CLI unit-test fixtures via `httptest` and (b) fuzz seed corpus.
- **`update_fixtures_test.go`**: Build-tagged `-update` harness from Phase 3 — pattern can be lifted for integration tests if needed.
- **`fakeClock` test helper (`clock_test.go`)**: Phase 4 deterministic clock — reusable for any bench/test that touches cache TTL or retry backoff.
- **`internal_test.go::TestNoInitOrGlobalState`** (CLIENT-10 AST audit): Already constrains package-level vars to an allowlist. The CLI lives in `cmd/ohcli/` — verify the audit scope doesn't accidentally include it (it scans the root package, so cmd/ohcli is naturally excluded).

### Established Patterns
- **One TestXxx per exported production function + `t.Run` subtests** (Gold Rule 3). CLI tests follow the same shape: one `TestCmdOhcli_<Subcommand>` per subcommand with subtests per scenario.
- **testify `assert` + `require`**: `require` for preconditions, `assert` for verifications. CL-17 PROJECT.md exception is available for cross-cutting tests (e.g., a CLI-wide "no nil-pointer panics" sweep).
- **English-only**: All identifiers, comments, godoc, error messages, README, CHANGELOG, design doc. Non-English strings only in `testdata/*.json` and in `Holiday.LocalizedText` values exercised by examples.
- **Em-dash style** (CL-17 / CONVENTIONS.md): em-dashes are deliberate; no mass-replace to `--`.
- **Stdlib-only at runtime**: The CLI module path is the same as the library; CLI imports the library + stdlib `flag`/`encoding/csv`/`encoding/json`/`fmt`/`os`/`strconv`/`strings`/`tabwriter`. No third-party CLI helpers.

### Integration Points
- **`cmd/ohcli/main.go`** is new. Lives under `cmd/ohcli/` (Go convention). The `package main` for the CLI is separate from the `openholidays` library package — clean import-path consumer relationship.
- **`.github/workflows/ci.yml`** is new. Matrix runs against the library's root package.
- **`.github/workflows/integration.yml`** is new — nightly cron, `OPENHOLIDAYS_LIVE=1` env, `//go:build integration` tag.
- **`.github/workflows/release.yml`** is new — triggered on `v*` tag, runs goreleaser + `attest-build-provenance`.
- **`.goreleaser.yaml`** is new — v2 schema (`version: 2`), `builds:` matrix, `archives:`, `checksum:`, `release:` config.
- **`.golangci.yml`** is the finalized form of `.golangci.yml_backup` (restore + verify against current code; the backup was disabled at some point — confirm whether the current code passes the backed-up config or needs adjustments).
- **`README.md`** is new (currently absent from repo root per `ls *.md` showing only `CLAUDE.md`, `GSD-PROJECT-BRIEF.md`).
- **`docs/design.md`** is new (no `docs/` directory exists yet).
- **`CONTRIBUTING.md`** is new.
- **`example_test.go`** is new (no example tests currently in repo).

</code_context>

<specifics>
## Specific Ideas

- GitHub Artifact Attestations is the explicit signing approach — `cosign` and other key-management flows are out of scope for v0.1.0 even though they could be added later.
- "Auto-generate from conventional commits" was the user's CHANGELOG choice — implies the project is committed to enforcing conventional-commit prefixes going forward. Existing commit history already uses this pattern (e.g. `fix(04): ...`, `docs(03): ...`, `feat(04): ...`), so the convention is already de facto in place; planner just needs to either codify it in CONTRIBUTING.md or add a lightweight commitlint check.
- CSV output was the user's explicit add-on to text+JSON. Implies expected use case is spreadsheet import / data pipelines. Planner: use stdlib `encoding/csv` (RFC 4180 compliant); header row mandatory; UTF-8 byte order mark optional (recommend omitting it — Excel handles UTF-8 without BOM on modern versions).
- Empty results print to **stderr** with `ohcli:` prefix and exit 0 (NOT empty stdout, NOT silent). Tests must assert this exact behavior.

</specifics>

<deferred>
## Deferred Ideas

- **Homebrew tap** — Defer to v0.2.x. Requires a sibling `homebrew-tap` repo and ongoing formula maintenance; not worth the maintenance burden until v0.1.x has settled.
- **cosign signatures (additional to GitHub attestations)** — Defer to v0.2.x. Only revisit if downstream consumers explicitly request cosign-backed signing alongside attestations.
- **Source tarballs from goreleaser** — Defer. `go install` and the GitHub-provided source archives cover the common cases.
- **CSV-with-BOM** — Defer. Adding a `--bom` flag if a user actually files a "my Excel can't open this CSV" bug.
- **`--format=ical`** — Out of scope for v0.1.0 per PROJECT.md (iCal output is M4).
- **Issue/PR templates in `.github/ISSUE_TEMPLATE/`** — Defer to v0.2.x. CONTRIBUTING.md for v0.1.0 has just the dev-loop + test-tier instructions.
- **DCO / sign-off enforcement** — Defer. Add only if outside contributors start arriving.
- **Code of Conduct** — Defer to v0.2.x.
- **Vanity import path** (e.g., `go.egeek-tech.io/openholidays`) — Defer. Once committed it can't be moved without a major-version bump.
- **`--lang` env-var fallback (LANG / LC_ALL)** — Defer. Considered then rejected for v0.1.0 in favor of explicit `--lang en` default.

</deferred>

---

*Phase: 5-distribution*
*Context gathered: 2026-05-28*
