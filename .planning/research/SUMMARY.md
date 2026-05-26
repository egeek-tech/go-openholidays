# Project Research Summary

**Project:** go-openholidays
**Domain:** Go HTTP/JSON client SDK wrapping a public REST API (OpenHolidays)
**Researched:** 2026-05-26
**Confidence:** HIGH

## Executive Summary

`go-openholidays` is a small, zero-runtime-dependency Go SDK library wrapping the public OpenHolidays REST API. Research across stack, features, architecture, and pitfalls converges on a single clear recommendation: build this as a single-package, stdlib-only library following the patterns used by `google/go-github`, `stripe/stripe-go`, and `aws-sdk-go-v2`. The public surface is a `Client` constructed via functional options, five endpoint methods all taking `context.Context` as their first argument, a RoundTripper decorator chain for cross-cutting concerns (retry, cache, hook, logging, headers), and a small set of typed domain errors. No runtime dependency is needed beyond the standard library; `github.com/google/go-cmp` is the sole test-only dependency.

The recommended approach has high confidence because this is a well-trodden pattern for Go SDK libraries. The only genuine design decision with meaningful debate is the five-layer RoundTripper chain order, and even that has a clear rationale (retry outermost so cache and hook see all attempts). The bigger risks are operational rather than architectural: the Go version inconsistency in PROJECT.md must be resolved before the first line of code is written, `Client.Close()` must be added from day one (adding it later is fragile), and the retry default must stay opt-in for M1 to avoid DoS-ing a free volunteer-run API with no rate-limit headers.

The primary risk is not technical complexity -- it is scope creep and premature v1.0 tagging. Every feature deferred to M2+ (auto-batching iterator, persistent cache, working-day arithmetic, Polish observances) is explicitly justified by the research, and the `v0.1.0` tag is the correct M1 exit criterion. The most dangerous single decision is whether to make retry opt-in or on-by-default: research recommends opt-in for M1, then flip default ON in M2 after observing real-world behavior. Doing it wrong in either direction at M1 is recoverable but costs trust with the upstream operators.

## Key Findings

### Recommended Stack

The Go community answer for a dependency-light SDK in 2025/2026 is to lean entirely on the standard library. `net/http` for transport, `encoding/json` v1 for marshaling, `log/slog` for structured logging, `sync.RWMutex` + `map` for the in-memory cache, and `math/rand/v2` (Go 1.22+) for jitter. The sole test dependency is `github.com/google/go-cmp` for `cmp.Diff` in table-driven tests. This is the entire dependency graph.

One decision requires explicit resolution before work starts: PROJECT.md states "Go >= 1.22" but lists `iter.Seq` (Go 1.23+) as an Active requirement. The unanimous recommendation from both STACK.md and PITFALLS.md is to **bump the `go.mod` directive to `go 1.23`**. Go 1.22 has been off mainline support since early 2025; by the time v0.1.0 ships in mid-2026 it will have been unsupported for over a year. Retaining Go 1.22 requires a `//go:build go1.23` guard on the `iter.Seq` helper file, which adds complexity for no practical benefit.

**Core technologies:**

- `net/http` + `http.RoundTripper` chain: HTTP transport and all cross-cutting concerns -- the idiomatic Go extensibility seam
- `encoding/json` v1: JSON marshaling -- stdlib, zero dep, fast enough for network-bound calls
- `log/slog` (stdlib): structured logging -- accept `*slog.Logger` via option, default to `slog.Default()`
- `sync.RWMutex` + `map`: TTL cache -- preferred over `sync.Map` for the TTL-cache access pattern (sync.Map is optimized for write-once-read-many, not TTL-churn)
- `math/rand/v2` (Go 1.22+): full-jitter backoff -- goroutine-safe, no manual seeding required
- `testing` + `httptest` + `go-cmp`: test infrastructure -- stdlib plus one pre-approved dep
- `golangci-lint` v2.x: linting -- v2 `version: "2"` config format, `linters.default: standard`
- `goreleaser` v2.x: CLI release pipeline -- for `cmd/ohcli` binaries only; library ships via git tag
- `govulncheck`: vulnerability scanning -- call-graph-aware, low noise

