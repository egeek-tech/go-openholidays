# Phase 3: Endpoints & Helpers - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in 03-CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-05-27
**Phase:** 03-endpoints-helpers
**Areas discussed:** Endpoint signatures & request structs, Holiday helpers (naming + IsInRegion), HTTP plumbing reuse, Golden fixture capture, Helper output type, Post-decode validation

---

## Endpoint signatures & request structs

### Question 1: Uniform Request structs vs REQUIREMENTS literal vs hybrid

| Option | Description | Selected |
|--------|-------------|----------|
| Uniform Request structs everywhere | All 4 new endpoints + retrofit Countries to take a Request struct; symmetry; future-proof; CL row required | ✓ |
| Mixed — follow REQUIREMENTS literal | Languages(ctx) and Subdivisions(ctx, country, lang) stay positional; PublicHolidays/SchoolHolidays use structs; no CL row | |
| Mixed but Subdivisions joins struct camp | Languages(ctx) bare, Subdivisions(ctx, SubdivisionsRequest); compromise | |

**User's choice:** Uniform Request structs everywhere (Recommended).
**Notes:** Retroactive change to Phase 2's `Countries(ctx)` signature accepted as part of this decision. CL-08 entry will be added to PROJECT.md by Phase 3 plan 1. Pre-1.0 backwards-compat policy explicitly permits this kind of surface change.

### Question 2: Country code field naming

| Option | Description | Selected |
|--------|-------------|----------|
| CountryIsoCode | Matches upstream wire param `countryIsoCode` and existing `Country.IsoCode` field; verbose | ✓ |
| CountryCode | Terser; matches ARCHITECTURE.md sample; diverges from existing IsoCode field name | |
| Country (just the noun) | Most terse; collides with the `Country` type name in mixed code | |

