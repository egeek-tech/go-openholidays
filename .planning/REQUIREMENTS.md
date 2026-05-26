# Requirements: go-openholidays

**Defined:** 2026-05-27
**Core Value:** A single, well-tested Go client returning both public holidays AND school holidays per administrative subdivision for the public OpenHolidays API, with zero runtime dependencies, full `context.Context` propagation, and typed errors.

## v1 Requirements

Requirements for `v0.1.0` initial release. Each maps to a roadmap phase.

### Client

- [ ] **CLIENT-01**: `NewClient(opts ...Option) *Client` constructs a client; never returns an error (validation happens per-request).
- [ ] **CLIENT-02**: `WithHTTPClient(*http.Client)` lets users supply a pre-configured HTTP client; SDK shallow-copies and wraps its Transport to avoid leaking back caller mutations.
- [ ] **CLIENT-03**: `WithBaseURL(string)` overrides the default base URL (`https://openholidaysapi.org`) for testing and mirrors.
- [ ] **CLIENT-04**: `WithUserAgent(string)` overrides the default `User-Agent: go-openholidays/<version>`.
- [ ] **CLIENT-05**: `WithLogger(*slog.Logger)` injects a structured logger; defaults to `slog.Default()`.
- [ ] **CLIENT-06**: `WithTimeout(time.Duration)` sets the per-request timeout (default 15 s).
- [ ] **CLIENT-07**: `Client` is goroutine-safe — concurrent calls from multiple goroutines work without races, verified by `TestClient_ConcurrentAccess` under `-race`.
- [ ] **CLIENT-08**: `Client.Close() error` stops background goroutines (cache sweeper) and is idempotent; safe to call from any goroutine.
- [ ] **CLIENT-09**: `context.Context` cancellation interrupts in-flight HTTP within ≤ 100 ms (verified by `TestClient_ContextCancel`).
- [ ] **CLIENT-10**: No `init()` side effects, no global mutable state.

### Endpoints

- [ ] **ENDPT-01**: `Countries(ctx) ([]Country, error)` fetches the supported-countries list.
- [ ] **ENDPT-02**: `Languages(ctx) ([]Language, error)` fetches the supported-languages list.
- [ ] **ENDPT-03**: `Subdivisions(ctx, country, lang) ([]Subdivision, error)` fetches administrative subdivisions for a country.
- [ ] **ENDPT-04**: `PublicHolidays(ctx, PublicHolidaysRequest) ([]Holiday, error)` fetches public holidays with country/language/date-range filters.
- [ ] **ENDPT-05**: `SchoolHolidays(ctx, SchoolHolidaysRequest) ([]Holiday, error)` fetches school holidays with the same filters plus optional subdivision filter.

### Types

- [ ] **TYPES-01**: `Holiday` struct with `StartDate`, `EndDate`, `Type`, `Name`, `RegionalScope`, `TemporalScope`, `Nationwide`, `Subdivisions`, `Comment`, `Quality` fields; all decoded from upstream JSON shape.
- [ ] **TYPES-02**: Custom `Date` type (`type Date struct { time.Time }`) with `UnmarshalJSON`/`MarshalJSON` round-tripping `YYYY-MM-DD`; embedded so `time.Time` methods are promoted.
- [ ] **TYPES-03**: `LocalizedText{Language, Text}` and `SubdivisionRef{Code, ShortName}` companion types.
- [ ] **TYPES-04**: Typed enum for `Holiday.Type` (`Public`, `School`, `Bank`, `Observance`) — a typed string with package-level constants.
- [ ] **TYPES-05**: `Country`, `Language`, `Subdivision` reference types with `Name(lang string) string` accessor that falls back to first entry if requested language missing.

### Errors