**Version pinning:** Go 1.23 module directive. CI matrix on `1.23.x` and `stable`. The 1.22 CI cell may be retained to confirm the code still compiles under 1.22 toolchain (it will, since the `go` directive controls which stdlib symbols are permitted), but is not required.

### Expected Features

The FEATURES.md research produced a clear P1/P2/P3 tiering. Everything in the M1 Active list maps to P1.

**Must have (table stakes) -- all P1, v0.1.0:**

- `NewClient(opts ...Option) *Client` with functional options -- canonical Go SDK constructor
- `context.Context` first argument on all endpoint methods -- required for cancellation, deadlines, tracing
- Five endpoint methods: `Countries`, `Languages`, `Subdivisions`, `PublicHolidays`, `SchoolHolidays`
- Typed domain structs with custom `Date` type for `YYYY-MM-DD` -- no raw `time.Time` at API boundary
- Client-side validation before network call (country code, date window, `validFrom <= validTo`)
- Typed errors: sentinels via `errors.Is`, `*APIError` via `errors.As`
- Default 15 s timeout, `io.LimitReader` cap at 10 MiB, goroutine-safe `Client`
- `slog`-based structured logging at Debug by default; response bodies never logged above Debug
- `Cache` interface in M1 even though only the in-memory impl ships -- enables M2 persistent backends without a breaking change
- `WithRequestHook` observability hook -- lets consumers wire OTel/Prometheus without SDK dependency
- `cmd/ohcli` demo CLI -- doubles as dogfooding smoke test; under 300 lines, stdlib `flag` only

**Should have (competitive differentiators) -- P1 for M1:**

- `Holiday.Name(lang)` with EN fallback chain -- avoids every caller writing the same 5-line scan
- `Holiday.IsInRegion(code)` -- the primary use case for school holidays
- `Holiday.Days()` and `Holiday.Range() iter.Seq[time.Time]` -- idiomatic Go 1.23+ range-over-func
- Opt-in retry with full jitter + `Retry-After` parsing -- `WithRetry(RetryConfig{...})`; **opt-in for M1**
- Opt-in in-memory TTL cache for reference endpoints (`Countries`, `Languages`, `Subdivisions` only)
- Strict-decoding mode via `WithStrictDecoding(bool)` -- OFF by default; upstream schema is not stable
- Fuzz tests for JSON parsers -- differentiates from competitors that skip this

**Defer (v2+):**

- Auto-batching iterator for >3-year windows -- P2, trigger: user asks "how do I get 5 years in one call?"
- Retry on-by-default -- flip in M2 once confident no retry amplification in the wild
- Persistent cache via `Cache` interface plug-in -- P2
- Working-day arithmetic -- P3
- Polish observances sub-package -- P3
- Generated types from OpenAPI spec -- P3/M4
- iCal output, multi-country aggregation, gRPC/GraphQL -- P3 or never

### Architecture Approach

The library uses a single root package `openholidays` containing the entire public surface. Sub-packages for transport, cache, or types are explicitly ruled out. All cross-cutting HTTP concerns are layered via a `http.RoundTripper` decorator chain. The brief's single `transport.go` should be split into per-concern files (`transport_retry.go`, `transport_cache.go`, `transport_hook.go`, `transport_logging.go`) so each layer is independently testable with a fake `next`. Only genuinely shared test infrastructure belongs in `internal/testhttp/`.

**Major components:**

1. **`Client` (client.go)** -- holds the composed `*http.Client`, base URL, slog logger, strict-decoding flag. Immutable after `NewClient`; goroutine-safe. Zero exported fields.
2. **Functional options (options.go)** -- mutate a `clientConfig` builder during `NewClient`; never mutate `Client` after construction.
3. **RoundTripper chain (transport*.go)** -- retry (outermost) -> cache -> hook -> logging -> header -> base transport. Each layer wraps `next http.RoundTripper`. Assembled by `composeHTTPClient(cfg)` which shallow-copies the caller's `*http.Client`.
4. **Domain types (types.go, date.go, holiday.go)** -- exported structs with stable JSON tags; custom `Date` wrapping `time.Time` (wrapper struct for ergonomic method promotion); `Holiday` helpers.
5. **Endpoint methods** -- thin validate->build-request->decode->return. `*APIError` constructed here on non-2xx; RoundTripper stays HTTP-pure.
6. **Validators (validate.go)** -- unexported package functions; called at top of each endpoint method.
7. **Errors (errors.go)** -- package-level sentinels + `*APIError`; all wrapped with `%w`.
8. **`cmd/ohcli`** -- imports the public library at its module path only; zero `internal/` access.

