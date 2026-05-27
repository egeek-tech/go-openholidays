---
phase: 03-endpoints-helpers
plan: 05
subsystem: api
tags: [school-holidays, endpoint, fixtures, testify, golang, polish-ferie]

# Dependency graph
requires:
  - phase: 03-endpoints-helpers/03-01
    provides: "doJSONGet[T] generic HTTP-and-decode pipeline in request.go"
  - phase: 03-endpoints-helpers/03-04
    provides: "validateHolidays(hs, path) post-decode helper + ErrMalformedResponse sentinel (reused verbatim — no new helpers added by this plan)"
  - phase: 01-foundation
    provides: "Date type, Holiday struct, HolidayType constants, errors.go sentinel surface, validate.go validators"
provides:
  - "Client.SchoolHolidays(ctx, SchoolHolidaysRequest) ([]Holiday, error) endpoint method — final missing endpoint per ENDPT-05"
  - "SchoolHolidaysRequest type with all six upstream-supported filters (CountryIsoCode + ValidFrom + ValidTo + LanguageIsoCode + SubdivisionCode + GroupCode per D-54/CL-13)"
  - "testdata/school_holidays_pl_2025.json: 7 live-captured PL 2025 school periods (D-70 sanity assertion); includes one Ferie zimowe entry with PL-SL subdivision (drives Plan 6 Holiday.IsInRegion test)"
  - "TestClient_SchoolHolidays — 11 subtests covering happy path + 4 validation paths + 4 transport/decode error paths + 2 query-contract pass-through paths + ErrMalformedResponse"
affects:
  - "03-06 (holiday helpers / Client.IsInRegion): the Ferie zimowe entry with PL-SL is the canonical fixture seam for HELP-02 hierarchical lookup tests"
  - "phase 04 (cache + decoders): cache layer transparently wraps SchoolHolidays; strict decoder may surface Quality field drift"
  - "phase 05 (cmd/ohcli): consumes SchoolHolidays(ctx, SchoolHolidaysRequest{...}) for `ohcli school PL 2025 --region PL-SL`"

# Tech tracking
tech-stack:
  added: []  # no new runtime deps; zero-runtime-dep policy preserved
  patterns:
    - "Endpoint scoped to (Request type + endpoint method + reused post-decode validate call) per D-64 — no new helpers needed because validateHolidays from Plan 03-04 covers the Holiday shape verbatim"
    - "Optional shape-tolerant pass-through fields (SubdivisionCode, GroupCode) — guarded by if v != \"\" {} before q.Set, no client-side validator per D-56"
    - "URL-builder contract assertions inside httptest handler (r.URL.Query().Get(...)) — catches mis-spelled query-param keys that fixture-only tests miss"
    - "Empty-array response handling: handler responds with `[]` for query-contract subtests so validateHolidays accepts the empty slice cleanly (no entries to iterate)"
    - "0001-01-01 round-trip for the ErrMalformedResponse subtest: Date.UnmarshalJSON parses successfully and the resulting Date.IsZero() == true, exercising validateHolidays' zero-StartDate predicate without bypassing the JSON layer"

key-files:
  created:
    - "school_holidays.go (175 lines — SchoolHolidaysRequest struct + Client.SchoolHolidays method, mirrors public_holidays.go with one extra optional GroupCode field)"
    - "school_holidays_test.go (339 lines — TestClient_SchoolHolidays with 11 t.Run subtests + schoolHolidaysPL2025FixtureCapturedAt const)"
    - "testdata/school_holidays_pl_2025.json (377 lines pretty-printed UTF-8 — 7 entries from live API on 2026-05-27)"
  modified: []  # no shared files touched — validateHolidays + ErrMalformedResponse + doJSONGet all reused from prior plans

key-decisions:
  - "Empty-array body for query-contract subtests (GroupCode/SubdivisionCode pass-through): an empty []Holiday is structurally valid AND validateHolidays accepts it (the for-range over an empty slice exits immediately). This keeps the query-contract subtests narrow — they assert exactly one thing (the URL query parameter reached the wire) without coupling to a larger fixture body that would force fixture-shape co-evolution. The plan's instruction explicitly says \"respond with an empty array (so validateHolidays returns nil — empty slice is valid)\" — this matches the documented behavior of validateHolidays in request.go (Plan 03-04 SUMMARY)."
  - "ErrMalformedResponse subtest uses startDate \"0001-01-01\" (verbatim from the plan) — Date.UnmarshalJSON parses this successfully via time.Parse(dateLayout, \"0001-01-01\") (the value is in-range for Go's time package), and the resulting time.Time IS the zero value (the time.Time documentation pins the zero value as January 1, year 1, UTC). validateHolidays' h.StartDate.IsZero() then fires, wrapping ErrMalformedResponse. This is materially the same exercise as Plan 03-04's EndDate-before-StartDate subtest but pins the StartDate.IsZero() predicate that the EndDate-before-StartDate case never reaches."

