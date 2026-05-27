# Phase 3: Endpoints & Helpers - Context

**Gathered:** 2026-05-27
**Status:** Ready for planning

<domain>
## Phase Boundary

Deliver the four remaining endpoint methods on `*Client` plus the four `Holiday` helpers, all backed by golden fixtures captured from the live OpenHolidays API:

- `Languages(ctx, LanguagesRequest) ([]Language, error)` — supported-languages list, optional `LanguageIsoCode` filter.
- `Subdivisions(ctx, SubdivisionsRequest) ([]Subdivision, error)` — administrative subdivisions for a country, optional `LanguageIsoCode` filter.
- `PublicHolidays(ctx, PublicHolidaysRequest) ([]Holiday, error)` — required `CountryIsoCode`/`ValidFrom`/`ValidTo`, optional `LanguageIsoCode`/`SubdivisionCode`.
- `SchoolHolidays(ctx, SchoolHolidaysRequest) ([]Holiday, error)` — same required fields as PublicHolidays plus optional `GroupCode` (Polish ferie cohort).
- Retrofit `Countries(ctx, CountriesRequest) ([]Country, error)` so all five endpoints follow the uniform `(ctx, Request)` shape (Phase 2 surface deviation; CL-08).
- `Holiday.NameFor(lang) string` — localized name via shared `pickLocalized` helper.
- `Holiday.IsInRegion(code) bool` — flat, no-I/O match against `Holiday.Subdivisions[].Code` with nationwide short-circuit.
- `Client.IsInRegion(ctx, holiday, code) (bool, error)` — hierarchical match that fetches `/Subdivisions` and walks `Subdivision.Children` (new public method beyond REQUIREMENTS HELP-02; CL-09).
- `Holiday.Days() int` — inclusive day count via `StartDate.DaysUntil(EndDate)`.
- `Holiday.Range() iter.Seq[Date]` — yields every `Date` from StartDate to EndDate inclusive (deviation from ROADMAP `iter.Seq[time.Time]`; CL-11).
- Shared `request.go` with generic `doJSONGet[T any](ctx, c *Client, path string, q url.Values) (T, error)` that encapsulates the full Phase 2 D-41..D-45 + D-24 oversize-gate pipeline.
- Post-decode Holiday validation: reject zero StartDate/EndDate, reject EndDate before StartDate (Pitfall JSON-4; new sentinel `ErrMalformedResponse`; CL-12).
- Golden fixtures in `testdata/` captured live during Phase 3 execution on 2026-05-27 via `OPENHOLIDAYS_LIVE=1 go test -tags=integration -update ./...`.

What this phase does NOT deliver: retry, cache, `WithRequestHook`, `WithStrictDecoding`, observability hook beyond Phase 2's logging, `cmd/ohcli`, CI workflows, release tooling, `Holiday.IsInRegion` parent-walking from a single client call (only `Client.IsInRegion` does that — `Holiday.IsInRegion` stays flat). All of those depend on the endpoint + helper contract being stable.

</domain>

<decisions>
## Implementation Decisions

### Endpoint signatures and request types

- **D-51:** Every endpoint takes a single Request struct as its second argument: `(c *Client) X(ctx context.Context, req XRequest) (..., error)`. The five Request types are `CountriesRequest`, `LanguagesRequest`, `SubdivisionsRequest`, `PublicHolidaysRequest`, `SchoolHolidaysRequest`. Symmetry across the public surface beats REQUIREMENTS literal text (where `Languages(ctx)` and `Subdivisions(ctx, country, lang)` were positional). Future filter additions become non-breaking field adds to the existing struct. Recorded as **CL-08** in PROJECT.md `Key Decisions` before Phase 3 closes.
- **D-52:** Phase 2's shipped `Countries(ctx) ([]Country, error)` signature is retrofitted to `Countries(ctx, CountriesRequest) ([]Country, error)`. Pre-1.0 (v0.x) breaking changes are allowed per PROJECT.md backwards-compat policy. `CountriesRequest{}` (zero value) reproduces the current behavior verbatim, so the retrofit is mechanically a struct-shape change with no semantic regression. CHANGELOG entry required at v0.2 if Phase 2 already shipped publicly (it has not — repo is pre-tag). Phase 2's `countries_test.go` and the `countries.json` fixture both update to the new signature in this phase.
- **D-53:** Country/Language code field is named `CountryIsoCode` and `LanguageIsoCode` on Request structs. Matches the upstream wire query-param names (`countryIsoCode`, `languageIsoCode`) and the existing `Country.IsoCode` / `Language.IsoCode` field names in `types.go`. Subdivision code field is `SubdivisionCode` (the upstream param is `subdivisionCode`; `Subdivision.Code` keeps the bare name because it is the unique key on the type). `GroupCode` matches the upstream `groupCode` param.
- **D-54:** v0.1.0 Request structs expose every upstream-supported filter for the four new endpoints AND for Countries:
  - `CountriesRequest{LanguageIsoCode string}` — optional language filter.
  - `LanguagesRequest{LanguageIsoCode string}` — optional language filter.
  - `SubdivisionsRequest{CountryIsoCode string; LanguageIsoCode string}` — required country, optional language.
  - `PublicHolidaysRequest{CountryIsoCode string; ValidFrom Date; ValidTo Date; LanguageIsoCode string; SubdivisionCode string}` — required country/dates, optional language/subdivision.
  - `SchoolHolidaysRequest{CountryIsoCode string; ValidFrom Date; ValidTo Date; LanguageIsoCode string; SubdivisionCode string; GroupCode string}` — adds optional group filter for Polish ferie cohorts (A/B/C/D).

  Recorded as **CL-13** in PROJECT.md `Key Decisions` (filters exceed the REQUIREMENTS literal-text shape).