**Chain order (outer -> inner):** retry -> cache -> hook -> logging -> header -> base transport

Order rationale: retry outermost so retried calls re-enter the cache and logging layers. Cache before hook so hook sees cache-hit responses. Logging before header so we log the canonical on-wire request. Header innermost as the final mutation before network.

### Critical Pitfalls

The full PITFALLS.md contains 35+ pitfalls across 8 categories. The five most dangerous if missed:

1. **`Client.Close()` absent from PROJECT.md Active** (PITFALLS CACHE-3) -- The in-memory TTL cache requires a janitor goroutine. Without `Client.Close()`, that goroutine leaks for the process lifetime. Marked "Never acceptable to skip." **Fix: add `Client.Close()` to Active requirements before Phase 4 begins.**

2. **Go version inconsistency** (PITFALLS OSS-3 / STACK.md) -- `go.mod` with `go 1.22` and `iter.Seq` in the codebase will not compile. **Fix: bump the `go` directive to `go 1.23` in Phase 1.**

3. **Strict decoding ON by default breaks on upstream schema drift** (PITFALLS JSON-1) -- OpenHolidays already has a `quality` field in real responses not in the OpenAPI spec. **Fix: `DisallowUnknownFields` is opt-in only; default is lenient.**

4. **Retry naively retrying 4xx** (PITFALLS RETRY-1) -- A 400 from a bad country code would be retried N times, appearing abusive to the free volunteer-run upstream. **Fix: retry-eligible set is `{408, 429, 500, 502, 503, 504}` plus network timeouts. Never retry 400/401/403/404.**

5. **Custom `Date.UnmarshalJSON` not handling `null` and empty string** (PITFALLS JSON-3) -- A three-line implementation will panic or silently produce a year-1 date. **Fix: the mandatory template handles `null`, empty string, non-JSON-string input, and propagates errors. A fuzz target enforces the invariant.**

Additional must-not-miss: drain body before close (HTTP-3), `io.LimitReader` on every response (HTTP-4), `context.Context` never stored in struct fields (CTX-1), `time.Sleep` replaced by `sleepCtx` (CTX-2), cache stores only success responses (CACHE-1), `sync.RWMutex`+`map` not `sync.Map` for the TTL cache (CACHE-5).

## Implications for Roadmap

Research naturally produces a 5-phase structure. The dependency DAG from ARCHITECTURE.md, the feature tiering from FEATURES.md, and the pitfall phase-mapping table from PITFALLS.md all independently suggest the same grouping.

### Phase 1: Foundation

**Rationale:** Types, the `Date` type, and errors are zero-dependency and must exist before anything else can be built or tested. This is also where the Go version decision and `go.mod` setup live.

**Delivers:** `Holiday`, `Subdivision`, `Country`, `Language`, `LocalizedText`, `SubdivisionRef`, `Date` (with correct `UnmarshalJSON`/`MarshalJSON`), all sentinel errors, `*APIError`, `validate.go` skeleton, `doc.go`, `go.mod` with `go 1.23` directive. Per-file unit tests and the fuzz target for `Date.UnmarshalJSON`.

**Critical actions:**
- Set `go 1.23` in `go.mod` -- resolves OSS-3 / stack inconsistency
- `Holiday` struct has **both** `StartDate` and `EndDate` -- never a single `Date` field (TZ-3)
- `Date` uses wrapper struct form `type Date struct{ time.Time }` for ergonomic method promotion
- Nullable fields (`comment`, `subdivisions`, `groups`) are slices -- nil-safe to iterate (OH-2)
- `name` field is `[]LocalizedText`, never `map[string]string` (OH-3)
- Lenient decoding is the default; `WithStrictDecoding` is opt-in (JSON-1)