patterns-established:
  - "Pattern: optional-field pass-through with no validator — per D-56 the only client-side guard is the empty-string omission (if v != \"\" {})  before q.Set; canonicalization and shape allowlists live on the upstream"
  - "Pattern: live-capture + sanity-check protocol — capture, run a quick Python json.load to verify count/key invariants, abort and surface the upstream issue if any sanity fails (no sanity failure occurred this run)"

requirements-completed:
  - ENDPT-05
  - TEST-01

# Metrics
duration: ~4min
completed: 2026-05-27
---

# Phase 03 Plan 05: SchoolHolidays Endpoint Summary

**SchoolHolidays endpoint shipped — sibling of PublicHolidays with one extra optional `GroupCode` field for Polish ferie cohort filtering; reuses validateHolidays and ErrMalformedResponse from Plan 03-04 verbatim with zero new helpers. 11-subtest coverage on a live PL 2025 fixture; closes the final missing endpoint of Phase 3.**

## Performance

- **Duration:** ~4 min wall-clock (start 2026-05-27T18:58:59Z; end 2026-05-27T19:03:09Z, ~250 seconds total).
- **Tasks:** 2 (both atomic).
- **Files created:** 3 (school_holidays.go, school_holidays_test.go, testdata/school_holidays_pl_2025.json).
- **Files modified:** 0 — no shared infrastructure touched (validateHolidays + ErrMalformedResponse + doJSONGet are already shipped by Plans 03-01 and 03-04).

## Accomplishments

