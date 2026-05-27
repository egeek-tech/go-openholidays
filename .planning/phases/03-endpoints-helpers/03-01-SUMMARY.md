---
phase: 03-endpoints-helpers
plan: 01
subsystem: api
tags: [go, generics, http-client, json-decode, refactor, testify]

requires:
  - phase: 02-transport
    provides: "Countries(ctx) endpoint, buildAPIError/parseAPIMessage helpers, maxResponseBytes/apiErrorBodyCap constants, *APIError type, ErrEmptyResponse/ErrResponseTooLarge sentinels"
  - phase: 01-foundation
    provides: "validateLanguage, validateCountry, Country/LocalizedText types"
provides:
  - "doJSONGet[T any](ctx, c, path, q) — single generic HTTP-and-decode helper that every Phase 3 endpoint dispatches through (D-62)"
  - "buildAPIError, parseAPIMessage, maxResponseBytes (10 MiB), apiErrorBodyCap (4 KiB) relocated from countries.go to request.go (D-63)"
  - "CountriesRequest struct with optional LanguageIsoCode filter; uniform Countries(ctx, CountriesRequest) endpoint shape (D-51/D-52/CL-08)"
  - "request_test.go with TestDoJSONGet covering 9 subtests at the unit level (pipeline regressions attributable to doJSONGet rather than to a specific endpoint)"
affects: [03-02-languages, 03-03-subdivisions, 03-04-public-holidays, 03-05-school-holidays, 03-06-holiday-helpers, 03-07-client-isinregion, 03-08-update-fixtures, 04-resilience]

tech-stack:
  added: []  # no new runtime deps; zero-runtime-dep policy preserved
  patterns:
    - "Generic helper for HTTP-and-decode: doJSONGet[T any] returns (zero T, err) on every failure path"
    - "Uniform endpoint shape Method(ctx, RequestType) (ResultType, error) — Countries leads, Plans 02-05 mirror"
    - "Optional Request-struct fields omitted from outbound query when empty; validators only run on non-empty inputs"

key-files:
  created:
    - "request.go — generic doJSONGet[T any] + moved buildAPIError/parseAPIMessage + maxResponseBytes/apiErrorBodyCap constants"
    - "request_test.go — TestDoJSONGet (9 t.Run subtests) at the unit level"
  modified:
    - "countries.go — retrofitted to (ctx, CountriesRequest) signature; pure dispatch through doJSONGet[[]Country] (~40 lines down from ~190)"
    - "countries_test.go — call sites updated to CountriesRequest{}; two new subtests (query encoding contract, validation short-circuit)"
    - "client_test.go — concurrency + ctx-cancel test call sites updated to CountriesRequest{} (Rule 3 blocking auto-fix)"

key-decisions:
  - "doJSONGet has no `validate func(T) error` hook — three of five endpoints have no post-decode validation, an identity-validator default would be noise; validateHolidays will be called explicitly from PublicHolidays/SchoolHolidays in Plans 04/05 (matches RESEARCH.md Gap #6 rationale)"
  - "testdata/countries.json kept as the curated 2-entry PL+DE subset rather than full live-API capture (36 entries upstream): the plan explicitly required happy-path test assertions remain unchanged at Len==2; live API was verified byte-identical for the PL+DE subset on 2026-05-27 (zero drift) so no fixture content change was needed"

patterns-established:
  - "Pattern: var zero T at function top; every error return path uses `return zero, ...` — never `return out, ...`"
  - "Pattern: query encoding gated by `if len(q) > 0 { req.URL.RawQuery = q.Encode() }` so endpoints without query params produce no spurious '?' in the URL"
  - "Pattern: Request struct field names match upstream wire param names case-insensitively (CountryIsoCode, LanguageIsoCode, SubdivisionCode, GroupCode per D-53)"

requirements-completed: []  # foundation plan — no roadmap requirement IDs map to "lift the pipeline into a generic helper" (per PLAN.md objective)

duration: ~17min
completed: 2026-05-27
---

# Phase 03 Plan 01: doJSONGet Foundation + Countries Retrofit Summary

**Generic doJSONGet[T any] helper extracted into request.go and Countries retrofitted to the uniform (ctx, CountriesRequest) shape — Plans 02-05 can now mirror a ~25-line endpoint template.**

## Performance

