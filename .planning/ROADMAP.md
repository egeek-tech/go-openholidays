# Roadmap: go-openholidays

**Created:** 2026-05-27
**Granularity:** Standard
**Mode:** Standard (Horizontal Layers — library has no UI; build order is types → transport → endpoints → resilience → distribution)
**Parallelization:** Enabled

**Core Value:** A single, well-tested Go client returning both public holidays AND school holidays per administrative subdivision for the public OpenHolidays API, with zero runtime dependencies, full `context.Context` propagation, and typed errors.

**Coverage:** 82/82 v1 requirements mapped (100%)

---

## Phases

- [x] **Phase 1: Foundation** — Domain types, custom `Date`, sentinel errors, `*APIError`, validators, `go.mod` at Go 1.23. (completed 2026-05-27)
- [x] **Phase 2: Transport** — `Client`, functional options, RoundTripper chain (header + logging), first endpoint (Countries) end-to-end. (completed 2026-05-27)
- [x] **Phase 3: Endpoints & Helpers** — Languages, Subdivisions, PublicHolidays, SchoolHolidays + `Holiday.Name/IsInRegion/Days/Range` helpers + golden fixtures. (completed 2026-05-27)
- [ ] **Phase 4: Resilience** — Retry, cache, observability hook, strict-decoding, `Client.Close()` wiring — all as transparent middleware that does NOT modify Phase 3 method signatures.
- [ ] **Phase 5: Distribution** — `cmd/ohcli`, examples, fuzz, benchmarks, integration tests, CI matrix, golangci-lint, govulncheck, goreleaser, docs, `v0.1.0` tag.

---

## Phase Details

### Phase 1: Foundation
**Goal**: Domain types, `Date`, errors, and validators exist as a zero-dependency package; `go.mod` declares Go 1.23; the public type contract is stable.
**Depends on**: Nothing (first phase).
**Requirements**: TYPES-01, TYPES-02, TYPES-03, TYPES-04, TYPES-05, ERR-01, ERR-02, ERR-03, ERR-04, VALID-01, VALID-02, VALID-03, VALID-04, CLIENT-10
**Success Criteria** (what must be TRUE):
  1. `var d Date; json.Unmarshal([]byte(`"2025-12-24"`), &d)` round-trips through `MarshalJSON` to the same bytes; `null` and empty strings produce errors, not silent zero values.
  2. `errors.Is(fmt.Errorf("country %q: %w", "ZZZ", ErrInvalidCountry), ErrInvalidCountry)` returns `true` for every sentinel (`ErrInvalidCountry`, `ErrInvalidLanguage`, `ErrDateRangeTooLarge`, `ErrEmptyResponse`).
  3. `errors.As(err, &apiErr)` extracts a `*APIError` with populated `StatusCode`, `Path`, `Body` fields from a wrapped error chain.
  4. `validate*` functions reject 1-letter, lowercase, 3-letter country codes, `validFrom > validTo`, and date windows > 3 years with the correct sentinel; no global state, no `init()` side effects.
  5. `go build ./...` and `go vet ./...` succeed on Go 1.23 with the `go 1.23` directive in `go.mod`.
**Plans**: 6 plans
  - [x] 01-01-PLAN.md — Module bootstrap: go.mod (github.com/egeek-tech/go-openholidays, go 1.23, testify v1.11.1), LICENSE (MIT), doc.go, version.go
  - [x] 01-02-PLAN.md — Sentinel errors + *APIError type (5 sentinels, Error(), Is() wildcard semantics, no Unwrap)
  - [x] 01-03-PLAN.md — Custom Date wrapper struct (Marshal/UnmarshalJSON, NewDate, ParseDate, comparison helpers, DaysUntil, FuzzDateUnmarshal)
  - [x] 01-04-PLAN.md — Domain types (Holiday, Country, Language, Subdivision, LocalizedText, SubdivisionRef, GroupRef, HolidayType×6, NameFor accessors)
  - [x] 01-05-PLAN.md — Validators (validateCountry, validateLanguage, validateDateRange) with leap-year boundary coverage
  - [x] 01-06-PLAN.md — CLIENT-10 AST audit (TestNoInitOrGlobalState) + PROJECT.md Key Decisions update (CL-01..CL-05)