- Shipped `Client.SchoolHolidays(ctx, SchoolHolidaysRequest) ([]Holiday, error)` — the final missing endpoint in Phase 3. Method body mirrors PublicHolidays line-for-line with exactly one structural addition: the `if req.GroupCode != "" { q.Set("groupCode", req.GroupCode) }` block (no validator because GroupCode is shape-tolerant per D-56).
- `SchoolHolidaysRequest` carries all six upstream-supported filters (CountryIsoCode, ValidFrom, ValidTo, LanguageIsoCode, SubdivisionCode, GroupCode) per D-54 / CL-13. Field order matches the plan's specified order.
- Captured a 7-entry PL 2025 school-holidays fixture from the live upstream on 2026-05-27 and locked the D-70 sanity assertions (exactly 7 entries; at least one "Ferie zimowe" entry carries the PL-SL subdivision — the seam Plan 6's `Holiday.IsInRegion` test depends on).
- No shared-infrastructure churn: validateHolidays, ErrMalformedResponse, doJSONGet, the input validators, and the testify-based test harness were all reused verbatim from Plans 03-01 and 03-04. The only new identifiers introduced by this plan are `SchoolHolidaysRequest`, `Client.SchoolHolidays`, `TestClient_SchoolHolidays`, and `schoolHolidaysPL2025FixtureCapturedAt`.
- All four threat-model mitigations (`T-3-InputVal-Country`, `T-3-InputVal-DateRange`, `T-3-InputVal-Lang`, `T-3-DoS-EmptyQueryParam`) verified by the new test subtests; the three inherited mitigations (`T-3-Tampering-MalformedHoliday`, `T-3-DoS-OverSize`, `T-3-InfoDisc-APIErrorBody`) are exercised by Plan 03-04's existing coverage of the shared helpers.

## Task Commits

Each task was committed atomically on branch `worktree-agent-a48c844612d6fe5a2`:

1. **Task 1: SchoolHolidays endpoint method + PL 2025 live-captured fixture** — `ccd2681` (feat). Captures `testdata/school_holidays_pl_2025.json` from `https://openholidaysapi.org/SchoolHolidays?countryIsoCode=PL&validFrom=2025-01-01&validTo=2025-12-31&languageIsoCode=PL`, then ships `school_holidays.go` (175 lines). `go build ./...` exits 0 immediately after.
2. **Task 2: TestClient_SchoolHolidays (11 subtests)** — `150faa3` (test). `go test -race -run TestClient_SchoolHolidays -count=1 ./...` exits 0; full package suite `go test -race -count=1 ./...` also exits 0 (1.873s).

_Note on TDD discipline: Per the project's hand-rolled TDD pattern established by Plan 03-04 (plan declares `tdd="true"` per task but the verify gates pin compile-then-functional rather than failing-test-first), Tasks 1 and 2 produced production code whose contract is exercised by Task 2's test file. The validation gates are: (a) `go build ./...` after Task 1 (compile contract for the endpoint + fixture), (b) `go test -race -run TestClient_SchoolHolidays -count=1 ./...` after Task 2 (11-subtest functional contract). No RED-then-GREEN split commits were produced because the plan's verify gates do not require them; this matches the pattern Plan 03-04 set._

## Subtest Names (11)

For downstream reference and the plan `<output>` requirement:

1. `happy path PL 2025 returns 7 periods incl. Ferie zimowe with PL-SL`
2. `validation error: empty CountryIsoCode wraps ErrInvalidCountry`
3. `validation error: from > to wraps ErrInvalidDateRange`
4. `validation error: invalid LanguageIsoCode wraps ErrInvalidLanguage`
5. `4xx returns *APIError with detail Message`
6. `5xx with title fallback`
7. `malformed JSON wraps decode error (no sentinel)`
8. `ctx cancel returns context.Canceled`
9. `optional GroupCode reaches the wire when non-empty`
10. `optional SubdivisionCode reaches the wire when non-empty`
11. `malformed holiday (zero StartDate) wraps ErrMalformedResponse`

## Sanity-Assertion Snapshot

For downstream tests (especially Plan 03-06 helpers) to copy verbatim:

- `len(holidays) == 7` for PL 2025 (verified live 2026-05-27).
- **Fixture capture date:** 2026-05-27 (recorded in `schoolHolidaysPL2025FixtureCapturedAt` and surfaced in fixture-missing failure messages).
- **Ferie zimowe seam for Plan 6:** at least one entry has `NameFor("pl")` matching `"Ferie zimowe"` (case-insensitive via `strings.EqualFold`) AND its `Subdivisions` slice contains an entry whose `Code` equals `"PL-SL"` (case-insensitive). Plan 6's `Holiday.IsInRegion("PL-SL")` test runs against this exact fixture entry.
- **Live PL upstream emits the language tag in uppercase:** the fixture entries have `{"language":"PL","text":"Ferie zimowe"}`. The library's `NameFor("pl")` lookup is case-insensitive (`pickLocalized` uses `strings.EqualFold`), so the lowercase ISO 639-1 argument matches the uppercase wire payload correctly.

### Ferie zimowe entry with PL-SL — exact details (for Plan 6)

For downstream tests that need to bind to the exact entry the seam was built around:

- **Polish name (literal upstream bytes):** `"Ferie zimowe"` (capitalized F; no diacritics in this string).
- **ID:** `503317eb-0375-41a8-bcb7-501800dc4098`.
- **StartDate / EndDate:** `2025-01-20` / `2025-02-02` (14-day inclusive span — `Holiday.Days()` will return 14 here in Plan 6).
- **Subdivisions on that entry:** `[PL-SL, PL-LU, PL-WP, PL-MA, PL-KP]` (so `Holiday.IsInRegion("PL-SL")` returns true; `Holiday.IsInRegion("PL-MZ")` returns false — convenient binary control for the Plan 6 test).
- **Other Ferie zimowe entries:** the fixture contains three more "Ferie zimowe" entries representing the other three Polish ferie cohorts (B/C/D windows). Their subdivision sets are disjoint from PL-SL; testing `Holiday.IsInRegion("PL-SL")` on those other entries returns false. This gives Plan 6 a binary positive/negative pair without needing a synthetic fixture.

### Other 6 entries in the fixture (Polish name → subdivision set length)

For Plan 6 / 7 to navigate the fixture without re-reading the JSON:

- `Ferie zimowe` × 4 (cohorts; 5/2/4/5 subdivisions respectively)
- `Wiosenna przerwa świąteczna` × 1 (16 subdivisions — nationwide-but-not-marked-Nationwide; carries all 16 województwa codes)
- `Ferie letnie` × 1 (16 subdivisions — same shape)
- `Zimowa przerwa świąteczna` × 1 (16 subdivisions — same shape)

## Decisions Made

- **Empty-array bodies for query-contract subtests.** The plan said "respond with an empty array (so validateHolidays returns nil — empty slice is valid)" for the GroupCode and SubdivisionCode pass-through subtests. Implemented exactly as instructed — the handler writes `[]` and validateHolidays' for-range exits immediately. This keeps the query-contract assertion crisp: the only thing under test is whether the URL query parameter reached the wire, with no fixture-shape coupling.
- **The ErrMalformedResponse subtest uses `"0001-01-01"` as the malformed StartDate.** The plan specified this verbatim. `Date.UnmarshalJSON` parses the literal successfully (the time.Parse succeeds and the resulting time.Time IS the documented zero value, per the Go time package's zero-value definition: `time.Time{}` equals `time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)`). `validateHolidays`' `h.StartDate.IsZero()` predicate fires, wrapping `ErrMalformedResponse`. This is materially different from Plan 03-04's EndDate-before-StartDate subtest because it pins the zero-StartDate predicate specifically — both predicates are now covered by package tests (the EndDate-before-StartDate one via Plan 03-04's coverage, the zero-StartDate one via this plan).
- **Test happy-path matches the fixture's Polish name case-insensitively.** The plan explicitly says "with case-insensitive comparison ok" so the test uses `strings.EqualFold(plName, "ferie zimowe")` — this is robust against both the actual upstream casing ("Ferie zimowe" with capital F) and a hypothetical future upstream switch to all-lowercase. The seam Plan 6 builds against is the case-insensitive predicate, not the literal byte form.

## Deviations from Plan

None — plan executed exactly as written. The plan body, the inherited Plan 03-04 infrastructure, and the live API's actual shape were all internally consistent and matched the documented expectations. No Rule 1/2/3 auto-fixes were needed. CLAUDE.md project rules (English-only, verify-or-ask, testify+t.Run+one-test-per-prod-function, error prefix `openholidays:`, godoc on every exported symbol) all honored.

## Threat Mitigations Verified

Per the plan's `<threat_model>` register, this plan ships / exercises the following mitigations:

- **T-3-InputVal-Country** → `validateCountry` called before HTTP (W-01 hardening inherited from Phase 2). Subtest 2 (`empty CountryIsoCode wraps ErrInvalidCountry`) exercises the empty-string path; the bytes-shape path is exercised by Plan 03-04's coverage of the same shared validator.
- **T-3-InputVal-DateRange** → `validateDateRange` called before HTTP; rejects `from > to`. Subtest 3 (`from > to wraps ErrInvalidDateRange`) exercises this. The 3-year window-too-large path is exercised by Plan 03-04's same-validator coverage (the predicate path through validateDateRange is identical regardless of caller).
- **T-3-InputVal-Lang** → `validateLanguage` called when `LanguageIsoCode` is non-empty. Subtest 4 (`invalid LanguageIsoCode wraps ErrInvalidLanguage`) exercises this.
- **T-3-DoS-EmptyQueryParam** → Empty-string guards confirmed for `LanguageIsoCode`, `SubdivisionCode`, AND `GroupCode` (the third one new to this plan). The empty `SubdivisionCode`/`GroupCode` paths are implicit in subtests 1-8 (none of which pass either field non-empty); the non-empty pass-through paths are exercised by subtests 9 and 10 (`optional GroupCode reaches the wire when non-empty` and `optional SubdivisionCode reaches the wire when non-empty`).
- **T-3-Tampering-MalformedHoliday** → `validateHolidays` runs unconditionally on every endpoint response. Subtest 11 (`malformed holiday (zero StartDate) wraps ErrMalformedResponse`) exercises the zero-StartDate predicate; the other two predicates (zero EndDate, EndDate-before-StartDate) are covered by Plan 03-04's `TestValidateHolidays`.
- **T-3-DoS-OverSize** → 10 MiB cap inherited from `doJSONGet` (verified by the existing `TestClient_Countries` oversize subtest in countries_test.go and `TestDoJSONGet` in request_test.go).
- **T-3-InfoDisc-APIErrorBody** → 4 KiB `APIError.Body` cap inherited from `buildAPIError` (verified by `TestClient_Countries`'s 4 KiB cap subtest).

## D-51..D-66 Compliance Audit

The plan `<output>` block requested a recording of any deviation from D-51..D-66. None observed:

- **D-51:** `SchoolHolidays(ctx, SchoolHolidaysRequest)` follows the uniform `(ctx, Request)` shape (verified via the `func (c *Client) SchoolHolidays(ctx context.Context, req SchoolHolidaysRequest)` signature).
- **D-53:** Field names `CountryIsoCode`, `LanguageIsoCode`, `SubdivisionCode`, `GroupCode` match upstream wire param names case-insensitively (`countryIsoCode`, `languageIsoCode`, `subdivisionCode`, `groupCode`).
- **D-54:** `SchoolHolidaysRequest` exposes every upstream-supported filter (six fields total; one more than `PublicHolidaysRequest`, matching `groupCode`).
- **D-55:** Empty optional fields omitted from outbound query (verified by subtest 9 reading `r.URL.Query().Get("subdivisionCode")` as empty when not passed, and subtest 10 the same for `groupCode`; implicit in every other subtest that doesn't set these fields).
- **D-56:** Validator wiring matches the matrix: `validateCountry` on required `CountryIsoCode`; `validateDateRange` on `(ValidFrom, ValidTo)`; `validateLanguage` only when `LanguageIsoCode` non-empty; `SubdivisionCode` and `GroupCode` are shape-tolerant pass-throughs with no client-side validator.
- **D-64:** `school_holidays.go` is scoped to the SchoolHolidaysRequest type + SchoolHolidays endpoint method + the endpoint-specific post-decode `validateHolidays` call (reused from Plan 03-04 — no new helper here). HTTP pipeline lives only in request.go's doJSONGet. (Method body is ~30 lines per the soft cap.)
- **D-69:** `schoolHolidaysPL2025FixtureCapturedAt` const recorded as `"2026-05-27"` and included in fixture-drift failure messages (`fixture missing — re-capture from live API per Plan 03-05 Task 1 (captured %s)` and `fixture captured %s — re-capture if upstream shape drifted`).
- **D-70:** Sanity assertion `len(holidays) == 7` + "at least one Ferie zimowe with PL-SL" present in the happy-path subtest.
- **D-71:** Fixture path is `testdata/school_holidays_pl_2025.json` (root-level testdata/ convention).

## Issues Encountered

None — all sanity checks on the live capture passed first try; the `school_holidays.go` file compiled clean on the first build; all 11 subtests passed on the first run with `-race`; full package suite stayed green continuously.

## User Setup Required

None — no external service configuration required. The live fixture was captured by the executor at plan time; consumers of the library do not need to re-capture unless the upstream schema drifts.

## Next Phase Readiness

- **ENDPT-05 satisfied** (last missing endpoint in Phase 3).
- **TEST-01 partially satisfied** for this endpoint (the per-endpoint TEST-01 row is now green for `/SchoolHolidays`; the phase-level TEST-01 across all five endpoints is now fully green per the running ledger across Plans 03-01..03-05).
- **Plan 03-06 (helpers) unblocked.** The `Ferie zimowe` + PL-SL fixture seam is the canonical input for `Holiday.IsInRegion("PL-SL")`, `Holiday.Days()` (14 for the cohort A entry), and `Holiday.Range()` (14 yielded Dates from 2025-01-20 to 2025-02-02).
- **Plan 03-07 (Client.IsInRegion)** has the country-derivation surface it needs (`PL-SL` → `PL` via `splitCountryFromSubdivision` in the planned client_isinregion.go) — the fixture's subdivision codes all follow the `PL-XX` shape so the prefix split is well-defined.
- **No blockers.** No architectural unknowns. Zero new sentinels, zero new helpers, zero new test-only deps.

## Self-Check: PASSED

- File `school_holidays.go` — `FOUND` at repo root (175 lines).
- File `school_holidays_test.go` — `FOUND` at repo root (339 lines).
- File `testdata/school_holidays_pl_2025.json` — `FOUND` (7 entries; `Ferie zimowe` with PL-SL present).
- Commit `ccd2681` — `FOUND` in git log (`feat(03-05): add SchoolHolidays endpoint + PL 2025 live-captured fixture`).
- Commit `150faa3` — `FOUND` in git log (`test(03-05): add TestClient_SchoolHolidays with 11 subtests`).
- `go build ./...` exits 0 (verified).
- `go vet ./...` exits 0 (verified).
- `go test -race -run TestClient_SchoolHolidays -count=1 ./...` exits 0 (verified).
- `go test -race -count=1 ./...` exits 0 (verified — full package, 1.873s).
- `grep -c "type SchoolHolidaysRequest" school_holidays.go` returns 1.
- `grep -c "doJSONGet\[\[\]Holiday\]" school_holidays.go` returns 1.
- `grep -c "validateHolidays" school_holidays.go` returns 2 (godoc reference + call site).
- `grep -c "GroupCode" school_holidays.go` returns 11 (≥ 2 minimum).
- `grep -c 't.Run(' school_holidays_test.go` returns 11.

---
*Phase: 03-endpoints-helpers*
*Plan: 05*
*Completed: 2026-05-27*
