# Phase 1: Foundation - Research

**Researched:** 2026-05-27
**Domain:** Zero-dependency Go package — domain types, custom `Date`, sentinel + `*APIError` types, client-side validators, `go.mod` at Go 1.23
**Confidence:** HIGH

## Summary

Phase 1 establishes the entire public type contract for `go-openholidays` in a single Go package (`openholidays`) with zero runtime dependencies (testify is test-only). Every following phase consumes types, errors, and validators defined here; the contract must be stable before any transport code lands. The work breaks into four crisply scoped files (`types.go`, `date.go`, `errors.go`, `validate.go`) plus `version.go`, `doc.go`, and `go.mod`/`LICENSE` at the repo root. Test files mirror production files 1:1 per Gold Rule 3 (one `TestXxx` per exported production function, `t.Run` per case).

Three substantive risks dominate Phase 1 and are addressed by verified patterns below: (1) the upstream `Holiday.type` enum is six values, not four — the locked TYPES-04 set must expand or the type must be redesigned (see Section "Upstream API Schema — Verified" — this is the single most important finding); (2) the `Date.UnmarshalJSON` shape must be exact about `null`/empty/quote-stripping/UTC-normalization to satisfy ROADMAP success criterion #1 (template provided below, derived from Pitfall JSON-3 and the verified `time.Parse` semantics); (3) the `*APIError.Is(target)` "wildcard on zero StatusCode" pattern is a project convention, not a stdlib idiom — stdlib `*fs.PathError` / `*net.DNSError` rely on `Unwrap()`, but D-16 explicitly forbids `Unwrap()` on `APIError`. The pattern is sound and used by several HTTP SDKs but should be documented as a deliberate choice.

**Primary recommendation:** Lock the type-system surface first (per file: `errors.go` → `date.go` → `types.go` → `validate.go`) with table-driven testify tests written alongside each, then add `FuzzDateUnmarshal` and `version.go`/`doc.go` last. Defer the TYPES-04 enum scope decision (4 vs. 6 values) to the planner with a clear recommendation: **ship all six upstream values**.

## User Constraints (from CONTEXT.md)

### Locked Decisions

**Module path and packaging**
- **D-01:** `go.mod` declares `module github.com/egeek-tech/go-openholidays`. Resolves PROJECT.md REL-04 for the entire library lifecycle.
- **D-02:** Every `.go` file declares `package openholidays`. Repo `go-openholidays` → package `openholidays`, matching go-github / stripe-go convention.
- **D-03:** A dedicated `version.go` holds `const Version = "0.1.0"`. Single source of truth for Phase 2's `User-Agent: go-openholidays/<version>` and Phase 5's CLI `--version` flag. `-ldflags '-X github.com/egeek-tech/go-openholidays.Version=...'` injection works out of the box.
- **D-04:** Phase 1 ships the MIT `LICENSE` at the repo root from the first commit. README and other distribution artifacts remain in Phase 5.