- **D-55:** Empty-string optional fields are treated as "not set" and therefore omitted from the outbound query string. Each endpoint method builds query params via `url.Values{}.Set(key, value)` only when the value is non-empty after canonicalization. Validators run only on non-empty inputs for the optional fields, on every call for the required ones.
- **D-56:** Validator wiring per endpoint method (free functions per ARCHITECTURE.md Pattern 5 — no Request.Validate methods):
  - `Countries`: validateLanguage on `LanguageIsoCode` when non-empty.
  - `Languages`: validateLanguage on `LanguageIsoCode` when non-empty.
  - `Subdivisions`: validateCountry on `CountryIsoCode` (required); validateLanguage on `LanguageIsoCode` when non-empty.
  - `PublicHolidays` / `SchoolHolidays`: validateCountry on `CountryIsoCode`; validateDateRange on `(ValidFrom, ValidTo)`; validateLanguage on `LanguageIsoCode` when non-empty; the optional `SubdivisionCode`/`GroupCode` are shape-tolerant strings — pass them through to the upstream and let it reject (no client-side shape allowlist exists for those).

### Holiday helpers

- **D-57:** `(h Holiday) NameFor(lang string) string` — symmetric with `Country.NameFor` / `Language.NameFor` / `Subdivision.NameFor`. Same CL-05 rationale: `Holiday.Name` is the `[]LocalizedText` field, so a `Name(lang)` method would collide. Implementation delegates to the existing unexported `pickLocalized` helper in `types.go`. Recorded as **CL-10** in PROJECT.md.
- **D-58:** `(h Holiday) IsInRegion(code string) bool` — flat match with these rules in order:
  1. `code == ""` → return false (no panic; defensive).
  2. `h.Nationwide` → return true (REQUIREMENTS HELP-02 literal: "or is nationwide").
  3. Iterate `h.Subdivisions` and return true on the first `strings.EqualFold(sub.Code, code)` match.
  4. Return false otherwise.
  No I/O; no allocation; no recursion into a tree (Holiday only carries flat `[]SubdivisionRef`).
- **D-59:** `(c *Client) IsInRegion(ctx context.Context, h Holiday, code string) (bool, error)` — hierarchical match that handles "is `PL-SL-KAT` covered by a holiday that applies to `PL-SL`?". Flow:
  1. Same fast-path guards as `Holiday.IsInRegion` (empty code → `(false, nil)`; `h.Nationwide` → `(true, nil)`).
  2. Fetch `/Subdivisions` for the country implied by the matched Holiday subdivisions' prefix (e.g., `PL-SL` → `CountryIsoCode: "PL"`). Reuse `Subdivisions(ctx, SubdivisionsRequest{CountryIsoCode: <derived>})`.
  3. Build a parent-index from the returned tree by walking `Subdivision.Children` recursively (`map[string]string` — child code to parent code).
  4. Walk upward from `code` using the parent-index until a match against `h.Subdivisions[].Code` is found, or the root is reached.
  5. Return `(true, nil)` or `(false, nil)`; surface any `Subdivisions(...)` error verbatim.

  Hidden I/O is documented in godoc. Caller is responsible for caching when called in a hot loop (Phase 4 cache will help). Recorded as **CL-09** in PROJECT.md (new public method beyond REQUIREMENTS HELP-02 literal text).
