# go-openholidays

## What This Is

`go-openholidays` is an idiomatic, dependency-light Go client library for the public OpenHolidays API (https://www.openholidaysapi.org). It exposes public holidays, school holidays, country and language metadata, and administrative subdivisions through a clean, well-tested Go-first API targeted at backend engineers building HR, scheduling, education, and leave-management applications — especially those needing **regional school-break granularity** (e.g. Polish *ferie* per województwo) that competing libraries do not cover.

## Core Value

A single, well-tested Go client that returns both **public holidays AND school holidays per administrative subdivision** for the public OpenHolidays API, with zero runtime dependencies, full `context.Context` propagation, and typed errors. If everything else fails, this must work: `client.PublicHolidays(ctx, ...)` and `client.SchoolHolidays(ctx, ...)` returning correctly-typed, validated data for Poland 2025.

## Requirements

### Validated

(None yet — ship to validate)

### Active

- [ ] Client construction with functional Options (`NewClient(opts ...Option) *Client`) supporting `WithHTTPClient`, `WithBaseURL`, `WithUserAgent`, `WithLogger`, `WithTimeout`.
- [ ] Five endpoints implemented, all `context.Context`-aware: `Countries`, `Languages`, `Subdivisions`, `PublicHolidays`, `SchoolHolidays`.
- [ ] Domain types (`Holiday`, `Subdivision`, `Country`, `Language`, `LocalizedText`, `SubdivisionRef`) with custom `UnmarshalJSON` for `YYYY-MM-DD` dates.
- [ ] Client-side parameter validation: 2-letter uppercase country code, `validFrom <= validTo`, date window ≤ 3 years.
- [ ] Typed errors: sentinels (`ErrInvalidCountry`, `ErrInvalidLanguage`, `ErrDateRangeTooLarge`, `ErrEmptyResponse`) and `*APIError{StatusCode, Path, Body}` inspectable via `errors.Is`/`errors.As`.
- [ ] Opt-in retry with exponential backoff + full jitter, honoring `Retry-After`, bounded by `ctx`.
- [ ] Opt-in in-memory TTL cache for reference endpoints (`Countries`, `Languages`, `Subdivisions`). Holiday endpoints not cached by default.
- [ ] Helper methods on `Holiday`: `Name(lang)`, `IsInRegion(code)`, `Days()`, `Range() iter.Seq[time.Time]` (Go 1.23 range-over-func).
- [ ] `cmd/ohcli` demo CLI: `ohcli public PL 2025`, `ohcli school PL 2025 --region PL-SL`.
- [ ] Strict-decoding mode via `WithStrictDecoding(bool)`.
- [ ] Observability hook via `WithRequestHook(func(*http.Request, *http.Response, error))`.
- [ ] Goroutine-safe: `Client` can be shared across goroutines; `-race`-clean.
- [ ] Test coverage ≥ 85 % with unit tests (httptest), table-driven cases, golden JSON fixtures.
- [ ] Integration tests behind `//go:build integration` and `OPENHOLIDAYS_LIVE=1` env gate; nightly CI.
- [ ] Fuzz tests for JSON parsers.
- [ ] Benchmarks for hot paths.
- [ ] CI: GitHub Actions matrix (Go 1.22, 1.23, stable) running `go vet`, `go build`, `go test -race -cover`, `golangci-lint`, `govulncheck`.
- [ ] Release pipeline: `goreleaser` on `v*` tags producing CLI binaries for linux/darwin/windows × amd64/arm64.
- [ ] Documentation: README quickstart ≤ 20 lines, `doc.go`, `example_test.go` with one example per public method, `docs/design.md`, `CHANGELOG.md`, `CONTRIBUTING.md`.
- [ ] Tag `v0.1.0` published to `pkg.go.dev` with Go report card grade A.

### Out of Scope

- Generated types from upstream OpenAPI spec — deferred to milestone M4 (codegen brings churn we don't need for v0.1.0).
- iCal output and parsing — deferred to M4 (caller can pass JSON through their own iCal lib if needed sooner).
- Persistent (file/SQLite) cache — deferred to M2/M3; in-memory cache covers the dominant use case.
- Working-day arithmetic (`IsWorkingDay`, `NextWorkingDay`) — deferred to M3; significantly broadens the public contract.
- Polish "observances" sub-package (Mother's Day, Children's Day, Father's Day, Andrzejki, end-of-school-year) — deferred to M3; not in the upstream OpenHolidays data for PL.
- gRPC/GraphQL transports — out of scope; the upstream API is REST/JSON, transport translation is a separate library concern.
- Multi-tenant API-key support — out of scope; OpenHolidays is currently keyless.
- Persisting holidays to a database — caller responsibility.
- Calendar UI / web frontend — this is a library, not an application.
- Multi-country aggregation helpers in M1 — deferred.
- Non-Go client ports (TypeScript, Python) — out of scope for this repo.
- Self-hosted OpenHolidays mirror — out of scope.
- Localization of error messages — errors stay English.

## Context

- **Prior art in this directory**: two prototype Go programs live alongside this `.planning/` tree.
  - `main.go` — mock-backed client demoing `holidays-rest/sdk-go` + `rickb777/date/v2`, plus a side-by-side coverage matrix.
  - `openholidays/main.go` — live POC hitting `openholidaysapi.org` for Poland 2025. Confirmed: 14 public holidays (incl. new Dec 24 Christmas Eve from 2025) and 7 school-holiday periods (4 staggered ferie zimowe cohorts, wiosenna przerwa świąteczna, ferie letnie, zimowa przerwa świąteczna), plus 16 województwa subdivisions.
  - These POCs are reference material only and will be replaced when M1 lands `go-openholidays/`.
- **Why this library exists**: research during the POCs surfaced no first-class idiomatic Go SDK for OpenHolidays. `holidays-rest/sdk-go` is a paid REST-only SDK; `rickar/cal/v2/pl` is offline but covers only 12 public holidays (no school breaks); `rickb777/date` is a date-arithmetic library, not a holidays library.
- **Primary user is in Poland** and cares about regional school breaks per województwo — exactly the gap this library fills.
- **Upstream API stability**: OpenHolidays is publicly accessible without auth, data available from 2020+, query window capped at 3 years per call. Schema observed in POCs to vary across responses (optional fields `comment`, `quality`, `subdivisions`) — schema-drift defenses needed.
- **No rate-limit headers** observed in POC responses → retry strategy must stay conservative.

## Constraints

- **Tech stack**: Go ≥ 1.22 minimum. CI matrix tests 1.22, 1.23, and `stable`. — Ensures Go 1.23 `iter.Seq` features remain feasible while keeping a one-version backwards window.
- **Dependency policy**: zero runtime dependencies (no `require` entries beyond stdlib). Test-only deps must be vetted; `github.com/google/go-cmp` is pre-approved. — Reduces supply-chain attack surface and keeps `go get` fast for consumers.
- **License**: MIT, single root `LICENSE`; no per-file headers required. — Standard for Go OSS libraries.
- **Style**: `gofmt`-clean; `.golangci.yml` shipped in repo; lints required: `govet`, `errcheck`, `staticcheck`, `gosec`, `revive`, `gocritic`. — Enforces code quality without bikeshedding.
- **Public surface area**: minimize. Every exported symbol must have a doc comment. Internal helpers live under `internal/`. — Stable v1.0 API later requires a disciplined v0.x surface now.
- **No `init()` side effects, no global mutable state.** — Predictability and testability.
- **HTTP semantics**: every request sends `Accept: application/json` and `User-Agent: go-openholidays/<version>`. Default timeout 15 s. `io.LimitReader` caps response body at 10 MiB. — Robustness against misbehaving servers.
- **Cancellation**: `context.Context` cancellation must interrupt in-flight HTTP within ≤ 100 ms. — Standard Go ctx-aware client contract.
- **Performance**: listing 1 year of PL public holidays must be < 500 ms cold and < 5 ms when cached. — Modest but measurable; a microbenchmark proves it.
- **Security**: no secrets in repo; `govulncheck` clean in CI; inputs validated client-side before hitting network. — OSS supply-chain hygiene.
- **Logging**: default `slog.Default()`, structured. HTTP calls logged at `Debug`. Response bodies must never be logged at `Info` or above. — Avoid accidentally exposing data in operator logs.
- **Backwards compat**: pre-1.0 (`v0.x`) — breaking changes allowed with CHANGELOG entries. From `v1.0` onward, strict SemVer. — OSS norm.

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Variant M scope (Production-grade, OSS-ready) for M1, defer Variant L items to M2–M5 | Estimation discussion concluded 24–36 h of focused work for a publishable v0.1.0 is the right shape — large enough to be valuable, small enough to ship | — Pending |
| Treat repo as greenfield; POC files are reference, not the codebase to extend | The POCs proved data scope and API contract but use ad-hoc patterns; the library starts clean under `go-openholidays/` | — Pending |
| YOLO mode + Standard granularity + Parallel execution | Brief is comprehensive enough to skip per-step confirmation; standard granularity fits the 5-milestone sketch; parallelization buys speed where plans are independent | — Pending |
| Horizontal Layers project structure | Library has no UI to slice vertically; building types → transport → endpoints → helpers → CLI is the natural order | — Pending |
| Quality model profile (Opus) for research + roadmap agents | OSS library that needs to last; deeper analysis upfront beats cheap-then-rewrite | — Pending |
| Research, Plan Check, and Verifier agents all enabled | OSS quality bar warrants the extra tokens; surfaces gotchas before they ship | — Pending |
| MIT license, public visibility | Standard Go OSS conventions; maximizes adoption | — Pending |
| Module path owner deferred to discuss-phase | User has not confirmed GitHub org/user — will be resolved before tagging `v0.1.0` | — Pending |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd-transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd:complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-05-26 after initialization*