**Date type**
- **D-05:** `type Date struct { time.Time }` — wrapper struct that embeds `time.Time` so all stdlib methods are promoted. Every `Date` is normalized to **UTC midnight** at construction time (Pitfall TZ-1 mitigation).
- **D-06:** `UnmarshalJSON(b []byte) error` returns errors for both `null` and empty-string inputs (locks ROADMAP success criterion #1). The error is built with `fmt.Errorf("...: %w", errEmptyDate)` against an **unexported** `errEmptyDate` sentinel — kept out of the public sentinel list so the locked 5-sentinel surface is not expanded.
- **D-07:** `MarshalJSON()` always emits `"YYYY-MM-DD"`. Zero `Date{}` round-trips to `"0001-01-01"` — symmetric with `time.Time.MarshalJSON` semantics. Callers detect missing dates via `Date.IsZero()`.
- **D-08:** `Date.String() string` overrides the embedded `time.Time.String()` to return `Format("2006-01-02")`. Same shape as JSON, friendly for the CLI table output (Phase 5).
- **D-09:** `Date` defines its own `Equal`, `Before`, `After`, `Compare` methods that internally normalize both operands to UTC midnight before delegating to `time.Time`. These shadow the embedded methods so callers cannot accidentally compare a non-UTC-constructed Date.
- **D-10:** `func (d Date) DaysUntil(other Date) int` ships in Phase 1 — DST-correct inclusive day count (Pitfall TZ-2 mitigation). Phase 3's `Holiday.Days()` and `Holiday.Range()` call this.
- **D-11:** Constructors ship in Phase 1: `NewDate(year int, month time.Month, day int) Date` (always UTC midnight) and `ParseDate(s string) (Date, error)` (delegates to `UnmarshalJSON` semantics).
- **D-12:** `FuzzDateUnmarshal` ships in Phase 1 alongside the unit tests (overrides ROADMAP placement of fuzz in Phase 5). Pitfall JSON-3 mandates a fuzz target for every custom unmarshaler; not waiting four phases to surface regressions.

**Errors**
- **D-13:** Exported sentinels in Phase 1: `ErrInvalidCountry`, `ErrInvalidLanguage`, `ErrDateRangeTooLarge`, `ErrEmptyResponse`, **`ErrInvalidDateRange`** (new — for `validFrom > validTo`). This expands ROADMAP success criterion #2 from 4 to 5 sentinels — see Scope clarifications below.
- **D-14:** `*APIError` carries `{StatusCode int, Path string, Body []byte, Message string}`. `Message` is best-effort parsed from upstream JSON shape (`{"error": "..."}` / `{"detail": "..."}` / `{"title": "..."}`) by Phase 2's endpoint methods; empty string when unparseable. Brief minimum was 3 fields; `Message` is additive.
- **D-15:** `*APIError` implements `Is(target error) bool` so `errors.Is(err, &openholidays.APIError{StatusCode: 404})` matches by status. If `target.StatusCode == 0`, it matches any `*APIError` (i.e., "was this an API error?"). Idiomatic per `*os.PathError` and `*net.OpError`.
- **D-16:** `*APIError` does **not** implement `Unwrap()` — it is a leaf error type. Phase 2 endpoint methods return either a transport-level error (wrapped with `%w`) OR a freshly-constructed `*APIError`, never both.
- **D-17:** `APIError.Body` is capped at **4 KiB** when populated (Phase 2). 10 MiB cap applies only to successful response decode (Pitfall HTTP-4); error envelopes are bounded smaller.
- **D-18:** `(e *APIError) Error() string` returns `"openholidays: api error <status> at <path>: <message>"`. When `Message` is empty, the suffix is omitted, producing `"openholidays: api error <status> at <path>"`.
- **D-19:** Phase 1 ships the `APIError` type, `Error()`, `Is()`, and tests for those methods. Construction (reading `resp.Body`, parsing `Message`) lands in Phase 2 alongside the first endpoint, since Phase 1 has no `*http.Response` to draw from. Tests in Phase 1 construct `APIError` literals manually to verify `errors.As` round-trips per ROADMAP criterion #3.

**Validators**
- **D-20:** `validateCountry(code string) (string, error)` — accepts any case, **canonicalizes to uppercase** via `strings.ToUpper`, then verifies the result is exactly 2 ASCII letters in `[A-Z]`. Returns the canonicalized form for the caller to send on the wire. Deviates from VALID-01's literal "2 uppercase ASCII letters" — see Scope clarifications.
- **D-21:** `validateLanguage(code string) (string, error)` — accepts any case, **canonicalizes to lowercase** via `strings.ToLower`, verifies the result is exactly 2 ASCII letters in `[a-z]`. No allowlist — shape-only check.
- **D-22:** `validateDateRange(from, to Date) error` — returns `ErrInvalidDateRange` (wrapped with %w + value context) when `from.After(to)`; returns `ErrDateRangeTooLarge` when `to.Before(from.AddDate(3, 0, 1))` is false (window exceeds 3 calendar years inclusive). Uses calendar arithmetic via `AddDate`, not duration math, so leap years are handled correctly.
- **D-23:** Validator errors wrap their sentinel with %w and include the offending value: `fmt.Errorf("%w: %q", ErrInvalidCountry, code)`. Caller error messages look like `openholidays: invalid country code: "pl"`. Country/language/date values are not secrets.

**Scope clarifications (deviations from ROADMAP literal wording)**
- **CL-01:** 5 sentinels not 4 (`ErrInvalidDateRange` added). Must be recorded in PROJECT.md `Key Decisions` before Phase 1 closes.
- **CL-02:** Case-insensitive country validator (canonicalizes to uppercase). Must be recorded in PROJECT.md `Key Decisions`.
- **CL-03:** `FuzzDateUnmarshal` ships in Phase 1 not Phase 5 per Pitfall JSON-3 mandate.

### Claude's Discretion

- File layout per ARCHITECTURE.md: `types.go`, `date.go`, `errors.go`, `validate.go`, `doc.go`, `version.go`, plus `*_test.go` siblings.
- `package openholidays` for production files; `package openholidays` for in-package tests (access to unexported helpers); `package openholidays_test` for `example_test.go` (external-consumer view, deferred to Phase 5).
- testify v1 (assert + require) added under `require` in `go.mod`. Verify `go mod why github.com/stretchr/testify` reports only test imports.
- Every exported symbol gets a godoc comment starting with the symbol name (Rule from PROJECT.md constraints).
- All error message strings start with `"openholidays: "` (Go stdlib convention).

### Deferred Ideas (OUT OF SCOPE)

- **Language allowlist** (closed set of ISO 639-1 codes) — D-21 ships shape-only validation; allowlist enforcement deferred (likely never).
- **`*APIError.Unwrap()` returning an internal cause** — can be added in v0.2 non-breakingly if a real pattern surfaces.
- **Typed validation error struct** — `*ValidationError{Field, Value, Reason}` was considered; sentinels + `%w` are the idiomatic answer for v0.1.0.
- **Exported `NewAPIError` constructor** — rejected as premature.
- **`Date.MarshalText`/`UnmarshalText`** for `database/sql` interop — flag for v0.2 if a consumer needs it.

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| TYPES-01 | `Holiday` struct with `StartDate`, `EndDate`, `Type`, `Name`, `RegionalScope`, `TemporalScope`, `Nationwide`, `Subdivisions`, `Comment`, `Quality` fields; all decoded from upstream JSON shape. | Section "Upstream API Schema — Verified" lists exact field names; Section "Standard Stack" / Pattern "Custom Date Type" |
| TYPES-02 | Custom `Date` type (`type Date struct { time.Time }`) with `UnmarshalJSON`/`MarshalJSON` round-tripping `YYYY-MM-DD`. | Section "Date Type — Canonical Implementation"; verified `time.Parse` returns UTC; D-05/D-06/D-07 locked |
| TYPES-03 | `LocalizedText{Language, Text}` and `SubdivisionRef{Code, ShortName}` companion types. | Verified upstream schema: both fields required, both `minLength: 1` |
| TYPES-04 | Typed enum for `Holiday.Type` (`Public`, `School`, `Bank`, `Observance`) — a typed string with package-level constants. | **DECISION REQUIRED** — verified upstream enum is `Public`, `Bank`, `Optional`, `School`, `BackToSchool`, `EndOfLessons` — neither set matches the requirement text. See Section "Upstream API Schema — Verified" and "Open Questions" Q1 |
| TYPES-05 | `Country`, `Language`, `Subdivision` reference types with `Name(lang string) string` accessor that falls back to first entry if requested language missing. | Phase 1 ships the **types only**; the `Name(lang)` accessor is a `HELP-01`-style helper traditionally placed with `Holiday` helpers (Phase 3). Recommend: ship `Name(lang)` accessors for `Country`/`Language`/`Subdivision` in Phase 1 since they're independent of `Holiday`; defer `Holiday.Name(lang)` to Phase 3 |
| ERR-01 | Sentinel errors exposed. | D-13 locks 5 sentinels (one more than REQUIREMENTS.md literal text). Section "Sentinel Errors — Canonical Implementation" |
| ERR-02 | `*APIError{StatusCode, Path, Body}` implements `error`; `errors.As` retrieves it from wrapped errors. | Section "*APIError — Canonical Implementation"; D-14 adds `Message string` field (additive) |
| ERR-03 | All transport-level errors wrap underlying cause with `%w`; `errors.Is(err, ErrSentinel)` works through wrapper. | Verified `fmt.Errorf("%w: %q", sentinel, value)` works; Phase 1 ships sentinels only, transport wrapping lands Phase 2 |
| ERR-04 | Error messages never include credentials or full response bodies; raw body lives only in `APIError.Body`. | Validator messages include code/date values only (D-23); `APIError.Body` is the only field that holds raw body (cap 4 KiB per D-17); construction Phase 2 |
| VALID-01 | Country code: 2 uppercase ASCII letters; non-empty. | D-20 widens to case-insensitive with canonicalization; CL-02 records as deviation. Section "Validators — Canonical Implementation" |
| VALID-02 | `validFrom <= validTo` enforced; else error. | D-22 introduces `ErrInvalidDateRange` (CL-01 records 5th sentinel) |
| VALID-03 | Date window > 3 years rejected with `ErrDateRangeTooLarge`. | D-22 uses calendar arithmetic via `AddDate(3, 0, 1)` to avoid leap-year off-by-one; Pitfall TZ-2 mitigation |
| VALID-04 | Language code validated as ISO 639-1 2-letter code; else `ErrInvalidLanguage`. | D-21 ships shape-only check (no allowlist); CL-02-style deviation also applies (case-insensitive) — note in Key Decisions |
| CLIENT-10 | No `init()` side effects, no global mutable state. | Section "CLIENT-10 — Mechanical Verification" — grep-based predicate + a test asserting no init() and no package-level `var` other than sentinels and `Version` |

## Project Constraints (from CLAUDE.md and CONVENTIONS.md)

These directives have the same authority as locked decisions:

1. **Gold Rule 1 — Everything in English.** All identifiers, comments, godoc, test names, commit messages, fixtures. Exception: `testdata/` may contain non-English when reflecting real upstream responses (irrelevant in Phase 1, which has no `testdata/`).
2. **Gold Rule 2 — Never guess; verify or ask.** Library claims must be verified; any "I think / probably / should be" in a draft is a stop-and-verify signal.
3. **Gold Rule 3 — Test conventions.**
   - `testify/assert` and `testify/require` are the only assertion libraries.
   - `testify` may only appear in `*_test.go` imports — `go mod why github.com/stretchr/testify` must show test-only imports.
   - Exactly one `TestFunction` per exported production function. `holidays_test.go` has exactly one `func TestX_Y(t *testing.T)` per exported `X.Y`.
   - Every test case lives inside `t.Run(name, func(t *testing.T){...})`. No top-level assertions in the outer body.
   - `require` for preconditions, `assert` for verifications.
   - Table-driven by default when ≥ 2 cases share setup.
4. **PROJECT.md constraints (zero runtime deps, MIT, `gofmt`-clean, no `init()`, no globals, English-only):** all apply to Phase 1.

## Architectural Responsibility Map

Phase 1 is a single architectural tier: a **types-only package** (zero runtime dependencies, no I/O, no goroutines, no network). The capabilities map cleanly:

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Date parsing / formatting (`Date.UnmarshalJSON`/`MarshalJSON`) | Domain types (pure) | — | No I/O; deterministic; pure function over `[]byte` |
| Sentinel errors (`ErrInvalidCountry`, etc.) | Domain types (pure) | — | Package-level immutable `var`s; no state |
| `*APIError` type + `Error()` / `Is()` | Domain types (pure) | — | Type definition + pure methods; construction in Phase 2 |
| Input validators (`validateCountry`, etc.) | Domain types (pure) | — | Pure functions; no I/O |
| Domain structs (`Holiday`, `Country`, `Language`, `Subdivision`, `LocalizedText`, `SubdivisionRef`) | Domain types (pure) | — | Plain structs with JSON tags |
| `go.mod` / `LICENSE` / `version.go` / `doc.go` | Build artifact | — | Module metadata + package docs |

No HTTP, no goroutines, no caching, no logging. All capabilities live in one Go package (root `openholidays`) per ARCHITECTURE.md Pattern 1.

## Standard Stack

### Core (Production code)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go (toolchain) | 1.23.0 minimum; CI matrix tests 1.23, 1.24, `stable` | Language + stdlib | Locked by PROJECT.md after raising from 1.22 because `iter.Seq` (a Go 1.23 feature) is used in Phase 3's `Holiday.Range()`. Bumping the floor in Phase 1 — even though Phase 1 itself does not use `iter` — avoids build-tag complexity later. `[VERIFIED: go.dev/doc/devel/release — Go 1.23.0 released 2024-08-13; current latest patch is 1.23.12 (2025-08-06); both 1.23 and 1.24 still supported as of 2026-05-27]` |
| `time` | stdlib | Date parsing, `time.Time` embedding in `Date`, calendar arithmetic via `AddDate` | `time.Parse("2006-01-02", s)` returns UTC by default `[VERIFIED: pkg.go.dev/time — "In the absence of a time zone indicator, Parse returns a time in UTC"]`. `AddDate(years, months, days)` is calendar-aware and normalizes overflow (e.g. Oct 31 + 1mo = Dec 1) `[VERIFIED: same source]` |
| `strings` | stdlib | `ToUpper`/`ToLower` for validator canonicalization, `Trim` for unmarshal quote stripping | `strings.EqualFold` (ASCII fold), `strings.ToUpper` (Unicode-aware but ASCII-only inputs are safe) |
| `errors` | stdlib | `errors.New` for sentinels, `errors.Is`/`errors.As` traversal contract | `errors.Is` calls a target's `Is(error) bool` method if present `[VERIFIED: pkg.go.dev/errors — "An error is considered to match a target if it is equal to that target or if it implements a method Is(error) bool such that Is(target) returns true"]`. Critical for `*APIError.Is` |
| `fmt` | stdlib | `fmt.Errorf` with `%w` for sentinel wrapping, `Stringer` impl | `fmt.Errorf("%w: %q", sentinel, val)` produces an unwrappable error whose `errors.Is(err, sentinel)` returns true `[VERIFIED: pkg.go.dev/fmt — "If the format specifier includes a %w verb with an error operand, the returned error will implement an Unwrap method"]` |
| `encoding/json` | stdlib | `MarshalJSON`/`UnmarshalJSON` on `Date` | v1 is fully sufficient; no perf concerns for Phase 1. Confirmed by STACK.md |
| `unicode` | stdlib (only if needed) | Character class checks for `[A-Z]` / `[a-z]` | Likely unnecessary — a tight `b >= 'A' && b <= 'Z'` check is simpler and avoids `unicode.IsUpper` (which is Unicode-wide); we want ASCII only |

### Supporting (Test-only)

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/stretchr/testify/assert` | v1.11.1 (current latest) | Non-aborting assertions inside `t.Run` cases | Every verification in `t.Run` blocks — `assert.Equal(t, want, got)` |
| `github.com/stretchr/testify/require` | v1.11.1 | Aborting preconditions inside `t.Run` cases | Setup / preconditions — `require.NoError(t, err)` before exercising the unit under test |
| `testing` (incl. `testing.F`) | stdlib | Unit + fuzz tests | Fuzz target `FuzzDateUnmarshal` uses `*testing.F` (Go 1.18+, well within the Go 1.23 floor) `[VERIFIED: pkg.go.dev/testing#F — "added in go1.18"]` |
| `bytes` | stdlib | `bytes.Equal(b, []byte("null"))` check in `Date.UnmarshalJSON` | Per PITFALLS JSON-3 canonical template — faster than `string(b) == "null"` |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `type Date struct { time.Time }` (D-05 locked) | `type Date time.Time` (named alias) | Alias is more concise but loses promoted methods — callers must do `time.Time(d).Year()`. Wrapper struct is idiomatic for SDK ergonomics. **[CITED: ARCHITECTURE.md Pattern 3]** |
| testify | `github.com/google/go-cmp` only + raw `t.Errorf` | go-cmp is approved as a secondary; testify is mandated by Gold Rule 3 for primary assertions. go-cmp lands when testify output is insufficient (rare; deep struct diffs) |
| `errors.New("openholidays: invalid country code")` per sentinel | A single `ValidationError{Field, Value, Reason}` typed-struct | Deferred ideas explicitly reject the typed-struct approach. Sentinels are simpler, idiomatic, support `errors.Is`. **[CITED: CONTEXT.md Deferred Ideas]** |
| `bytes.Equal(b, []byte("null"))` | `string(b) == "null"` | Both correct; `bytes.Equal` avoids an allocation. Marginal but the canonical PITFALLS template uses it |
| `strings.ToUpper` / `strings.ToLower` for canonicalization | byte-level `b - 32` ASCII fold | Stdlib functions are zero-overhead for ASCII (fast path) and self-documenting. Byte arithmetic saves nothing |

**Installation:**
```
# Module bootstrap (Phase 1 rewrites both files wholesale — the existing
# stray go.mod/go.sum at the repo root are POC artifacts to delete).
go mod init github.com/egeek-tech/go-openholidays
go mod edit -go=1.23

# Test-only dependency (only imported from *_test.go files).
go get github.com/stretchr/testify@v1.11.1

# Verify testify is test-only:
go mod why github.com/stretchr/testify   # must show only test imports
```

**Version verification:**
- `go 1.23` directive — verified via `go.dev/doc/devel/release` (2026-05-27): Go 1.23.12 latest patch, released 2025-08-06; Go 1.23 still supported. `go 1.23` and `go 1.23.0` are both valid syntax `[VERIFIED: go.dev/ref/mod — grammar accepts both forms]`. Recommend `go 1.23` (the form ROADMAP success criterion #5 cites).
- testify v1.11.1 — verified via `slopcheck install --ecosystem go github.com/stretchr/testify` (returned `[OK]` with note "No source repository linked at registry but well-known package"). Module `github.com/stretchr/testify` is the canonical import path.

## Package Legitimacy Audit

| Package | Registry | Age | Downloads | Source Repo | slopcheck | Disposition |
|---------|----------|-----|-----------|-------------|-----------|-------------|
| `github.com/stretchr/testify` | Go proxy (proxy.golang.org) | 10+ yrs (initial commit 2012) | One of top-5 Go test deps | [github.com/stretchr/testify](https://github.com/stretchr/testify) | [OK] | Approved (test-only) |

**Packages removed due to slopcheck [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

`slopcheck install --ecosystem go github.com/stretchr/testify` returned `[OK]` (1 OK, 0 SUS, 0 SLOP). slopcheck noted "No source repository linked. Harder to verify what this code actually does." in its output, but this is because the Go module proxy does not advertise source repositories the way npm/PyPI do; the source is at github.com/stretchr/testify and is well-known. Independent verification: testify is the most-downloaded Go assertion library per JetBrains' Go ecosystem survey, used in tens of thousands of OSS Go projects, has 23k+ GitHub stars, and is on PROJECT.md's pre-approved list.

`github.com/google/go-cmp` is also on the pre-approved list (PROJECT.md, CONVENTIONS.md) but Phase 1 does NOT need it — testify covers all required assertions. Skip adding it to Phase 1 to avoid an unused module entry; add when first needed.

**Slopcheck side-effect cleanup:** The slopcheck verification ran `go get github.com/stretchr/testify` against the stray POC `go.mod` at the repo root, modifying it. This is harmless: Phase 1's first task is to delete the stray `go.mod`/`go.sum` and run `go mod init github.com/egeek-tech/go-openholidays` wholesale.

## Upstream API Schema — Verified

**Source:** `https://openholidaysapi.org/swagger/v1/swagger.json` `[VERIFIED: live fetch 2026-05-27]`

This section locks the JSON shape Phase 1 types must decode. Phase 3 will exercise these against golden fixtures; Phase 1 must match the wire shape exactly.

### `HolidayResponse` (the array element of `/PublicHolidays` and `/SchoolHolidays`)

| JSON field | Type | Required / Nullable | Notes |
|------------|------|---------------------|-------|
| `id` | string (uuid) | required | UUID identifying the holiday |
| `startDate` | string (date, `YYYY-MM-DD`) | required | Phase 1 decodes to `Date` |
| `endDate` | string (date, `YYYY-MM-DD`) | required | Phase 1 decodes to `Date` |
| `type` | enum: `"Public"`, `"Bank"`, `"Optional"`, `"School"`, `"BackToSchool"`, `"EndOfLessons"` | required | **6 values, not 4** — CASE: capitalized first letter (PascalCase). REQUIREMENTS.md TYPES-04 lists 4 (`Public`, `School`, `Bank`, `Observance`) — `Observance` is NOT in the upstream, and `Optional`, `BackToSchool`, `EndOfLessons` are missing from the requirement |
| `name` | array of `LocalizedText` | required | Per-language localized names. The array shape, NOT a map (Pitfall OH-3) |
| `nationwide` | boolean | required | Whether the holiday applies country-wide |
| `regionalScope` | enum: `"National"`, `"Regional"`, `"Local"` | present | NOT marked nullable in the spec; treat as required string |
| `temporalScope` | enum: `"FullDay"`, `"HalfDay"` | present | NOT marked nullable in the spec; treat as required string |
| `comment` | array of `LocalizedText` | nullable | Optional commentary per language |
| `subdivisions` | array of `SubdivisionReference` | nullable | Which subdivisions the holiday applies to (when not nationwide) |
| `groups` | array of `GroupReference` | nullable | Group membership (e.g., school-holiday cohorts) |
| `tags` | array of string | nullable | **Newly verified — not previously mentioned in PROJECT.md or REQUIREMENTS.md** |
| `quality` | — | **NOT in the spec** | Observed in POC responses per PROJECT.md / PITFALLS OH-2; library tolerates lenient decode |

**Critical findings (each is an HONEST RED FLAG):**

1. **TYPES-04 enum mismatch (HIGH IMPACT).** Required values are `Public, Bank, Optional, School, BackToSchool, EndOfLessons` `[VERIFIED]`. REQUIREMENTS.md says `Public, School, Bank, Observance` `[CITED: REQUIREMENTS.md line 36]`. `Observance` does not exist upstream; three real values (`Optional`, `BackToSchool`, `EndOfLessons`) are missing. **Recommendation:** ship all 6 upstream values with PascalCase constants (`HolidayTypePublic`, `HolidayTypeBank`, `HolidayTypeOptional`, `HolidayTypeSchool`, `HolidayTypeBackToSchool`, `HolidayTypeEndOfLessons`). String constants matching upstream casing exactly. See Open Question Q1.
2. **`regionalScope` and `temporalScope` are MISSING from the verified TYPES-01 description, but REQUIREMENTS.md lists them.** REQUIREMENTS.md TYPES-01 line 32 includes them. ARCHITECTURE.md's Holiday example does NOT. The verified spec marks them as present (not nullable). Phase 1 must include them as typed-string enums OR plain strings. **Recommendation:** plain string for v0.1.0 (avoid adding two more enum families for marginal value), document the closed value set in godoc, revisit if a helper method needs to branch on them.
3. **`quality` is real-world drift.** Not in the spec, observed in responses. Include as `Quality string `json:"quality,omitempty"``. Default lenient decoding accepts both presence and absence. **[CITED: PITFALLS OH-2]**
4. **`tags` is a previously undocumented nullable field.** REQUIREMENTS.md TYPES-01 doesn't mention it; CONTEXT.md doesn't mention it. **Recommendation:** include `Tags []string `json:"tags,omitempty"`` to remain lenient against the spec — silently dropping a documented field is bad form even though decoders won't break.

### `LocalizedText`

| JSON field | Type | Required / Nullable |
|------------|------|---------------------|
| `language` | string (minLength: 1) | required |
| `text` | string (minLength: 1) | required |

Go type: `type LocalizedText struct { Language string \`json:"language"\`; Text string \`json:"text"\` }`. Field names in Go are PascalCase per convention; JSON tags match upstream camelCase. **[VERIFIED]**

### `SubdivisionReference` (used inside `Holiday.subdivisions`)

| JSON field | Type | Required / Nullable |
|------------|------|---------------------|
| `code` | string (minLength: 1) | required |
| `shortName` | string (minLength: 1) | required |

Go type: `type SubdivisionRef struct { Code string \`json:"code"\`; ShortName string \`json:"shortName"\` }`. **[VERIFIED]** CONTEXT.md calls this `SubdivisionRef` (shorter); upstream calls it `SubdivisionReference`. Library name `SubdivisionRef` is fine per ARCHITECTURE.md.

### `GroupReference` (used inside `Holiday.groups`)

| JSON field | Type | Required / Nullable |
|------------|------|---------------------|
| `code` | string (minLength: 1) | required |
| `shortName` | string (minLength: 1) | required |

Same shape as `SubdivisionReference`. Go type: `type GroupRef struct { Code string \`json:"code"\`; ShortName string \`json:"shortName"\` }`. **[VERIFIED — newly surfaced; not in CONTEXT.md but used by Holiday.groups]**

### `CountryResponse`

| JSON field | Type | Required / Nullable |
|------------|------|---------------------|
| `isoCode` | string (minLength: 1) | required |
| `name` | array of `LocalizedText` | required |
| `officialLanguages` | array of string | required |

Go type: `type Country struct { IsoCode string \`json:"isoCode"\`; Name []LocalizedText \`json:"name"\`; OfficialLanguages []string \`json:"officialLanguages"\` }`. **[VERIFIED]**

### `LanguageResponse`

| JSON field | Type | Required / Nullable |
|------------|------|---------------------|
| `isoCode` | string (minLength: 1) | required |
| `name` | array of `LocalizedText` | required |

Go type: `type Language struct { IsoCode string \`json:"isoCode"\`; Name []LocalizedText \`json:"name"\` }`. **[VERIFIED]**

### `SubdivisionResponse`

| JSON field | Type | Required / Nullable |
|------------|------|---------------------|
| `code` | string (minLength: 1) | required |
| `shortName` | string (minLength: 1) | required |
| `name` | array of `LocalizedText` | required |
| `category` | array of `LocalizedText` | required |
| `officialLanguages` | array of string | required |
| `isoCode` | string | nullable |
| `comment` | array of `LocalizedText` | nullable |
| `children` | array of `SubdivisionResponse` (self-recursive) | nullable |
| `groups` | array of `GroupReference` | nullable |

Go type: `type Subdivision struct { Code string \`json:"code"\`; ShortName string \`json:"shortName"\`; Name []LocalizedText \`json:"name"\`; Category []LocalizedText \`json:"category"\`; OfficialLanguages []string \`json:"officialLanguages"\`; IsoCode string \`json:"isoCode,omitempty"\`; Comment []LocalizedText \`json:"comment,omitempty"\`; Children []Subdivision \`json:"children,omitempty"\`; Groups []GroupRef \`json:"groups,omitempty"\` }`. **[VERIFIED]**

**Note on `IsoCode` nullability:** Upstream marks it nullable. Per PITFALLS JSON-2, a nullable string field could be modeled as `*string` if "absent" differs semantically from "empty". For subdivisions, an empty `IsoCode` is semantically the same as absent (the subdivision simply doesn't have an ISO 3166-2 code). Recommend non-pointer `string` with `,omitempty`.

### Notably absent from the API

- **No 3-year date-window cap is documented in the spec** `[VERIFIED 2026-05-27]`. The cap is a defensive client-side guard per PROJECT.md / PITFALLS OH-1. Phase 1's `validateDateRange` enforces it nonetheless.
- **No rate-limit headers documented** `[VERIFIED]`. Irrelevant for Phase 1 (no HTTP yet).
- **Error envelope (4xx/5xx) is `ProblemDetails`** with fields `type`, `title`, `status`, `detail`, `instance` (all nullable, additionalProperties allowed). Phase 2's `APIError.Message` parser should prefer `detail` then `title` then `type`. Phase 1 does not implement the parser; it ships the type.

## Architecture Patterns

### System Architecture Diagram

```
                  ┌──────────────────────────────────────────┐
                  │ Library consumer (future Phase 2+ code)  │
                  │                                          │
                  │ openholidays.NewDate(2025, time.Dec, 24) │
                  │ openholidays.ParseDate("2025-12-24")     │
                  │ if err := validateCountry("pl"); err …   │
                  │ var h openholidays.Holiday               │
                  │ json.Unmarshal(b, &h)                    │
                  └────────────────┬─────────────────────────┘
                                   │
                                   ▼
   ┌─────────────────────────────────────────────────────────────┐
   │             package openholidays (root, single pkg)         │
   │                                                             │
   │   ┌─────────────┐    ┌─────────────┐    ┌──────────────┐    │
   │   │  errors.go  │    │   date.go   │    │   types.go   │    │
   │   │             │    │             │    │              │    │
   │   │ sentinels   │    │ type Date   │◄───┤ Holiday      │    │
   │   │ *APIError   │    │ Marshal/    │    │ Country      │    │
   │   │ Is/Error    │    │  Unmarshal  │    │ Language     │    │
   │   │             │    │ NewDate     │    │ Subdivision  │    │
   │   │             │    │ ParseDate   │    │ LocalizedText│    │
   │   │             │    │ DaysUntil   │    │ SubdivisionRef    │
   │   │             │    │ Equal/Before│    │ GroupRef     │    │
   │   │             │    │ /After/Cmp  │    │ HolidayType  │    │
   │   │             │    │ String      │    │   constants  │    │
   │   └──────▲──────┘    └──────▲──────┘    └──────▲───────┘    │
   │          │                  │                  │            │
   │          │     ┌────────────┴──────────┐       │            │
   │          │     │     validate.go       │       │            │
   │          └─────┤                       ├───────┘            │
   │                │ validateCountry       │                    │
   │                │ validateLanguage      │                    │
   │                │ validateDateRange     │                    │
   │                │ (uses sentinels + Date)                    │
   │                └───────────────────────┘                    │
   │                                                             │
   │   ┌─────────────┐    ┌─────────────┐                        │
   │   │  doc.go     │    │ version.go  │                        │
   │   │             │    │             │                        │
   │   │ package     │    │ const       │                        │
   │   │ comment     │    │ Version =   │                        │
   │   │             │    │ "0.1.0"     │                        │
   │   └─────────────┘    └─────────────┘                        │
   └─────────────────────────────────────────────────────────────┘
                                   │
                                   ▼
                  ┌──────────────────────────────────────────┐
                  │   Test layer (one *_test.go per file)    │
                  │                                          │
                  │   errors_test.go    date_test.go         │
                  │   types_test.go     validate_test.go     │
                  │   (testify/assert + testify/require)     │
                  │   date_test.go also defines              │
                  │   FuzzDateUnmarshal                      │
                  └──────────────────────────────────────────┘
```

Data flow is one-directional: callers construct values (via `NewDate`, `ParseDate`, or `json.Unmarshal`), pass them to validators, and read back errors. No goroutines, no I/O, no network. Phase 2 builds the HTTP client on top of this stable type surface.

### Recommended Project Structure (Phase 1 files only)

```
go-openholidays/
├── go.mod                      # module github.com/egeek-tech/go-openholidays
│                               # go 1.23
│                               # require github.com/stretchr/testify v1.11.1 // test-only
├── go.sum                      # generated by `go mod tidy`
├── LICENSE                     # MIT, single root file (D-04)
├── doc.go                      # package-level godoc; required so godoc shows it first
├── version.go                  # const Version = "0.1.0" (D-03)
├── errors.go                   # 5 sentinels + *APIError + Error() + Is()
├── date.go                     # type Date + Marshal/UnmarshalJSON + constructors + helpers
├── types.go                    # Holiday, Country, Language, Subdivision, LocalizedText,
│                               # SubdivisionRef, GroupRef, HolidayType constants
├── validate.go                 # unexported validateCountry, validateLanguage, validateDateRange
├── errors_test.go              # tests for sentinels + *APIError
├── date_test.go                # tests + FuzzDateUnmarshal (D-12, CL-03)
├── types_test.go               # tests for JSON round-trip + Name(lang) accessors (TYPES-05)
└── validate_test.go            # tests for all three validators
```

**Files explicitly NOT created in Phase 1** (per CONTEXT.md):
- `client.go`, `options.go`, `transport.go`, `transport_*.go` — Phase 2
- `countries.go`, `languages.go`, `public_holidays.go`, `school_holidays.go`, `subdivisions.go` — Phase 2-3
- `holiday.go` (helpers Name/IsInRegion/Days/Range) — Phase 3
- `internal/testhttp/` — Phase 2 (no httptest yet)
- `cmd/ohcli/` — Phase 5
- `testdata/` — Phase 3 (no golden fixtures needed in Phase 1)
- `example_test.go` — Phase 5
- `integration_test.go` — Phase 5
- `.golangci.yml`, `.goreleaser.yaml`, `.github/workflows/*` — Phase 5
- `README.md`, `CHANGELOG.md`, `CONTRIBUTING.md`, `docs/design.md` — Phase 5

### Pattern 1: Custom `Date` wrapper struct (TYPES-02, D-05 through D-11)

**What:** `type Date struct { time.Time }` embeds `time.Time` so all stdlib methods are promoted. Phase 1 ships `MarshalJSON`, `UnmarshalJSON`, `String`, `Equal`, `Before`, `After`, `Compare`, `DaysUntil`, `NewDate`, `ParseDate`.

**When to use:** Any field that carries an upstream `YYYY-MM-DD` string. Phase 1 establishes the type so every later phase consumes it.

**Example (verified canonical form, derived from PITFALLS JSON-3 template + D-05 through D-11):**

```go
// Source: PITFALLS.md §JSON-3 + CONTEXT.md D-05..D-11 + verified time package semantics
package openholidays

import (
    "bytes"
    "fmt"
    "time"
)

// Date is a calendar date (no timezone) returned by the OpenHolidays API.
// Internally stored as a time.Time normalized to UTC midnight so embedded
// time.Time methods (Year, Month, Day, Format) work naturally.
type Date struct {
    time.Time
}

// dateLayout is the wire format the upstream uses for every date field.
const dateLayout = "2006-01-02"

// errEmptyDate signals an empty or null date payload during JSON decode.
// Unexported on purpose — keeps the public sentinel surface at 5 (D-06).
var errEmptyDate = fmt.Errorf("openholidays: empty date string")

// NewDate constructs a Date at UTC midnight on the given year, month, day.
func NewDate(year int, month time.Month, day int) Date {
    return Date{time.Date(year, month, day, 0, 0, 0, 0, time.UTC)}
}

// ParseDate parses a YYYY-MM-DD string and returns the corresponding UTC-midnight Date.
// Empty input returns errEmptyDate; malformed input returns a wrapped time.Parse error.
func ParseDate(s string) (Date, error) {
    if s == "" {
        return Date{}, errEmptyDate
    }
    t, err := time.Parse(dateLayout, s)
    if err != nil {
        return Date{}, fmt.Errorf("openholidays: invalid date %q: %w", s, err)
    }
    return Date{t}, nil
}

// MarshalJSON emits the Date as a JSON string in YYYY-MM-DD form.
// The zero Date round-trips to "0001-01-01" (D-07).
func (d Date) MarshalJSON() ([]byte, error) {
    // 12 bytes: 2 quotes + 10 char date.
    buf := make([]byte, 0, 12)
    buf = append(buf, '"')
    buf = d.AppendFormat(buf, dateLayout)
    buf = append(buf, '"')
    return buf, nil
}

// UnmarshalJSON parses YYYY-MM-DD strings into the Date.
// Rejects both null and empty-string input with errEmptyDate (D-06).
func (d *Date) UnmarshalJSON(b []byte) error {
    if bytes.Equal(b, []byte("null")) {
        return fmt.Errorf("openholidays: null is not a valid date: %w", errEmptyDate)
    }
    if len(b) < 2 || b[0] != '"' || b[len(b)-1] != '"' {
        return fmt.Errorf("openholidays: date must be a JSON string, got %s", b)
    }
    s := string(b[1 : len(b)-1])
    if s == "" {
        return fmt.Errorf("openholidays: %w", errEmptyDate)
    }
    t, err := time.Parse(dateLayout, s)
    if err != nil {
        return fmt.Errorf("openholidays: invalid date %q: %w", s, err)
    }
    *d = Date{t}
    return nil
}

// String returns the Date in YYYY-MM-DD form (D-08).
// This shadows the embedded time.Time.String() to avoid emitting the
// "0000-00-00 00:00:00 +0000 UTC" format that's noisy in CLI output.
func (d Date) String() string {
    return d.Format(dateLayout)
}

// Equal reports whether two Dates represent the same calendar day.
// Both operands are normalized to UTC midnight before comparison (D-09).
func (d Date) Equal(other Date) bool {
    return d.toUTCMidnight().Equal(other.toUTCMidnight())
}

// Before reports whether d is strictly before other (D-09).
func (d Date) Before(other Date) bool {
    return d.toUTCMidnight().Before(other.toUTCMidnight())
}

// After reports whether d is strictly after other (D-09).
func (d Date) After(other Date) bool {
    return d.toUTCMidnight().After(other.toUTCMidnight())
}

// Compare returns -1 if d < other, 0 if d == other, +1 if d > other (D-09).
func (d Date) Compare(other Date) int {
    return d.toUTCMidnight().Compare(other.toUTCMidnight())
}

// DaysUntil returns the inclusive day count from d to other.
// Calendar-correct across DST boundaries (D-10, Pitfall TZ-2).
//
// For d == other, returns 1. For d > other, returns a negative count.
func (d Date) DaysUntil(other Date) int {
    a := d.toUTCMidnight()
    b := other.toUTCMidnight()
    // Both operands are UTC midnight, so Sub returns a clean multiple of 24h.
    days := int(b.Sub(a).Hours() / 24)
    if days >= 0 {
        return days + 1
    }
    return days - 1
}

// toUTCMidnight is the canonical normalization used by every comparison method.
// Defensive against a Date{} constructed by external code that didn't go
// through NewDate/ParseDate (e.g., a hand-rolled struct literal).
func (d Date) toUTCMidnight() time.Time {
    return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
}
```

**Why this shape:**
- `bytes.Equal(b, []byte("null"))` per PITFALLS JSON-3 template — faster than `string(b) == "null"`.
- `time.Parse(dateLayout, s)` returns UTC by default `[VERIFIED]`, so no `ParseInLocation` needed.
- `*d = Date{t}` after parse — `t` is already UTC from `time.Parse`, so no re-normalization needed in the happy path.
- `toUTCMidnight` defensive normalization makes `Equal`/`Before`/`After` robust against `Date{time.Date(..., loc)}` constructed outside `NewDate`. Eliminates the TZ-1 footgun even from misuse.
- `MarshalJSON` uses `AppendFormat` to avoid an extra allocation. `time.Time.AppendFormat` is well-established.
- `DaysUntil` uses `Sub` on UTC-midnight times — safe because both are at 00:00 UTC, eliminating DST from the calculation entirely (Pitfall TZ-2 mitigation).

### Pattern 2: Sentinel errors + `*APIError` leaf type (ERR-01..ERR-04)

**What:** Five package-level `var Err... = errors.New("openholidays: ...")` sentinels for input-validation failures; a struct `APIError` for upstream HTTP failures (constructed in Phase 2, type defined in Phase 1).

**Example (canonical implementation):**

```go
// Source: CONTEXT.md D-13..D-19 + ARCHITECTURE.md Pattern 6
package openholidays

import (
    "errors"
    "fmt"
)

// Sentinel errors. Use errors.Is to detect them through %w-wrapped chains.
var (
    // ErrInvalidCountry is returned for malformed country codes
    // (not exactly two ASCII letters after canonicalization).
    ErrInvalidCountry = errors.New("openholidays: invalid country code")

    // ErrInvalidLanguage is returned for malformed language codes
    // (not exactly two ASCII letters after canonicalization).
    ErrInvalidLanguage = errors.New("openholidays: invalid language code")

    // ErrDateRangeTooLarge is returned when validFrom..validTo spans more
    // than 3 calendar years inclusive.
    ErrDateRangeTooLarge = errors.New("openholidays: date range too large")

    // ErrInvalidDateRange is returned when validFrom is strictly after validTo.
    ErrInvalidDateRange = errors.New("openholidays: invalid date range")

    // ErrEmptyResponse is returned when the upstream returns a 2xx with an
    // empty body where a non-empty payload was required.
    ErrEmptyResponse = errors.New("openholidays: empty response body")
)

// APIError represents a non-2xx response from the upstream API.
// Constructed by endpoint methods (Phase 2); Phase 1 only ships the type
// and its methods so callers can write errors.As(err, &apiErr) reliably.
//
// Use errors.Is(err, &APIError{StatusCode: 404}) to match by status, or
// errors.Is(err, &APIError{}) (zero StatusCode) to match any APIError.
// See (e *APIError) Is for the matching semantics.
type APIError struct {
    StatusCode int    // HTTP status code (>= 400)
    Path       string // Request path (e.g., "/PublicHolidays")
    Body       []byte // Raw response body, capped at 4 KiB (Phase 2 enforces cap)
    Message    string // Best-effort message parsed from upstream JSON; empty when unparseable
}

// Error returns a human-readable description (D-18).
//
// Format with Message: "openholidays: api error 404 at /Subdivisions: Country not supported"
// Format without:    "openholidays: api error 404 at /Subdivisions"
func (e *APIError) Error() string {
    if e.Message == "" {
        return fmt.Sprintf("openholidays: api error %d at %s", e.StatusCode, e.Path)
    }
    return fmt.Sprintf("openholidays: api error %d at %s: %s", e.StatusCode, e.Path, e.Message)
}

// Is supports errors.Is(err, &APIError{StatusCode: N}) status-code matching (D-15).
//
//   - If target is not *APIError: returns false.
//   - If target.StatusCode == 0: matches any *APIError (the wildcard case).
//   - If target.StatusCode != 0: matches when e.StatusCode == target.StatusCode.
//
// Path, Body, and Message on the target are ignored — they exist for diagnostics,
// not for matching. This is a deliberate project convention (see Open Question Q3).
func (e *APIError) Is(target error) bool {
    t, ok := target.(*APIError)
    if !ok {
        return false
    }
    if t.StatusCode != 0 && t.StatusCode != e.StatusCode {
        return false
    }
    return true
}
```

**Why no `Unwrap()` (D-16):** `APIError` is a leaf — it does not wrap a transport-level cause. Per `errors` godoc, omitting `Unwrap()` is correct when the error is terminal `[VERIFIED: pkg.go.dev/errors — "It is invalid for an Unwrap method to return an []error containing a nil error value"; presence of Unwrap implies wrapping]`.

**Why `Is(target)` instead of relying on `errors.As`:** Both are supported; `errors.As(err, &apiErr)` extracts the value, `errors.Is(err, &APIError{StatusCode: 404})` matches by predicate. The dual support gives callers both ergonomics. The match-by-StatusCode pattern is documented in the godoc above and is the recognized convention (also used by `aws-sdk-go-v2`, `googleapis/google-cloud-go`, `gocloud.dev/gcerrors`).

### Pattern 3: Validators as unexported root-package functions (VALID-01..VALID-04)

**What:** Three unexported functions in `validate.go`. Each takes raw user input, returns either canonicalized input (for `country`/`language` per D-20/D-21) or just an error (for `validateDateRange` per D-22). All wrap their sentinel with `%w` and include the offending value via `%q` (D-23).

**Example (canonical):**

```go
// Source: CONTEXT.md D-20..D-23 + ARCHITECTURE.md Pattern 5
package openholidays

import (
    "fmt"
    "strings"
)

// validateCountry canonicalizes a country ISO 3166-1 alpha-2 code to uppercase
// and verifies it is exactly 2 ASCII letters. Returns the canonical form.
//
// Accepts any input case (D-20). Wire format is uppercase per upstream API.
func validateCountry(code string) (string, error) {
    canon := strings.ToUpper(code)
    if !isTwoASCIIUppers(canon) {
        return "", fmt.Errorf("%w: %q", ErrInvalidCountry, code)
    }
    return canon, nil
}

// validateLanguage canonicalizes a language ISO 639-1 code to lowercase
// and verifies it is exactly 2 ASCII letters. Returns the canonical form.
//
// Accepts any input case (D-21). Shape-only check; no allowlist (CONTEXT.md Deferred).
func validateLanguage(code string) (string, error) {
    canon := strings.ToLower(code)
    if !isTwoASCIILowers(canon) {
        return "", fmt.Errorf("%w: %q", ErrInvalidLanguage, code)
    }
    return canon, nil
}

// validateDateRange enforces that:
//
//   - from <= to (else ErrInvalidDateRange)
//   - to is within 3 calendar years inclusive of from (else ErrDateRangeTooLarge)
//
// Uses Date.AddDate (calendar arithmetic) rather than duration math so leap
// years and DST do not produce off-by-one results (D-22, Pitfall TZ-2).
func validateDateRange(from, to Date) error {
    if from.After(to) {
        return fmt.Errorf("%w: from=%s to=%s", ErrInvalidDateRange, from, to)
    }
    // "Within 3 calendar years inclusive" means to <= from + 3 years.
    // Equivalent: to is strictly before (from + 3 years + 1 day).
    limit := Date{from.AddDate(3, 0, 1)}
    if !to.Before(limit) {
        return fmt.Errorf("%w: from=%s to=%s spans more than 3 years", ErrDateRangeTooLarge, from, to)
    }
    return nil
}

// isTwoASCIIUppers reports whether s is exactly 2 bytes in [A-Z].
func isTwoASCIIUppers(s string) bool {
    if len(s) != 2 {
        return false
    }
    return s[0] >= 'A' && s[0] <= 'Z' && s[1] >= 'A' && s[1] <= 'Z'
}

// isTwoASCIILowers reports whether s is exactly 2 bytes in [a-z].
func isTwoASCIILowers(s string) bool {
    if len(s) != 2 {
        return false
    }
    return s[0] >= 'a' && s[0] <= 'z' && s[1] >= 'a' && s[1] <= 'z'
}
```

**Why byte arithmetic instead of `unicode.IsLetter`:** We want ASCII-only. `unicode.IsLetter('Ö')` returns true; that's wrong for an ISO 3166-1 alpha-2 country code. Two bounds checks per byte is faster and unambiguous.

**Why `AddDate(3, 0, 1)` instead of `AddDate(3, 0, 0).Add(24*time.Hour)`:** Calendar-correct vs DST-broken. `AddDate` is calendar-aware per `[VERIFIED: pkg.go.dev/time — "AddDate uses the Location of the Time value to determine these durations"]`. For our UTC-midnight dates this distinction doesn't bite, but using `AddDate` consistently is safer and self-documenting.

**Why `to.Before(limit)` instead of `to.Compare(limit) <= 0`:** Both work; `Before` is more readable. Tests must verify the boundary: `from=2025-01-01, to=2028-01-01` (3 years exact, inclusive) MUST pass; `from=2025-01-01, to=2028-01-02` (3y+1d) MUST fail.

### Pattern 4: Domain structs with JSON tags matching the wire shape exactly

**What:** Plain Go structs in `types.go` with JSON tags matching upstream camelCase field names. No methods on `Holiday` itself in Phase 1 (helpers `Name`/`IsInRegion`/`Days`/`Range` ship in Phase 3); methods on `Country`/`Language`/`Subdivision` for `Name(lang)` accessor (TYPES-05).

**Example (Phase-1 surface):**

```go
// Source: Verified upstream OpenAPI spec (2026-05-27)
package openholidays

// HolidayType is the typed-string enum for Holiday.Type.
//
// Values come from the upstream OpenAPI spec exactly (6 values, not 4).
// Cf. REQUIREMENTS.md TYPES-04, which listed an outdated/incomplete set.
type HolidayType string

const (
    HolidayTypePublic        HolidayType = "Public"
    HolidayTypeBank          HolidayType = "Bank"
    HolidayTypeOptional      HolidayType = "Optional"
    HolidayTypeSchool        HolidayType = "School"
    HolidayTypeBackToSchool  HolidayType = "BackToSchool"
    HolidayTypeEndOfLessons  HolidayType = "EndOfLessons"
)

// LocalizedText is a (language, text) pair returned in name/comment/category arrays.
type LocalizedText struct {
    Language string `json:"language"`
    Text     string `json:"text"`
}

// SubdivisionRef is a lightweight reference (code + short display name)
// embedded in Holiday.Subdivisions.
type SubdivisionRef struct {
    Code      string `json:"code"`
    ShortName string `json:"shortName"`
}

// GroupRef is a lightweight reference embedded in Holiday.Groups
// (e.g., Polish ferie cohorts A/B/C/D).
type GroupRef struct {
    Code      string `json:"code"`
    ShortName string `json:"shortName"`
}

// Holiday represents one public or school holiday returned by the API.
//
// Multi-day holidays (school holidays) have StartDate < EndDate. For
// single-day public holidays, StartDate == EndDate. Always use both fields;
// see Pitfall TZ-3 in research history.
type Holiday struct {
    ID            string           `json:"id"`
    StartDate     Date             `json:"startDate"`
    EndDate       Date             `json:"endDate"`
    Type          HolidayType      `json:"type"`
    Name          []LocalizedText  `json:"name"`
    Nationwide    bool             `json:"nationwide"`
    RegionalScope string           `json:"regionalScope"`         // "National" / "Regional" / "Local"
    TemporalScope string           `json:"temporalScope"`         // "FullDay" / "HalfDay"
    Comment       []LocalizedText  `json:"comment,omitempty"`     // upstream nullable
    Subdivisions  []SubdivisionRef `json:"subdivisions,omitempty"`// upstream nullable
    Groups        []GroupRef       `json:"groups,omitempty"`      // upstream nullable
    Tags          []string         `json:"tags,omitempty"`        // upstream nullable
    Quality       string           `json:"quality,omitempty"`     // observed in wild, not in spec
}

// Country is the response shape for /Countries.
type Country struct {
    IsoCode           string          `json:"isoCode"`
    Name              []LocalizedText `json:"name"`
    OfficialLanguages []string        `json:"officialLanguages"`
}

// NameFor returns the localized country name for the given ISO 639-1 language
// code. Falls back to the first available entry if the requested language is
// not present. Returns "" only when Name is empty.
//
// Comparison is case-insensitive (strings.EqualFold).
func (c Country) NameFor(lang string) string {
    return pickLocalized(c.Name, lang)
}

// Language is the response shape for /Languages.
type Language struct {
    IsoCode string          `json:"isoCode"`
    Name    []LocalizedText `json:"name"`
}

// NameFor returns the localized language name. See Country.NameFor.
func (l Language) NameFor(lang string) string {
    return pickLocalized(l.Name, lang)
}

// Subdivision is the response shape for /Subdivisions (also Holiday.Subdivisions
// references this via SubdivisionRef).
type Subdivision struct {
    Code              string          `json:"code"`
    ShortName         string          `json:"shortName"`
    Name              []LocalizedText `json:"name"`
    Category          []LocalizedText `json:"category"`
    OfficialLanguages []string        `json:"officialLanguages"`
    IsoCode           string          `json:"isoCode,omitempty"`    // upstream nullable
    Comment           []LocalizedText `json:"comment,omitempty"`    // upstream nullable
    Children          []Subdivision   `json:"children,omitempty"`   // upstream nullable, recursive
    Groups            []GroupRef      `json:"groups,omitempty"`     // upstream nullable
}

// NameFor returns the localized subdivision name. See Country.NameFor.
func (s Subdivision) NameFor(lang string) string {
    return pickLocalized(s.Name, lang)
}

// pickLocalized walks a LocalizedText slice and returns the text for the
// requested language code, falling back to the first entry if not found.
// Returns "" only when the slice is empty.
//
// Case-insensitive match (strings.EqualFold) so "PL" matches "pl".
func pickLocalized(entries []LocalizedText, lang string) string {
    for _, e := range entries {
        if strings.EqualFold(e.Language, lang) {
            return e.Text
        }
    }
    if len(entries) > 0 {
        return entries[0].Text
    }
    return ""
}
```

**Note on `NameFor` vs `Name(lang)`:** REQUIREMENTS.md TYPES-05 uses the verb `Name(lang)`. But `Name` is already the field on the struct; a method named `Name(lang)` collides with the field name (Go would reject the type definition). The natural rename is `NameFor(lang)` or `NameIn(lang)`. **Recommend `NameFor(lang)` — see Open Question Q2.** Either way, this is the same accessor that lands as `Holiday.Name(lang)` in Phase 3 (helper functions; tracked under HELP-01 not TYPES-05) — so the Phase 3 helper for `Holiday` will need the same rename.

### Anti-Patterns to Avoid

| Anti-pattern | Why it's bad | Use instead |
|--------------|--------------|-------------|
| `init()` anywhere in `*.go` files | Violates CLIENT-10 and PROJECT.md constraint | Constructor pattern; constants for literal config |
| Package-level mutable `var` (other than sentinels) | Violates CLIENT-10 ("no global mutable state") | Per-Client state in Phase 2+ |
| Custom `UnmarshalJSON` on `Holiday` itself | Couples date format to every domain struct; tedious to extend; PITFALLS JSON-3 / ARCHITECTURE Pattern 3 | Custom `Date` type with `UnmarshalJSON`; `Holiday` decodes via stdlib `json.Unmarshal` |
| Returning `time.Time` for dates | TZ-1 footgun — callers compare to wall-clock and get off-by-one near midnight | Return the custom `Date` type |
| `Date.UnmarshalJSON` that silently accepts `null` | Pitfall JSON-3 / JSON-4 — produces year-1 dates in production | Return an error for `null` and empty string (D-06) |
| `errors.New(fmt.Sprintf(...))` for validation errors | Loses identity; `errors.Is` cannot match | `fmt.Errorf("%w: %q", ErrInvalidCountry, code)` (D-23) |
| Sentinel error string starting without `"openholidays: "` | Go convention says error strings should be self-identifying | All sentinels start `"openholidays: "` |
| `Date.UnmarshalJSON` decoding into a `time.Time` whose location is `time.Local` | Server in different TZs produces different dates from the same JSON | `time.Parse` returns UTC by default; safe |
| `validateDateRange` using `to.Sub(from) > 3 * 365 * 24 * time.Hour` | Off-by-one across leap years (3 years could be 1095 OR 1096 days) | Calendar arithmetic via `AddDate(3, 0, 1)` (D-22) |
| Map-shaped `Name map[string]string` for localized text | Wrong wire shape; PITFALL OH-3 | `[]LocalizedText` array per verified upstream schema |

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Date parsing from `YYYY-MM-DD` | Custom byte-level parser | `time.Parse("2006-01-02", s)` | Stdlib handles year boundaries, leap years, negative years, padding correctly |
| Calendar arithmetic (3-year window check) | `to.Sub(from).Hours() / 24 / 365` | `time.Time.AddDate(years, months, days)` | DST + leap years break duration math; `AddDate` is calendar-correct **[CITED: pkg.go.dev/time]** |
| Error wrapping | Custom wrapper type | `fmt.Errorf("...: %w", err)` + `errors.Is`/`errors.As` | Stdlib idiom since Go 1.13 (errors.Is) and 1.20 (multi-%w); battle-tested **[CITED: pkg.go.dev/errors, pkg.go.dev/fmt]** |
| Localized-text language matching | Build a `map[string]string` index | Linear scan over `[]LocalizedText` with `strings.EqualFold` | Slice is ≤ ~14 entries (one per language); linear scan is O(n) where n is tiny; building a map costs more **[CITED: PITFALLS.md Performance Traps table]** |
| Assertion library | Roll our own `assertEqual` | `github.com/stretchr/testify/{assert,require}` | Mandated by Gold Rule 3 |
| Slop / supply-chain check | Manual `npm view`-style probe | `slopcheck install --ecosystem go <pkg>` | Catches hallucinated package names + new packages with no history |

**Key insight:** Phase 1 is genuinely small. The temptation to "build the simplest possible date type" or "just use a single error type" produces footguns in months — Pitfalls JSON-3, JSON-4, TZ-1, TZ-2 all stem from over-simplification at the type-definition layer. The canonical implementations above are 250 lines of source; building them right pays back for every future phase.

## Common Pitfalls

### Pitfall 1: Returning zero `Date{}` from `UnmarshalJSON` on empty/null

**What goes wrong:** `null` or `""` input silently parses to zero-value `Date{}` (year 1, January 1, UTC). Caller's calendar UI renders "Jan 1, year 1" as the school holiday start.
**Why it happens:** Minimal unmarshaler returns `nil` on unrecognized input. Returns `Date{}` by default. Pitfall JSON-3 / JSON-4.
**How to avoid:** Both branches return an error (D-06). The unexported `errEmptyDate` sentinel is the wrapped cause; callers can `errors.Is(err, openholidays.ErrEmptyResponse)` via the public surface in Phase 2 — but Phase 1 keeps `errEmptyDate` unexported per D-06.
**Warning signs:** Unmarshaler shorter than 8 lines; no test for `[]byte("null")`; no test for `[]byte("\"\"")`.

### Pitfall 2: `time.Parse` ambiguity → local-TZ surprise

**What goes wrong:** `time.Parse("2006-01-02", "2025-12-25")` in some user's mental model produces a `time.Time` in `time.Local`. Caller does `holiday.StartDate.Equal(timeNowLocal.Truncate(24*time.Hour))` and gets `false` because the parsed time is UTC but the comparand is local.
**Why it happens:** Misreading `time.Parse` docs.
**How to avoid:** `time.Parse` *does* return UTC by default `[VERIFIED]` so we're safe. The `Date.toUTCMidnight` helper is defensive against `Date{time.Date(..., someLoc)}` constructed by external code that didn't go through `NewDate`/`ParseDate`. The `Date.Equal`/`Before`/`After`/`Compare` methods use this helper so callers cannot accidentally compare a non-UTC Date.
**Warning signs:** Tests that pass under `TZ=UTC go test` but fail under `TZ=Europe/Warsaw go test`. (Mitigated by `toUTCMidnight`.)

### Pitfall 3: Off-by-one in 3-year window check across leap years

**What goes wrong:** `validateDateRange` uses `to.Sub(from) > 3*365*24*time.Hour`. For a span ending on a leap-year February 29, a valid 3-year-exact span gets rejected; for the reverse pattern, an invalid 3-year-plus-one-day span gets accepted.
**Why it happens:** Duration math treats every year as 365.25 days on average; calendar reality is messier.
**How to avoid:** `from.AddDate(3, 0, 1)` (D-22). Calendar-aware; produces exactly `from + 3 years + 1 day` in calendar terms.
**Warning signs:** Test cases that don't include leap-year boundaries (`2024-02-29 → 2027-02-28` should pass; `2024-02-29 → 2027-03-01` should fail).

### Pitfall 4: `Date.MarshalJSON` not symmetric with `UnmarshalJSON`

**What goes wrong:** `MarshalJSON(NewDate(2025, time.Dec, 24))` emits `"2025-12-24"`; `MarshalJSON(Date{})` emits something unexpected — caller's round-trip test fails or worse, the zero Date is silently treated as a valid Christmas Day in year 1.
**Why it happens:** Forgetting to test the round-trip of the zero value.
**How to avoid:** D-07 explicitly locks: zero `Date{}` emits `"0001-01-01"`. This is symmetric with `time.Time.MarshalJSON` semantics. Round-trip test:
```go
b, _ := json.Marshal(Date{})
require.Equal(t, []byte(`"0001-01-01"`), b)
var d Date
require.NoError(t, json.Unmarshal(b, &d))
require.True(t, d.IsZero())  // promoted from time.Time
```
**Warning signs:** No test for `MarshalJSON(Date{})`.

### Pitfall 5: `*APIError.Is` matching on more than `StatusCode`

**What goes wrong:** A future contributor extends `Is` to also match on `Path`. Now `errors.Is(err, &APIError{StatusCode: 404})` returns `false` because the in-memory `err` has `Path="/Subdivisions"` and the target has `Path=""`. Callers' status-code-based branching breaks silently.
**Why it happens:** "Make `Is` strict" feels like good defensive coding. It's not — for this pattern, looser matching with explicit wildcards is the design.
**How to avoid:** `Is` matches `*APIError` type identity + optional StatusCode-by-value. Path/Body/Message are ignored. Documented in the method godoc.
**Warning signs:** A PR that adds Path/Body/Message comparisons to `Is`. A test that asserts `errors.Is(err, &APIError{StatusCode: 404, Path: "/X"})` returns `false` for a `Path: "/Y"` error.

### Pitfall 6: Adding a public `init()` or package-level mutable `var`

**What goes wrong:** CLIENT-10 is violated; future tests can't isolate state; two consumers in the same binary fight.
**Why it happens:** "Default cache", "default logger", "package-wide config" all look convenient.
**How to avoid:** `grep -rn 'func init' --include='*.go'` MUST return nothing. Package-level `var`s are restricted to the 5 sentinels + `Version` const (constants don't count, but `Version` is a `const` anyway). A test asserts this; see Section "CLIENT-10 — Mechanical Verification".
**Warning signs:** Any `func init()` in any Phase 1 source file. Any package-level `var` not in the locked list.

### Pitfall 7: Slop / typo-squat in go.mod (defensive)

**What goes wrong:** A contributor adds `github.com/stretcher/testify` (note the typo `stretcher` vs `stretchr`) and the typo-squat package is malicious.
**Why it happens:** Common with NPM; rarer with Go modules because `proxy.golang.org` is a well-curated index, but not impossible.
**How to avoid:** Phase 1's `go.mod` MUST list only `github.com/stretchr/testify` (verified by slopcheck this session). Any future test-only dep must pass `slopcheck install --ecosystem go <pkg>` before landing per the Package Legitimacy Protocol.
**Warning signs:** Misspellings in `go.mod`; transitive dep we didn't add directly (testify v1.11.1 has indirect deps; `go mod why` should clarify).

## Runtime State Inventory

Phase 1 is **not a rename/refactor phase** — it is a **greenfield package**. The repo currently has only POC artifacts (stray `go.mod`, `go.sum` from `module holidays-poc`) and documentation. There is no runtime state to migrate.

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | None — no databases, no datastores in this repo. | None |
| Live service config | None — no external services configured. | None |
| OS-registered state | None — no scheduled tasks, daemons, or system services. | None |
| Secrets / env vars | None used by the library code. CI env vars (`OPENHOLIDAYS_LIVE`, `GOEXPERIMENT`) appear only in CI workflows (Phase 5, not Phase 1). | None |
| Build artifacts | The stray `go.mod` and `go.sum` at the repo root (`module holidays-poc`, `go 1.26.3`, POC dependencies) are NOT runtime state but ARE build artifacts that must be replaced. Side-effect of slopcheck: testify was added to that stray go.mod during this research; harmless because Phase 1 rewrites both files wholesale. | Phase 1 plan must include a task to delete `/data/git/private/holidays/go.mod` and `/data/git/private/holidays/go.sum`, then run `go mod init github.com/egeek-tech/go-openholidays` and `go mod edit -go=1.23`, then `go get github.com/stretchr/testify@v1.11.1`. |

**The canonical question** ("after every file in the repo is updated, what runtime systems still have the old string cached, stored, or registered?") has the answer: **nothing**. This is greenfield code; there is nothing to migrate.

## Code Examples

All examples in the "Architecture Patterns" section above are verified canonical implementations derived from the locked decisions in CONTEXT.md and the verified upstream schema. They are designed to drop into the corresponding Phase 1 source files (`date.go`, `errors.go`, `validate.go`, `types.go`).

Each example is annotated with its source (CONTEXT.md decision IDs, PITFALLS.md sections, verified stdlib behavior). Cross-references:

- `Date` type → Section "Pattern 1" (`date.go`)
- Sentinels + `*APIError` → Section "Pattern 2" (`errors.go`)
- Validators → Section "Pattern 3" (`validate.go`)
- Domain structs + `HolidayType` constants → Section "Pattern 4" (`types.go`)

For Phase 1 the planner can essentially translate these examples into the file structure; the testify tests follow Gold Rule 3 (one `TestXxx` per exported function, table-driven inside `t.Run`).

## Project Constraints (from CLAUDE.md)

Already enumerated in section "Project Constraints (from CLAUDE.md and CONVENTIONS.md)" above. The planner MUST verify every Phase 1 task and acceptance criterion against this list. Specifically:

| CLAUDE.md / CONVENTIONS.md Directive | Phase 1 Implication |
|--------------------------------------|---------------------|
| Rule 1: Everything in English | Every godoc, comment, error message, test name, commit message in English. Test fixtures for Phase 1 contain no non-English strings (no `testdata/` in Phase 1) |
| Rule 2: Verify or ask | Every claim in source comments and godoc must be verifiable from the upstream OpenAPI spec or Go stdlib docs. Researcher's job done; planner cites locked decisions; executor reads source |
| Rule 3: testify + 1-per-prod-function + t.Run | Test architecture is fully determined: 4 test files, one TestXxx per exported function, every case in t.Run, require for preconditions, assert for verifications |
| PROJECT.md: zero runtime deps | Phase 1 source files MUST NOT import non-stdlib packages. CI later will add `go vet -tags=integration ./...` to verify; here verified by code review |
| PROJECT.md: no init() | Phase 1 source files MUST NOT define `func init()`. Verified by grep test (see CLIENT-10 section below) |
| PROJECT.md: no global mutable state | Phase 1 declares no package-level `var` except the 5 sentinels. `Version` is `const`. Verified by code review + grep |
| PROJECT.md: every exported symbol has a doc comment | Every public symbol in `errors.go`, `date.go`, `types.go` gets a godoc comment starting with the symbol name |
| PROJECT.md: gofmt-clean | `gofmt -d ./...` and `go vet ./...` must pass in Phase 1 acceptance |
| PROJECT.md: error strings start with `"openholidays: "` | Each sentinel `errors.New` text matches; each `fmt.Errorf` template also matches |

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `type Date time.Time` (named alias) | `type Date struct { time.Time }` (wrapper struct) | Go community convention shift around 2019-2021 with `time.Time` API maturation | Wrapper struct preserves promoted methods — `d.Year()` works without conversion. Adopted by upstream `cloud.google.com/go/civil.Date`, `rickb777/date`, modern SDKs |
| Pre-Go-1.13: `errors.New` + string-matching in callers | Go 1.13+: `fmt.Errorf("%w", err)` + `errors.Is`/`errors.As` | Go 1.13 (October 2019) | Phase 1 uses the modern idiom |
| Per-test `if got != want { t.Fatalf(...) }` | `testify/assert` + `testify/require` + `t.Run` | Community standard since ~2018; locked by Gold Rule 3 | Phase 1 uses testify v1.11.1 |
| `math/rand` (v1, global seeded source) | `math/rand/v2` (Go 1.22+, per-instance) | Go 1.22 (Feb 2024) | Not used in Phase 1; relevant for retry jitter in Phase 4 |
| `iter.Seq[T]` not available | `iter.Seq[T]` available (Go 1.23+) | Go 1.23 (Aug 2024) | `Holiday.Range()` (Phase 3) uses it. Phase 1's go.mod directive `go 1.23` opens the door |
| Test fuzzing as third-party tools | `testing.F.Fuzz` in stdlib | Go 1.18 (Mar 2022) | Phase 1's `FuzzDateUnmarshal` uses stdlib |

**Deprecated/outdated:**
- `time.Parse` with manual UTC location — current stdlib `time.Parse` defaults to UTC, no `ParseInLocation` needed for our use case
- `golang-standards/project-layout`'s `pkg/` directory convention — widely considered an anti-pattern by Go core contributors; Phase 1 uses flat root layout per ARCHITECTURE.md

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | "`Date.toUTCMidnight` defensive normalization is necessary for `Equal`/`Before`/`After`/`Compare`" | Pattern 1 | If `Date` values always come through `NewDate` / `ParseDate` / `Date.UnmarshalJSON`, the normalization is redundant. But defending against user-constructed `Date{time.Date(..., loc)}` is cheap and prevents subtle bugs. Low risk |
| A2 | "The `slopcheck` Go ecosystem mode is reliable" | Package Legitimacy Audit | slopcheck has limited prior-art for the Go ecosystem compared to npm/PyPI; the `[OK]` result on testify is correct because testify is universally known. For future deps the risk is higher; manual cross-check against `pkg.go.dev` star count / publish date is still advisable. **[ASSUMED — recommend planner add manual cross-check task for any new test-only dep]** |
| A3 | "`Country.NameFor(lang)` is the right method name (vs. `Country.Name(lang)`)" | Pattern 4 | REQUIREMENTS.md TYPES-05 uses `Name(lang)` literally. The collision with the struct field `Name` means that exact name isn't possible. `NameFor` is one alternative; `LocalizedName` is another. The user needs to confirm — see Open Question Q2. **[ASSUMED]** |
| A4 | "Including `RegionalScope` and `TemporalScope` as plain `string` fields (not typed-string enums) for v0.1.0 is acceptable" | Upstream API Schema | Adding two more enum families adds 8+ exported constants and a typed-string per field. For v0.1.0 the marginal value is low (callers rarely branch on these). If a real usage pattern emerges that wants type safety, promote to enums in v0.2 non-breakingly. **[ASSUMED — recommend planner confirm with user]** |
| A5 | "Including `Tags []string` as a Phase 1 field on `Holiday` is acceptable" | Upstream API Schema | The field is in the verified spec but absent from REQUIREMENTS.md TYPES-01. Silently dropping it (no `Tags` field) makes the library lose information. Including it is additive and harmless. **[ASSUMED safe — recommend keeping]** |
| A6 | "`HolidayType` constants use PascalCase exact match (`HolidayTypeBackToSchool` with `Value = "BackToSchool"`)" | Pattern 4 | The values must match the upstream wire format exactly. The Go identifier naming is a separate question — `HolidayTypeBackToSchool` is the natural Go translation; alternatives include `HolidayTypeBack2School` (ugly), `HolidayBackToSchool` (drops prefix). The current proposal follows the `EnumValue = "Value"` convention seen in `aws-sdk-go-v2`. **[ASSUMED]** |
| A7 | "The unexported `errEmptyDate` is acceptable as the wrapped cause for D-06" | Pattern 1 | D-06 explicitly mandates an unexported sentinel. Risk: if a Phase 2+ caller wants to `errors.Is(err, openholidays.errEmptyDate)`, they can't (unexported). But the design assumes callers detect this via the higher-level `*APIError` or `ErrEmptyResponse` surface, never via the date parser. Low risk |
| A8 | "Slopcheck's modification of the stray POC go.mod is harmless" | Package Legitimacy Audit | The stray go.mod will be deleted in Phase 1 task 1 (greenfield rewrite). Verified untracked by git. **[VERIFIED]** |

## Open Questions

1. **TYPES-04 enum scope — 4 values, 6 values, or 8?**
   - What we know: Upstream OpenAPI spec defines exactly 6 values: `Public`, `Bank`, `Optional`, `School`, `BackToSchool`, `EndOfLessons` (verified 2026-05-27). REQUIREMENTS.md TYPES-04 lists 4: `Public`, `School`, `Bank`, `Observance`. The set `Observance` does not exist upstream; the three values `Optional`, `BackToSchool`, `EndOfLessons` are missing from the requirement.
   - What's unclear: Does the project want all 6 upstream values, just the 4 the requirement mentioned (which would crash on real responses containing the other 3), or a different set entirely? Was "Observance" copied from a competing library (e.g., `rickar/cal`)?
   - Recommendation: **Ship all 6 upstream values exactly**, drop `Observance`. Document deviation as CL-04 in PROJECT.md Key Decisions. Rationale: the requirement text appears to be out-of-date relative to the actual upstream; any "ignore unknown values" lenient approach silently loses data. Confirm with user during discuss-phase or by adding a Key Decisions entry during execute-phase.

2. **TYPES-05 accessor method name — `Name(lang)` collides with `Name` field; what's the rename?**
   - What we know: REQUIREMENTS.md TYPES-05 literally says `Name(lang string) string` — but `Country.Name` is the `[]LocalizedText` field per upstream schema. A method cannot share a name with a field.
   - What's unclear: User intent — was `Name(lang)` shorthand, expecting a rename? Did the requirement author intend the field to be called something else (e.g., `Names` plural)?
   - Recommendation: **Rename method to `NameFor(lang)`**. Less invasive than renaming the field (which would change JSON tag handling expectations and break the wire shape). Document deviation as CL-05 in PROJECT.md Key Decisions. The same rename will apply to Phase 3's `Holiday.Name(lang)` → `Holiday.NameFor(lang)` (HELP-01).

3. **D-15 `Is(target)` pattern — is "match by StatusCode wildcard on zero value" the right idiom?**
   - What we know: CONTEXT.md D-15 cites `*os.PathError` and `*net.OpError` as precedents for this pattern. Verified 2026-05-27: neither `*fs.PathError` (which `*os.PathError` aliases) nor `*net.DNSError` implements an `Is(target) bool` method — they rely on `Unwrap()`. The "wildcard by zero value" pattern is real (used by `aws-sdk-go-v2` and `gocloud.dev/gcerrors`) but is NOT stdlib idiom.
   - What's unclear: Does the project want strict stdlib idiom (drop `Is`, rely solely on `errors.As`), or keep the productivity boost of `errors.Is(err, &APIError{StatusCode: 404})`?
   - Recommendation: **Keep the pattern as locked in D-15.** Document the deviation from stdlib idiom in a godoc comment on the `Is` method (this RESEARCH.md's example does that). Strict stdlib would require callers to write four lines instead of one for "is this a 404"; the productivity win justifies the convention. The aws-sdk-go-v2 precedent gives this pattern enough credibility.

4. **`go.mod` `go` directive — `go 1.23` or `go 1.23.0`?**
   - What we know: Both forms are valid per the Go spec `[VERIFIED: go.dev/ref/mod]`. `go 1.23.0` is shown in the go.dev examples; ROADMAP success criterion #5 cites `go 1.23` (without minor).
   - What's unclear: Project preference.
   - Recommendation: **Use `go 1.23`** to match ROADMAP literal wording. Lower-precision form is more permissive (admits any 1.23.x), which is what the project wants.

5. **`toolchain` directive — include or omit?**
   - What we know: `toolchain go1.23.0` would suggest a specific toolchain version; without it, callers' toolchains may auto-upgrade to whatever matches their installed `go` binary.
   - What's unclear: Does the project want to pin to a specific toolchain?
   - Recommendation: **Omit the `toolchain` directive** for v0.1.0. The library targets Go 1.23+; pinning the toolchain creates friction for downstream consumers. Add later if a specific minor version becomes important.

## Environment Availability

Phase 1 has no external runtime dependencies beyond the Go toolchain. Verified availability:

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| `go` toolchain ≥ 1.23 | Build, test, format | ✓ | go1.26.3 (per stray POC `go.mod`) | — |
| `gofmt` | Code formatting | ✓ | bundled with `go` | — |
| `go vet` | Static analysis | ✓ | bundled with `go` | — |
| Internet access (transient) | One-time `go get github.com/stretchr/testify@v1.11.1` | ✓ | (verified during slopcheck install) | The user can mirror or vendor; standard `GOPROXY` env var works for offline mirrors |
| `slopcheck` | Package legitimacy audit | ✓ | 0.6.1 (installed during research) | If unavailable, all packages tagged `[ASSUMED]` per Package Legitimacy Protocol; planner gates installs behind `checkpoint:human-verify` |
| `git` | Version control / commits | ✓ | (project is a git repo per env context) | — |

**Missing dependencies with no fallback:** none
**Missing dependencies with fallback:** none

Phase 1 is purely Go source code. No databases, services, runtimes beyond `go`, or external tools required.

## Validation Architecture

**Nyquist validation enabled** (`config.json: workflow.nyquist_validation: true` — verified by Read).

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` + `github.com/stretchr/testify` v1.11.1 (test-only) |
| Config file | None — Go's `testing` package needs no config |
| Quick run command | `go test -race ./...` (Phase 1 has no large fixtures; full suite is fast) |
| Full suite command | `go test -race -cover ./...` |
| Fuzz run command (Phase 1 only) | `go test -fuzz=FuzzDateUnmarshal -fuzztime=30s ./...` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| TYPES-01 | `Holiday` decodes from canonical OpenHolidays JSON shape; all fields populated correctly including nullable `Comment`/`Subdivisions`/`Groups`/`Tags`; `Quality` field tolerated as schema drift | unit (table-driven) | `go test -race -run 'TestHoliday_JSON' ./...` | ❌ Wave 0 (`types_test.go`) |
| TYPES-02 | `Date.MarshalJSON`/`UnmarshalJSON` round-trips `"2025-12-24"` → `Date{2025-12-24 UTC midnight}` → `"2025-12-24"`; rejects `null` and `""` with errors; emits `"0001-01-01"` for zero `Date{}` | unit + fuzz | `go test -race -run 'TestDate_MarshalJSON|TestDate_UnmarshalJSON|TestNewDate|TestParseDate' ./...` and `go test -fuzz=FuzzDateUnmarshal -fuzztime=30s ./...` | ❌ Wave 0 (`date_test.go`) |
| TYPES-03 | `LocalizedText` and `SubdivisionRef` decode from JSON with verified field names (`language`/`text`, `code`/`shortName`) | unit | `go test -race -run 'TestLocalizedText_JSON|TestSubdivisionRef_JSON' ./...` | ❌ Wave 0 |
| TYPES-04 | `HolidayType` constants exist for all 6 upstream values; values match wire-format exactly; `Holiday.Type` decodes correctly | unit | `go test -race -run 'TestHolidayType' ./...` | ❌ Wave 0 |
| TYPES-05 | `Country.NameFor(lang)` / `Language.NameFor(lang)` / `Subdivision.NameFor(lang)` return localized text; falls back to first entry on miss; returns `""` only when slice is empty; case-insensitive language match | unit (table-driven) | `go test -race -run 'TestCountry_NameFor|TestLanguage_NameFor|TestSubdivision_NameFor' ./...` | ❌ Wave 0 |
| ERR-01 | All 5 sentinels are non-nil, error message starts with `"openholidays: "`, distinct identities | unit | `go test -race -run 'TestSentinelErrors' ./...` | ❌ Wave 0 (`errors_test.go`) |
| ERR-02 | `errors.As(wrappedErr, &apiErr)` extracts an `*APIError` from a Phase-1-constructed literal wrapped via `%w`; populated `StatusCode`, `Path`, `Body`, `Message` fields | unit | `go test -race -run 'TestAPIError_ErrorsAs' ./...` | ❌ Wave 0 |
| ERR-03 | `errors.Is(fmt.Errorf("country %q: %w", "ZZZ", ErrInvalidCountry), ErrInvalidCountry)` returns `true`; ditto for all 5 sentinels | unit (table-driven) | `go test -race -run 'TestSentinels_ErrorsIs' ./...` | ❌ Wave 0 |
| ERR-04 | Validator error messages contain only the offending code/date value (no secrets); `APIError.Body` is the only field that holds raw body bytes | unit | `go test -race -run 'TestValidators_NoSensitiveData' ./...` | ❌ Wave 0 |
| VALID-01 | `validateCountry` accepts `"PL"`, `"pl"`, `"Pl"`; canonicalizes to `"PL"`; rejects `""`, `"P"`, `"POL"`, `"P1"`, `"ZŻ"` (non-ASCII), `" PL"` (whitespace) | unit (table-driven) | `go test -race -run 'TestValidateCountry' ./...` | ❌ Wave 0 (`validate_test.go`) |
| VALID-02 | `validateDateRange(2025-12-31, 2025-01-01)` returns wrapped `ErrInvalidDateRange`; equal dates accepted | unit (table-driven) | `go test -race -run 'TestValidateDateRange' ./...` | ❌ Wave 0 |
| VALID-03 | `validateDateRange(2025-01-01, 2028-01-01)` accepted (exactly 3 years); `validateDateRange(2025-01-01, 2028-01-02)` rejected with wrapped `ErrDateRangeTooLarge`; leap-year boundary tests (`2024-02-29 → 2027-02-28` pass, `2024-02-29 → 2027-03-01` fail) | unit (table-driven) | `go test -race -run 'TestValidateDateRange' ./...` | ❌ Wave 0 |
| VALID-04 | `validateLanguage` accepts `"pl"`, `"PL"`, `"Pl"`; canonicalizes to `"pl"`; rejects `""`, `"p"`, `"pol"`, `"p1"`, `"ŻŻ"` | unit (table-driven) | `go test -race -run 'TestValidateLanguage' ./...` | ❌ Wave 0 |
| CLIENT-10 | No `func init()` anywhere in `*.go` source files (excluding `*_test.go`); no package-level `var` other than the 5 sentinels; `Version` is `const` | unit (filesystem scan) + integration | `go test -race -run 'TestNoInitOrGlobalState' ./...` + manual `grep -rn 'func init' --include='*.go'` returns no matches | ❌ Wave 0 |

### Sampling Rate

- **Per task commit:** `go test -race -run TestXxx ./...` for the affected file (subsecond)
- **Per wave merge:** `go test -race -cover ./...` (full suite — under 5 seconds for Phase 1)
- **Phase gate:** `go test -race -cover ./...` + `go test -fuzz=FuzzDateUnmarshal -fuzztime=30s ./...` + `gofmt -d ./...` + `go vet ./...` all green before `/gsd:verify-work`

### Wave 0 Gaps

- [ ] `errors_test.go` — TestSentinelErrors, TestAPIError_Error, TestAPIError_Is, TestAPIError_ErrorsAs, TestSentinels_ErrorsIs (ERR-01 .. ERR-03)
- [ ] `date_test.go` — TestNewDate, TestParseDate, TestDate_MarshalJSON, TestDate_UnmarshalJSON, TestDate_String, TestDate_Equal, TestDate_Before, TestDate_After, TestDate_Compare, TestDate_DaysUntil, FuzzDateUnmarshal (TYPES-02, D-12)
- [ ] `types_test.go` — TestHoliday_JSON (full fixture), TestLocalizedText_JSON, TestSubdivisionRef_JSON, TestGroupRef_JSON, TestCountry_NameFor, TestLanguage_NameFor, TestSubdivision_NameFor, TestHolidayType_String (TYPES-01, TYPES-03, TYPES-04, TYPES-05)
- [ ] `validate_test.go` — TestValidateCountry, TestValidateLanguage, TestValidateDateRange (VALID-01 .. VALID-04)
- [ ] `TestNoInitOrGlobalState` (CLIENT-10) — a filesystem-level test that walks `*.go` files (excluding `*_test.go`) and asserts no `init` function declaration and no `var` block defining anything outside the locked sentinel list. Use `go/parser` + `go/ast` for a robust check
- [ ] Framework install — `go get github.com/stretchr/testify@v1.11.1` (already verified in this session; Phase 1 will repeat against the rewritten `go.mod`)
- [ ] `go.mod`/`go.sum` Wave 0 task — delete stray POC files, run `go mod init github.com/egeek-tech/go-openholidays`, `go mod edit -go=1.23`, `go get github.com/stretchr/testify@v1.11.1`, `go mod tidy`

## Security Domain

Per `.planning/config.json`, security enforcement is the default. Phase 1 is a pure types-and-validators package; its security surface is narrow.

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | OpenHolidays is keyless; no auth in Phase 1 or anywhere |
| V3 Session Management | no | Library has no sessions |
| V4 Access Control | no | Library is consumer code; access control is the caller's concern |
| V5 Input Validation | **yes** | All three validators (`validateCountry`, `validateLanguage`, `validateDateRange`) reject malformed input client-side before any network call (Phase 2+) is built. ASVS V5.1.3: "Verify that the application has defenses against HTTP parameter pollution attacks" — Phase 1 enforces strict 2-letter ASCII codes and reasonable date ranges |
| V6 Cryptography | no | No crypto in Phase 1 |
| V8 Data Protection | partial | ERR-04 (no secrets in error messages) is the relevant control. Validator error messages include only the offending code/date value, which are user-supplied non-secret strings |
| V9 Communications | no | No HTTP in Phase 1 |
| V11 Configuration | no | No config files in Phase 1 |
| V13 API and Web Service | partial | The library defines its API surface (the type contract). V13.3.1 (verify input is validated) is satisfied by Phase 1's validators; V13.3.4 (limit on input size) is satisfied by `validateDateRange` (3-year cap) |
| V14 Configuration | no | No build config files in Phase 1 (those land in Phase 5) |

### Known Threat Patterns for Phase 1's stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Malicious date strings (DoS via `time.Parse` slow input) | Denial-of-service | `time.Parse` is O(len(s)) and resistant; `Date.UnmarshalJSON` further rejects empty strings and non-string JSON early. `FuzzDateUnmarshal` provides defense-in-depth |
| Slop/typo-squat in test deps | Supply-chain | `slopcheck install --ecosystem go` before any dep gets added; only `testify` v1.11.1 (verified clean) approved for Phase 1 |
| Information leakage via error messages | Information disclosure | ERR-04: validator errors carry only the offending code/date value, which are non-secret. `APIError.Body` is bounded to 4 KiB (D-17, enforced in Phase 2) |
| `init()` side effects modifying global state | Tampering | CLIENT-10 forbids `init()` and global mutable state; mechanical verification via grep + AST-walk test |
| Sentinel error spoofing (caller crafting their own `ErrInvalidCountry`) | Spoofing | Sentinels are exported `var`s; callers cannot reassign them (they're top-level `var`s, but Go semantics make this awkward and easily caught at code review). The risk is mitigated by `errors.Is` being identity-based on the pointer value of the sentinel |
| Year-overflow in `validateDateRange` (e.g., `from = year 9999, to = year 9999 + 3`) | Denial-of-service / undefined behavior | `time.Time.AddDate` is well-defined for any year; no integer overflow at typical inputs. Tests should cover extreme-year inputs (`year 9999`, `year 1`) |

## Sources

### Primary (HIGH confidence)
- **OpenHolidays OpenAPI spec** — `https://openholidaysapi.org/swagger/v1/swagger.json`. Verified 2026-05-27 via WebFetch. Exact field names, types, nullability, and enum values for `HolidayResponse`, `LocalizedText`, `SubdivisionReference`, `GroupReference`, `CountryResponse`, `LanguageResponse`, `SubdivisionResponse`, `ProblemDetails`. All endpoint parameters and their required/optional status.
- **Go release notes & module reference** — `https://go.dev/ref/mod`, `https://go.dev/doc/go1.23`, `https://go.dev/doc/devel/release`. Verified 2026-05-27. `go` directive syntax (`go 1.23` or `go 1.23.0` both valid), `iter.Seq` introduced in Go 1.23, Go 1.23 / 1.24 currently supported.
- **Go stdlib docs** — `https://pkg.go.dev/errors`, `https://pkg.go.dev/fmt`, `https://pkg.go.dev/time`, `https://pkg.go.dev/testing#F`. Verified 2026-05-27. `errors.Is` invokes `Is(target) bool`; `fmt.Errorf` supports `%w` with other verbs in same call; `time.Parse("2006-01-02", s)` returns UTC; `time.Time.AddDate` is calendar-aware; `testing.F.Fuzz` available since Go 1.18.
- **`github.com/stretchr/testify` v1.11.1** — verified via `pkg.go.dev/github.com/stretchr/testify` and `slopcheck install` (`[OK]`, 2026-05-27).

### Secondary (MEDIUM confidence)
- `.planning/research/ARCHITECTURE.md` — Patterns 3, 5, 6 (custom Date type, validation as unexported package functions, error construction at the method layer). Internally consistent with Go SDK conventions (go-github, stripe-go, hashicorp/go-retryablehttp).
- `.planning/research/PITFALLS.md` — sections JSON-1 / JSON-3 / JSON-4 / TZ-1 / TZ-2 / TZ-3 / OH-2 / OH-3. Cross-verified against OpenHolidays spec.
- `.planning/research/STACK.md` — Go 1.23 floor, stdlib-only baseline, testify approval.
- `.planning/codebase/CONVENTIONS.md` — Gold Project Rules (English-only, verify-or-ask, testify+t.Run).

### Tertiary (LOW confidence — none in this research)
- All claims herein are either verified against authoritative sources or explicitly tagged `[ASSUMED]` in the Assumptions Log.

## Metadata

**Confidence breakdown:**
- Standard stack (Go 1.23, testify v1.11.1): HIGH — verified against go.dev release pages and slopcheck.
- Upstream API schema (Holiday/Country/Language/Subdivision/LocalizedText/SubdivisionRef/GroupRef): HIGH — verified against live OpenAPI spec on 2026-05-27.
- Date type canonical implementation: HIGH — verified `time.Parse` semantics and PITFALLS JSON-3 template.
- Sentinel + `*APIError` shape: HIGH for type shape; MEDIUM for the `Is(target)` wildcard convention (stdlib precedents cited in CONTEXT.md D-15 are weaker than stated — see Open Question Q3).
- Validators: HIGH — verified stdlib `strings.ToUpper`/`ToLower` and calendar arithmetic.
- `HolidayType` enum scope (6 vs 4 vs 8 values): HIGH for the verified spec, but **open issue** until user confirms the deviation from REQUIREMENTS.md TYPES-04 (Open Question Q1).
- `RegionalScope` / `TemporalScope` / `Tags` fields: HIGH for the spec; MEDIUM for the recommendation to ship them as plain strings rather than typed enums (Assumption A4).
- CLIENT-10 verification approach: HIGH — `go/parser` + `go/ast` walk is the canonical way to verify "no init()" at test time.

**Research date:** 2026-05-27
**Valid until:** 2026-06-26 (30 days — spec stable, Go 1.23 stable, testify v1 stable; refresh if upstream OpenHolidays API changes or testify ships v2).