**Avoids:** JSON-1, JSON-2, JSON-3, JSON-4, TZ-1, TZ-3, OH-3, OSS-3.
**Research flag:** Standard patterns -- no deeper research needed.

### Phase 2: Transport

**Rationale:** The `Client` struct, functional options, and the RoundTripper chain scaffold gate every subsequent endpoint. The first working endpoint (`Countries`) proves the entire pipeline end-to-end.

**Delivers:** `client.go`, `options.go`, `request.go` (unexported `newRequest`, `decode[T]`), `transport.go` (`composeHTTPClient`, `headerTransport`), `transport_logging.go`, `internal/testhttp/`, `countries.go` + `countries_test.go` (smoke-test endpoint), `version.go`.

**Critical actions:**
- Shallow-copy `*http.Client` in `composeHTTPClient` -- never store user's pointer directly
- `drainAndClose(b io.ReadCloser)` helper -- drain before close (HTTP-3)
- `io.LimitReader(resp.Body, 10<<20)` wrapping every decode (HTTP-4)
- Own `*http.Client` with 15 s timeout -- never touch `http.DefaultClient` (HTTP-1)
- `slog.Default()` logger, never `log.Printf` (LOG-2)
- `context.Context` first arg everywhere, never stored in `Client` (CTX-1)
- `User-Agent: go-openholidays/<version>` injected in `headerTransport` (HTTP-5)
- `Client` has zero exported fields (API-1)
- `Client.Close()` stub present -- even as no-op -- before Phase 4 adds the sweeper

**Avoids:** HTTP-1, HTTP-2, HTTP-3, HTTP-4, HTTP-5, CTX-1, CTX-3, LOG-1, LOG-2, API-1, API-3, CONC-1.
**Research flag:** Standard patterns -- no deeper research needed.

### Phase 3: Endpoints

**Rationale:** With the scaffold proven by `Countries`, the remaining four endpoints are mechanical. `Holiday` helpers depend on the type existing; this phase completes the type surface.

**Delivers:** `languages.go`, `subdivisions.go`, `public_holidays.go`, `school_holidays.go` with per-file unit tests and golden `testdata/*.json` fixtures. `holiday.go` with `Name(lang)`, `IsInRegion(code)`, `Days()`, `Range() iter.Seq[time.Time]` and helper tests. `validate.go` fully implemented.

**Critical actions:**
- `validateDateRange` enforces the 3-year cap before any network call (OH-1)
- `*APIError` constructed in endpoint methods, not in RoundTripper
- `ctx.Err()` propagated with `%w`, not flattened into `*APIError` (CTX-3)
- `Holiday.Days()` uses UTC-normalized midnight arithmetic, never `time.Sub` for day counting (TZ-2)
- `Holiday.Range()` iterates inclusively using `time.AddDate`
- Golden fixtures cover: no `comment`, `quality` field present, no `subdivisions`, 14-day ferie zimowe, full PL set

**Avoids:** OH-1, OH-2, OH-3, JSON-4, TZ-2, TZ-3, CTX-3.
**Research flag:** Standard patterns -- no deeper research needed. Generate golden fixtures from the live API here.

### Phase 4: Resilience

**Rationale:** Retry and cache are transparent middleware that slot into the RoundTripper chain without touching endpoint method signatures. Building them after endpoints confirms the architecture held.

**Delivers:** `transport_retry.go` (`retryTransport`, `RetryConfig`, `sleepCtx`, `parseRetryAfter`, `shouldRetry`), `transport_cache.go` (exported `Cache` interface, unexported `memoryCache` with `sync.RWMutex`+`map`, sweeper goroutine), `transport_hook.go`, `WithRetry`, `WithCache`, `WithRequestHook`, `WithStrictDecoding` options, `Client.Close()` wiring sweeper stop. `internal/clock` interface. Per-transport unit tests.