- **D-60:** `(h Holiday) Days() int` — `h.StartDate.DaysUntil(h.EndDate)`. Phase 1 D-10 designed `DaysUntil` precisely to be the inclusive day count helper for this method (UTC-midnight operands, so DST-safe). For single-day holidays (`StartDate == EndDate`), returns 1. For Polish ferie zimowe Śląskie 2025 (Jan 18 – Jan 31 = 14 days), returns 14. ROADMAP success criterion 4 + Pitfall TZ-2 are satisfied by Phase 1's existing implementation.
- **D-61:** `(h Holiday) Range() iter.Seq[Date]` — yields `Date` values (not `time.Time`). Deviation from ROADMAP success criterion 4 literal `iter.Seq[time.Time]`; rationale: the rest of the type surface (`StartDate`, `EndDate`, `Equal`/`Before`/`After`/`Compare`, `DaysUntil`) is all Date-typed, so composition with helpers is direct. Callers who need a `time.Time` use the embedded field: `for d := range h.Range() { t := d.Time }`. Implementation walks via `d.AddDate(0, 0, 1)` and rebuilds a Date at each step using `NewDate(year, month, day)` so the UTC-midnight invariant is preserved on every yielded value. Recorded as **CL-11** in PROJECT.md.

### HTTP plumbing — `request.go`

- **D-62:** A new file `request.go` introduces an unexported generic helper:
  ```go
  func doJSONGet[T any](ctx context.Context, c *Client, path string, q url.Values) (T, error)
  ```
  The helper encapsulates: nil-ctx guard, `context.WithTimeout(ctx, c.timeout)` when timeout > 0, `http.NewRequestWithContext`, query-string assembly via `req.URL.RawQuery = q.Encode()`, `c.http.Do`, the `io.LimitReader(maxResponseBytes+1)` drain-then-close defer, status check (`buildAPIError` for ≥ 400), empty-body → `ErrEmptyResponse` wrap, `LimitReader(maxResponseBytes)` decode, mid-truncation gate (`limited.N == 0` → `ErrResponseTooLarge`), and boundary gate (`decoder.More()` → `ErrResponseTooLarge`). Returns the zero value of `T` plus the error on every failure path. Per ARCHITECTURE.md §132 ("request.go — unexported newRequest, decode[T], buildURL helpers").
- **D-63:** `countries.go` is refactored to call `doJSONGet[[]Country]` instead of inlining the pipeline. The existing `buildAPIError` and `parseAPIMessage` helpers move to `request.go` alongside `doJSONGet`. The `maxResponseBytes` and `apiErrorBodyCap` constants move with them (their natural home is the shared helper, not the Countries-specific endpoint file).
- **D-64:** Each new endpoint file (`languages.go`, `subdivisions.go`, `public_holidays.go`, `school_holidays.go`) contains only: the Request type, the endpoint method (≤ ~30 lines), and any endpoint-specific post-decode validation (e.g., the `validateHolidays` slice walk from D-65 lives in a helper called by `public_holidays.go` and `school_holidays.go`). Endpoint files do NOT re-declare the HTTP pipeline — they call `doJSONGet`.
- **D-65:** Post-decode Holiday validation lives in a single unexported function `validateHolidays(hs []Holiday, path string) error` in either `request.go` or a new `holiday.go`. The function iterates the slice and returns an error wrapping `ErrMalformedResponse` (new sentinel — CL-12) on the first violation:
  - `h.StartDate.IsZero()` → reject.
  - `h.EndDate.IsZero()` → reject.
  - `h.EndDate.Before(h.StartDate)` → reject.
  Error message includes the offending Holiday's ID and the failing predicate. Only `PublicHolidays` and `SchoolHolidays` call this; the other endpoints' types do not contain dates. Phase 2's `ErrEmptyResponse` is reused only for genuinely empty bodies (HTTP 200 with no JSON), not for decoded-but-invalid Holiday entries.

### Error surface additions

- **D-66:** New exported sentinel `ErrMalformedResponse` is appended to `errors.go`'s existing `var (...)` block. Phase 1's `TestNoInitOrGlobalState` allowlist (Plan 06 `internal_test.go`) must extend to include this new sentinel — same operational pattern that Phase 2 followed for `ErrResponseTooLarge`. Total exported sentinel count after Phase 3: 7 (`ErrInvalidCountry`, `ErrInvalidLanguage`, `ErrDateRangeTooLarge`, `ErrInvalidDateRange`, `ErrEmptyResponse`, `ErrResponseTooLarge`, `ErrMalformedResponse`). Recorded as **CL-12** in PROJECT.md.