- [ ] **ERR-01**: Sentinel errors exposed: `ErrInvalidCountry`, `ErrInvalidLanguage`, `ErrDateRangeTooLarge`, `ErrEmptyResponse`.
- [ ] **ERR-02**: `*APIError{StatusCode int, Path string, Body []byte}` implements `error`; `errors.As` retrieves it from wrapped errors.
- [ ] **ERR-03**: All transport-level errors wrap underlying cause with `%w`; `errors.Is(err, ErrSentinel)` works through the wrapper.
- [ ] **ERR-04**: Error messages never include credentials or full response bodies; raw body lives only in `APIError.Body` (caller opt-in).

### Validation

- [ ] **VALID-01**: `PublicHolidaysRequest` and `SchoolHolidaysRequest` validate before sending HTTP: `countryIsoCode` is 2 uppercase ASCII letters; non-empty.
- [ ] **VALID-02**: `validFrom <= validTo` enforced; else `ErrDateRangeTooLarge` family error.
- [ ] **VALID-03**: Date window > 3 years rejected with `ErrDateRangeTooLarge` (defensive client-side enforcement of upstream cap).
- [ ] **VALID-04**: Language code validated as ISO 639-1 2-letter code; else `ErrInvalidLanguage`.

### Resilience

- [ ] **RESIL-01**: `WithRetry(maxAttempts int, baseDelay time.Duration)` opt-in retry with exponential backoff + full jitter (using `math/rand/v2`).
- [ ] **RESIL-02**: Retry triggers on HTTP 429, 5xx, and transient network errors (`net.Error.Temporary()` legacy + connection reset); does NOT retry on 4xx (except 429).
- [ ] **RESIL-03**: Retry honors `Retry-After` header in both integer-seconds and HTTP-date forms; takes the larger of jitter delay and `Retry-After`.
- [ ] **RESIL-04**: Retry loop is `ctx`-aware — `ctx.Done()` interrupts the sleep and returns `ctx.Err()`.
- [ ] **RESIL-05**: Retry is implemented in the endpoint layer (not as a RoundTripper) to avoid double-retry when the caller's `*http.Client` already has its own retry.
- [ ] **RESIL-06**: `Cache` interface exposed publicly (`Get(key) (val, ok)` / `Set(key, val, ttl)` / `Close()`) even though only the in-memory impl ships in v0.1.0.
- [ ] **RESIL-07**: `WithCache(ttl time.Duration)` opt-in in-memory TTL cache for `Countries`, `Languages`, `Subdivisions` only — holiday endpoints are never cached by default.
- [ ] **RESIL-08**: Cache sweeper goroutine started lazily on first `Set` and stopped by `Client.Close()`; no goroutine leak verified by `goleak` (test-only dep) or manual leak check.
- [ ] **RESIL-09**: Cache stores raw response bytes keyed by `(method, path, query)`; decoding happens on read so strict-decoding mode does not break cached entries.

### Transport

- [ ] **TRANS-01**: All requests include `Accept: application/json` and `User-Agent: go-openholidays/<version>` headers.
- [ ] **TRANS-02**: All response bodies are read through `io.LimitReader` capped at 10 MiB; oversized responses return a typed error.
- [ ] **TRANS-03**: Response bodies are always drained then closed via `defer`, including on early returns and parse errors.
- [ ] **TRANS-04**: Custom RoundTripper chain composes header injection, logging, and the observability hook; each RoundTripper is independently unit-tested.
- [ ] **TRANS-05**: `WithRequestHook(func(*http.Request, *http.Response, error))` exposes the round-trip for OTel/metrics integration without forcing a dependency.

### Helpers

- [ ] **HELP-01**: `Holiday.Name(lang string) string` returns the localized name with fallback to the first entry when `lang` not present.
- [ ] **HELP-02**: `Holiday.IsInRegion(subdivisionCode string) bool` returns true when the holiday applies in the given subdivision (or is nationwide).
- [ ] **HELP-03**: `Holiday.Days() int` returns inclusive day count from StartDate to EndDate.
- [ ] **HELP-04**: `Holiday.Range() iter.Seq[time.Time]` emits every day from StartDate to EndDate inclusive (Go 1.23 range-over-func).

