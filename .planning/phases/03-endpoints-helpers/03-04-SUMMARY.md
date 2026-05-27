---
phase: 03-endpoints-helpers
plan: 04
subsystem: api
tags: [public-holidays, validation, sentinels, fixtures, testify, golang]

# Dependency graph
requires:
  - phase: 03-endpoints-helpers/03-01
    provides: "doJSONGet[T] generic HTTP-and-decode pipeline + maxResponseBytes + apiErrorBodyCap + buildAPIError + parseAPIMessage in request.go"
  - phase: 02-countries
    provides: "Countries endpoint analog (HTTP pipeline, *APIError, ErrEmptyResponse, ErrResponseTooLarge surface)"
  - phase: 01-foundation
    provides: "Date type, Holiday struct, HolidayType constants, errors.go sentinel surface, validate.go validators, internal_test.go CLIENT-10 AST audit"
provides:
  - "Client.PublicHolidays(ctx, PublicHolidaysRequest) ([]Holiday, error) endpoint method"
  - "PublicHolidaysRequest type with all five upstream-supported filters (CountryIsoCode + ValidFrom + ValidTo + LanguageIsoCode + SubdivisionCode)"
  - "validateHolidays(hs, path) post-decode invariant helper in request.go"
  - "ErrMalformedResponse exported sentinel (seventh in the package surface)"
  - "internal_test.go allowedVars extended to 8 entries; CLIENT-10 AST audit stays green"
  - "testdata/public_holidays_pl_2025.json: 14 live-captured PL public holidays for 2025 (D-70 sanity fixtures)"
  - "TestClient_PublicHolidays + TestValidateHolidays — 13 + 7 = 20 subtests covering happy path + 4 error paths + 4 validation paths + 3 query-builder shape paths + ErrMalformedResponse + validator-unit invariants"
affects:
  - "03-05 (SchoolHolidays): sibling endpoint analog — same validate→build→doJSONGet→validateHolidays flow with one extra optional GroupCode field"
  - "03-06 (holiday helpers / Client.IsInRegion): consume Holiday values that have passed validateHolidays"
  - "phase 04 (cache + decoders): cache layer transparently wraps PublicHolidays; strict decoder may surface Quality field drift"

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Endpoint files scoped to (Request type + endpoint method ≤30 lines + post-decode validate call) per D-64"
    - "validateXxx helpers placed next to doJSONGet in request.go (RESEARCH.md Pattern 6)"
    - "Pointer iteration for read-only slice walks (h := &hs[i]) to avoid rangeValCopy linter regressions"
    - "URL-builder contract assertions inside httptest handler (r.URL.Query().Get(...)) — catches mis-spelled query-param keys that fixture-only tests miss"
    - "Live-captured testdata fixture with publicHolidaysPL2025FixtureCapturedAt const echoed in failure messages (D-69)"

key-files:
  created:
    - "public_holidays.go"
    - "public_holidays_test.go"
    - "testdata/public_holidays_pl_2025.json"
  modified:
    - "errors.go (appended ErrMalformedResponse sentinel + updated file doc)"
    - "internal_test.go (extended allowedVars + updated file doc)"
    - "request.go (added validateHolidays helper next to doJSONGet)"

key-decisions:
  - "CL-12: ErrMalformedResponse sentinel for post-decode Holiday schema-drift detection — D-65 / D-66 / public_holidays.go and request.go"
  - "CL-13: Request structs expose every upstream-supported filter — CountriesRequest.LanguageIsoCode, LanguagesRequest.LanguageIsoCode, SubdivisionsRequest, PublicHolidaysRequest.SubdivisionCode, SchoolHolidaysRequest.GroupCode — exceeding REQUIREMENTS literal text; D-54"
  - "validateHolidays placed in request.go (not a new holiday.go) per RESEARCH.md Pattern 6 — keeps the helper conceptually part of the response pipeline and unexported"
  - "Fixture Wigilia entry text stored with proper Polish diacritics ('Wigilia Bożego Narodzenia') because that is what the live API returns on 2026-05-27 — admitted under CONVENTIONS.md Rule 1 testdata-fixture exception"