### Golden fixtures and test architecture

- **D-67:** Fixture refresh mechanism: a top-level `var updateFixtures = flag.Bool("update", false, "regenerate testdata fixtures from live API")` declared once in a new `testdata_update_test.go` (build-tagged `//go:build integration`). Each endpoint's table-driven test branches on `*updateFixtures` — when set, the test issues a real HTTP request to the upstream and writes the response body to the fixture path (gofmt-style overwrite). When unset (the default), tests assert against the committed fixture via `httptest.NewServer` per Phase 2 D-46.
- **D-68:** Fixture capture is gated by both `//go:build integration` (compile-time gate) AND `OPENHOLIDAYS_LIVE=1` (run-time gate). The combination prevents accidental overwrite during normal `go test ./...` (which doesn't include `-tags=integration`) and during CI (which doesn't set `OPENHOLIDAYS_LIVE`). The plan executor refreshes fixtures once during Phase 3 with: `OPENHOLIDAYS_LIVE=1 go test -tags=integration -update -run TestUpdateFixtures ./...`.
- **D-69:** Each endpoint test file declares a Go const recording the fixture capture date, following Phase 2's `countriesFixtureCapturedAt = "2026-05-27"` convention. Constant names: `languagesFixtureCapturedAt`, `subdivisionsPLFixtureCapturedAt`, `publicHolidaysPL2025FixtureCapturedAt`, `schoolHolidaysPL2025FixtureCapturedAt`. The constant is included in test failure messages so a stale fixture surfaces with its capture date at the diff.
- **D-70:** Fixture-shape sanity assertions live in every happy-path test. Concrete asserts that lock the verified 2026-05-27 PL data:
  - `TestClient_PublicHolidays` happy path: `len(holidays) == 14`; one entry has `NameFor("pl") == "Wigilia Bożego Narodzenia"`; that same entry's `StartDate == NewDate(2025, 12, 24)`.
  - `TestClient_SchoolHolidays` happy path: `len(periods) == 7`; an entry with `NameFor("pl") == "ferie zimowe"` has `IsInRegion("PL-SL")` true while an entry from a different cohort (e.g., `groupCode == "B"`) returns false for "PL-SL".
  - `TestClient_Subdivisions` happy path for PL: `len(subdivisions) == 16`; first level contains all 16 województwa codes.
  - `TestClient_Languages` happy path: `len(languages) >= 14` (loose lower bound to tolerate upstream additions).
  Per-endpoint TEST-01 still requires the table-driven 4 error paths: network failure, 4xx (APIError), 5xx (APIError), malformed JSON (decode error), ctx cancel; the new sentinel ErrMalformedResponse + the existing ErrResponseTooLarge are tested per endpoint too.
- **D-71:** Fixture files live at root `testdata/`: `countries.json` (Phase 2 — refreshed in Phase 3 when Countries gets retrofitted), `languages.json`, `subdivisions_pl.json`, `public_holidays_pl_2025.json`, `school_holidays_pl_2025.json`. Per ARCHITECTURE.md §158-164. No per-test subdirectories until fixtures exceed ~10 files.

### Plan sequencing