### Phase 2: Transport
**Goal**: `Client` constructed via functional options, RoundTripper chain composes header + logging, `Countries` proves the end-to-end pipeline (NewClient → chain → decode → typed return).
**Depends on**: Phase 1 (types and errors exist).
**Requirements**: CLIENT-01, CLIENT-02, CLIENT-03, CLIENT-04, CLIENT-05, CLIENT-06, CLIENT-07, CLIENT-08, CLIENT-09, ENDPT-01, TRANS-01, TRANS-02, TRANS-03, TRANS-04, OBS-01, OBS-02, TEST-04
**Success Criteria** (what must be TRUE):
  1. `c := NewClient(WithBaseURL(ts.URL), WithUserAgent("test/1"))` returns a usable client without errors; subsequent `c.Countries(ctx)` against an `httptest.Server` returns typed `[]Country` and the server received `Accept: application/json` plus a `User-Agent` matching `^go-openholidays/`.
  2. `ctx, cancel := context.WithCancel(...); cancel(); c.Countries(ctx)` returns within ≤ 100 ms with `errors.Is(err, context.Canceled)` true (TestClient_ContextCancel).
  3. `TestClient_ConcurrentAccess` runs N parallel `Countries` calls under `-race` and exits cleanly with zero data-race reports.
  4. A server returning a 12 MiB body causes `c.Countries(ctx)` to return a typed oversized-response error (10 MiB cap via `io.LimitReader`); response bodies are always drained then closed on every code path (verified by a `goleak`-style FD audit in tests).
  5. `Client.Close()` exists as an idempotent no-op stub callable from any goroutine; logging emits structured `slog` records at `Debug` level with `method`, `path`, `status`, `duration_ms`, `attempt`, `bytes_in` fields — never response bodies above `Debug`.
**Plans**: 4 plans
  - [x] 02-01-PLAN.md — Transport RoundTrippers (headerTransport + loggingTransport) with per-RT unit tests; covers TRANS-01, TRANS-04, OBS-01, OBS-02
  - [x] 02-02-PLAN.md — Client + Options + Config scaffolding (NewClient, WithX, composeHTTPClient, buildTransport, Close stub); covers CLIENT-01..06, CLIENT-08
  - [x] 02-03-PLAN.md — Countries endpoint + ErrResponseTooLarge sentinel + CLIENT-10 allowlist + httptest suite + fixture + concurrent/ctx-cancel umbrella tests; covers ENDPT-01, TRANS-02, TRANS-03, CLIENT-07, CLIENT-09, TEST-04
  - [x] 02-04-PLAN.md — W-01 validator hardening (ASCII shape check before case canonicalization) folded into Phase 2 per CONTEXT.md D-32; covers VALID-01, VALID-04