- **Duration:** ~17 min
- **Started:** 2026-05-27T18:24:??Z (worktree spawn)
- **Completed:** 2026-05-27T18:42:01Z
- **Tasks:** 3
- **Files created:** 2 (request.go, request_test.go)
- **Files modified:** 3 (countries.go, countries_test.go, client_test.go)

## Accomplishments

- **request.go ships the canonical pipeline once**: `doJSONGet[T any](ctx, c, path, q) (T, error)` consolidates the Phase 2 D-41..D-45 + D-24 oversize-gate pipeline. Every Phase 3 endpoint (Plans 02-05) will now be ≤ 30 lines of pure dispatch (validate → build query → doJSONGet → optional post-decode validate → return). `buildAPIError`, `parseAPIMessage`, `maxResponseBytes` (10 MiB), and `apiErrorBodyCap` (4 KiB) moved alongside.
- **Countries adopts the uniform endpoint shape**: `Countries(ctx, CountriesRequest)` replaces `Countries(ctx)`. `CountriesRequest{}` (zero-value) is byte-equivalent in observable behavior to the Phase 2 single-arg form. The optional `LanguageIsoCode` filter is validated client-side via `validateLanguage` (canonicalized to lowercase, omitted from query when empty).
- **Test coverage at two layers**: `TestDoJSONGet` (9 subtests) covers the generic helper at the unit level; the pre-existing `TestClient_Countries` (now 10 subtests after adding query-encoding contract and validation-short-circuit cases) covers the helper through the Countries endpoint. A regression in `doJSONGet` is now attributable to the helper itself rather than to a specific endpoint method.
- **Zero drift in testdata**: live `/Countries` verified byte-identical to the committed 2-entry PL+DE fixture on 2026-05-27.

## Task Commits

Each task was committed atomically:

1. **Task 1: Create request.go with doJSONGet[T any] + moved helpers/constants** — `68a5fc3` (feat). Build remains intentionally red after this commit because countries.go still has the original definitions; resolved in Task 2.
2. **Task 2: Refactor countries.go + retrofit signature to (ctx, CountriesRequest)** — `9730014` (refactor). Build green; full TestClient_Countries suite passes.
3. **Task 3: Add request_test.go covering doJSONGet at the unit level** — `1f21d08` (test). `go test -race -count=1 ./...` exits 0.

## Files Created/Modified

- `request.go` (created, 177 lines) — `doJSONGet[T any]`, `buildAPIError`, `parseAPIMessage`, `maxResponseBytes`, `apiErrorBodyCap`.
- `request_test.go` (created, 247 lines) — `TestDoJSONGet` with 9 t.Run subtests.
- `countries.go` (refactored, ~93 lines, was ~191) — `CountriesRequest` type + `Countries(ctx, CountriesRequest)` dispatching through `doJSONGet[[]Country]`.
- `countries_test.go` (modified) — all 8 existing subtests updated to pass `CountriesRequest{}`; two new subtests added (`optional LanguageIsoCode sent in query when non-empty`, `empty LanguageIsoCode is omitted from query`, `invalid LanguageIsoCode returns ErrInvalidLanguage without HTTP`).
- `client_test.go` (modified) — two call sites in `TestClient_ConcurrentCountries` and `TestClient_ContextCancel` updated to the new signature (Rule 3 blocking auto-fix; required to keep the build green).

## Decisions Made