### Observability

- [ ] **OBS-01**: HTTP requests/responses logged at `slog.LevelDebug` only; response body content never logged at Info or above.
- [ ] **OBS-02**: Structured fields: `method`, `path`, `status`, `duration_ms`, `attempt`, `bytes_in`.
- [ ] **OBS-03**: `WithStrictDecoding(bool)` opt-in mode uses `json.Decoder.DisallowUnknownFields()` to surface upstream schema drift.

### CLI

- [ ] **CLI-01**: `cmd/ohcli public <country> <year> [--lang xx]` prints public holidays as an aligned text table.
- [ ] **CLI-02**: `cmd/ohcli school <country> <year> [--region CC-RR]` prints school holidays optionally filtered by subdivision.
- [ ] **CLI-03**: CLI uses stdlib `flag` only (zero deps); imports the library at its module path (treats it as an external consumer).
- [ ] **CLI-04**: `go install` of the CLI builds clean on Linux + macOS in CI.

### Testing

- [ ] **TEST-01**: Unit tests per endpoint cover happy path + 4 error paths (network failure, 4xx, 5xx, malformed JSON, ctx cancel); table-driven.
- [ ] **TEST-02**: Tests use `httptest.NewServer` — no live network calls outside integration tests.
- [ ] **TEST-03**: Golden JSON fixtures in `testdata/` captured from live API; tests assert unmarshal correctness; `-update` flag regenerates.
- [ ] **TEST-04**: `TestClient_ConcurrentAccess` runs N parallel requests under `-race` and verifies no races.
- [ ] **TEST-05**: Retry/backoff tests using a fake transport that returns 429/500 then 200; verifies jitter, `Retry-After`, and ctx-cancellation paths.
- [ ] **TEST-06**: Cache tests cover TTL eviction, hit/miss, default-off behavior; use a fake clock to avoid `time.Sleep`.
- [ ] **TEST-07**: Fuzz tests `FuzzParseLocalizedText` and `FuzzUnmarshalHoliday` surface panics on malformed inputs.
- [ ] **TEST-08**: Integration tests gated by `//go:build integration` and `OPENHOLIDAYS_LIVE=1` env var; hit live API for PL 2025; run nightly in CI.
- [ ] **TEST-09**: `Example_*` tests in `example_test.go` compile under `go test` and render in `pkg.go.dev`.
- [ ] **TEST-10**: Coverage ≥ 85 % enforced in CI (`go test -cover ./...`).
- [ ] **TEST-11**: Benchmarks for cold and cached `PublicHolidays(PL, 2025)` confirm < 500 ms cold and < 5 ms cached targets.

### CI/CD

- [ ] **CI-01**: GitHub Actions `ci.yml` runs on push + PR with matrix `{go: [1.23, 1.24, stable], os: [ubuntu-latest]}`.
- [ ] **CI-02**: CI pipeline steps: `go vet`, `go build ./...`, `go test -race -cover ./...`, `golangci-lint run` (govet, errcheck, staticcheck, gosec, revive, gocritic), `govulncheck ./...`.
- [ ] **CI-03**: `go mod tidy` produces no diff (`go mod tidy && git diff --exit-code`).
- [ ] **CI-04**: Nightly `integration.yml` runs integration tests against live API.
- [ ] **CI-05**: `release.yml` on `v*` tag runs `goreleaser` producing CLI binaries for linux/darwin/windows × amd64/arm64.
- [ ] **CI-06**: Dependabot configured for GitHub Actions versions.
- [ ] **CI-07**: Coverage badge wired (Codecov or coveralls).

### Documentation