### Phase 3: Endpoints & Helpers
**Goal**: All four remaining endpoints (`Languages`, `Subdivisions`, `PublicHolidays`, `SchoolHolidays`) ship with golden-fixture tests; `Holiday` helpers (`Name`, `IsInRegion`, `Days`, `Range`) return correct values for the verified Polish 2025 data.
**Depends on**: Phase 2 (Client + transport scaffold exists).
**Requirements**: ENDPT-02, ENDPT-03, ENDPT-04, ENDPT-05, HELP-01, HELP-02, HELP-03, HELP-04, TEST-01, TEST-02, TEST-03
**Success Criteria** (what must be TRUE):
  1. `c.PublicHolidays(ctx, PublicHolidaysRequest{CountryIsoCode: "PL", ValidFrom: date(2025,1,1), ValidTo: date(2025,12,31)})` against the golden PL-2025 fixture returns exactly 14 typed `Holiday` structs (including Dec 24 Christmas Eve added 2025) without panics or decode errors.
  2. `c.SchoolHolidays(ctx, SchoolHolidaysRequest{CountryIsoCode: "PL", ...})` against the golden fixture returns 7 periods, and `holiday.IsInRegion("PL-SL")` correctly identifies the Śląskie ferie zimowe cohort while excluding the other three regional cohorts.
  3. `holiday.Name("pl")` returns the Polish localized name; `holiday.Name("xx")` falls back to the first available `LocalizedText` entry (not the empty string).
  4. `holiday.Days()` returns 14 for a 14-day Śląskie ferie zimowe period crossing a DST boundary; `holiday.Range()` (Go 1.23 `iter.Seq[time.Time]`) iterates exactly 14 dates inclusively from StartDate to EndDate.
  5. Each endpoint has a table-driven unit test covering happy path + ≥ 4 error paths (network failure, 4xx, 5xx, malformed JSON, ctx cancel); all fixtures in `testdata/` come from captured live responses and a `-update` flag regenerates them.
**Plans**: 8 plans
  - [x] 03-01-PLAN.md — request.go extract (doJSONGet[T any]) + Countries refactor + Countries(ctx, CountriesRequest) retrofit (CL-08 foundation)
  - [x] 03-02-PLAN.md — Languages endpoint + LanguagesRequest + fixture + test
  - [x] 03-03-PLAN.md — Subdivisions endpoint + SubdivisionsRequest + PL & DE fixtures + test (DE fixture seeds Plan 7 hierarchical test per Assumption A3)
  - [x] 03-04-PLAN.md — PublicHolidays endpoint + PublicHolidaysRequest + validateHolidays helper + ErrMalformedResponse sentinel + allowedVars extension + fixture + test (CL-12)
  - [x] 03-05-PLAN.md — SchoolHolidays endpoint + SchoolHolidaysRequest + fixture + test
  - [x] 03-06-PLAN.md — Holiday.NameFor + Holiday.IsInRegion (flat) + Holiday.Days + Holiday.Range (iter.Seq[Date]) + tests (CL-10, CL-11)
  - [x] 03-07-PLAN.md — Client.IsInRegion hierarchical + splitCountryFromSubdivision + buildParentIndex + tests against DE fixture (CL-09)
  - [x] 03-08-PLAN.md — update_fixtures_test.go: build-tagged integration -update mechanism + drift detection (covers TEST-02, TEST-03)

### Phase 4: Resilience
**Goal**: Retry, cache, observability hook, and strict-decoding land as transparent middleware. Endpoint method signatures from Phase 3 remain UNCHANGED — the RoundTripper chain absorbs all new behavior. `Client.Close()` becomes load-bearing (stops cache sweeper).
**Depends on**: Phase 3 (endpoints exist and have golden tests that must still pass).
**Requirements**: RESIL-01, RESIL-02, RESIL-03, RESIL-04, RESIL-05, RESIL-06, RESIL-07, RESIL-08, RESIL-09, TRANS-05, OBS-03, TEST-05, TEST-06
**Success Criteria** (what must be TRUE):
  1. `c := NewClient(WithRetry(5, 250*time.Millisecond))` against a fake transport returning 429 → 500 → 200 retries with exponential backoff + full jitter (`math/rand/v2`), honors `Retry-After: 2` and `Retry-After: <HTTP-date>`, and exits with `ctx.Err()` if context cancels mid-sleep — verified by deterministic-clock unit tests; retry is implemented in the endpoint layer (not as a RoundTripper) so caller-supplied `*http.Client` retries do not double-fire.
  2. `c := NewClient(WithCache(24*time.Hour))` caches `Countries`/`Languages`/`Subdivisions` raw bytes keyed by `(method, path, query)`; second call returns in < 5 ms; `PublicHolidays`/`SchoolHolidays` are NEVER cached by default; cache hits still work after `WithStrictDecoding(true)` is added (decoding happens on read).
  3. `Client.Close()` stops the cache sweeper goroutine; `goleak.VerifyNone(t)` (or equivalent) passes after `Close()`; `Close()` is safe to call twice from concurrent goroutines.
  4. `c := NewClient(WithRequestHook(hook))` invokes `hook(req, resp, err)` after every round trip (including retries); the hook sees cache-hit responses as well; the hook does not introduce dependencies on OpenTelemetry or any external observability library.
  5. `c := NewClient(WithStrictDecoding(true))` fails the decode when an upstream JSON response contains a field absent from our struct (verified with a fixture containing an injected `extra_unknown_field`); strict-decoding is OFF by default (lenient decode is the default, so adding upstream fields does not break consumers); ALL Phase 3 endpoint signatures and tests are untouched (no diff to `public_holidays.go` API surface).