**Critical actions:**
- Retry is **opt-in** -- default is no retry; flip ON in M2 (M1 anti-feature decision)
- Retry-eligible: `{408, 429, 500, 502, 503, 504}` + network timeouts. Never retry 4xx except 408/429 (RETRY-1)
- `parseRetryAfter` handles integer-seconds and HTTP-date forms, capped at 60 s (RETRY-2)
- `sleepCtx` everywhere -- no bare `time.Sleep` in retry loop (CTX-2, RETRY-3)
- Full jitter formula per AWS canonical recommendation (RETRY-4)
- Cache stores only `err == nil && resp.StatusCode == 200` (CACHE-1)
- Cache lives on `Client`, keyed by `method + " " + fullURL` (CACHE-2)
- Sweeper stopped by `Client.Close()` (CACHE-3)
- `sync.RWMutex` not `sync.Map` (CACHE-5)
- `Cache` interface exported for M2 plug-in backends
- Defensive copy on cache hits (API-4)
- `Cache` and `RetryPolicy` injection points use interfaces (API-2)
- `Clock` interface in `internal/clock` for testable time (TEST-3)
- Fresh `*http.Request` built per retry attempt -- never reused (HTTP-6)

**Avoids:** RETRY-1 through RETRY-4, CACHE-1 through CACHE-5, CTX-2, HTTP-6, API-2, API-4, TEST-3.
**Research flag:** Standard patterns -- no deeper research needed.

### Phase 5: Distribution

**Rationale:** The library is feature-complete after Phase 4. This phase proves it with the demo CLI, hardens it with fuzz/integration/benchmark tests, and ships it.

**Delivers:** `cmd/ohcli/` (stdlib `flag`, under 300 lines), `example_test.go` (one runnable `Example` per public method), `integration_test.go` (`//go:build integration` + `OPENHOLIDAYS_LIVE=1`), fuzz targets, benchmarks, `.github/workflows/`, `.golangci.yml` (v2), `.goreleaser.yaml` (v2, 6 binary targets), `README.md`, `docs/design.md`, `CHANGELOG.md`, `CONTRIBUTING.md`. `v0.1.0` tag.

**Critical actions:**
- `cmd/ohcli` imports `github.com/<owner>/go-openholidays` at module path -- never `internal/`
- `goreleaser check` + `--snapshot --clean` dry-run in CI on every PR (OSS-4)
- `go test -run Example ./...` CI step confirms all examples compile and run (OSS-2)
- `goleak.VerifyTestMain` in every test package's `TestMain` (CONC-2)
- Pre-tag API surface review: every exported symbol justified or removed (API-1, OSS-1)
- Tag is `v0.1.0` not `v1.0.0` (OSS-1)
- `CHANGELOG.md` updated before tagging (OSS-5)
- `govulncheck` clean in CI
- `go test -race -coverprofile=cover.out ./...` at >= 85 % coverage
- Module path owner confirmed before the tag

**Avoids:** CONC-2, TEST-1, TEST-2, TEST-3, TEST-4, API-1, OSS-1, OSS-2, OSS-3, OSS-4, OSS-5.
**Research flag:** Standard patterns -- no deeper research needed.

### Phase Ordering Rationale

- Types before everything: the public contract; no test, endpoint, or helper can be finalized without it.
- Transport scaffold before first endpoint: proves the entire `NewClient -> chain -> decode` pipeline. ARCHITECTURE.md estimates ~4 h for the scaffold, ~2 h for `Countries` once in place.
- `Countries` as smoke-test before the remaining four: smallest payload, no date fields, no complex validation.
- Resilience after endpoints: if endpoints had to change when retry/cache were added, the RoundTripper design failed. Absence of churn is the correctness test.
- Distribution last: the CLI dogfoods the public API; running it against an incomplete API produces misleading feedback.
- `Cache` interface and `Client.Close()` must be decided in Phase 1 pre-planning and implemented in Phase 4; neither can be deferred to M2 without a contract break.

### Research Flags

No phase requires `/gsd:plan-phase --research-phase`. All research files contain sufficient detail to plan and implement directly. The only pre-implementation actions are PROJECT.md updates documented in Gaps to Address below.