patterns-established:
  - "Pattern: required-validators → optional-validator wiring (D-56) — country + dateRange always run; language only when non-empty; subdivision is shape-tolerant pass-through"
  - "Pattern: validateXxx post-decode helpers wrap a single typed sentinel via fmt.Errorf %w so caller errors.Is(err, ErrXxx) works through endpoint-method wrappers"
  - "Pattern: live-capture-then-pretty-print fixtures committed alongside their failure-message-bearing date const"
  - "Pattern: validation-only test subtests use WithBaseURL('http://example.invalid') (RFC 6761) so an accidental HTTP dispatch fails loudly"

requirements-completed:
  - ENDPT-04
  - TEST-01

# Metrics
duration: 4min
completed: 2026-05-27
---

# Phase 03 Plan 04: PublicHolidays Endpoint Summary

**PublicHolidays endpoint with all five upstream filters + ErrMalformedResponse sentinel + validateHolidays post-decode invariant helper + 20-subtest coverage on a live PL 2025 fixture**

## Performance

- **Duration:** ~4 min wall-clock between commits (task 1 → task 3)
- **First commit (Task 1):** 2026-05-27T18:50:00Z
- **Last commit (Task 3):** 2026-05-27T18:53:24Z
- **Tasks:** 3 (all atomic)
- **Files modified:** 3 (errors.go, internal_test.go, request.go); **Files created:** 3 (public_holidays.go, public_holidays_test.go, testdata/public_holidays_pl_2025.json)

## Accomplishments

- Shipped `Client.PublicHolidays(ctx, PublicHolidaysRequest) ([]Holiday, error)` — the most invariant-heavy endpoint in Phase 3 (two required validators, two optional fields, one post-decode pass).
- Added the seventh exported sentinel `ErrMalformedResponse` and its companion `validateHolidays` helper, closing Pitfall JSON-4 (time.Time zero value masquerading as a valid Date).
- Captured a 14-entry PL 2025 public-holidays fixture from the live upstream and locked the D-70 sanity assertions (Wigilia Bożego Narodzenia on 2025-12-24) into both `TestClient_PublicHolidays` and a date-stamped file-header const.
- Extended the CLIENT-10 AST audit (`internal_test.go::allowedVars`) atomically in the same commit as the new sentinel, per the Pitfall 6 same-commit protocol — preventing a transient red CI between commits.

## Task Commits

Each task was committed atomically:

1. **Task 1: ErrMalformedResponse sentinel + allowedVars extension + validateHolidays helper** — `0a648f1` (feat)
2. **Task 2: PublicHolidays endpoint method + PL 2025 live-captured fixture** — `688b524` (feat)
3. **Task 3: TestClient_PublicHolidays (13 subtests) + TestValidateHolidays (7 subtests) coverage** — `a04b6ef` (test)