- [ ] **DOC-01**: `README.md` includes badges, install snippet, ≤ 20-line quickstart that compiles, full public API table, contributing pointer, license.
- [ ] **DOC-02**: `doc.go` package-level overview with one runnable example for `Client.PublicHolidays`.
- [ ] **DOC-03**: `example_test.go` provides at least one `Example_*` per public method.
- [ ] **DOC-04**: `docs/design.md` short architecture doc — Client lifecycle, RoundTripper chain, retry/cache architecture, error model.
- [ ] **DOC-05**: `CHANGELOG.md` in keep-a-changelog format with `v0.1.0` initial entry.
- [ ] **DOC-06**: `CONTRIBUTING.md` documents local dev loop, how to run unit vs integration vs fuzz tests.
- [ ] **DOC-07**: Every exported symbol has a doc comment that begins with the symbol name (Go style).

### Release

- [ ] **REL-01**: `pkg.go.dev` renders package docs cleanly with all examples runnable.
- [ ] **REL-02**: Go Report Card grade A on first publish.
- [ ] **REL-03**: `v0.1.0` git tag pushed; GoReleaser produces release artifacts attached to the GitHub Release.
- [ ] **REL-04**: Module path owner confirmed and committed to `go.mod` before tagging (resolves the deferred decision).

## v2 Requirements

Deferred to milestone M2+; tracked but not in this roadmap.

### Resilience (v2)

- **RESIL-V2-01**: Flip retry default to ON after observing production behavior.
- **RESIL-V2-02**: Auto-batching `iter.Seq2[Holiday, error]` iterator for date windows > 3 years.
- **RESIL-V2-03**: Single-flight on cache miss for thundering-herd protection.
- **RESIL-V2-04**: Persistent cache backends (Redis, SQLite) using the M1 `Cache` interface.

### Errors (v2)

- **ERR-V2-01**: Category-level typed errors (`*RateLimitError`, `*ServerError`) if real-world `errors.As` patterns emerge.

### Extensions (v3+)

- **EXT-V3-01**: Working-day arithmetic (`IsWorkingDay`, `NextWorkingDay`) across regions (M3).
- **EXT-V3-02**: Polish observances sub-package — Mother's Day, Children's Day, Father's Day, Andrzejki (M3).
- **EXT-V4-01**: OpenAPI codegen pipeline replacing hand-written types (M4).
- **EXT-V4-02**: iCal parser and exporter (M4).
- **EXT-V5-01**: Stable `v1.0.0` API freeze, full docs site (M5).

## Out of Scope

Explicitly excluded; documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Persisting holidays to a database | Caller responsibility, not an SDK concern. |
| Calendar UI / web frontend | This is a library; UIs build on top of it. |
| Multi-country aggregation helpers in M1 | Adds complexity; defer until single-country path is proven. |
| Non-Go ports (TypeScript, Python) | Out of scope for this repo. |
| Self-hosted OpenHolidays mirror | Different project entirely. |
| Localization of error messages | Errors stay English; matches Go stdlib norms. |
| gRPC / GraphQL transports | Upstream is REST/JSON; transport translation is a separate library concern. |
| Multi-tenant API-key support | OpenHolidays has no auth; would speculate ahead of demand. |
| Retry-as-RoundTripper | Conflicts with caller-supplied `*http.Client` that may already have retry; would double-retry. |
| Direct OTel dependency | Forces a heavy dep on consumers; `WithRequestHook` covers the same use case. |
| Per-endpoint typed errors (`*NotFoundError`, etc.) | Premature at five endpoints; `*APIError` + sentinels are enough. Add category-level types in M2 only if real patterns emerge. |
| `gen.go` / OpenAPI codegen in M1 | Upstream schema observed to drift (`quality` not in spec); hand-written types decouple library from spec instability. Revisit in M4. |

## Traceability

Mapping requirements → roadmap phases. Populated after roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| _Will be populated by gsd-roadmapper_ | | |

**Coverage:**
- v1 requirements: 65 total
- Mapped to phases: 0 (pending roadmap)
- Unmapped: 65 ⚠️ (will be 0 after roadmap commits)

---
*Requirements defined: 2026-05-27*
*Last updated: 2026-05-27 after initial definition*