**User's choice:** CountryIsoCode (Recommended).
**Notes:** Implies `LanguageIsoCode` and `SubdivisionCode` for symmetry across all Request structs; `GroupCode` matches the upstream `groupCode` query param (no Iso prefix because the upstream doesn't use one).

### Question 3: Which optional upstream filters to expose in v0.1.0

| Option | Description | Selected |
|--------|-------------|----------|
| SchoolHolidaysRequest.GroupCode | Polish ferie cohort filter A/B/C/D | ✓ |
| Public/SchoolHolidaysRequest.SubdivisionCode | Server-side subdivision filter | ✓ |
| Public/SchoolHolidaysRequest.LanguageIsoCode | Server-side language filter (payload optimization) | ✓ |
| LanguagesRequest.LanguageIsoCode & CountriesRequest.LanguageIsoCode | Adds filter to /Countries (Phase 2 retrofit) and /Languages | ✓ |

**User's choice:** All four (multi-select).
**Notes:** This compounds with the uniform-Request-struct decision: Countries gets retrofitted to `Countries(ctx, CountriesRequest{LanguageIsoCode: ...})` in Phase 3 plan 1. CL-13 entry will be added to PROJECT.md.

---

## Holiday helpers — naming + IsInRegion semantics

### Question 4: Holiday.Name vs Holiday.NameFor (field collision)

| Option | Description | Selected |
|--------|-------------|----------|
| Holiday.NameFor(lang) | Symmetric with Country/Language/Subdivision (CL-05); CL-10 row required | ✓ |
| Holiday.Name(lang) | Follow ROADMAP literal; requires renaming the field to Names or LocalizedNames | |
| Holiday.LocalizedName(lang) | Sidesteps collision but introduces a third naming convention | |

**User's choice:** Holiday.NameFor(lang) (Recommended).
**Notes:** Same pattern as CL-05. Recorded as CL-10 in PROJECT.md.

### Question 5: IsInRegion matching rules

| Option | Description | Selected |
|--------|-------------|----------|
| Nationwide=true short-circuits to true | REQUIREMENTS HELP-02 literal | ✓ |
| Match Subdivisions[].Code case-insensitively | strings.EqualFold matching | ✓ |
| Empty input returns false (don't panic) | Defensive default | ✓ |
| Walk Subdivision.Children recursively | Hierarchical match — needs the subdivision tree | ✓ |

**User's choice:** All four (multi-select).
**Notes:** Hierarchical match requires a follow-up clarification because Holiday only carries `[]SubdivisionRef`, not the full tree — see Question 6.

### Question 6: How to expose hierarchical IsInRegion

| Option | Description | Selected |
|--------|-------------|----------|
| Defer hierarchy to v0.2; v0.1.0 flat-only | Cheapest; documents limitation | |
| Ship IsInRegionTree(code, subs []Subdivision) as separate helper | Caller supplies tree; pure helper | |
| Client.IsInRegion(ctx, holiday, code) fetches /Subdivisions on demand | Hidden I/O per call; ergonomic; cache-friendly in Phase 4 | ✓ |

**User's choice:** Client.IsInRegion(ctx, holiday, code) (chosen over the Recommended deferred option).
**Notes:** This adds a NEW public Client method beyond REQUIREMENTS HELP-02 literal text. Recorded as CL-09. Phase 3 ships BOTH the flat `Holiday.IsInRegion(code)` (HELP-02 literal) and the hierarchical `Client.IsInRegion(ctx, holiday, code)` (CL-09). Godoc documents the per-call HTTP cost on the Client variant until Phase 4's cache lands.

---

## HTTP plumbing reuse

### Question 7: Generic helper vs hybrid vs per-endpoint copy

| Option | Description | Selected |
|--------|-------------|----------|
| Extract generic doJSONGet[T] into request.go | DRY; per ARCHITECTURE.md §132; refactors countries.go | ✓ |
| Hybrid: small helpers + per-endpoint orchestration | ~30 lines per endpoint instead of ~90 | |
| Copy-paste pattern per endpoint | Self-contained files; grep-friendly; ~360 lines of duplication | |

**User's choice:** Extract generic doJSONGet[T] into request.go (Recommended).
**Notes:** Countries refactor lands in Phase 3 plan 1 alongside the Countries retrofit. `maxResponseBytes` and `apiErrorBodyCap` constants move from `countries.go` to `request.go`. The `buildAPIError` and `parseAPIMessage` helpers move with them.

---

## Golden fixture capture

### Question 8: Fixture refresh mechanism

| Option | Description | Selected |
|--------|-------------|----------|
| `go test -update ./...` regenerates from live | Idiomatic Go pattern | ✓ |
| Sidecar script scripts/refresh-fixtures.sh | Tests stay pure; script outside go test | |
| One-shot manual capture, no regeneration | Simplest; defers refresh story | |

**User's choice:** `go test -update ./...` (Recommended).

### Question 9: Fixture mechanics follow-ups

| Option | Description | Selected |
|--------|-------------|----------|
| Capture date as Go const per test file | Self-documenting; mirrors Phase 2 D-46 | ✓ |
| Gated by OPENHOLIDAYS_LIVE=1 + integration build tag | Defense in depth | ✓ |
| Capture during Phase 3 execution (2026-05-27) | Real data instead of guessed structure | ✓ |
| Fixture-shape sanity assertions in tests | Hard-coded asserts catch fixture-regression | ✓ |

**User's choice:** All four (multi-select).
**Notes:** The `-update` mechanism only runs when both gates are open. The plan executor refreshes fixtures once during Phase 3 with the documented invocation. Per-test sanity asserts lock the verified PL 2025 data (14 public, 7 school, 16 województwa, "Wigilia Bożego Narodzenia" entry).

---

## Helper output type + post-decode validation

### Question 10: Range() output type

| Option | Description | Selected |
|--------|-------------|----------|
| iter.Seq[time.Time] | Follow ROADMAP/REQUIREMENTS literal; direct stdlib composition | |
| iter.Seq[Date] | Symmetric with rest of Date-typed surface; CL row required | ✓ |

**User's choice:** iter.Seq[Date] (chosen over the Recommended ROADMAP literal).
**Notes:** Recorded as CL-11. Callers needing time.Time use the embedded field: `for d := range h.Range() { t := d.Time }`. Implementation walks via `d.AddDate(0, 0, 1)` and rebuilds Date via `NewDate(year, month, day)` to preserve UTC-midnight invariant.

### Question 11: Post-decode Holiday validation

| Option | Description | Selected |
|--------|-------------|----------|
| Validate each Holiday after decode; reject malformed | Pitfall JSON-4 mandate; new sentinel ErrMalformedResponse | ✓ |
| Trust upstream; document caller responsibility | Lighter API; loses JSON-4 defense | |
| Hybrid: reject zero dates only (re-use ErrEmptyResponse) | No new sentinel; allows inverted ranges through | |

**User's choice:** Validate each Holiday after decode (Recommended).
**Notes:** New sentinel `ErrMalformedResponse` added — recorded as CL-12. `validateHolidays` helper called by PublicHolidays and SchoolHolidays after Decode. Phase 1's `TestNoInitOrGlobalState` allowlist updates to include the new sentinel.

---

## Claude's Discretion

The following were not asked because they are derivable from already-locked architecture, conventions, or prior-phase decisions:

- File layout: one endpoint per `*.go` file at repo root (`languages.go`, `subdivisions.go`, `public_holidays.go`, `school_holidays.go`); shared plumbing in `request.go`; helpers may stay in `types.go` or move to `holiday.go` (planner's call).
- Test file layout follows Phase 2's per-file siblings.
- testify with `require` for preconditions, `assert` for verifications, one `TestXxx` per exported function, every case in `t.Run`.
- Error messages start with `"openholidays: "`.
- godoc comment on every exported symbol starting with the symbol name.
- English-only invariant with `testdata/` exception for real upstream Polish strings.
- Validator wiring uses free functions (ARCHITECTURE.md Pattern 5) — no `Request.Validate()` methods.
- Empty-string optional Request fields are omitted from the outbound query string.

---

## Deferred Ideas

Captured in 03-CONTEXT.md `<deferred>` section. Highlights:

- Hierarchical IsInRegion variants beyond the Client method (tree-builder helpers, pure-data IsInRegionTree).
- `PublicHolidaysByDate`, `SchoolHolidaysByDate`, `Statistics/*` upstream endpoints (exist; not in REQUIREMENTS).
- `Holiday.CommentFor(lang)` helper (symmetric with NameFor; HELP-01 covers Name only).
- Single-flight on `/Subdivisions` for `Client.IsInRegion` cold-start fan-out.
- Defensive deep-copy of returned `[]Holiday` if Phase 4 cache moves to typed storage.
- Exported `ValidateHoliday` if a real consumer asks for it.