_Note: Per the project's hand-rolled TDD discipline (plan declares `tdd="true"` per task), Tasks 1 and 2 produced production code whose contract is exercised by Task 3's test file. The validation gate is `TestNoInitOrGlobalState` passing after Task 1 (the AST audit's allowlist extension is the executable contract for the sentinel addition); the unit-level contract for `validateHolidays` is the seven-subtest `TestValidateHolidays` in Task 3. Task 2's contract is the 12-subtest `TestClient_PublicHolidays`. No RED-then-GREEN split commits were produced because the plan's verify gates pin compile-then-functional rather than failing-test-first._

## Files Created/Modified

- `public_holidays.go` — declares `PublicHolidaysRequest` (5 fields per D-54/CL-13) and the `Client.PublicHolidays` endpoint method (calls `validateCountry` → `validateDateRange` → optional `validateLanguage` → optional shape-tolerant `SubdivisionCode` pass-through → `doJSONGet[[]Holiday]("/PublicHolidays", q)` → `validateHolidays(holidays, "/PublicHolidays")` per the RESEARCH.md verbatim template).
- `public_holidays_test.go` — `TestClient_PublicHolidays` (13 subtests) + `TestValidateHolidays` (7 subtests). Top-level `publicHolidaysPL2025FixtureCapturedAt = "2026-05-27"` const recorded per D-69 and surfaced in fixture-missing failure messages.
- `testdata/public_holidays_pl_2025.json` — 14 holidays, pretty-printed UTF-8, captured live on 2026-05-27 from `https://openholidaysapi.org/PublicHolidays?countryIsoCode=PL&validFrom=2025-01-01&validTo=2025-12-31&languageIsoCode=PL`.
- `errors.go` — appended `ErrMalformedResponse = errors.New("openholidays: malformed response")` to the existing var block; updated file-header doc from "five sentinels" to "seven sentinels".
- `internal_test.go` — appended `"ErrMalformedResponse": {}` to `allowedVars`; updated file-header doc from "six entries" to "eight entries"; extended the `allowedVars` godoc with an explanatory bullet for the new sentinel.
- `request.go` — appended `validateHolidays(hs []Holiday, path string) error` next to `doJSONGet`/`buildAPIError`/`parseAPIMessage`. Body verbatim from RESEARCH.md §"Pattern 6". Pointer-iteration discipline (`h := &hs[i]`) per the rangeValCopy linter recommendation.

## Decisions Made

- **CL-12** (locked by this plan): `ErrMalformedResponse` is the seventh exported sentinel and the only branch-point for post-decode Holiday schema drift. The wrapping convention is `fmt.Errorf("openholidays: malformed holiday %q at %s: <predicate>: %w", h.ID, path, ErrMalformedResponse)` — the holiday's UUID, the endpoint path, and the failing predicate name are all in the error string so a bug report has full diagnostics.
- **CL-13** (locked by this plan): Request structs in this phase expose every upstream-supported filter even where REQUIREMENTS.md's literal text only mentioned a subset. PublicHolidaysRequest carries `SubdivisionCode` even though REQUIREMENTS-ENDPT-04 didn't enumerate it — the upstream supports it, the cost to consumers is one zero-value field, and the alternative (omitting it now, adding it later) would force a breaking change once a caller actually needed regional filtering.
- The Wigilia Bożego Narodzenia entry's Polish text in the fixture is `"Wigilia Bożego Narodzenia"` with the diacritic `ż` (U+017C). PLAN.md's `read_first` section spelled it without the diacritic in one place, but live API verification showed the upstream emits the diacritic, and CONVENTIONS.md Rule 1's testdata exception explicitly admits real upstream bytes. The test assertion uses the live-verified form.

## Deviations from Plan

None — plan executed exactly as written. The plan body, RESEARCH.md Pattern 6, and PATTERNS.md skeletons were all internally consistent and matched the existing Phase 2 Countries analog; no Rule 1/2/3 auto-fixes were needed.

The one minor calibration noted under "Decisions Made" above (Wigilia diacritic) was a documentation-level choice that defers to live-verified bytes per Gold Rule 2 — not a deviation from the plan's behavioral contract (the plan explicitly says "with proper Polish diacritics — the literal text per D-70" in Task 2's action block).

## Issues Encountered

None — all three tasks compiled clean on the first build, all 19 new subtests passed on the first run with `-race`, and `TestNoInitOrGlobalState` stayed green continuously.

## User Setup Required

None — no external service configuration required. The live fixture was captured by the executor at plan time; consumers of the library do not need to re-capture unless the upstream schema drifts.

## Threat Mitigations Verified

Per the plan's `<threat_model>` register, this plan ships the following mitigations:

- **T-3-InputVal-Country** → `validateCountry` called before HTTP (W-01 hardening inherited from Phase 2).
- **T-3-InputVal-DateRange** → `validateDateRange` called before HTTP; rejects `from > to` and windows wider than 3 calendar years.
- **T-3-InputVal-Lang** → `validateLanguage` called when `LanguageIsoCode` is non-empty.
- **T-3-DoS-EmptyQueryParam** → Empty-string guards confirmed for both `LanguageIsoCode` and `SubdivisionCode` (the `if req.X != ""` branch omits the query parameter entirely).
- **T-3-Tampering-MalformedHoliday** → `validateHolidays` runs unconditionally on every endpoint response; the new `ErrMalformedResponse` sentinel gives callers a typed branch for upstream schema drift. Closes Pitfall JSON-4 (D-65 / CL-12).
- **T-3-DoS-OverSize** → 10 MiB cap inherited from `doJSONGet` (verified by the existing `TestClient_Countries` oversize subtest in countries_test.go).
- **T-3-InfoDisc-APIErrorBody** → 4 KiB `APIError.Body` cap inherited from `buildAPIError` (verified by `TestClient_Countries`'s 4 KiB cap subtest).

## PROJECT.md Key Decisions — Lines to Append

Per the plan's `<output>` section, this plan owns BOTH CL-12 AND CL-13. The orchestrator (or a follow-up doc commit) must append the two lines below to PROJECT.md's `Key Decisions` table:

- `CL-12: ErrMalformedResponse sentinel for post-decode Holiday schema-drift detection — D-65 / D-66 / public_holidays.go and request.go`
- `CL-13: Request structs expose every upstream-supported filter — CountriesRequest.LanguageIsoCode, LanguagesRequest.LanguageIsoCode, SubdivisionsRequest, PublicHolidaysRequest.SubdivisionCode, SchoolHolidaysRequest.GroupCode — exceeding REQUIREMENTS literal text; D-54`

## Sanity-Assertion Snapshot

For downstream tests to copy verbatim:

- `len(holidays) == 14` for PL 2025 (verified live 2026-05-27).
- One holiday has `NameFor("pl") == "Wigilia Bożego Narodzenia"` (with `ż`, U+017C).
- That same holiday's `StartDate.Equal(NewDate(2025, time.December, 24)) == true`.
- `internal_test.go::allowedVars` now has 8 entries (7 exported sentinels + `errEmptyDate`).
- `grep -c 't.Run(' public_holidays_test.go` returns **20** (13 in `TestClient_PublicHolidays`, 7 in `TestValidateHolidays`).

## Self-Check: PASSED

- `public_holidays.go` exists (verified).
- `public_holidays_test.go` exists (verified).
- `testdata/public_holidays_pl_2025.json` exists, 14 entries, Wigilia on 2025-12-24 (verified).
- Commit `0a648f1` exists in git log (verified).
- Commit `688b524` exists in git log (verified).
- Commit `a04b6ef` exists in git log (verified).
- `go build ./...` exits 0 (verified).
- `go test -race -run "TestClient_PublicHolidays|TestValidateHolidays|TestNoInitOrGlobalState" -count=1 ./...` exits 0 (verified).

## Next Phase Readiness

- ENDPT-04 satisfied; TEST-01 satisfied for this endpoint.
- The `validateHolidays` + `ErrMalformedResponse` infrastructure is the load-bearing dependency for plan 03-05 (SchoolHolidays) — that plan can call `validateHolidays(holidays, "/SchoolHolidays")` with zero new code.
- The PL 2025 fixture is the reference fixture for downstream `Holiday.Range` / `Holiday.Days` / `Holiday.IsInRegion` helper tests in plan 03-06.
- No blockers for plan 03-05; no architectural unknowns; the analog (Countries, this plan) is in production code.

---
*Phase: 03-endpoints-helpers*
*Completed: 2026-05-27*
