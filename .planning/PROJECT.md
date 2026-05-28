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
- [ ] `Client.Close() error` stops the cache eviction sweeper goroutine and releases resources. Documented as idempotent and safe to call from any goroutine. (Added after PITFALLS research flagged the janitor goroutine leak.)
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

- **Tech stack**: Go ≥ 1.23 minimum (raised from 1.22 after research surfaced that `iter.Seq` is a Go 1.23 feature). CI matrix tests 1.23, 1.24, and `stable`. — `iter.Seq` is core to the helper API; aligning the floor avoids build tags or a separate compat shim.
- **Dependency policy**: zero runtime dependencies — no non-stdlib import in any `.go` file outside `*_test.go`. Test-only deps must be vetted and may only appear in `*_test.go` imports; pre-approved set: `github.com/stretchr/testify` (assert + require — primary assertion library per Gold Rule 3), `github.com/google/go-cmp` (deep-equal diffs when testify output is insufficient). Any additional test-only dep requires a `Key Decisions` entry. — Reduces supply-chain attack surface and keeps `go get` fast for consumers.
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
| Gold Rule 1 — English-only across all code/comments/docs/tests/commits | Library targets global OSS audience and `pkg.go.dev`; mixed-language sources block contributors and reviewers | — Pending |
| Gold Rule 2 — Verify or ask; never guess | Confidently-stated guesses produce silent bugs and erode trust; one tool call to verify costs less than a debug cycle | — Pending |
| Gold Rule 3 — `testify` (assert + require) is the test framework; one `TestXxx` per production function; every case wrapped in `t.Run` | Matches Go 2025/2026 community norm, makes `go test -run` deterministic, gives per-case CI rows, aligns IDE "go to test" navigation | — Pending |
| CL-01: Phase 1 ships 5 sentinel errors instead of ROADMAP-literal 4 (adds `ErrInvalidDateRange`) | Two semantically distinct failure modes (`validFrom > validTo` vs window > 3 years) deserve two `errors.Is`-distinguishable identities; source: `.planning/phases/01-foundation/01-CONTEXT.md` D-13 + Scope clarifications CL-01 | Adopted in Phase 1 |
| CL-02: `validateCountry` is case-insensitive (canonicalizes input to uppercase) — deviates from VALID-01 literal "2 uppercase ASCII letters" | Ergonomic input parity with `validateLanguage`'s case-insensitive lowercase behavior; wire format remains uppercase per upstream API; source: `.planning/phases/01-foundation/01-CONTEXT.md` D-20 + Scope clarifications CL-02 | Adopted in Phase 1 |
| CL-03: `FuzzDateUnmarshal` ships in Phase 1 instead of Phase 5 as ROADMAP places fuzz tests | Pitfall JSON-3 mandates a fuzz target for every custom unmarshaler; not waiting four phases to surface regressions; source: `.planning/phases/01-foundation/01-CONTEXT.md` D-12 + Scope clarifications CL-03 | Adopted in Phase 1 |
| CL-04: `HolidayType` ships 6 PascalCase upstream-verified values (`Public`, `Bank`, `Optional`, `School`, `BackToSchool`, `EndOfLessons`) instead of REQUIREMENTS.md TYPES-04's 4 (`Public`, `School`, `Bank`, `Observance`) | Verified live OpenAPI spec on 2026-05-27 lists 6 values; `Observance` does not exist upstream; three real values were missing from the requirement; source: `.planning/phases/01-foundation/01-RESEARCH.md` §"Upstream API Schema — Verified" Open Question Q1 + Assumption A6 | Adopted in Phase 1 |
| CL-05: `Country`/`Language`/`Subdivision` `NameFor(lang)` renamed from TYPES-05's literal `Name(lang)` to avoid Go method-vs-field name collision with the existing `Name []LocalizedText` field | Go forbids a method and a field on the same type to share a name; renaming the method (less invasive than renaming the field, which would change JSON-tag handling); source: `.planning/phases/01-foundation/01-RESEARCH.md` §"Pattern 4" + Open Question Q2 + Assumption A3 | Adopted in Phase 1 |
| CL-06: `validateDateRange` uses backward-anchored `to.AddDate(-3, 0, 0)` (compare `from.Before(threshold)`) instead of plan-prescribed forward-anchored `from.AddDate(3, 0, 1)` | Go's `time.AddDate` normalizes overflow toward later dates: `2024-02-29 + 3yr` lands on `2027-03-01` (non-leap year), then `+1d` lands on `2027-03-02`, making the forward formula accept `validateDateRange(2024-02-29, 2027-03-01)` which ROADMAP success criterion #4 requires to reject. Backward-anchored formulation satisfies every locked boundary case verbatim. Source: `.planning/phases/01-foundation/01-05-SUMMARY.md` "Deviations from Plan" Rule 1 | Adopted in Phase 1 |
| CL-14: Narrow Gold-Rule-3 exception — a second top-level TestXxx tied to `Client.SchoolHolidays` (`TestClient_SchoolHolidays_IsInRegion_FerieZimowe` in `school_holidays_test.go`) is permitted ALONGSIDE the existing `TestClient_SchoolHolidays` in order to satisfy the literal ROADMAP SC#2 wording — "correctly identifies the Śląskie ferie zimowe cohort while excluding the other three regional cohorts" — as a single integrated test scenario against the golden fixture rather than a compositional proof split across `school_holidays_test.go` (fixture-has-the-entry) and `holiday_test.go` (IsInRegion-logic-is-correct). Scope: THIS test only — future SchoolHolidays-related tests must continue to live inside the single `TestClient_SchoolHolidays` t.Run tree per Gold Rule 3. | Gold Rule 3 ("one TestXxx per exported production function") has no documented exception mechanism in CLAUDE.md or PROJECT.md, so without this entry future maintainers would believe the rule is silently broken when they discover the second test function. Recording the narrow exception keeps Gold Rule 3 enforceable for every other test by making the one allowed exception explicit, named, and scope-limited. Source: `.planning/phases/03-endpoints-helpers/03-VERIFICATION.md` gap `SC2-COMBINED` + `.planning/phases/03-endpoints-helpers/03-11-PLAN.md` Task 1. | Adopted in Phase 3 (gap closure) |
| CL-15: Phase 4 ships an EXPORTED `Cache` interface (`Get`/`Put`/`Close`) plus an exported `NewMemoryCache(ttl)` constructor and `MemoryCache` type, even though only the in-memory implementation ships in v0.1.0. `WithCache(ttl)` and `WithCacheBackend(c Cache)` are the two opt-in option constructors; `ttl<=0` disables; `WithCacheBackend(nil)` is a no-op; `WithCacheBackend` supersedes prior `WithCache` (last-wins per functional-options convention). The cache key encoding is `"<method> <path>?<sorted-query>"` and excludes Host — per-Client cache isolation handles Pitfall CACHE-2 without putting Host in the key. | RESIL-06 mandates a public `Cache` interface so a v0.2 consumer can supply a Redis/BoltDB backend without forking the library. Exposing the interface AND the in-memory impl in v0.1.0 commits us to that contract before any consumer requests it; the alternative (keeping `Cache` internal and forcing M2 to break it open) would require a breaking change. Anchored on `.planning/phases/04-resilience/04-CONTEXT.md` D-79/D-80 + RESEARCH.md "Pattern 4" + Pitfall CACHE-2/CACHE-4. The exported `CacheHitContextKey` var is also part of this surface — added to `internal_test.go::allowedVars` in Plan 04 (deviation from CONTEXT.md D-97 step 6 noted in `04-04-SUMMARY.md`). | Adopted in Phase 4 |
| CL-16: `WithStrictDecoding(bool)` is OFF by default and IMMUTABLE after `NewClient`. No per-call override, no runtime toggle. A strict-mode client cannot be made lenient at runtime; a lenient-mode client cannot be made strict. Cache+strict composition: cache stores raw bytes; decode runs on read; therefore strict-mode applies to every cached entry on every `Get`. A consumer wanting "lenient for cache + strict for fresh" must run two `Client`s. | Pitfall JSON-1 — upstream OpenHolidays schema has been observed to drift (the `"quality"` field on `Holiday` was added without spec update). Strict-by-default would break consumers on upstream additions. Immutability prevents the trap where toggling at runtime would corrupt the cache (cached bytes decoded with one mode could surface as a strict-failure after toggle). Anchored on `.planning/phases/04-resilience/04-CONTEXT.md` D-91/D-92/D-93 + Pitfall JSON-1. | Adopted in Phase 4 |

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
*Last updated: 2026-05-27 after adding Gold Project Rules (English-only, verify-don't-guess, testify+t.Run) and approving testify as test-only dep.*
