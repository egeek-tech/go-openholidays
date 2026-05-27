# Phase 3: Endpoints & Helpers - Research

**Researched:** 2026-05-27
**Domain:** Idiomatic Go 1.23 generics, `iter.Seq[T]` range-over-func, table-driven `httptest.NewServer` golden-fixture testing, hierarchical subdivision tree walking, post-decode validation hooks for `[]Holiday`-returning endpoints
**Confidence:** HIGH (every recommendation is grounded in either an existing Phase 1/2 source file, the verified live upstream API response from 2026-05-27, the Go 1.23 release notes / `pkg.go.dev/iter` docs, or `pkg.go.dev/testing`/`pkg.go.dev/flag` reference docs)

## Summary

CONTEXT.md is exceptionally detailed and locks 22 decisions (D-51..D-72) covering endpoint signatures, helper bodies, `request.go` extraction, `validateHolidays`, error sentinels, fixture mechanism, and plan sequencing. This research fills the seven residual gaps the planner asked about (generic `doJSONGet[T]` ergonomics, `iter.Seq[Date]` body, `-update` fixture mechanism, `httptest` table-test patterns for endpoints with required query params, hierarchical subdivision walking, post-decode validation idiom, and fixture-capture protocol).

The most consequential live-API finding is that **PL `/Subdivisions` returns 16 flat województwa with NO `children` field** (verified live 2026-05-27 against `https://openholidaysapi.org/Subdivisions?countryIsoCode=PL&languageIsoCode=EN`). German `/Subdivisions` is also flat for 15 of 16 Bundesländer; only Bavaria (`DE-BY`) carries a single child (Augsburg). D-59's hierarchical `Client.IsInRegion` therefore degrades to a flat lookup for PL — the recursive-walk logic still belongs in the codebase as a safety net for countries/upstream future state that exposes nesting, but **the canonical PL test fixture cannot exercise the recursive path**. The plan must either (a) accept that `Client.IsInRegion` for PL behaves identically to `Holiday.IsInRegion` and test the recursive path against a *constructed* (non-fixture) tree, or (b) capture the German fixture (which has at least one `children`-bearing entry under DE-BY) and test the recursive path there.

**Primary recommendation:** Use the **post-decode validation idiom** — `doJSONGet[T]` stays purely transport-and-decode and has NO validation hook; the two affected endpoints (`PublicHolidays`, `SchoolHolidays`) call `validateHolidays(holidays, "/PublicHolidays")` explicitly on the decode result before returning. This keeps the generic helper minimal, makes the validation site visible at the call site, and avoids the `validate func(T) error` parameter that would need an awkward identity-validator default for the three endpoints whose types have no temporal invariants to enforce.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Input validation (country/language/date) | API method layer (endpoint file) | — | Phase 1 D-20..D-22 + ARCHITECTURE.md Pattern 5: free functions in `validate.go` called from each endpoint method before HTTP. Cannot live in the transport — RoundTrippers do not have endpoint context. |
| HTTP request shape (URL, query, ctx, timeout) | API method layer (endpoint file) | request.go generic helper | Endpoint method builds `url.Values` and passes (path, query) to `doJSONGet[T]`. Generic helper owns ctx-timeout wrap, `http.NewRequestWithContext`, `Do`, drain-then-close, and decode pipeline. |
| Header injection + structured logging | Transport (RoundTripper chain) | — | `headerTransport` + `loggingTransport` already shipped in Phase 2 (`transport.go`). Phase 3 changes nothing here. |
| JSON decode + oversize gate | request.go generic helper | — | `io.LimitReader`-bounded `json.NewDecoder` + `decoder.More()` boundary gate + `limited.N == 0` mid-truncation gate (Phase 2 `countries.go` lines 106-138 — the exact pattern that becomes `doJSONGet[T]`). |
| `*APIError` construction (4xx/5xx) | request.go generic helper | — | Constructed inside `doJSONGet[T]` via `buildAPIError(resp, path)`. ARCHITECTURE.md Pattern 6: endpoint-aware semantic error, but path is passed in as a parameter, so the helper stays endpoint-agnostic. |
| Post-decode shape validation (Holiday only) | API method layer (endpoint file) | helper function in holiday.go | `validateHolidays(hs []Holiday, path string) error` called from `PublicHolidays` and `SchoolHolidays` after `doJSONGet` returns. The three non-Holiday endpoints do NOT call it. |
| `Holiday.NameFor` / `IsInRegion` / `Days` / `Range` | Pure type method on `Holiday` | — | Side-effect-free; consume only the Holiday value plus an arg. No I/O, no Client dependency. |
| `Client.IsInRegion` hierarchical lookup | API method layer (new `Client` method) | `Subdivisions` endpoint call | Issues an extra HTTP call to `/Subdivisions`; walks the returned tree to build a parent-index. Only public-API method in Phase 3 that issues hidden I/O. |
| Golden-fixture playback | Test infrastructure (`httptest.NewServer` + table tests) | testdata/ files | Each `*_test.go` file constructs a per-test `httptest.NewServer` whose handler returns the on-disk fixture bytes. Pattern proven in `countries_test.go` (Phase 2 D-46). |
| Live-fixture refresh (`-update`) | Test infrastructure (`update_fixtures_test.go` build-tagged `integration`) | live upstream API | Single `flag.Bool` declared in build-tagged file; each endpoint test branches on `*updateFixtures` to either compare-or-overwrite. |

## User Constraints (from CONTEXT.md)

### Locked Decisions

Every D-51..D-72 in CONTEXT.md is binding. Highlights the planner MUST honor:

- **Endpoint signature shape (D-51, D-52, CL-08):** `(c *Client) X(ctx context.Context, req XRequest) (..., error)` for ALL FIVE endpoints. The Phase 2 `Countries(ctx) ([]Country, error)` signature MUST be retrofitted to `Countries(ctx, CountriesRequest) ([]Country, error)` in Plan 1; CountriesRequest{} zero-value reproduces current behavior.
- **Request struct shapes (D-54, CL-13):** Five Request types with the exact field sets in D-54. `CountriesRequest{LanguageIsoCode string}`; `LanguagesRequest{LanguageIsoCode string}`; `SubdivisionsRequest{CountryIsoCode string; LanguageIsoCode string}`; `PublicHolidaysRequest{CountryIsoCode string; ValidFrom Date; ValidTo Date; LanguageIsoCode string; SubdivisionCode string}`; `SchoolHolidaysRequest{...same five plus GroupCode string}`.
- **Validator wiring (D-56):** Free-function calls per the matrix; NO `Request.Validate()` methods (ARCHITECTURE.md Pattern 5 — explicit rejection).
- **Empty-string optional handling (D-55):** Empty optional fields are omitted from the outbound query; validators run only on non-empty inputs for optional fields.
- **Helper bodies (D-57..D-61, CL-10, CL-11):** `Holiday.NameFor` delegates to `pickLocalized`; `Holiday.IsInRegion` flat-only with `code==""→false`, `Nationwide→true`, `strings.EqualFold` match short-circuit; `Holiday.Days` returns `h.StartDate.DaysUntil(h.EndDate)`; `Holiday.Range` yields `iter.Seq[Date]` (NOT `iter.Seq[time.Time]` — ROADMAP success criterion #4 literal is overridden by CL-11).
- **Hierarchical Client.IsInRegion (D-59, CL-09):** New public method beyond REQUIREMENTS HELP-02. Fetches `/Subdivisions`, walks `Subdivision.Children`, builds a `map[string]string` parent-index, walks upward from `code` until match or root.
- **request.go scope (D-62, D-63, D-64):** New file declares `func doJSONGet[T any](ctx context.Context, c *Client, path string, q url.Values) (T, error)` containing the entire Phase 2 D-41..D-45 + D-24 pipeline. `countries.go` refactored to call it. `maxResponseBytes` and `apiErrorBodyCap` constants MOVE to `request.go`. `buildAPIError` + `parseAPIMessage` MOVE to `request.go`.
- **Post-decode validation (D-65, CL-12):** `validateHolidays(hs []Holiday, path string) error` lives in `request.go` OR `holiday.go` (planner's discretion); called ONLY from `PublicHolidays`/`SchoolHolidays`. Returns first violation wrapping `ErrMalformedResponse` (new sentinel, D-66).
- **Sentinel addition (D-66, CL-12):** `ErrMalformedResponse` appended to `errors.go`'s `var (...)` block. `internal_test.go` `allowedVars` map updated (currently has 7 entries — 6 exported sentinels + `errEmptyDate`; will have 8 after Phase 3).
- **Fixture refresh mechanism (D-67, D-68):** `var updateFixtures = flag.Bool("update", false, ...)` declared once in `update_fixtures_test.go` (build-tagged `//go:build integration`). Run command: `OPENHOLIDAYS_LIVE=1 go test -tags=integration -update -run TestUpdateFixtures ./...`.
- **Fixture sanity asserts (D-70):** Specific values locked: PL public holidays 2025 → 14 items + `Wigilia Bożego Narodzenia` on 2025-12-24 (VERIFIED LIVE — see Step 2.6 below). PL school holidays 2025 → 7 items + `ferie zimowe`-bearing entry with `IsInRegion("PL-SL")` true (VERIFIED LIVE). PL subdivisions → 16 województwa (VERIFIED LIVE). Languages → ≥ 14 (VERIFIED LIVE — actual count was 31 on 2026-05-27).
- **Plan sequencing (D-72):** 8 plans in the specified order. Planner may refine but should NOT reorder Plan 1 (request.go + Countries refactor) — it gates all other endpoint plans.

### Claude's Discretion

- File layout: per-endpoint at repo root (`languages.go`, `subdivisions.go`, `public_holidays.go`, `school_holidays.go`); `request.go` shared; Holiday helpers may live in `types.go` OR a new `holiday.go` (planner's call).
- Test file layout: `languages_test.go`, `subdivisions_test.go`, `public_holidays_test.go`, `school_holidays_test.go`, `request_test.go`, `update_fixtures_test.go`.
- Where exactly `validateHolidays` lives (`request.go` vs `holiday.go`) — planner's call.
- Exact `t.Run` subtest names per file — planner's call as long as Gold Rule 3 (one `TestXxx` per exported prod func, every case in `t.Run`) is honored.
- Whether to split `Holiday.IsInRegion` and `Client.IsInRegion` into one test file (`holiday_test.go`) or two — planner's call.

### Deferred Ideas (OUT OF SCOPE)

- `Holiday.IsInRegionTree(code, subs []Subdivision) bool` — no-Client tree-walking method. Out of scope for v0.1.0.
- `BuildRegionIndex(subs []Subdivision) map[string][]string` exposed helper. Out of scope.
- `Request.Validate() error` methods. Out of scope (ARCHITECTURE.md explicit rejection).
- `PublicHolidaysByDate`, `SchoolHolidaysByDate`, `Statistics/*` endpoints. Out of scope.
- Single-flight on `/Subdivisions` for `Client.IsInRegion` cold-start. Deferred to Phase 4 cache scope.
- Defensive deep copy of returned `[]Holiday`. Caller mutation only affects their own copy; cache lands in Phase 4 with raw bytes (no intersection).
- Subdivision-tree memoization in `Client`. Deferred to Phase 4 cache.
- `Holiday.CommentFor(lang)` helper. Defer to v0.2.
- Exported `ValidateHoliday` function. Defer until a real consumer asks.

## Phase Requirements

| ID | Description (from REQUIREMENTS.md) | Research Support |
|----|-------------|------------------|
| ENDPT-02 | `Languages(ctx) ([]Language, error)` fetches supported-languages list | §"Recommended generic-helper signature" (`doJSONGet[[]Language]`). Upstream `/Languages` returns 31 items on 2026-05-27 (verified live); JSON shape `{isoCode, name: []LocalizedText}`. |
| ENDPT-03 | `Subdivisions(ctx, country, lang) ([]Subdivision, error)` | §"Subdivision tree shape — live verification" — PL returns 16 flat województwa, no `children`; DE returns 16 Bundesländer with `children` only under DE-BY. Recommendation: capture both PL and DE fixtures during Phase 3. |
| ENDPT-04 | `PublicHolidays(ctx, PublicHolidaysRequest) ([]Holiday, error)` | §"validateHolidays placement" (post-decode); §"Recommended generic-helper signature"; verified 14 PL 2025 public holidays including Wigilia 2025-12-24. |
| ENDPT-05 | `SchoolHolidays(ctx, SchoolHolidaysRequest) ([]Holiday, error)` | §"validateHolidays placement"; verified 7 PL 2025 school holidays. Note: PL school-holiday items observed live have NO `groups` field in the response (groupCode filter is query-only for PL); German entries DO carry `groups` (e.g. `DE-MV-BBS`). |
| HELP-01 | `Holiday.Name(lang) string` — localized name with fallback | CONTEXT.md D-57: implemented as `NameFor(lang)` via existing `pickLocalized` helper in `types.go` (CL-05 rationale already locked). |
| HELP-02 | `Holiday.IsInRegion(code) bool` | CONTEXT.md D-58: flat match. Plus D-59/CL-09 ships `Client.IsInRegion` as a separate method for hierarchical lookup. |
| HELP-03 | `Holiday.Days() int` | CONTEXT.md D-60: delegates to `Date.DaysUntil` (Phase 1 D-10). Verified to return 14 for a 14-day span via `date.go` lines 154-164 (`DaysUntil` adds +1 for inclusive count). |
| HELP-04 | `Holiday.Range() iter.Seq[time.Time]` → **CL-11 deviation to `iter.Seq[Date]`** | §"iter.Seq[Date] canonical body" — yields Date at each step via `AddDate(0, 0, 1)` + `NewDate` rebuild to preserve UTC-midnight invariant. |
| TEST-01 | Per-endpoint unit tests cover happy path + 4 error paths; table-driven | §"httptest table-test pattern for endpoints with required query params". Standard `httptest.NewServer` + `WithBaseURL` + per-case handler; required-query-param validation kicks in BEFORE the HTTP call so the test path differs slightly from Countries. |
| TEST-02 | Tests use `httptest.NewServer` — no live network outside integration | Inherited Phase 2 D-46 pattern. The new `update_fixtures_test.go` is build-tagged `integration` so it is silently excluded from `go test ./...`. |
| TEST-03 | Golden JSON fixtures captured from live API; `-update` flag regenerates | §"-update fixture mechanism" — full body of the build-tagged `update_fixtures_test.go` shown below. |

## Standard Stack

No new dependencies — entire phase ships against the existing stack already locked in Phase 1 and Phase 2.

### Core (already in `go.mod`)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go toolchain | 1.23 (`go 1.23` directive in `go.mod` verified) | Generics + `iter.Seq[T]` | Locked by D-61 / CL-11 / Phase 1 D-13. `iter` package is Go 1.23+. [VERIFIED: `go.mod` line 3] |
| `net/http`, `encoding/json`, `context`, `errors`, `fmt`, `io`, `time`, `iter`, `strings`, `net/url` | stdlib | HTTP + JSON + ctx + URL query encoding + iteration | Zero-runtime-dep policy; every import is stdlib. [CITED: PROJECT.md "zero runtime deps" + STACK.md §"Core Technologies"] |
| `flag` | stdlib | Declare `-update` Boolean | The Go-idiomatic mechanism for `-flag` parameters consumed during `go test`. [CITED: `pkg.go.dev/flag`] |

### Test-only (already in `go.mod`)

| Library | Version | Purpose |
|---------|---------|---------|
| `github.com/stretchr/testify` | v1.11.1 (already in go.mod, verified) | `assert` + `require` per Gold Rule 3 + Phase 1 CL convention. |
| `net/http/httptest` | stdlib | Per-test server stub. |

**Version verification:** Both `stretchr/testify` and the Go toolchain are already pinned. `go.mod` confirms `module github.com/egeek-tech/go-openholidays`, `go 1.23`, `require github.com/stretchr/testify v1.11.1`. No new dependencies — Phase 3 adds zero rows to `go.mod`. [VERIFIED: `go.mod` lines 1-4]

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Hand-rolled flat `iter.Seq[Date]` body | A library like `samber/lo` `lo.RangeFrom` | Brings a runtime dep; project forbids. Stdlib `iter.Seq` is one-screen of code — write it. |
| Custom `parent map[string]string` for `Client.IsInRegion` | Caching the entire `[]Subdivision` tree on Client | Phase 3 is non-caching by design; Phase 4 will add the cache transparently. Local per-call construction is correct for v0.1.0. |
| `flag` for `-update` | `os.Getenv("OPENHOLIDAYS_UPDATE_FIXTURES")` | `-update` is the Go community convention (used by `cmd/go`, `golangci-lint`, etc.); env-var would diverge from idiom. Use `flag`. [CITED: `pkg.go.dev/testing`] |

## Package Legitimacy Audit

> **Not applicable.** Phase 3 installs zero new packages. All transitive imports are stdlib + the test-only `stretchr/testify` already pinned in Phase 1. No row to audit. The 4 transitive testify deps (`davecgh/go-spew`, `pmezard/go-difflib`, `gopkg.in/yaml.v3`, plus testify itself) are all `[VERIFIED: npm registry equivalent]` via the existing Phase 1 lockfile.

## Architecture Patterns

### System Architecture Diagram

```
        User code: c.PublicHolidays(ctx, req)
                              │
                              ▼
   ┌─────────────────────────────────────────────────────────┐
   │  public_holidays.go: (c *Client) PublicHolidays         │
   │   1. validateCountry(req.CountryIsoCode)                │
   │   2. validateLanguage(req.LanguageIsoCode) if non-empty │
   │   3. validateDateRange(req.ValidFrom, req.ValidTo)      │
   │   4. q := url.Values{} (only non-empty fields set)      │
   │   5. holidays, err := doJSONGet[[]Holiday](             │
   │        ctx, c, "/PublicHolidays", q)                    │
   │   6. if err != nil → return nil, err                    │
   │   7. if err := validateHolidays(                        │
   │        holidays, "/PublicHolidays"); err != nil →       │
   │        return nil, err                                  │
   │   8. return holidays, nil                               │
   └────────────────────────┬────────────────────────────────┘
                            │
                            ▼
   ┌─────────────────────────────────────────────────────────┐
   │  request.go: doJSONGet[T any](ctx, c, path, q)          │
   │   1. nil-ctx defensive guard                            │
   │   2. ctx, cancel = WithTimeout(ctx, c.timeout) if > 0   │
   │   3. req := http.NewRequestWithContext(...)             │
   │   4. req.URL.RawQuery = q.Encode()                      │
   │   5. resp, err := c.http.Do(req)                        │
   │   6. defer drain-then-close                             │
   │   7. if resp.StatusCode >= 400 → buildAPIError(...)     │
   │   8. limited := &io.LimitedReader{R: body, N: 10MiB}    │
   │   9. dec := json.NewDecoder(limited)                    │
   │   10. dec.Decode(&out)                                  │
   │   11. mid-truncation gate: limited.N == 0               │
   │   12. boundary gate: dec.More() == true                 │
   │   13. return out, nil  (or zero value of T + err)       │
   └────────────────────────┬────────────────────────────────┘
                            │
                            ▼
   c.http.Do dispatches through the RoundTripper chain
   (headerTransport → loggingTransport → underlying — Phase 2)
                            │
                            ▼
                  openholidaysapi.org/PublicHolidays?…
```

### Recommended Project Structure (Phase 3 deltas only)

```
go-openholidays/
├── request.go                 # NEW: doJSONGet[T any], buildAPIError, parseAPIMessage
│                              #      (moved from countries.go);
│                              #      maxResponseBytes, apiErrorBodyCap consts (moved).
├── request_test.go            # NEW: tests for doJSONGet at the unit level
│                              #      (httptest.Server backed; tests through
│                              #      countries.go for the typed-T path).
│
├── countries.go               # REFACTORED: ~30 lines now; calls doJSONGet[[]Country]
│                              #             with CountriesRequest validator.
├── countries_test.go          # MODIFIED: tests now pass CountriesRequest{} (zero
│                              #            value reproduces current behavior).
│
├── languages.go               # NEW: Languages + LanguagesRequest
├── languages_test.go          # NEW
├── subdivisions.go            # NEW: Subdivisions + SubdivisionsRequest
├── subdivisions_test.go       # NEW
├── public_holidays.go         # NEW: PublicHolidays + PublicHolidaysRequest
│                              #      + (optionally) validateHolidays helper
├── public_holidays_test.go    # NEW
├── school_holidays.go         # NEW: SchoolHolidays + SchoolHolidaysRequest
├── school_holidays_test.go    # NEW
│
├── holiday.go                 # NEW (recommended): NameFor, IsInRegion, Days,
│                              #      Range methods on Holiday. Alternatively
│                              #      these may live in types.go alongside the
│                              #      Holiday struct (planner's call).
├── holiday_test.go            # NEW
├── client_helpers.go          # NEW (or append to client.go): Client.IsInRegion
├── client_helpers_test.go     # NEW
│
├── update_fixtures_test.go    # NEW: build-tagged //go:build integration;
│                              #      declares -update flag; TestUpdateFixtures
│                              #      walks every endpoint and overwrites
│                              #      testdata/*.json.
│
├── errors.go                  # MODIFIED: append ErrMalformedResponse var
│
├── internal_test.go           # MODIFIED: extend allowedVars map with
│                              #            "ErrMalformedResponse"
│
└── testdata/
    ├── countries.json              # REFRESHED (Plan 1, Countries retrofit)
    ├── languages.json              # NEW (Plan 2)
    ├── subdivisions_pl.json        # NEW (Plan 3); 16 flat województwa
    ├── subdivisions_de.json        # NEW (Plan 3, optional — see "Hierarchical
    │                                  IsInRegion fixture problem" below)
    ├── public_holidays_pl_2025.json # NEW (Plan 4); 14 entries incl. Wigilia
    └── school_holidays_pl_2025.json # NEW (Plan 5); 7 entries incl. ferie zimowe
```

### Pattern 1: Generic `doJSONGet[T any]` — the Phase 2 countries.go pipeline as a reusable function

**What:** A single generic function consolidates lines 78-138 of the current `countries.go` so each new endpoint method shrinks to ~25 lines (validate → build query → `doJSONGet` → optional post-decode validate → return). The generic parameter is the *response type* (`[]Country`, `[]Language`, `[]Subdivision`, `[]Holiday`).

**When to use:** Every endpoint in this library uses GET-with-JSON-body-response. All five endpoints follow this template. `doJSONGet[T]` is the single load-bearing helper.

**Signature (recommended):**

```go
// doJSONGet performs a GET to c.baseURL+path with the supplied query
// parameters, decodes the JSON response body into a value of type T, and
// returns it. It encapsulates the Phase 2 D-41..D-45 + D-24 pipeline:
//
//   - nil-ctx defensive guard
//   - per-request context.WithTimeout(ctx, c.timeout) when timeout > 0
//   - http.NewRequestWithContext + req.URL.RawQuery = q.Encode()
//   - c.http.Do dispatch through the RoundTripper chain
//   - deferred drain-then-close (10 MiB cap on the drain itself)
//   - 4xx/5xx → *APIError via buildAPIError(resp, path)
//   - 2xx + empty body → fmt.Errorf("...: %w", ErrEmptyResponse)
//   - mid-truncation gate (limited.N == 0 + decode error) → ErrResponseTooLarge
//   - boundary-truncation gate (decoder.More() == true) → ErrResponseTooLarge
//
// On every failure path, doJSONGet returns the zero value of T plus the
// wrapped error. Callers MUST NOT use the returned T when err != nil; the
// `var zero T` return convention follows Go community idiom for generic
// error-bearing helpers (e.g., x/exp/slices.BinarySearchFunc returns
// zero on miss).
//
// Post-decode validation is the caller's responsibility — doJSONGet does
// NOT inspect the decoded value. See validateHolidays for the Holiday
// schema-drift checks that PublicHolidays / SchoolHolidays apply.
func doJSONGet[T any](ctx context.Context, c *Client, path string, q url.Values) (T, error) {
    var zero T
    if ctx == nil {
        return zero, errors.New("openholidays: nil context")
    }
    if c.timeout > 0 {
        var cancel context.CancelFunc
        ctx, cancel = context.WithTimeout(ctx, c.timeout)
        defer cancel()
    }
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
    if err != nil {
        return zero, fmt.Errorf("openholidays: build %s request: %w", path, err)
    }
    if len(q) > 0 {
        req.URL.RawQuery = q.Encode()
    }
    resp, err := c.http.Do(req)
    if err != nil {
        return zero, fmt.Errorf("openholidays: GET %s: %w", path, err)
    }
    defer func() {
        _, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes+1))
        _ = resp.Body.Close()
    }()
    if resp.StatusCode >= 400 {
        return zero, buildAPIError(resp, path)
    }
    var out T
    limited := &io.LimitedReader{R: resp.Body, N: maxResponseBytes}
    decoder := json.NewDecoder(limited)
    if decodeErr := decoder.Decode(&out); decodeErr != nil {
        if errors.Is(decodeErr, io.EOF) {
            return zero, fmt.Errorf("openholidays: empty %s response: %w", path, ErrEmptyResponse)
        }
        if limited.N == 0 {
            return zero, fmt.Errorf("openholidays: response exceeded %d bytes: %w", maxResponseBytes, ErrResponseTooLarge)
        }
        return zero, fmt.Errorf("openholidays: decode %s: %w", path, decodeErr)
    }
    if decoder.More() {
        return zero, fmt.Errorf("openholidays: response exceeded %d bytes: %w", maxResponseBytes, ErrResponseTooLarge)
    }
    return out, nil
}
```

**Critical zero-value-return rationale (Gap #1):** The `var zero T` pattern is the Go idiom for generic functions that may fail. For slice types (`[]Country`, `[]Holiday`), `var zero T` is `nil`, matching the existing Phase 2 behavior where `Countries` returns `nil, err` on error. The function never returns a non-nil partial result alongside an error — that is the contract callers depend on. [CITED: `pkg.go.dev/cmp` and `golang.org/x/exp/slices` both use this convention]

**Why NO `validate func(T) error` parameter (Gap #6):** Adding a validation hook to `doJSONGet[T]` was considered. Rejected:

1. Three of five endpoints (`Countries`, `Languages`, `Subdivisions`) have no post-decode validation needs. They would each need to pass `nil` or an identity validator — both are noise.
2. Putting the call inside `doJSONGet` hides it from the endpoint method, making the validation point harder to reason about during code review.
3. The validation context (which `path` to attribute the error to) is endpoint-specific anyway and the endpoint already has that string literal at hand.
4. Phase 4 will add a `WithStrictDecoding` option that hooks the decoder, NOT the validator. That hook lives inside `doJSONGet`'s decode step (existing decoder configured via Client option) — it doesn't conflict with the call-site validateHolidays pattern.

Recommended pattern:

```go
// public_holidays.go
func (c *Client) PublicHolidays(ctx context.Context, req PublicHolidaysRequest) ([]Holiday, error) {
    countryCanonical, err := validateCountry(req.CountryIsoCode)
    if err != nil { return nil, err }
    // (more validators per D-56...)
    q := url.Values{}
    q.Set("countryIsoCode", countryCanonical)
    q.Set("validFrom", req.ValidFrom.String())
    q.Set("validTo", req.ValidTo.String())
    if req.LanguageIsoCode != "" { q.Set("languageIsoCode", langCanonical) }
    if req.SubdivisionCode != "" { q.Set("subdivisionCode", req.SubdivisionCode) }
    holidays, err := doJSONGet[[]Holiday](ctx, c, "/PublicHolidays", q)
    if err != nil { return nil, err }
    if err := validateHolidays(holidays, "/PublicHolidays"); err != nil {
        return nil, err
    }
    return holidays, nil
}
```

**Validation site is visible in the endpoint method — line 12 above.** A reviewer reads `PublicHolidays`, sees the explicit `validateHolidays` call, and understands the schema-drift defense. If it were buried inside a generic hook parameter, the reviewer would have to know the convention. Explicit > implicit per Gold Rule 2 (verify or ask — don't hide).

### Pattern 2: `iter.Seq[Date]` canonical body for `Holiday.Range` (Gap #2)

**What:** Go 1.23's `iter.Seq[T]` is a type alias for `func(yield func(T) bool)`. The yield function returns `false` when the consumer wants to stop early (e.g., `break` inside a `for d := range h.Range()` loop). [VERIFIED: `pkg.go.dev/iter` — type Seq definition]

**Canonical body for `Holiday.Range()`:**

```go
// Range returns an iterator that yields every Date from StartDate to EndDate
// inclusive. For a single-day holiday (StartDate == EndDate), the iterator
// yields exactly one Date. For a multi-day holiday (e.g., Polish ferie zimowe
// spanning 14 calendar days), the iterator yields each calendar day in
// chronological order.
//
// The iterator is single-use: each call to Range returns a fresh closure, so
// re-ranging over a stored iter.Seq[Date] value works (the closure has no
// internal state apart from the captured StartDate/EndDate).
//
// The yielded Date values are constructed via NewDate(year, month, day) so
// every yielded Date is at UTC midnight, preserving the Date type's
// timezone-free calendar-day invariant (Phase 1 D-05). Iteration uses
// time.Time.AddDate(0, 0, 1), which is calendar-correct across DST
// boundaries (Phase 1 D-10 / Pitfall TZ-2 mitigation — DST cannot perturb
// a step between UTC-midnight timestamps).
//
// Range yields nothing when EndDate is strictly before StartDate. Such
// Holiday values are rejected by validateHolidays before any endpoint
// method returns them, so callers iterating over endpoint results never
// observe this case; the defensive check exists for hand-built Holidays.
func (h Holiday) Range() iter.Seq[Date] {
    return func(yield func(Date) bool) {
        if h.EndDate.Before(h.StartDate) {
            return
        }
        d := h.StartDate
        for {
            if !yield(d) {
                return
            }
            if !d.Before(h.EndDate) {
                return
            }
            next := d.AddDate(0, 0, 1)
            d = NewDate(next.Year(), next.Month(), next.Day())
        }
    }
}
```

**Why `NewDate(next.Year(), next.Month(), next.Day())` rather than `Date{next}`:** `AddDate` returns a `time.Time` that retains the receiver's location. Wrapping it directly in `Date{t}` would skip the UTC-midnight normalization that `NewDate` performs (`date.go` lines 44-46). The rebuild via `NewDate` is one extra allocation per step but preserves the invariant unconditionally — a Holiday constructed with a non-UTC StartDate would otherwise yield Dates that drift in their internal Location. Cheap insurance.

**Why guard with `if h.EndDate.Before(h.StartDate)`:** `validateHolidays` (D-65) rejects such Holidays before they leave an endpoint method, but the public `Holiday.Range` is documented as side-effect-free and must not panic on a hand-built malformed Holiday. Returning an empty iterator is the safest behavior — callers see no Dates and a zero-iteration loop body runs zero times. [CITED: `pkg.go.dev/iter` — `iter.Seq` yields zero items by returning before calling yield]

**Why no `yield` panic recovery:** Per `pkg.go.dev/iter` docs, "Yield panics if called after it returns false". The body above never calls `yield` after a `false` return; the immediate `return` on `!yield(d)` is the canonical contract.

### Pattern 3: `httptest.NewServer` table tests for endpoints with required query params (Gap #4)

**What:** Each endpoint test file constructs a per-test `httptest.NewServer` whose handler returns fixture bytes. For endpoints with required query params, the handler MAY inspect `r.URL.Query()` to validate that the endpoint method sent the expected `countryIsoCode`, `validFrom`, `validTo` — turning the test into both a happy-path fixture replay AND a contract check on the URL builder.

**Pattern (recommended for `public_holidays_test.go`):**

```go
const publicHolidaysPL2025FixtureCapturedAt = "2026-05-27"

func TestClient_PublicHolidays(t *testing.T) {
    t.Parallel()

    t.Run("happy path PL 2025 returns 14 holidays incl. Wigilia 2025-12-24", func(t *testing.T) {
        t.Parallel()
        body, err := os.ReadFile(filepath.Join("testdata", "public_holidays_pl_2025.json"))
        require.NoError(t, err, "fixture missing — re-capture via -update")
        t.Logf("fixture captured %s", publicHolidaysPL2025FixtureCapturedAt)

        srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Contract assertions on the URL builder.
            q := r.URL.Query()
            assert.Equal(t, "PL", q.Get("countryIsoCode"))
            assert.Equal(t, "2025-01-01", q.Get("validFrom"))
            assert.Equal(t, "2025-12-31", q.Get("validTo"))
            assert.Equal(t, "/PublicHolidays", r.URL.Path)
            w.Header().Set("Content-Type", "application/json")
            _, _ = w.Write(body)
        }))
        t.Cleanup(srv.Close)

        c := NewClient(WithBaseURL(srv.URL))
        holidays, err := c.PublicHolidays(context.Background(), PublicHolidaysRequest{
            CountryIsoCode: "PL",
            ValidFrom:      NewDate(2025, time.January, 1),
            ValidTo:        NewDate(2025, time.December, 31),
        })
        require.NoError(t, err)
        require.Len(t, holidays, 14)

        // Find Wigilia by name and assert StartDate.
        var wigilia *Holiday
        for i := range holidays {
            if holidays[i].NameFor("pl") == "Wigilia Bożego Narodzenia" {
                wigilia = &holidays[i]
                break
            }
        }
        require.NotNil(t, wigilia, "Wigilia Bożego Narodzenia not found in PL 2025 fixture")
        assert.True(t, wigilia.StartDate.Equal(NewDate(2025, time.December, 24)),
            "Wigilia must start on 2025-12-24, got %s", wigilia.StartDate)
    })

    t.Run("validation error: empty CountryIsoCode wraps ErrInvalidCountry", func(t *testing.T) {
        t.Parallel()
        c := NewClient(WithBaseURL("http://example.invalid")) // no server reached
        _, err := c.PublicHolidays(context.Background(), PublicHolidaysRequest{
            ValidFrom: NewDate(2025, time.January, 1),
            ValidTo:   NewDate(2025, time.December, 31),
        })
        require.Error(t, err)
        assert.True(t, errors.Is(err, ErrInvalidCountry))
    })

    t.Run("validation error: from > to wraps ErrInvalidDateRange", func(t *testing.T) {
        t.Parallel()
        c := NewClient(WithBaseURL("http://example.invalid"))
        _, err := c.PublicHolidays(context.Background(), PublicHolidaysRequest{
            CountryIsoCode: "PL",
            ValidFrom:      NewDate(2026, time.January, 1),
            ValidTo:        NewDate(2025, time.December, 31),
        })
        require.Error(t, err)
        assert.True(t, errors.Is(err, ErrInvalidDateRange))
    })

    t.Run("4xx returns *APIError with detail", func(t *testing.T) { /* ... */ })
    t.Run("5xx returns *APIError with title fallback", func(t *testing.T) { /* ... */ })
    t.Run("malformed JSON wraps decode error", func(t *testing.T) { /* ... */ })
    t.Run("ctx cancel within 100ms returns context.Canceled", func(t *testing.T) { /* ... */ })
    t.Run("malformed holiday (zero StartDate) wraps ErrMalformedResponse", func(t *testing.T) {
        t.Parallel()
        // Server returns a Holiday with startDate "" — Date.UnmarshalJSON
        // currently rejects empty strings at decode time, so this case
        // actually exercises the decode error path. For validateHolidays
        // to be triggered, the upstream must return a structurally-valid
        // JSON (non-empty dates) that nonetheless violates the invariant
        // (EndDate before StartDate). Construct accordingly.
        bad := `[{
            "id":"bad-uuid","startDate":"2025-12-25","endDate":"2025-01-01",
            "type":"Public","name":[{"language":"en","text":"X"}],
            "nationwide":true,"regionalScope":"National","temporalScope":"FullDay"
        }]`
        srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            w.Header().Set("Content-Type", "application/json")
            _, _ = w.Write([]byte(bad))
        }))
        t.Cleanup(srv.Close)
        c := NewClient(WithBaseURL(srv.URL))
        _, err := c.PublicHolidays(context.Background(), PublicHolidaysRequest{
            CountryIsoCode: "PL",
            ValidFrom:      NewDate(2025, time.January, 1),
            ValidTo:        NewDate(2025, time.December, 31),
        })
        require.Error(t, err)
        assert.True(t, errors.Is(err, ErrMalformedResponse))
    })
}
```

**Why the URL-builder contract check inside the handler:** Without it, a bug that mis-spells `countryIsoCode` → `country` would still pass the test if the fixture happens to be returned regardless. Inspecting `r.URL.Query()` inside the handler turns the test into a positive contract — the test fails fast on URL drift. This is the standard pattern from `aws-sdk-go-v2`'s service-stub tests and similar SDKs.

**Why `WithBaseURL("http://example.invalid")` for validation-error subtests:** No HTTP call is dispatched (the validator returns first). `example.invalid` is a reserved test domain (RFC 6761) that cannot resolve, so an accidental call would fail loudly — exactly what we want during a test that asserts no-call.

### Pattern 4: Hierarchical `Client.IsInRegion` — flat-fallback aware (Gap #5)

**What:** D-59 specifies a hierarchical match that fetches `/Subdivisions`, walks `Subdivision.Children` recursively, and walks upward from `code` until a match against `h.Subdivisions[].Code` is found.

**Live-verified shape of PL Subdivisions (2026-05-27):**
- 16 województwa, each with `code` (e.g. `PL-SL`), `isoCode` (e.g. `PL-24`), `shortName`, `category`, `name`, `officialLanguages`.
- **No `children` field on any entry.** The recursive walk visits each top-level entry once and terminates immediately.

**Live-verified shape of DE Subdivisions (2026-05-27):**
- 16 Bundesländer, 15 flat, 1 with `children` (DE-BY has one nested child: Augsburg).

**Implication:** For PL, `Client.IsInRegion(ctx, h, "PL-SL-KAT")` cannot succeed by walking children — there is no `PL-SL-KAT` in the live API's subdivision tree. The hierarchical method's value is realized only when a holiday applies to a parent subdivision AND the user passes a child code AND the upstream actually nests them. As of 2026-05-27, the live API does this for DE only (and only for one entry).

**Recommended implementation (defensive: works for both flat and nested upstream shapes):**

```go
// IsInRegion reports whether the given subdivision code is covered by the
// holiday h, accounting for hierarchical subdivision nesting. When the
// upstream returns a flat list of subdivisions (as for PL on 2026-05-27),
// this method's behavior reduces to Holiday.IsInRegion. When the upstream
// returns a nested tree (as for DE-BY's Augsburg child on 2026-05-27),
// this method walks the tree to discover whether `code` is a descendant
// of any subdivision the holiday applies to.
//
// I/O: this method issues an HTTP GET to /Subdivisions for the country
// implied by h.Subdivisions[0].Code (the prefix before the first hyphen,
// e.g. "PL" from "PL-SL"). When h.Subdivisions is empty and h.Nationwide
// is false, the method returns (false, nil) without HTTP — there is no
// country context to fetch a tree for.
//
// Phase 4 will add a cache transport that memoizes /Subdivisions per
// (baseURL, countryIsoCode); until then, repeated calls in a hot loop
// incur a round-trip per call.
func (c *Client) IsInRegion(ctx context.Context, h Holiday, code string) (bool, error) {
    if code == "" { return false, nil }
    if h.Nationwide { return true, nil }
    // Fast path: flat match on Holiday.Subdivisions directly.
    for _, s := range h.Subdivisions {
        if strings.EqualFold(s.Code, code) {
            return true, nil
        }
    }
    // No flat match. The hierarchical path needs a country to fetch the
    // tree for. Use the holiday's first subdivision's country prefix.
    if len(h.Subdivisions) == 0 {
        return false, nil
    }
    countryCode, ok := splitCountryFromSubdivision(h.Subdivisions[0].Code)
    if !ok {
        return false, nil
    }
    tree, err := c.Subdivisions(ctx, SubdivisionsRequest{CountryIsoCode: countryCode})
    if err != nil {
        return false, err
    }
    parentIdx := buildParentIndex(tree) // map[string]string, child→parent
    // Walk upward from `code` until a match against h.Subdivisions or
    // the root is reached. Defensive cycle bound: stop after len(parentIdx)+1
    // iterations (cycles cannot exceed the index size).
    current := code
    for i := 0; i <= len(parentIdx); i++ {
        for _, s := range h.Subdivisions {
            if strings.EqualFold(s.Code, current) {
                return true, nil
            }
        }
        parent, found := parentIdx[strings.ToUpper(current)]
        if !found { return false, nil } // reached root or unknown code
        current = parent
    }
    return false, nil // cycle defense — should never trigger
}

// splitCountryFromSubdivision extracts "PL" from "PL-SL", "DE" from "DE-BY",
// etc. Returns false when the input has no hyphen.
func splitCountryFromSubdivision(code string) (string, bool) {
    if i := strings.IndexByte(code, '-'); i > 0 {
        return code[:i], true
    }
    return "", false
}

// buildParentIndex walks the recursive Subdivision tree and returns a
// child→parent map keyed by uppercase subdivision code. Top-level
// subdivisions are not present in the map (they have no parent).
//
// The walk is depth-first and visits every node exactly once (the upstream
// tree is a forest of trees, not a graph with cycles — but the cycle bound
// in IsInRegion is a defense-in-depth guard against malformed upstream data).
func buildParentIndex(tree []Subdivision) map[string]string {
    idx := make(map[string]string)
    var walk func(parent string, nodes []Subdivision)
    walk = func(parent string, nodes []Subdivision) {
        for _, n := range nodes {
            if parent != "" {
                idx[strings.ToUpper(n.Code)] = parent
            }
            if len(n.Children) > 0 {
                walk(n.Code, n.Children)
            }
        }
    }
    walk("", tree)
    return idx
}
```

**The hierarchical-test fixture problem (Gap #5 follow-up):** The locked test for `Client.IsInRegion` in CONTEXT.md §"Specific Ideas" assumes PL ferie-zimowe Śląskie maps `Holiday.Subdivisions: [PL-SL]` to a child code under `PL-SL` in `subdivisions_pl.json`. **The live upstream does not nest PL.** The planner has three options:

1. **Capture both PL and DE subdivisions; test hierarchical against DE-BY/Augsburg.** Plan 3 captures `subdivisions_pl.json` AND `subdivisions_de.json`. The `TestClient_IsInRegion` hierarchical subtest uses DE-BY as the parent and Augsburg as the child code. This keeps fixtures purely live-captured. **RECOMMENDED.**
2. **Hand-construct a Subdivision tree literal in the test.** Bypass the fixture for the recursive subtest; build a tree like `Subdivision{Code:"PL-SL", Children:[Subdivision{Code:"PL-SL-KAT"}]}` in Go. The test still exercises the parent-index walk and the upward walk, but doesn't depend on the upstream's choice to flatten PL. Trade-off: the fixture no longer authoritatively documents what the upstream returns for PL, but a flat fixture combined with a synthetic tree is still informative.
3. **Skip the hierarchical case entirely; declare flat-only behavior for v0.1.0.** Aggressively reduces scope and removes the `Client.IsInRegion` method from v0.1.0. Trade-off: CL-09 is rolled back, REQUIREMENTS HELP-02 satisfied only by the flat `Holiday.IsInRegion`.

**Recommended: Option 1.** It honors CL-09's commitment, keeps the test grounded in a real upstream shape, and gives the library a verified end-to-end test for hierarchical lookup that future upstream changes (PL upstream eventually adopting nested subdivisions) will not invalidate.

### Pattern 5: `-update` fixture refresh mechanism (Gap #3)

**What:** A `flag.Bool("update", false, ...)` declared once in a build-tagged test file. When set, integration tests overwrite fixture files; when unset (the default), `go test ./...` skips the integration build entirely so the flag is never reached.

**Body of `update_fixtures_test.go` (recommended):**

```go
//go:build integration

// Package openholidays — fixture refresh utility (live API).
//
// This file is compiled only when -tags=integration is supplied to go test
// AND has effect only when OPENHOLIDAYS_LIVE=1 is also set. Both gates must
// be true to issue any HTTP request to the live upstream; either being unset
// causes the test to skip silently.
//
// Run command:
//
//   OPENHOLIDAYS_LIVE=1 go test -tags=integration -update \
//     -run TestUpdateFixtures ./...
//
// The -update flag controls whether the test overwrites testdata/*.json
// files (true) or only verifies that the live response matches the committed
// fixture (false; this becomes a useful drift-detection mode in CI nightly).

package openholidays

import (
    "context"
    "encoding/json"
    "flag"
    "net/http"
    "os"
    "path/filepath"
    "testing"
    "time"

    "github.com/stretchr/testify/require"
)

// updateFixtures is true when -update is supplied. False (the default)
// means TestUpdateFixtures verifies the live response matches the committed
// fixture instead of overwriting it. Declared once here; visible to any
// other build-tagged file in this package (but no other file declares an
// -update flag of its own).
var updateFixtures = flag.Bool("update", false,
    "regenerate testdata/*.json fixtures from the live API")

const updateFixturesGuardEnv = "OPENHOLIDAYS_LIVE"

// TestUpdateFixtures captures every fixture from the live upstream in one
// pass. The captures are ordered so a partial failure leaves the remaining
// fixtures untouched — the helper writes each capture to a temp file in
// testdata/ and only renames it into place on success.
func TestUpdateFixtures(t *testing.T) {
    if os.Getenv(updateFixturesGuardEnv) != "1" {
        t.Skipf("set %s=1 to enable live-API capture", updateFixturesGuardEnv)
    }

    // 30-second per-call cap is more than enough for any single endpoint
    // even with mild upstream latency. Phase 2 default is 15s; we use 30s
    // here to give the slowest endpoint (Subdivisions per country) extra
    // headroom on a cold day.
    client := http.Client{Timeout: 30 * time.Second}
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
    defer cancel()

    type capture struct {
        path     string // upstream path
        query    string // raw query string
        fixture  string // testdata/<fixture>
        validate func([]byte) error // sanity check before overwrite
    }
    captures := []capture{
        {"/Countries", "", "countries.json", nonEmptyJSONArray},
        {"/Languages", "", "languages.json", nonEmptyJSONArray},
        {"/Subdivisions", "countryIsoCode=PL&languageIsoCode=EN",
            "subdivisions_pl.json", nonEmptyJSONArray},
        {"/Subdivisions", "countryIsoCode=DE&languageIsoCode=EN",
            "subdivisions_de.json", nonEmptyJSONArray},
        {"/PublicHolidays",
            "countryIsoCode=PL&validFrom=2025-01-01&validTo=2025-12-31&languageIsoCode=PL",
            "public_holidays_pl_2025.json", nonEmptyJSONArray},
        {"/SchoolHolidays",
            "countryIsoCode=PL&validFrom=2025-01-01&validTo=2025-12-31&languageIsoCode=EN",
            "school_holidays_pl_2025.json", nonEmptyJSONArray},
    }

    for _, cap := range captures {
        cap := cap
        t.Run(cap.fixture, func(t *testing.T) {
            url := "https://openholidaysapi.org" + cap.path
            if cap.query != "" { url += "?" + cap.query }
            req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
            require.NoError(t, err)
            req.Header.Set("Accept", "application/json")
            req.Header.Set("User-Agent", "go-openholidays-fixture-refresh/"+Version)

            resp, err := client.Do(req)
            require.NoError(t, err, "live HTTP failed — aborting overwrite")
            t.Cleanup(func() { _ = resp.Body.Close() })

            require.Equalf(t, http.StatusOK, resp.StatusCode,
                "live API returned non-200 %d for %s — aborting overwrite",
                resp.StatusCode, url)

            body := readAll(t, resp.Body, 11<<20)
            require.NoError(t, cap.validate(body),
                "live response failed sanity check — aborting overwrite for %s",
                cap.fixture)

            if !*updateFixtures {
                // Drift-detection mode: compare committed fixture to live.
                committed, err := os.ReadFile(filepath.Join("testdata", cap.fixture))
                require.NoError(t, err)
                require.Equalf(t, string(committed), string(body),
                    "DRIFT: live response for %s differs from committed fixture",
                    cap.fixture)
                return
            }

            // Overwrite mode: pretty-print to make diffs reviewable, then
            // write atomically via temp file + rename.
            var pretty bytes.Buffer
            require.NoError(t, json.Indent(&pretty, body, "", "  "))
            tmpDir := filepath.Join("testdata")
            tmp, err := os.CreateTemp(tmpDir, cap.fixture+".tmp-*")
            require.NoError(t, err)
            defer os.Remove(tmp.Name()) // safe even after rename succeeds
            _, err = tmp.Write(pretty.Bytes())
            require.NoError(t, err)
            require.NoError(t, tmp.Close())
            target := filepath.Join(tmpDir, cap.fixture)
            require.NoError(t, os.Rename(tmp.Name(), target))
            t.Logf("captured %s (%d bytes pretty-printed)", cap.fixture, pretty.Len())
        })
    }
}

func nonEmptyJSONArray(b []byte) error {
    var v []json.RawMessage
    if err := json.Unmarshal(b, &v); err != nil {
        return fmt.Errorf("not a JSON array: %w", err)
    }
    if len(v) == 0 {
        return errors.New("empty array — refusing to overwrite fixture")
    }
    return nil
}
```

**Why atomic write via temp file + rename (CONTEXT.md §"Specific Ideas" guard):** A panic mid-write or a SIGINT during `Write` would leave a truncated file at `testdata/countries.json`. The temp-file + rename pattern guarantees that either the old fixture remains intact OR the new fixture is fully written — never a half-written file. `os.Rename` on a single filesystem is atomic per POSIX. [CITED: `pkg.go.dev/os#Rename`]

**Why `nonEmptyJSONArray` sanity check before overwrite (CONTEXT.md §"Specific Ideas" guard):** Protects against the upstream returning `[]` during a transient outage (verified observed pattern with some public APIs during DB failover). Without this guard, a transient outage would corrupt every fixture.

**Why drift-detection mode without `-update`:** Running `OPENHOLIDAYS_LIVE=1 go test -tags=integration -run TestUpdateFixtures` (without `-update`) is a useful CI signal: it tells you when the upstream's response shape has changed without committing a new fixture. CI nightly can wire this and surface drift in PR-blocking form before consumers hit it.

**Why declared in only this one build-tagged file:** A flag declared in a non-tagged file would be visible during normal `go test ./...` runs, which would cause `go test -update` (with no `-tags=integration`) to potentially do nothing visibly but also wouldn't error out — confusing failure mode. By declaring only in the tagged file, `go test ./...` without `-tags=integration` doesn't even see the flag; running with `-update` and no tag fails with `flag provided but not defined: -update`, which is the correct loud failure.

### Pattern 6: `validateHolidays` placement and body (Gap #6 — locked location)

```go
// validateHolidays runs the post-decode Holiday schema-drift checks
// mandated by D-65 / Pitfall JSON-4. Returns the first violation wrapping
// ErrMalformedResponse.
//
// Invariants enforced:
//   - StartDate is non-zero (Date.IsZero()==false)
//   - EndDate is non-zero
//   - EndDate is not strictly before StartDate (single-day holidays
//     where EndDate==StartDate are valid)
//
// path is the request path (e.g. "/PublicHolidays" or "/SchoolHolidays")
// included in the wrapped error message so a multi-endpoint failure surfaces
// which endpoint the violation came from.
func validateHolidays(hs []Holiday, path string) error {
    for i := range hs {
        h := &hs[i]
        if h.StartDate.IsZero() {
            return fmt.Errorf("openholidays: malformed holiday %q at %s: zero StartDate: %w",
                h.ID, path, ErrMalformedResponse)
        }
        if h.EndDate.IsZero() {
            return fmt.Errorf("openholidays: malformed holiday %q at %s: zero EndDate: %w",
                h.ID, path, ErrMalformedResponse)
        }
        if h.EndDate.Before(h.StartDate) {
            return fmt.Errorf("openholidays: malformed holiday %q at %s: EndDate %s before StartDate %s: %w",
                h.ID, path, h.EndDate, h.StartDate, ErrMalformedResponse)
        }
    }
    return nil
}
```

**Why pointer iteration `h := &hs[i]`:** Avoids per-iteration struct copies of the Holiday value (each Holiday is ~10 fields including slices — pointer iteration is materially cheaper and idiomatic). The function does NOT mutate `*h`; the pointer is just for read efficiency. [CITED: `golangci-lint` `rangeValCopy` linter recommendation]

**Why include `h.ID` in the error:** The Holiday UUID is the upstream's stable identifier. A consumer's bug report including the UUID lets a maintainer reproduce against the same row in the same response. The error string `malformed holiday "503317eb-0375-41a8-bcb7-501800dc4098" at /PublicHolidays: EndDate before StartDate: ...` is far more actionable than a generic "bad data" message.

**Why NOT log here:** `validateHolidays` is pure — it returns errors. The endpoint method's logger is the right place to emit a Debug record about the validation failure (which it does implicitly via Phase 2's `loggingTransport` for the upstream request). Avoiding logger awareness in this helper keeps it trivially unit-testable.

**Why placement in `request.go` (recommended) vs `holiday.go`:** Either works. Recommendation: `request.go`. Reasoning: `validateHolidays` is conceptually post-decode validation owned by the response pipeline, not a Holiday-type method. Putting it in `holiday.go` would tempt future contributors to add it to the Holiday method set (which it should NOT be — D-65 is explicit). Keeping it as a package-private function next to `doJSONGet` reinforces "this is part of the response pipeline, just not a generic-able part".

### Anti-Patterns to Avoid

- **`doJSONGet[T]` with a `validate func(T) error` parameter:** See Pattern 1 rationale — hides the validation site, forces nil/identity defaults for the three endpoints that don't need it. Use call-site `validateHolidays` instead.
- **Calling `Date.UnmarshalJSON` directly inside `validateHolidays`:** That's a decode-time concern; by the time `validateHolidays` runs, the dates are already typed. `IsZero()` and `Before()` are sufficient.
- **Using `time.Time` as the `iter.Seq` element type:** CL-11 deviates from ROADMAP literal for the exact reason that callers will compose `Range()` with `IsInRegion`/`NameFor`/`Days` — keeping the type Date avoids per-iteration `t.Time.Year(), t.Time.Month(), t.Time.Day()` conversions in user code.
- **Stuffing the `-update` flag into a non-build-tagged file:** Pollutes `go test ./...` with a spurious flag, and accidentally running `go test -update` without `-tags=integration` would silently no-op. Declare in the tagged file only.
- **Caching `/Subdivisions` inside `Client.IsInRegion`:** Phase 3 is non-caching by deliberate design (D-62..D-65 explicitly preserve the no-cache invariant). Phase 4 adds the cache transparently. Adding ad-hoc caching here would create a second cache surface that Phase 4 has to back out.
- **`for _, h := range hs { ... h.X = ... }` in `validateHolidays`:** `h` is a per-iteration value copy; assignments don't persist. Use pointer iteration if you ever need to mutate (this validator doesn't).
- **Returning `(holidays, nil)` from `PublicHolidays` when `validateHolidays` fails:** The contract is "either all valid or error". A partial valid prefix with the bad row dropped would surprise callers writing `errors.Is(err, ErrMalformedResponse)` branches.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| URL query-string assembly | Manual `path + "?" + key + "=" + url.QueryEscape(val) + "&" + ...` concatenation | `url.Values{}` + `.Set` + `.Encode` (stdlib) | `url.Values.Encode` handles encoding, ordering, multi-value collapse, and empty-value semantics. Manual concatenation invariably forgets one. [CITED: `pkg.go.dev/net/url#Values`] |
| ISO-8601 date emission for query params | `fmt.Sprintf("%04d-%02d-%02d", ...)` | `req.ValidFrom.String()` (Phase 1 Date.String returns `YYYY-MM-DD` per `date.go` line 113-115) | Phase 1 already gives the right format. Re-rolling it is error-prone (e.g., missing zero-padding for January). |
| Inclusive day count for two dates | `int(h.End.Sub(h.Start).Hours()/24) + 1` | `h.StartDate.DaysUntil(h.EndDate)` (Phase 1 D-10) | DST-correct because both operands are UTC midnight. Phase 1 D-10 was designed precisely for this. Re-rolling it forfeits Pitfall TZ-2 protection. |
| Date iteration | `for d := h.Start; d.Before(h.End); d = d.Add(24*time.Hour)` | `h.StartDate.AddDate(0, 0, 1)` + `NewDate(...)` | `Add(24h)` is wrong across DST; `AddDate(0,0,1)` is calendar-correct. The Date type's `NewDate` rebuild enforces the UTC-midnight invariant after every step. |
| Generic decode helper | Hand-write a per-endpoint copy of `LimitReader + Decoder + More()` boundary gate | `doJSONGet[T]` (this phase introduces it) | A second copy of the Phase 2 pipeline immediately invites drift between the two. Generic helper is one source of truth. |
| Test server fixture loading | Inline `ioutil.ReadFile` + `httptest.NewServer` boilerplate per test | A `loadFixture(t, "name.json") []byte` helper next to `internal/testhttp/` (deferred) | For Phase 3, the inline pattern is fine — Phase 2 already uses it in `countries_test.go`. Defer the helper to Phase 4 if duplication becomes painful. |
| JSON pretty-printing for fixture write | Manual indent | `json.Indent(&buf, body, "", "  ")` | Stdlib `json.Indent` is the canonical formatter. Manual indent produces inconsistent output and forfeits stable diffs. [CITED: `pkg.go.dev/encoding/json#Indent`] |
| Atomic file rename | `os.WriteFile` (truncates first, vulnerable to partial-write) | `os.CreateTemp` + `os.Rename` | `os.Rename` is the atomic-replace guarantee. `WriteFile` is a write-then-truncate-on-EOF that leaves a partial file on SIGINT. [CITED: `pkg.go.dev/os#Rename`] |

**Key insight:** Phase 1 and Phase 2 already shipped most of the load-bearing primitives (`Date.DaysUntil`, `Date.String`, `pickLocalized`, the decode pipeline, `*APIError`, sentinels). Phase 3's job is to compose them, not to re-derive them. Look for an existing helper before writing one.

## Runtime State Inventory

**Not applicable — this is a greenfield code-and-test addition with no rename, refactor, or stored-state mutation.** Phase 3 adds new files and new code paths. It modifies two existing files only:

- `countries.go` is refactored (Plan 1, D-63) — the public method signature changes from `Countries(ctx)` to `Countries(ctx, CountriesRequest)`. Existing test file updates with it. NOT a runtime-state migration: there are no stored records keyed by the old signature; the change is purely compile-time.
- `errors.go` appends `ErrMalformedResponse` to the existing `var (...)` block. Existing sentinels are untouched. No data migration needed.
- `internal_test.go` extends `allowedVars` map. No runtime state.
- `testdata/countries.json` is refreshed via the `-update` mechanism. Stored fixture data IS being overwritten by Plan 1, but the test commits the new file alongside the code — the new fixture goes through the same commit as the new code.

**Stored data:** None — go-openholidays has no datastore. Fixtures live in `testdata/` and are version-controlled.
**Live service config:** None — no external service registration.
**OS-registered state:** None.
**Secrets/env vars:** `OPENHOLIDAYS_LIVE=1` is consumed by the test-only `update_fixtures_test.go`; no secret value.
**Build artifacts/installed packages:** None — `go build` produces no installed state for a library.

## Common Pitfalls

### Pitfall 1: `doJSONGet[T]` returning a partial typed value alongside an error

**What goes wrong:** A caller writes `holidays, err := c.PublicHolidays(...); for _, h := range holidays { ... }; if err != nil { ... }`. If `doJSONGet` returns a non-nil `[]Holiday` plus an error, the loop runs on partial data before the error check.

**Why it happens:** Forgetting to assign `var zero T` on every failure path. The compiler doesn't enforce "zero on error".

**How to avoid:** Every error return statement uses `return zero, err` (where `zero` is the declared zero-value local). Code review checklist: grep `\breturn .*, fmt.Errorf` in `request.go` and verify each line returns the zero local, not a partial slice.

**Warning signs:** A unit test that passes `assert.Nil(t, holidays)` next to `assert.Error(t, err)` is the contract. If that assertion is missing or relaxed, the contract drifts.

### Pitfall 2: Empty-string optional query param becomes `?subdivisionCode=`

**What goes wrong:** `q.Set("subdivisionCode", req.SubdivisionCode)` when `req.SubdivisionCode == ""` produces a URL with `&subdivisionCode=` in it. Some upstream WAFs (verified historically with Cloudflare) reject the empty-value param outright; OpenHolidays does not, but it may in the future.

**Why it happens:** Forgetting the `if v != "" { q.Set(...) }` guard for every optional field.

**How to avoid:** Each endpoint method has one `if v != "" { q.Set(...) }` block per optional Request field. Pattern (recommended):

```go
q := url.Values{}
q.Set("countryIsoCode", countryCanonical)   // required
q.Set("validFrom", req.ValidFrom.String())  // required
q.Set("validTo", req.ValidTo.String())      // required
if req.LanguageIsoCode != "" {
    q.Set("languageIsoCode", langCanonical)
}
if req.SubdivisionCode != "" {
    q.Set("subdivisionCode", req.SubdivisionCode)
}
```

**Warning signs:** A handler in a test that does `assert.NotContains(t, r.URL.RawQuery, "subdivisionCode=")` fails. Add this as a positive test for the optional-omission contract.

### Pitfall 3: `Holiday.Range()` constructing `Date{t}` directly instead of via `NewDate`

**What goes wrong:** A caller passes a Holiday with `StartDate` constructed via a struct literal (e.g., `Date{time.Date(2025, 1, 1, 12, 0, 0, 0, time.Local)}` — non-UTC, non-midnight). `Range` walks via `Add(24h)`. Each yielded Date inherits the non-UTC location. The caller's `d.Equal(NewDate(2025, time.January, 2))` returns false unexpectedly.

**Why it happens:** The Date struct embedding allows literal construction that bypasses `NewDate`'s normalization. Phase 1 D-09's defensive normalization in `Equal`/`Before`/`After` handles comparison, but `AddDate` returns a new `time.Time` with the original Location.

**How to avoid:** `Range`'s implementation MUST rebuild each yielded Date via `NewDate(next.Year(), next.Month(), next.Day())` (see Pattern 2). Document the invariant in the godoc.

**Warning signs:** A test that constructs a Holiday with `Date{time.Date(..., time.Local)}` and iterates via `Range()` should verify every yielded Date has `Location() == time.UTC` and hour/minute/second/nanosecond zero.

### Pitfall 4: `Subdivision.Children` cycle in upstream data

**What goes wrong:** The upstream returns a malformed `/Subdivisions` response where subdivision A's `Children` contains B, and B's `Children` contains A. `buildParentIndex`'s recursive walk loops forever.

**Why it happens:** The upstream's data is curated, but defensive code should not assume curation correctness. ASVS V5.1.4 (input validation): never trust upstream structural invariants.

**How to avoid:** Two defenses, both cheap:

1. `buildParentIndex` uses an explicit visited-set (`map[string]struct{}`) to skip already-visited codes. Cycles produce a truncated index, not an infinite recursion.
2. `Client.IsInRegion`'s upward walk has an iteration cap of `len(parentIdx) + 1`. After that many steps, return `false` defensively.

**Warning signs:** Local unit test with a constructed cyclic tree must terminate within milliseconds and return without panic. Add as a defensive regression test.

### Pitfall 5: Fixture drift unnoticed because `httptest` handler always returns the fixture

**What goes wrong:** Upstream renames `temporalScope` to `temporalScope` (camelCase to PascalCase). The committed fixture still has `temporalScope`. Unit tests pass. Production calls fail at decode time. Drift is undetected until a real user complains.

**Why it happens:** Fixtures encode a point-in-time view of the upstream and never expire.

**How to avoid:**
- `fixtureCapturedAt` const in each test file (Phase 2 D-69 + D-69 extended). Surface stale dates in test log output.
- Nightly CI runs `OPENHOLIDAYS_LIVE=1 go test -tags=integration -run TestUpdateFixtures ./...` WITHOUT `-update` — drift-detection mode flags any mismatch loudly.
- A documented re-capture cadence (every 90 days or before each release, whichever first) lives in `CONTRIBUTING.md` (Phase 5).

**Warning signs:** A `fixtureCapturedAt` const older than 90 days. CI badges showing drift-detection job red.

### Pitfall 6: `ErrMalformedResponse` added but `allowedVars` not updated → AST audit fails

**What goes wrong:** Plan 4 adds `ErrMalformedResponse` to `errors.go`. Plan 4 forgets to extend `internal_test.go`'s `allowedVars` map. `TestNoInitOrGlobalState` fails with "unexpected package-level var 'ErrMalformedResponse' in errors.go:N".

**Why it happens:** The AST audit is one of the project's bedrock invariants — every new package-level var requires explicit allowlisting (`internal_test.go` lines 54-62 currently has 7 entries — 6 exported sentinels + `errEmptyDate`).

**How to avoid:** Plan 4 explicitly lists the `internal_test.go` allowedVars extension as a task line. Test order matters: the sentinel-test (the new `ErrMalformedResponse`-uses-errors.Is test) lands in `errors_test.go` AND the allowlist extension lands in `internal_test.go` in the SAME commit.

**Warning signs:** A grep for `allowedVars` in the planner's task list should show one task in Plan 4 (or earlier in the same wave).

## Code Examples

### Endpoint method body — `Subdivisions` (template for all four new endpoints)

```go
// subdivisions.go
package openholidays

import (
    "context"
    "net/url"
)

// SubdivisionsRequest carries the inputs for the Subdivisions endpoint.
//
// CountryIsoCode is required (ISO 3166-1 alpha-2; case-insensitive).
// LanguageIsoCode is optional (ISO 639-1; case-insensitive). When non-empty
// the upstream returns localized Name/Category/Comment text in the requested
// language; when empty the upstream returns every supported language.
type SubdivisionsRequest struct {
    CountryIsoCode  string
    LanguageIsoCode string
}

// Subdivisions fetches the administrative subdivisions of the given country.
// Returned subdivisions may carry recursive Children (see Subdivision.Children
// godoc). Use the returned slice with Client.IsInRegion for hierarchical
// region-membership checks.
//
// Errors:
//   - validateCountry failure on CountryIsoCode wraps ErrInvalidCountry.
//   - validateLanguage failure on a non-empty LanguageIsoCode wraps
//     ErrInvalidLanguage.
//   - Transport, decode, and HTTP errors surface verbatim via the
//     doJSONGet contract.
func (c *Client) Subdivisions(ctx context.Context, req SubdivisionsRequest) ([]Subdivision, error) {
    country, err := validateCountry(req.CountryIsoCode)
    if err != nil { return nil, err }
    q := url.Values{}
    q.Set("countryIsoCode", country)
    if req.LanguageIsoCode != "" {
        lang, err := validateLanguage(req.LanguageIsoCode)
        if err != nil { return nil, err }
        q.Set("languageIsoCode", lang)
    }
    return doJSONGet[[]Subdivision](ctx, c, "/Subdivisions", q)
}
```

### Endpoint method body — `PublicHolidays` (the full Holiday-validation path)

```go
// public_holidays.go
package openholidays

import (
    "context"
    "net/url"
)

type PublicHolidaysRequest struct {
    CountryIsoCode  string
    ValidFrom       Date
    ValidTo         Date
    LanguageIsoCode string
    SubdivisionCode string
}

func (c *Client) PublicHolidays(ctx context.Context, req PublicHolidaysRequest) ([]Holiday, error) {
    country, err := validateCountry(req.CountryIsoCode)
    if err != nil { return nil, err }
    if err := validateDateRange(req.ValidFrom, req.ValidTo); err != nil {
        return nil, err
    }
    q := url.Values{}
    q.Set("countryIsoCode", country)
    q.Set("validFrom", req.ValidFrom.String())
    q.Set("validTo", req.ValidTo.String())
    if req.LanguageIsoCode != "" {
        lang, err := validateLanguage(req.LanguageIsoCode)
        if err != nil { return nil, err }
        q.Set("languageIsoCode", lang)
    }
    if req.SubdivisionCode != "" {
        q.Set("subdivisionCode", req.SubdivisionCode)
    }
    holidays, err := doJSONGet[[]Holiday](ctx, c, "/PublicHolidays", q)
    if err != nil { return nil, err }
    if err := validateHolidays(holidays, "/PublicHolidays"); err != nil {
        return nil, err
    }
    return holidays, nil
}
```

### Helper — `Holiday.IsInRegion` (flat-only per D-58)

```go
// holiday.go  (or types.go — planner's call)
func (h Holiday) IsInRegion(code string) bool {
    if code == "" { return false }
    if h.Nationwide { return true }
    for _, s := range h.Subdivisions {
        if strings.EqualFold(s.Code, code) {
            return true
        }
    }
    return false
}
```

### Helper — `Holiday.Days`

```go
// Days returns the inclusive number of calendar days from StartDate to
// EndDate. For a single-day holiday (StartDate == EndDate) Days returns 1.
// For a 14-day Polish ferie zimowe period Days returns 14.
//
// The implementation delegates to Date.DaysUntil, which is calendar-correct
// across DST boundaries (Phase 1 D-10 / Pitfall TZ-2).
func (h Holiday) Days() int {
    return h.StartDate.DaysUntil(h.EndDate)
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Per-endpoint copy of the LimitReader+Decoder+drain pipeline | One generic `doJSONGet[T any]` | This phase (Plan 1) | -100 lines duplication; one place to fix bugs; one place to add the Phase 4 strict-decoding hook. |
| `Holiday.IsInRegion` doing both flat AND tree walks (one method) | `Holiday.IsInRegion` flat + `Client.IsInRegion` hierarchical (two methods) | This phase (D-58 / D-59 / CL-09) | The pure (no-I/O) method stays pure. Callers who don't need hierarchy don't pay for the Subdivisions HTTP call. |
| `Holiday.Range() iter.Seq[time.Time]` (ROADMAP literal) | `Holiday.Range() iter.Seq[Date]` (CL-11) | This phase | Iteration values compose with `Holiday.IsInRegion`, `Date.Equal`, `Date.DaysUntil` without conversion churn. |
| Manual `panic`-on-bad-data inside endpoints | Sentinel-error `ErrMalformedResponse` from `validateHolidays` | This phase (D-65, D-66, CL-12) | Callers can branch on `errors.Is(err, ErrMalformedResponse)`. Panic was never an option for a library — surfaced here for completeness. |
| `*http.Response.Body` short-reads without sanity check (`io.ReadAll(resp.Body)`) | `io.LimitReader` cap + `decoder.More()` boundary gate (Phase 2 already shipped) | Phase 2 D-24 | Re-stated here for completeness: Phase 3's `doJSONGet` inherits the gate. |

**Deprecated/outdated:**
- `time.Time` as the iter element type: superseded by `Date` (CL-11). Don't ship the `time.Time` variant; if a real consumer ever asks, expose a `Holiday.RangeTime() iter.Seq[time.Time]` as a thin adapter in v0.2.
- `Holiday.Validate() error` as a public method: explicitly NOT shipping. `validateHolidays` is private. Defer the public exposure until a real consumer asks for hand-built-Holiday validation.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Plan 8 (`-update` mechanism) can land last because Plans 2-5 capture fixtures manually during their authoring (planner: the dev runs the live API in a one-off shell command to seed the fixture file, then writes the test against the committed file) | §"Plan sequencing notes" | If the planner sequences Plan 8 first, Plans 2-5 need a different mechanism to seed fixtures (e.g., committing them inline from a shell capture). Risk: LOW — both orderings work; the recommendation is "land 8 last so it doesn't gate the substantive work". Confirmed by CONTEXT.md D-72 step 8. |
| A2 | The live upstream's `/SchoolHolidays?countryIsoCode=PL` PL 2025 response carries no `groups` field on its 7 entries (verified live 2026-05-27); German entries DO carry `groups` (verified live). D-70's "groupCode == B → false for PL-SL" assertion in `TestClient_SchoolHolidays` therefore needs a different shape: use the `groupCode` filter as the query param distinguishing variants, OR test against a DE fixture | §"Live API verification — SchoolHolidays" | If the planner writes a PL test that asserts on `groups` field content, the test won't reproduce a real response shape. Risk: MEDIUM — the test logic still works if it filters by `groupCode` URL param rather than inspecting response field, but the assertion specifics must match live data. |
| A3 | Capturing both `subdivisions_pl.json` (flat) AND `subdivisions_de.json` (with at least DE-BY/Augsburg children) gives `Client.IsInRegion` an authentic test target for the recursive path | §"Pattern 4 — hierarchical-test fixture problem" | If the planner captures only PL, the recursive path can be unit-tested only via a synthetically-constructed Subdivision tree. Risk: LOW — synthetic trees are common in Go tests; the only real loss is "no live-validated end-to-end hierarchical test". |
| A4 | `ErrMalformedResponse` is exported (not unexported) per CL-12 | §"User Constraints — Locked Decisions" | If accidentally shipped unexported, callers can't write `errors.Is(err, openholidays.ErrMalformedResponse)`. Risk: LOW — D-66 is explicit. |
| A5 | The 4 KiB `apiErrorBodyCap` and 10 MiB `maxResponseBytes` consts are safe to move from `countries.go` to `request.go` without breaking the other files (verified — they are currently the only consts in `countries.go` and the only consumer is `countries.go` itself; after Phase 3 they are shared across 5 endpoints) | §"User Constraints — D-63" | Moving the consts changes nothing semantically. Risk: NONE. |

**Note:** No `[ASSUMED]` package-name tags exist in this research because Phase 3 installs ZERO new packages. All assumptions are about test-design choices, not third-party software.

## Open Questions (RESOLVED)

1. **Hierarchical-test target — PL synthetic tree vs DE live fixture (recommendation locked in Pattern 4 / Assumption A3)**
   - What we know: PL upstream returns flat subdivisions; DE upstream returns 15 flat + 1 nested (DE-BY → Augsburg).
   - What's unclear: Does the planner prefer adding `subdivisions_de.json` as a Phase 3 fixture (one extra file, ~5 KiB) or synthesizing a tree literal in `client_helpers_test.go` (zero extra fixtures but the test stops authentically documenting upstream shape).
   - Recommendation: capture `subdivisions_de.json`. The fixture file is tiny, and it makes the recursive test live-grounded.
   - RESOLVED: Plan 3 captures `testdata/subdivisions_de.json` from the live API alongside `subdivisions_pl.json`; Plan 7 consumes the DE fixture for the hierarchical `Client.IsInRegion` test. See `03-03-PLAN.md` and `03-07-PLAN.md`.

2. **`validateHolidays` placement — `request.go` vs `holiday.go`?**
   - What we know: Either compiles. Either passes tests.
   - What's unclear: Project file-organization preference.
   - Recommendation: `request.go` (next to `doJSONGet`). Reinforces "response pipeline concern, not Holiday type concern".
   - RESOLVED: Plan 4 Task 1 places `validateHolidays` in `request.go` next to `doJSONGet`. See `03-04-PLAN.md` Task 1 `<action>` block.

3. **Pretty-printing fixture writes (Pattern 5 helper)**
   - What we know: `json.Indent` produces stable, reviewable diffs.
   - What's unclear: Whether reviewer prefers compact (1-line) fixtures (smaller diff size) vs pretty (one-field-per-line, easier to grep).
   - Recommendation: pretty-print with `json.Indent(..., "", "  ")`. The existing `testdata/countries.json` is already pretty-printed (verified — lines 1-38).
   - RESOLVED: Plan 8 Task 1 uses `json.Indent(&pretty, body, "", "  ")` before the atomic `os.Rename` write. See `03-08-PLAN.md` Task 1 overwrite-mode body.

4. **Hierarchical `Client.IsInRegion` — what if `h.Subdivisions` spans multiple countries?**
   - What we know: A Holiday today only carries subdivisions for one country (it's a per-country endpoint).
   - What's unclear: Future upstream behavior is uncertain.
   - Recommendation: derive country from `h.Subdivisions[0].Code` as in Pattern 4. If `h.Subdivisions` is empty AND `h.Nationwide` is false, return `(false, nil)` without HTTP. Document the assumption in the godoc so a future contributor knows this method's contract.
   - RESOLVED: Plan 7 Task 1 implements `Client.IsInRegion` with `splitCountryFromSubdivision(h.Subdivisions[0].Code)`; the godoc documents the single-country assumption and the (false, nil) no-HTTP path when `h.Subdivisions` is empty and `h.Nationwide` is false. See `03-07-PLAN.md` Task 1 `<action>` block.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | All Phase 3 work | ✓ | 1.23 (per go.mod) | — |
| `github.com/stretchr/testify` | Tests | ✓ | v1.11.1 (already in go.mod) | — |
| Live upstream `openholidaysapi.org` | `update_fixtures_test.go` only (build-tagged + env-gated) | ✓ (verified 2026-05-27: 200 OK for /Countries, /Languages, /Subdivisions?countryIsoCode=PL, /PublicHolidays?countryIsoCode=PL&validFrom=2025-01-01&validTo=2025-12-31, /SchoolHolidays?countryIsoCode=PL&validFrom=2025-01-01&validTo=2025-12-31) | — | When upstream is down: drift-detection mode (run without `-update`) and overwrite mode (with `-update`) both fail-loud via require.NoError + nonEmptyJSONArray sanity check. No silent corruption. |
| `golangci-lint` | Recommended but NOT required for Phase 3 closure | Unknown — not invoked in Phase 1 or Phase 2; Phase 5 owns | — | Phase 3 closes without lint; Phase 5 wires CI lint. CONTEXT.md §"Specific Ideas" notes ad-hoc local runs are advisable for `iter.Seq` introduction (which trips `gocritic` historically). |

**Missing dependencies with no fallback:** None.
**Missing dependencies with fallback:** None.

## Validation Architecture

> `workflow.nyquist_validation` not set in `.planning/config.json` → treat as enabled.

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` + `github.com/stretchr/testify` v1.11.1 |
| Config file | None (Go test framework requires none) |
| Quick run command | `go test ./...` |
| Full suite command | `go test -race -count=1 ./...` |
| Integration command | `OPENHOLIDAYS_LIVE=1 go test -tags=integration -count=1 ./...` |
| Fixture-refresh command | `OPENHOLIDAYS_LIVE=1 go test -tags=integration -update -run TestUpdateFixtures ./...` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| ENDPT-02 | Languages endpoint returns ≥14 entries from fixture | unit | `go test -run TestClient_Languages -count=1 ./...` | ❌ Plan 2 creates |
| ENDPT-03 | Subdivisions endpoint returns 16 PL województwa from fixture | unit | `go test -run TestClient_Subdivisions -count=1 ./...` | ❌ Plan 3 creates |
| ENDPT-04 | PublicHolidays returns 14 PL 2025 holidays incl. Wigilia | unit | `go test -run TestClient_PublicHolidays -count=1 ./...` | ❌ Plan 4 creates |
| ENDPT-05 | SchoolHolidays returns 7 PL 2025 periods | unit | `go test -run TestClient_SchoolHolidays -count=1 ./...` | ❌ Plan 5 creates |
| HELP-01 | `Holiday.NameFor("pl")` returns Polish name with fallback | unit | `go test -run TestHoliday_NameFor -count=1 ./...` | ❌ Plan 6 creates |
| HELP-02 | `Holiday.IsInRegion("PL-SL")` returns true for Śląskie ferie | unit | `go test -run TestHoliday_IsInRegion -count=1 ./...` | ❌ Plan 6 creates |
| HELP-02 (extended) | `Client.IsInRegion` walks DE-BY/Augsburg tree | unit | `go test -run TestClient_IsInRegion -count=1 ./...` | ❌ Plan 7 creates |
| HELP-03 | `Holiday.Days()` returns 14 for 14-day ferie | unit | `go test -run TestHoliday_Days -count=1 ./...` | ❌ Plan 6 creates |
| HELP-04 | `Holiday.Range()` yields 14 Dates inclusive | unit | `go test -run TestHoliday_Range -count=1 ./...` | ❌ Plan 6 creates |
| TEST-01 | Each endpoint covers 4 error paths in table | unit | included in per-endpoint `Test*` runs above | ❌ per plan |
| TEST-02 | All unit tests use `httptest.NewServer` (no live calls) | grep | `! grep -l 'http\.Get\|http\.DefaultClient\.Do' *_test.go` | ✓ enforced by Phase 2 invariant |
| TEST-03 | `-update` flag regenerates fixtures from live API | integration | `OPENHOLIDAYS_LIVE=1 go test -tags=integration -update -run TestUpdateFixtures ./...` | ❌ Plan 8 creates |

### Sampling Rate
- **Per task commit:** `go test ./... -run TestClient_<EndpointJustWritten> -count=1` (~1-2 seconds)
- **Per wave merge:** `go test -race -count=1 ./...` (~10 seconds; includes Phase 1 AST audit + Phase 2 transport tests + new Phase 3 tests)
- **Phase gate:** Full suite green AND fixture-drift check passes nightly (`OPENHOLIDAYS_LIVE=1 go test -tags=integration -run TestUpdateFixtures ./...` without `-update`)

### Wave 0 Gaps
- [ ] `request.go` + `request_test.go` — `doJSONGet[T any]` (Plan 1)
- [ ] `languages.go` + `languages_test.go` — covers ENDPT-02 (Plan 2)
- [ ] `subdivisions.go` + `subdivisions_test.go` — covers ENDPT-03 (Plan 3)
- [ ] `public_holidays.go` + `public_holidays_test.go` — covers ENDPT-04 + TEST-01 (Plan 4)
- [ ] `school_holidays.go` + `school_holidays_test.go` — covers ENDPT-05 (Plan 5)
- [ ] `holiday.go` + `holiday_test.go` — covers HELP-01..04 (Plan 6)
- [ ] `client_helpers.go` + `client_helpers_test.go` (or appended to `client.go`/`client_test.go`) — covers `Client.IsInRegion` (Plan 7)
- [ ] `update_fixtures_test.go` (build-tagged integration) — covers TEST-03 (Plan 8)
- [ ] `testdata/languages.json`, `subdivisions_pl.json`, `subdivisions_de.json`, `public_holidays_pl_2025.json`, `school_holidays_pl_2025.json` — captured live during Phase 3
- [ ] Re-capture `testdata/countries.json` after Countries retrofit (Plan 1)
- [ ] Framework install: none needed (testify already in go.mod)

## Security Domain

> `security_enforcement` not explicitly set in `.planning/config.json` → treat as enabled.

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | OpenHolidays API is keyless; no auth surface in this phase. |
| V3 Session Management | no | Stateless HTTP client; no sessions. |
| V4 Access Control | no | No authorization decisions. |
| V5 Input Validation | yes | `validateCountry`, `validateLanguage`, `validateDateRange` already pre-validate all user-supplied inputs (Phase 1 D-20..D-22). Phase 3 endpoint methods wire each call per D-56. ASVS V5.1.3 (positive validation against allowlist) satisfied by the byte-range `[A-Za-z]{2}` check in `validate.go` lines 110-121. |
| V5 Input Validation (post-decode) | yes | `validateHolidays` (D-65) enforces shape invariants on upstream-supplied data before returning to callers — addresses ASVS V5.5.2 ("verify that data ingested from external services is parsed and validated"). |
| V6 Cryptography | no | No crypto operations in this phase. |
| V7 Error Handling & Logging | yes | Error messages quote inputs (country/language codes, dates) but never secrets (no secrets exist in this API). `*APIError.Body` is capped at 4 KiB (Phase 1 D-17) to prevent log-bloat attacks via hostile upstream bodies. Phase 2 `loggingTransport` already enforces no-body-at-Info (D-31). |
| V14 Configuration | yes | Build-tag `//go:build integration` + `OPENHOLIDAYS_LIVE=1` env gate prevent accidental live-API calls during normal CI runs (Pitfall TEST-1 mitigation). |

### Known Threat Patterns for {Go HTTP client SDK}

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Hostile upstream returns 10+ GiB response → OOM | DoS | `io.LimitReader` cap (10 MiB, Phase 2 D-24/D-25). Inherited by `doJSONGet[T]`. |
| Hostile upstream returns malformed Holiday (zero StartDate, EndDate < StartDate) → silent bad data | Tampering | `validateHolidays` post-decode check (D-65); wraps `ErrMalformedResponse`. |
| Hostile upstream's RFC 7807 error body contains stack-trace or PII → leaks into operator logs | Information Disclosure | `*APIError.Body` capped at 4 KiB (Phase 1 D-17). `Error()` method omits Body from string output (`errors.go` line 82). |
| Empty optional query param sent as `?foo=` → upstream WAF rejects | DoS (self-inflicted) | Empty-string guards per D-55 (every `if v != "" { q.Set(...) }`). |
| Slopsquatted upstream domain via misconfigured `WithBaseURL` | Spoofing | Out of scope for this phase; Phase 4 / Phase 5 may revisit URL validation. v0.1.0 trusts the consumer's `WithBaseURL` value (documented contract). |
| Test-only `-update` flag accidentally runs in production CI → fixtures clobbered | Tampering (test infrastructure) | Build tag `//go:build integration` + env gate `OPENHOLIDAYS_LIVE=1` (double gate). Atomic temp-file + rename for write (single-fs atomicity). `nonEmptyJSONArray` sanity check before overwrite (transient-outage protection). |
| Cycle in `Subdivision.Children` from malformed upstream → infinite recursion in `buildParentIndex` | DoS | Visited-set in walk + iteration cap (`len(parentIdx)+1`) in upward walk (Pattern 4). |

### Project Constraints (from CLAUDE.md)

The project-level CLAUDE.md and `.planning/codebase/CONVENTIONS.md` lock the following invariants that Phase 3 MUST honor:

- **English only** (Gold Rule 1): All code, comments, godoc, test names, error messages, commit messages in English. `testdata/*.json` exception: Polish strings ("Wigilia Bożego Narodzenia", "ferie zimowe") are permitted because they reflect real upstream API responses.
- **Verify or ask** (Gold Rule 2): No "I think" / "probably" / "should be" in code or comments. Where research is uncertain, surface as an Assumption (see `## Assumptions Log`) — do not encode as a silent default.
- **Test conventions** (Gold Rule 3): `github.com/stretchr/testify` (assert + require), one `TestXxx` per exported production function, every case wrapped in `t.Run`, `require` for preconditions, `assert` for verifications. No further test-only dep without explicit user approval and PROJECT.md Key Decisions row.
- **Zero runtime deps**: No non-stdlib import in any `.go` file outside `*_test.go`. Phase 3 ships ZERO new imports.
- **godoc on every exported symbol** starting with the symbol name (Gold Rule 1 / PROJECT.md style).
- **Error string convention**: every error message starts with `"openholidays: "` (Phase 1 D-23).
- **No `init()` side effects, no global mutable state** (CLIENT-10): `internal_test.go::TestNoInitOrGlobalState` is the AST-walking enforcement. Plan 4 MUST extend the `allowedVars` map when adding `ErrMalformedResponse`.

These constraints rank above any pattern/convention recommendation in this research. If a recommendation appears to conflict, the constraint wins and the recommendation should be adjusted.

## Plan Sequencing Notes

Per CONTEXT.md D-72, 8 plans. The dependency edges between them:

```
                  Plan 1 (request.go + Countries refactor)
                            │
       ┌────────────────────┼────────────────────┐
       │                    │                    │
       ▼                    ▼                    ▼
   Plan 2              Plan 3                Plan 4
   (Languages)       (Subdivisions)      (PublicHolidays)
       │                    │                    │
       │                    │                    │
       │                    ▼                    │
       │              Plan 7 needs              ▼
       │              Subdivisions for      Plan 5
       │              hierarchical          (SchoolHolidays)
       │              Client.IsInRegion         │
       │                    │                   │
       └─────── Plan 6 (Holiday helpers; parallel-eligible) ──┐
                            │                                  │
                            ▼                                  │
                       Plan 7 (Client.IsInRegion)              │
                            │                                  │
                            ▼                                  │
                       Plan 8 (-update mechanism) ◄────────────┘
                       (lands last; touches every fixture)
```

**Plan 6 (`Holiday.NameFor/IsInRegion/Days/Range`)** depends on no other Phase 3 plan — only on Phase 1's Date + `pickLocalized`. It can be authored in parallel with Plans 2-5 if the planner enables parallelization.

**Plan 7 (`Client.IsInRegion`)** depends on Plan 3 (Subdivisions endpoint) and Plan 6 (`Holiday.IsInRegion` flat fast-path).

**Plan 8 (`-update`)** lands LAST per D-72 step 8. Fixtures used by Plans 2-7 are seeded by ad-hoc shell commands during their authoring (a developer runs e.g. `curl https://openholidaysapi.org/Languages > testdata/languages.json` before writing the table test, then commits the fixture alongside the code). Plan 8 then formalizes the in-code mechanism so future re-captures are scripted.

**Wave layout (recommended for parallelization):**
- Wave 0 (Plan 1): request.go + Countries refactor.
- Wave 1 (Plans 2, 3, 4, 5, 6 — parallel): four endpoints + Holiday helpers. Each plan owns its fixture commit.
- Wave 2 (Plan 7): Client.IsInRegion (needs Subdivisions endpoint from Wave 1 + Holiday flat helper from Wave 1).
- Wave 3 (Plan 8): -update mechanism + drift-detection wiring.

## Sources

### Primary (HIGH confidence)

- **Live upstream API verification (2026-05-27):**
  - `https://openholidaysapi.org/PublicHolidays?countryIsoCode=PL&validFrom=2025-01-01&validTo=2025-12-31&languageIsoCode=PL` — returned 14 holidays including Wigilia Bożego Narodzenia on 2025-12-24 (matches D-70 lock).
  - `https://openholidaysapi.org/SchoolHolidays?countryIsoCode=PL&validFrom=2025-01-01&validTo=2025-12-31&languageIsoCode=EN` — returned 7 periods, one item carries `subdivisions: [{code: PL-SL, ...}]` (matches D-70 lock).
  - `https://openholidaysapi.org/Subdivisions?countryIsoCode=PL&languageIsoCode=EN` — returned 16 flat województwa, NO `children` field on any entry (drives Pattern 4 hierarchical-fixture decision).
  - `https://openholidaysapi.org/Subdivisions?countryIsoCode=DE&languageIsoCode=EN` — returned 16 Bundesländer, DE-BY carries a `children` array (one entry: Augsburg).
  - `https://openholidaysapi.org/Languages` — returned 31 items; JSON shape `{isoCode: string, name: [{language: string, text: string}]}`.
- **Project codebase (2026-05-27 working tree):**
  - `/data/git/private/holidays/go.mod` lines 1-4 — module path, Go 1.23 directive, testify v1.11.1.
  - `/data/git/private/holidays/countries.go` lines 78-138 — the exact pipeline that `doJSONGet[T]` consolidates (`io.LimitedReader` + `decoder.More()` + `limited.N == 0` gates).
  - `/data/git/private/holidays/date.go` lines 144-164 — `DaysUntil` returns inclusive count (with sign awareness for negative spans).
  - `/data/git/private/holidays/types.go` lines 184-216 — `Subdivision.Children []Subdivision` recursive field already declared.
  - `/data/git/private/holidays/types.go` lines 224-243 — `pickLocalized` already implements the `strings.EqualFold` + first-entry-fallback semantics that `Holiday.NameFor` will reuse.
  - `/data/git/private/holidays/internal_test.go` lines 54-62 — current `allowedVars` map: 7 entries (6 sentinels + errEmptyDate).
- **Go documentation:**
  - `pkg.go.dev/iter` — `type Seq[V any] func(yield func(V) bool)`; yield panic semantics; early-termination contract.
  - `pkg.go.dev/net/url` — `url.Values{}.Set` + `.Encode` standard query-string idiom.
  - `pkg.go.dev/os#Rename` — single-filesystem atomic rename guarantee.
  - `pkg.go.dev/encoding/json#Indent` — canonical pretty-printer.
  - `pkg.go.dev/flag` — declaring test-only flags (e.g. `-update` per Go community convention).

### Secondary (MEDIUM confidence)

- `.planning/research/ARCHITECTURE.md` — internal research doc; Pattern 1/5/6/7 cited for chain composition, validator placement, error shape, test architecture.
- `.planning/research/PITFALLS.md` — internal research doc; Pitfalls HTTP-3/HTTP-4/JSON-1/JSON-4/TZ-2/TZ-3/OH-1/OH-2/OH-3/API-4 cross-referenced.
- `.planning/research/STACK.md` — internal research doc; Go 1.23 floor and stdlib-only baseline.

### Tertiary (LOW confidence)

- None. All claims in this research are grounded in either the live API, the codebase, or first-party Go documentation.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — zero new dependencies; reusing fully-verified Phase 1/2 imports.
- Architecture: HIGH — `doJSONGet[T]` body is a verbatim consolidation of the verified Phase 2 `countries.go` pipeline; `iter.Seq[Date]` body uses verified Go 1.23 `iter` semantics; hierarchical walk uses Subdivision.Children from existing types.
- Pitfalls: HIGH — every pitfall ties to either an existing Phase 1/2 mitigation that Phase 3 inherits or a new concern explicitly addressed by a CONTEXT.md decision.
- Live API verification: HIGH — five distinct upstream paths verified live with full body reads on 2026-05-27.
- Hierarchical-test fixture problem: HIGH for the diagnosis (PL is flat in live data); MEDIUM for the recommended remediation (capture DE fixture vs synthetic tree — both valid; recommended choice flagged as Assumption A3).

**Research date:** 2026-05-27
**Valid until:** 2026-08-25 (90 days for the upstream-shape findings; the codebase-grounded findings remain valid until the relevant source files change)