**Plans**: 6 plans
  - [x] 04-01-PLAN.md — Wave 0 test scaffold: clock_test.go with fakeClock (Now/Advance/Sleep) + race-free smoke test (D-95); unblocks retry + cache fake-clock tests
  - [x] 04-02-PLAN.md — Strict decoding + Client field plumbing: extends Client struct with 8 Phase 4 fields (closeOnce, nowFunc, sleepFunc, rand, strict, cache, requestHook, retry); declares Cache interface + RequestHookFunc + retryConfig stub; ships WithStrictDecoding(bool); adds ctxSleep + newClientRand helpers + newClientForTest seam (D-91/D-92/D-94/D-78/D-85)
  - [x] 04-03-PLAN.md — Retry layer (retry.go): retryConfig (filled), shouldRetry, parseRetryAfter, computeBackoff + WithRetry(n, baseDelay) + WithMaxRetryWait(d); wraps c.http.Do inside doJSONGet (D-77 + RESIL-05); 10 tests (TestShouldRetry, TestParseRetryAfter, TestComputeBackoff, TestRetry_E2E_429Then500Then200, TestRetry_HonorsRetryAfter[Seconds|Date], TestRetry_CtxCancel, TestRetry_NeverRetriesCtxErrors, TestRetry_DeterministicClock, TestRetry_NotARoundTripper)
  - [x] 04-04-PLAN.md — Cache layer (cache.go + transport_cache.go): MemoryCache + NewMemoryCache + sweeper goroutine + cacheTransport + isCacheablePath + CacheHitContextKey; WithCache(ttl) + WithCacheBackend(c Cache); buildTransport edit (cache above logging); allowedVars adds CacheHitContextKey (DEVIATION from CONTEXT.md D-97 step 6); composition tests for sweeper-stop, strict+cache, default-off, per-Client isolation (RESIL-06..09 + CLIENT-08 full wiring)
  - [ ] 04-05-PLAN.md — Hook RoundTripper (transport_hook.go): hookTransport (outermost per D-89) + WithRequestHook(fn); buildTransport edit (hook outermost when cfg.hook != nil); composition tests for fires-per-attempt, sees-cache-hits, does-not-fire-on-decode-error, panic-propagates (TRANS-05)
  - [ ] 04-06-PLAN.md — PROJECT.md Key Decisions append: CL-15 (Cache public surface) + CL-16 (strict-decoding immutability); DEVIATION from CONTEXT.md D-80 wording — Phase 3 already took CL-14, so cache uses CL-15 and strict uses CL-16