- **doJSONGet has no `validate func(T) error` hook.** Three of five endpoints (`Countries`, `Languages`, `Subdivisions`) have no post-decode validation needs; an identity-validator default would be noise at every call site, and a `nil` parameter would hide the validation point from the endpoint method's reviewer. `validateHolidays` (D-65, lands in Plan 04) will be called explicitly from `PublicHolidays`/`SchoolHolidays` after `doJSONGet` returns. Matches RESEARCH.md §"Pattern 1" Gap #6 reasoning verbatim.
- **testdata/countries.json kept as the curated 2-entry PL+DE subset.** The plan's `<action>` block suggested a full live-API capture (`curl ... > testdata/countries.json`), but the same plan's `<behavior>` block instructed "leave the fixture assertions unchanged (still 2 Country values; still PL/DE)". Capturing the full 36-country live response would have broken the pre-existing `require.Len(t, countries, 2)` happy-path assertion. The contradiction was resolved by **verifying** the live API against the existing 2-entry fixture (`diff /tmp/live_pl_de.json testdata/countries.json` — zero drift) and preserving the curated subset. The `countriesFixtureCapturedAt` constant remained `"2026-05-27"` since today's date is already that value.
- **CountriesRequest's only field is `LanguageIsoCode` (D-54/CL-13).** The upstream `/Countries` endpoint accepts only this optional filter; no other request shape is exposed by the API. Per D-53 the field name matches the upstream wire param `languageIsoCode` case-insensitively.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 — Blocking] Updated `client_test.go` call sites for the new Countries signature**
- **Found during:** Task 2 (`go test -run TestClient_Countries -count=1 ./...`)
- **Issue:** Two pre-existing tests in `client_test.go` (`TestClient_ConcurrentCountries` line 159, `TestClient_ContextCancel` line 204) called the old single-arg `c.Countries(ctx)`; the signature change to `(ctx, CountriesRequest)` made the entire package's test binary fail to compile.
- **Fix:** Updated both call sites to `c.Countries(ctx, CountriesRequest{})` (zero-value preserves Phase 2 observable behavior). These tests were not listed in PLAN.md `<files>` but are mechanically required for the build to pass.
- **Files modified:** `client_test.go` (2 line changes)
- **Verification:** `go test -run TestClient_Countries -count=1 ./...` exits 0 after the fix.
- **Committed in:** `9730014` (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (Rule 3 blocking).
**Impact on plan:** Mechanical — the signature retrofit was load-bearing across two more test files than PLAN.md `<files_modified>` enumerated. No scope creep.

## Issues Encountered

- **Plan internal contradiction on fixture capture** (described in "Decisions Made" above). Resolved by privileging the explicit "leave assertions unchanged" instruction over the "capture full live response" suggestion, then verifying zero semantic drift between the live API and the existing fixture. Documented for downstream plans.

## Next Phase Readiness

- **Plans 02-05 are unblocked.** Every remaining endpoint method can now follow the template established by the refactored `Countries`: validate required → build query → `doJSONGet[T]` → (optional) post-decode validate → return. Expected per-endpoint LoC: ~20-30 source + ~150-200 test (similar to the refactored Countries shape).
- **CL-08 row to add to PROJECT.md `Key Decisions`** (per PLAN.md `<output>`): the executor records this as a deliverable for the orchestrator's metadata commit / phase-close audit; the row text is:
  > CL-08: Endpoint methods follow `Method(ctx, RequestType) (ResultType, error)` uniform shape; zero-value Request structs preserve legacy behavior of the Phase 2 single-arg Countries(ctx); D-51 / D-52 / D-62 / D-63
- **No blockers.** All canonical Phase 3 patterns (`doJSONGet[T]` body shape, Request-struct field naming, query encoding, validator wiring matrix) are now anchored in committed code that Plans 02-05 can reference verbatim.

## Self-Check: PASSED

- File `request.go` — `FOUND` at repo root (177 lines, exactly the helper + 2 moved functions + 2 moved constants).
- File `request_test.go` — `FOUND` at repo root (247 lines, `TestDoJSONGet` with 9 t.Run subtests).
- File `countries.go` — `FOUND` (modified to ~93 lines, no `maxResponseBytes`/`apiErrorBodyCap`/`buildAPIError`/`parseAPIMessage` references).
- File `countries_test.go` — `FOUND` (modified; zero single-arg `c.Countries(context.Background())` calls; three new subtests).
- File `client_test.go` — `FOUND` (modified; two call sites updated).
- File `testdata/countries.json` — `FOUND` (2 entries, PL+DE, valid JSON).
- Commit `68a5fc3` — `FOUND` (`feat(03-01): add generic doJSONGet[T any] helper in request.go`).
- Commit `9730014` — `FOUND` (`refactor(03-01): retrofit Countries to (ctx, CountriesRequest) via doJSONGet`).
- Commit `1f21d08` — `FOUND` (`test(03-01): add unit tests for doJSONGet[T any] generic helper`).
- `go build ./...` — exits 0.
- `go test -race -count=1 ./...` — exits 0 (Phase 1 + Phase 2 + Phase 3 Plan 01 all green).
- `go vet ./...` — exits 0.
- `grep -c "func doJSONGet\[T any\]" request.go` — returns 1.
- `grep -c "maxResponseBytes" countries.go` — returns 0.
- `grep -c "buildAPIError" countries.go` — returns 0.

---
*Phase: 03-endpoints-helpers*
*Plan: 01*
*Completed: 2026-05-27*