- **D-72:** Suggested plan order (planner may refine):
  1. `request.go` extract + Countries refactor + Countries retrofit to `CountriesRequest`. Phase 2's `countries_test.go` and `testdata/countries.json` update with it. CL-08 row added.
  2. `Languages` endpoint + `LanguagesRequest` + fixture + test.
  3. `Subdivisions` endpoint + `SubdivisionsRequest` + fixture + test.
  4. `PublicHolidays` endpoint + `PublicHolidaysRequest` + `validateHolidays` helper + `ErrMalformedResponse` sentinel + CLIENT-10 allowlist update + fixture + test. CL-12, CL-13 rows added.
  5. `SchoolHolidays` endpoint + `SchoolHolidaysRequest` + fixture + test.
  6. `Holiday.NameFor` + `Holiday.IsInRegion` + `Holiday.Days` + `Holiday.Range` + tests (the helpers don't touch HTTP — can land in parallel with endpoint plans 2-5 if dependency tracking permits). CL-10, CL-11 rows added.
  7. `Client.IsInRegion` hierarchical helper + tests against captured Subdivisions tree. CL-09 row added.
  8. Fixture-refresh test-only mechanism (`testdata_update_test.go`, `-update` flag, integration build tag). Land last so it doesn't gate the substantive endpoint work.

### Claude's Discretion

The following are inferred from already-locked architecture and conventions; no need to re-ask:

- File layout: one endpoint per `*.go` file at repo root (`languages.go`, `subdivisions.go`, `public_holidays.go`, `school_holidays.go`); shared HTTP plumbing in `request.go`; helpers stay in `types.go` (Holiday helpers may move to a new `holiday.go` if planner prefers split-by-type — either is acceptable since both live in `package openholidays`).
- Test file layout: `languages_test.go`, `subdivisions_test.go`, `public_holidays_test.go`, `school_holidays_test.go`, `request_test.go`, plus an `update_fixtures_test.go` for the `-update` mechanism.
- Every test follows Gold Rule 3: one `TestXxx` per exported production function, every case in `t.Run`, `require` for preconditions, `assert` for verifications.
- Every error string starts with `"openholidays: "` (Phase 1 D-23 convention; Phase 2 reused it).
- godoc comment on every exported symbol, starting with the symbol name (Gold Rule 1 + PROJECT.md).
- English-only invariant (Gold Rule 1). Polish strings in `testdata/` fixtures are permitted since they reflect real upstream responses (`"Wigilia Bożego Narodzenia"`, `"ferie zimowe"`, `"Śląskie"`).

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project baseline (read first)
- `.planning/PROJECT.md` — locked constraints (zero runtime deps, 10 MiB cap, default timeout 15s, slog Debug-only HTTP logging, ≤ 100 ms ctx cancellation), Key Decisions table (CL-01..CL-07; CL-08..CL-13 will be added by this phase).
- `.planning/REQUIREMENTS.md` — Phase 3 owns: ENDPT-02..05 (4), HELP-01..04 (4), TEST-01..03 (3) = 11 requirements.
- `.planning/ROADMAP.md` §"Phase 3: Endpoints & Helpers" — goal + 5 success criteria (note: D-61 deviates from criterion #4's `iter.Seq[time.Time]` literal; tests must assert against `iter.Seq[Date]`).
- `.planning/STATE.md` — running ledger of decisions inherited from Phases 1-2 (CL-01..CL-07).
- `.planning/codebase/CONVENTIONS.md` — Gold Project Rules (English-only, verify-or-ask, testify+one-test-per-prod-func+t.Run).

### Architecture and patterns (read before writing endpoints or helpers)
- `.planning/research/ARCHITECTURE.md` §"Pattern 1: Functional Options" (lines 209-264) — clientConfig builder shape that the existing Client already follows.
- `.planning/research/ARCHITECTURE.md` §"Recommended Project Structure" (lines 119-181) — per-endpoint-file layout, `request.go` placement, `testdata/` at root.
- `.planning/research/ARCHITECTURE.md` §"Pattern 6: Error Construction at the Method Layer, Sentinels at Package Level" (lines 451-524) — `*APIError` construction site, `errors.Is`/`errors.As` traversal idiom.
- `.planning/research/ARCHITECTURE.md` §"Pattern 7: Test Architecture — Per-File + Build-Tagged Integration" (lines 526-599) — `httptest.NewServer` per case, `//go:build integration` for live tests, shared `internal/testhttp/` helper (defer to a later phase if not actively needed).
- `.planning/research/ARCHITECTURE.md` §"Data Flow — Request Flow: client.PublicHolidays(ctx, req) traversal" (lines 620-720) — verified end-to-end traversal that D-62's `doJSONGet[T]` consolidates.
- `.planning/research/STACK.md` §"Core Technologies" — Go 1.23 floor (locks `iter.Seq` availability for D-61's `Holiday.Range`), `net/http`, `encoding/json` v1, `log/slog`.

### Pitfalls (read before writing endpoint or helper code)
- `.planning/research/PITFALLS.md` §"Pitfall HTTP-3: Closing without draining" — D-62 inherits Phase 2 D-45's defer-drain-then-close pattern verbatim.
- `.planning/research/PITFALLS.md` §"Pitfall HTTP-4: No `io.LimitReader` cap" — D-62 inherits the 10 MiB cap from Phase 2.
- `.planning/research/PITFALLS.md` §"Pitfall JSON-1: Strict decoding by default" — lenient by default; strict mode lands in Phase 4 (do not preempt here).
- `.planning/research/PITFALLS.md` §"Pitfall JSON-2: Non-pointer fields for optional values" — Holiday's optional fields (Comment, Subdivisions, Groups, Tags, Quality) are already shaped per types.go; do not re-design.
- `.planning/research/PITFALLS.md` §"Pitfall JSON-4: time.Time zero value passing as valid" — drives D-65's `validateHolidays` post-decode pass.
- `.planning/research/PITFALLS.md` §"Pitfall TZ-2: DST off-by-one in date arithmetic" — Phase 1 D-10's `DaysUntil` already mitigates; D-60 reuses it. D-61's `Range` uses `AddDate` + `NewDate` so it inherits the same UTC-midnight invariant.
- `.planning/research/PITFALLS.md` §"Pitfall TZ-3: Treating a multi-day school holiday as a single date" — D-60/D-61 explicitly handle StartDate ≠ EndDate; tests must assert against Polish ferie zimowe Śląskie 2025 (14 days inclusive).
- `.planning/research/PITFALLS.md` §"Pitfall OH-1: 3-year query window cap" — Phase 1's `validateDateRange` already enforces; PublicHolidays/SchoolHolidays just call it.
- `.planning/research/PITFALLS.md` §"Pitfall OH-2: Optional fields missing in some responses" — types.go's `omitempty` tags handle the marshal direction; lenient decode handles the unmarshal direction; no Phase 3 work required.
- `.planning/research/PITFALLS.md` §"Pitfall OH-3: `name` is an array, not a map" — `pickLocalized` already handles; `Holiday.NameFor` reuses it.
- `.planning/research/PITFALLS.md` §"Pitfall API-4: Returning slice/map references callers can mutate" — endpoints return freshly-decoded slices, so caller mutation only affects their own copy (cache lands in Phase 4 and stores raw bytes — defensive copy is not required for v0.1.0).

### Phase 1 & 2 anchors (read for state inherited from prior phases)
- `.planning/phases/01-foundation/01-CONTEXT.md` — D-01..D-23 + CL-01..CL-06. Especially D-05..D-12 (Date type + DaysUntil), D-13..D-19 (errors + APIError), D-20..D-23 (validators).
- `.planning/phases/02-transport/02-CONTEXT.md` — D-24..D-50 + CL-07. Especially D-24/D-25 (ErrResponseTooLarge + 10 MiB cap), D-26..D-28 (timeout enforcement), D-29..D-31 (transport chain), D-41..D-45 (Countries endpoint flow that `doJSONGet` consolidates), D-46..D-50 (test architecture conventions).
- `.planning/phases/02-transport/02-VERIFICATION.md` and `.planning/phases/02-transport/02-REVIEW.md` (when present) — verify any open follow-ups that Phase 3 should not regress.
- Phase 1-2 source files (`errors.go`, `date.go`, `types.go`, `validate.go`, `client.go`, `config.go`, `options.go`, `transport.go`, `countries.go`) — reusable assets enumerated in `<code_context>` below.

### Upstream API (verify before writing endpoint decoders)
- `https://openholidaysapi.org/swagger/v1/swagger.json` — OpenAPI 3 spec. Re-verified live on 2026-05-27 during this discussion. Confirmed endpoint contracts:
  - `/Languages` accepts optional `languageIsoCode` query param.
  - `/Subdivisions` requires `countryIsoCode`; accepts optional `languageIsoCode`.
  - `/PublicHolidays` requires `countryIsoCode`, `validFrom`, `validTo`; accepts optional `languageIsoCode`, `subdivisionCode`.
  - `/SchoolHolidays` adds optional `groupCode` on top of PublicHolidays' shape.
  - Date params are `string/date` (YYYY-MM-DD); query encoding via `url.Values{}.Set` + `Encode` produces the expected wire format.
  - 4xx error envelopes follow RFC 7807 ProblemDetails (Phase 2 `parseAPIMessage` already handles).
  - No rate-limit headers observed (Phase 1 PITFALLS OH-4 — no Phase 3 work required).

### Gold Project Rules (apply everywhere)
- `CLAUDE.md` §"Project Rules (Gold)" — Rule 1 (English-only), Rule 2 (verify-or-ask), Rule 3 (testify + t.Run + one-test-per-prod-function).

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets (from Phases 1 & 2)

- `types.go` — `Holiday`, `Country`, `Language`, `Subdivision`, `LocalizedText`, `SubdivisionRef`, `GroupRef` already in place with verified upstream JSON tags. `pickLocalized()` (unexported) backs the three existing `NameFor` accessors and will back `Holiday.NameFor` (D-57).
- `date.go` — `Date.DaysUntil` (Phase 1 D-10) is the exact helper `Holiday.Days` needs (D-60). `NewDate(year, month, day)` and `AddDate` round-trip preserve UTC-midnight, used by `Holiday.Range` (D-61).
- `errors.go` — Phase 3 appends `ErrMalformedResponse` (D-66/CL-12); the existing 6 sentinels remain untouched. `*APIError`, `buildAPIError`, `parseAPIMessage` move to `request.go` along with `doJSONGet` (D-63).
- `validate.go` — `validateCountry`, `validateLanguage`, `validateDateRange` are unexported and W-01-hardened (Phase 2 D-32). Phase 3 endpoint methods call them per D-56's wiring matrix.
- `client.go` — `*Client.http`, `*Client.baseURL`, `*Client.timeout` are exactly what `doJSONGet` consumes. The Client struct stays immutable; no new fields required for Phase 3 (Phase 4 will add `closeOnce` and the cache sweeper handle).
- `countries.go` — D-63 refactors this file to call `doJSONGet[[]Country]`. The `Countries(ctx, CountriesRequest{LanguageIsoCode: ...})` retrofit is in this plan too. `maxResponseBytes` and `apiErrorBodyCap` constants move from here to `request.go`.
- `transport.go` — `headerTransport` and `loggingTransport` are unchanged. The `attempt` field stays hardcoded to 1 — retry doesn't land until Phase 4.
- `config.go` / `options.go` — no changes required for Phase 3. All option semantics already in place (timeout, base URL, UA, logger).
- `testdata/countries.json` — refreshed during Phase 3 (Countries retrofit). `countriesFixtureCapturedAt = "2026-05-27"` const updates to the Phase 3 capture date.

### Established Patterns (continue using)

- One file per endpoint at repo root (`languages.go` + `languages_test.go`, etc.); no `endpoints/` subdirectory.
- testify-based tests (Gold Rule 3): one `TestXxx` per exported prod function, every case in `t.Run`, `require` for preconditions, `assert` for verifications.
- English-only invariant (Gold Rule 1) with `testdata/` exception for real upstream Polish strings.
- Error-message convention: every error string starts with `"openholidays: "` (Phase 1 D-23).
- godoc comment on every exported symbol, starting with the symbol name.
- testify-only test deps; `cmp` import allowed only when testify output is insufficient (PROJECT.md test-only dep allowlist).

### Integration Points

- **Phase 4 (Resilience) MUST NOT alter Phase 3 method signatures** (ROADMAP architectural note). Retry, cache, hook, strict-decoding all land as transparent middleware that wraps the chain — endpoint methods stay byte-identical. This is the explicit correctness test Phase 4 must pass before merging.
- `doJSONGet[T]` is the natural injection point for Phase 4's retry: the retry loop wraps the `c.http.Do(req)` call inside `doJSONGet`. The endpoint methods don't need to know.
- `validateHolidays` is the natural injection point for Phase 4's strict-decoding side effects: when strict mode is on, decode would fail earlier (unknown-field error); `validateHolidays` still runs on whatever did decode and surfaces the structural complaints in addition.
- `Client.IsInRegion` (D-59) issues an extra HTTP call to `/Subdivisions`. Phase 4's cache (path-prefix matched on `/Subdivisions`) makes this method cheap on hot loops; Phase 3 documents the cost in godoc but does not preempt the cache.
- `Holiday.Range()` yielding `Date` (D-61) means downstream callers iterating over school holidays compose with `Holiday.IsInRegion` and the `Date` math helpers without conversion churn.
- Phase 5's `cmd/ohcli` consumes `PublicHolidays(ctx, PublicHolidaysRequest{CountryIsoCode: "PL", ValidFrom: ..., ValidTo: ...})` and `SchoolHolidays(...)`; the uniform Request-struct shape (D-51/CL-08) is the contract the CLI table-printer will build against.

</code_context>

<specifics>
## Specific Ideas

- The retrofit of `Countries(ctx)` to `Countries(ctx, CountriesRequest{})` should arrive in plan 1 alongside `request.go`. Postponing it would leave the codebase inconsistent for the rest of Phase 3 and risk muscle-memory drift in subsequent endpoint plans.
- Test assertions for `Client.IsInRegion` (D-59) should use Polish ferie zimowe Śląskie 2025 as the canonical hierarchical case: a holiday that applies to `PL-SL` (województwo) should return true when asked about an arbitrary powiat code that does NOT appear in `Holiday.Subdivisions` but DOES appear under `Subdivision.Children` of `PL-SL` in the captured `/Subdivisions` tree. The captured `subdivisions_pl.json` must include at least one nested powiat under one województwo to exercise this path — verify during the fixture capture and amend the capture if upstream returns flat subdivisions for PL.
- `validateHolidays` (D-65) error messages should include the offending Holiday's `ID` field (UUID string) so debugging an upstream regression doesn't require diffing the entire response. Use `fmt.Errorf("openholidays: malformed holiday %q: %s: %w", h.ID, predicate, ErrMalformedResponse)`.
- The `-update` mechanism (D-67/D-68) must NOT overwrite a fixture file when the live response is empty or HTTP non-200 — the executor's plan should add a guard that aborts the write and surfaces a descriptive error. Otherwise a transient upstream outage corrupts the committed fixtures.
- Adding `iter.Seq[Date]` to the surface bumps the type's relevance for `golangci-lint`'s `gocritic` rule set; verify lint clean before commit (the W-01 fix plan in Phase 2 left golangci-lint as a Phase 5 concern, but ad-hoc local runs are advisable).

</specifics>

<deferred>
## Deferred Ideas

- **Hierarchical IsInRegion on Holiday itself (no client)** — `Holiday.IsInRegionTree(code, subs []Subdivision) bool` was considered. Rejected for v0.1.0 in favor of D-59's `Client.IsInRegion` because callers who already hold the subdivision tree can build their own parent-index helper, and the more common case (one-off lookup) is better served by the Client method that handles the fetch.
- **A `BuildRegionIndex(subs []Subdivision) map[string][]string`-style helper** — would let callers cache the parent-index across many `IsInRegion` calls. Defer to v0.2; revisit if a real consumer needs the optimization. Phase 4's cache covers the dominant case (repeated `/Subdivisions` calls).
- **Request validation methods (`Req.Validate() error`)** — explicitly rejected per ARCHITECTURE.md Pattern 5. Validators remain free functions called from endpoint methods. If a real consumer wants to validate without dispatching, expose a `ValidateXRequest(req XRequest) error` later; not in scope for v0.1.0.
- **`CountriesRequest.LanguageIsoCode` could go in Phase 4** instead of being a Phase 3 retrofit. Rejected: keeping Countries on the old `Countries(ctx)` signature for one more phase forces an awkward "5 different shapes" state that no consumer would tolerate. Retrofit now or accept permanent asymmetry.
- **`Languages` filter parity with `Countries`** — the `LanguagesRequest{LanguageIsoCode}` field IS in Phase 3 (D-54) for the same uniform-shape reason as CountriesRequest.
- **`PublicHolidaysByDate` and `SchoolHolidaysByDate` endpoints** — exist in the upstream API (verified 2026-05-27). Not in REQUIREMENTS.md. Defer to v0.2 or M2. Document existence in `docs/design.md` (Phase 5 deliverable) so future consumers know to ask.
- **`Statistics/PublicHolidays` and `Statistics/SchoolHolidays` endpoints** — exist upstream. Not in REQUIREMENTS. Out of scope for v0.1.0.
- **Single-flight on `/Subdivisions` for `Client.IsInRegion`** — when many goroutines call `Client.IsInRegion` concurrently on a cold cache, they all hit upstream. Phase 4's cache mitigates the steady-state cost but not the cold start. Defer; revisit if a real consumer hits the cold-start scenario.
- **Defensive deep copy of returned `[]Holiday`** — Pitfall API-4. Endpoints currently return freshly-decoded slices, so caller mutation is local to their copy. Phase 4's cache stores raw bytes (not typed slices), so cache + mutation don't intersect. If the cache design changes in Phase 4 to store typed values, defensive copy becomes load-bearing — flag for Phase 4 discussion.
- **Subdivision-tree memoization in `Client`** — would speed `Client.IsInRegion`. Defer to Phase 4's cache scope (path-prefix match on `/Subdivisions` already handles the steady-state case).
- **Localized `comment` field accessor** (`Holiday.CommentFor(lang)`) — Holiday carries `Comment []LocalizedText`. A `CommentFor(lang)` helper symmetric with `NameFor` would be ergonomic. Out of scope for v0.1.0 (HELP-01 covers only Name); flag for v0.2.
- **A `Holiday.Validate() error` method** — D-65 puts validation in an unexported package function. If consumers ever need to validate a hand-built Holiday before passing it elsewhere, the function could be exported as `ValidateHoliday`. Defer until a real consumer asks.

</deferred>

---

*Phase: 03-endpoints-helpers*
*Context gathered: 2026-05-27*