### Phase 5: Distribution
**Goal**: Library is feature-complete and demonstrably production-ready: demo CLI dogfoods the public surface, fuzz/integration/benchmark tests are wired, CI matrix is green across Go 1.23/1.24/stable, docs render cleanly on `pkg.go.dev`, and `v0.1.0` ships via `goreleaser`.
**Depends on**: Phase 4 (resilience landed; library API is now stable for tagging).
**Requirements**: CLI-01, CLI-02, CLI-03, CLI-04, TEST-07, TEST-08, TEST-09, TEST-10, TEST-11, CI-01, CI-02, CI-03, CI-04, CI-05, CI-06, CI-07, DOC-01, DOC-02, DOC-03, DOC-04, DOC-05, DOC-06, DOC-07, REL-01, REL-02, REL-03, REL-04
**Success Criteria** (what must be TRUE):
  1. `go install ./cmd/ohcli` builds clean on Linux + macOS in CI; `ohcli public PL 2025` prints an aligned text table of all 14 PL 2025 public holidays; `ohcli school PL 2025 --region PL-SL` prints the Śląskie school-holiday subset; the CLI imports the library at its module path with zero non-stdlib dependencies (stdlib `flag` only).
  2. CI matrix `{go: [1.23, 1.24, stable], os: ubuntu-latest}` runs `go vet`, `go build`, `go test -race -cover` (coverage ≥ 85%), `golangci-lint` (govet, errcheck, staticcheck, gosec, revive, gocritic), `govulncheck`, and `go mod tidy && git diff --exit-code` — all green; a nightly `integration.yml` runs `OPENHOLIDAYS_LIVE=1 go test -tags=integration` against the live API.
  3. `FuzzParseLocalizedText` and `FuzzUnmarshalHoliday` run for ≥ 60 s without surfacing panics; benchmarks confirm `PublicHolidays(PL, 2025)` < 500 ms cold and < 5 ms cached.
  4. `pkg.go.dev` renders the package with at least one `Example_*` per public method (all examples compile under `go test -run Example`); every exported symbol has a doc comment beginning with its name; `README.md` ≤ 20-line quickstart compiles; `docs/design.md`, `CHANGELOG.md` (keep-a-changelog), and `CONTRIBUTING.md` exist; Go Report Card grade A on first scan.
  5. `v0.1.0` git tag is pushed with a confirmed module path owner committed to `go.mod`; `goreleaser` produces CLI binaries for linux/darwin/windows × amd64/arm64 attached to the GitHub Release; `release.yml` workflow ran clean on the tag; Dependabot is configured for GitHub Actions versions; coverage badge wired to Codecov or coveralls.
**Plans**: TBD

---

## Progress

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Foundation | 6/6 | Complete   | 2026-05-27 |
| 2. Transport | 4/4 | Complete   | 2026-05-27 |
| 3. Endpoints & Helpers | 11/11 | Complete   | 2026-05-27 |
| 4. Resilience | 4/6 | In Progress|  |
| 5. Distribution | 0/0 | Not started | - |

---

## Architectural Notes

- **RoundTripper-chain decoupling is load-bearing**: Phase 4 (Resilience) MUST NOT alter any method signature shipped in Phase 3. If retry or cache work forces a signature change, the architecture has failed — back out and redesign before continuing. This is the explicit correctness test for the transport layer.
- **`Client.Close()` is required from Phase 2 (stub) and load-bearing in Phase 4 (sweeper stop)**. Adding it post-1.0 would be a breaking change; it is in v1 scope by deliberate decision.
- **TEST-* requirements are partly cross-cutting**: TEST-01/02/03 land alongside endpoints in Phase 3; TEST-04 lands with the Client in Phase 2; TEST-05/06 land with retry/cache in Phase 4. Fuzz (TEST-07), integration (TEST-08), examples (TEST-09), coverage gate (TEST-10), and benchmarks (TEST-11) all roll up to Phase 5 because that is where CI scheduling exists.
- **CLIENT-10 (no `init()` side effects, no global mutable state)** is a Phase 1 invariant because it must be true from the first line of code; verifying it requires no runtime artifacts.
- **REL-04 (module path owner)**: must be resolved before Phase 5 ends. PROJECT.md flags this as deferred — Phases 1-4 use a placeholder; tagging at the end of Phase 5 requires the real owner.

---

*Roadmap created: 2026-05-27 by gsd-roadmapper from REQUIREMENTS.md + research synthesis*