All five phases have standard, well-documented patterns:
- **Phase 1:** Go types and JSON unmarshaling -- mature, well-documented.
- **Phase 2:** RoundTripper chain, functional options -- canonical patterns in go-github, stripe-go, hashicorp/go-retryablehttp.
- **Phase 3:** Five thin endpoint wrappers -- main work is accurate fixture data from the live API.
- **Phase 4:** Retry (~80 lines), cache (~50 lines) -- well-understood code with canonical reference implementations.
- **Phase 5:** golangci-lint v2, goreleaser v2, GitHub Actions matrix -- standard tooling with clear documentation.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | All decisions backed by official Go release notes, maintainer blog posts for golangci-lint v2 and goreleaser v2, cross-validated against 2025/2026 community practice. Sole MEDIUM item is gofumpt adoption (community split). |
| Features | HIGH | Verified against stripe-go, go-github, aws-sdk-go-v2, slack-go, and otelhttp at current versions. P1/P2/P3 tiering reflects real patterns from mature Go SDK libraries. |
| Architecture | HIGH | Single-package layout, RoundTripper chain, and build order cross-checked against go-github, stripe-go, hashicorp/go-retryablehttp, gregjones/httpcache. Chain order has MEDIUM on the exact hook/logging sub-ordering (defensible either way). |
| Pitfalls | HIGH | HIGH for Go HTTP/JSON pitfalls (stdlib-grade knowledge). HIGH for OpenHolidays-specific gotchas (verified against live OpenAPI spec 2026-05-26). MEDIUM for the 3-year window cap (asserted by PROJECT.md; absent from the OpenAPI spec). |

**Overall confidence:** HIGH

### Gaps to Address

1. **PROJECT.md requires two updates before Phase 1 planning:**
   - Change "Go >= 1.22" to "Go >= 1.23" in the Constraints section.
   - Add `Client.Close()` to Active requirements: "Client exposes a `Close()` method that stops the cache sweeper goroutine and releases resources." PITFALLS.md marks skipping this as "Never acceptable."

2. **Module path owner is unresolved** (PROJECT.md Key Decisions). Not a blocker for Phases 1-4 (use a placeholder), but must be resolved before Phase 5 tagging.

3. **3-year window cap is unverified in the upstream OpenAPI spec** (PITFALLS OH-1). Enforcement is correct regardless -- `ErrDateRangeTooLarge` is the right client-side behavior -- but validate the exact cap in an integration test.

## Sources

### Primary (HIGH confidence)

- `openholidaysapi.org/swagger/v1/swagger.json` (verified 2026-05-26) -- field shapes, nullable fields, status codes, absence of rate-limit headers
- Go 1.23 Release Notes (go.dev/doc/go1.23) -- `iter.Seq` package, range-over-func
- `pkg.go.dev/log/slog` -- official slog reference and library usage guidelines
- `go.dev/doc/modules/layout` -- official module layout guidance
- golangci-lint v2 launch post (ldez.github.io, 2025-03-23) -- v2 config schema
- goreleaser v2 docs (goreleaser.com) -- build matrix, version 2 format
- `pkg.go.dev/sync#Map` -- documented use cases confirming sync.Map is wrong for TTL caches
- `pkg.go.dev/context` -- official "do not store contexts in structs" guidance
- AWS Architecture Blog -- Exponential Backoff and Jitter -- canonical full-jitter formula

### Secondary (MEDIUM confidence)

- `github.com/google/go-github` -- verified typed error categories and context.Context first-arg pattern
- `github.com/stripe/stripe-go` -- verified functional options, no CLI in library repo
- `github.com/aws/aws-sdk-go-v2/aws/retry` -- verified Standard retryer, jitter formula
- `github.com/hashicorp/go-retryablehttp` -- reference for retry-as-RoundTripper pattern
- `github.com/gregjones/httpcache` -- reference for cache-as-RoundTripper pattern
- `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` -- verified NewTransport wrapping as canonical OTel integration
- `go.uber.org/goleak` -- goroutine leak detection in tests

### Tertiary (LOW confidence, validate during implementation)

- OpenHolidays 3-year query window cap -- asserted in PROJECT.md from POC observation; not documented in OpenAPI spec. Enforce defensively and validate in integration tests.

---
*Research completed: 2026-05-26*
*Ready for roadmap: yes*
