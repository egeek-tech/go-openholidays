# Phase 1: Foundation - Context

**Gathered:** 2026-05-27
**Status:** Ready for planning

<domain>
## Phase Boundary

Deliver the zero-dependency Go package that exposes the public type contract of `go-openholidays`:

- Domain types (`Holiday`, `Country`, `Language`, `Subdivision`, `LocalizedText`, `SubdivisionRef`).
- Custom `Date` type with `MarshalJSON`/`UnmarshalJSON`, comparison helpers, constructor, parser, and `DaysUntil`.
- Sentinel errors (`ErrInvalidCountry`, `ErrInvalidLanguage`, `ErrDateRangeTooLarge`, `ErrEmptyResponse`, `ErrInvalidDateRange`) and the `*APIError{StatusCode, Path, Body, Message}` type (construction lands in Phase 2).
- Client-side validators (`validateCountry`, `validateLanguage`, `validateDateRange`) as unexported root-package functions.
- `go.mod` declaring `module github.com/egeek-tech/go-openholidays` and `go 1.23`.
- `LICENSE` (MIT) and `version.go` (`const Version = "0.1.0"`).
- No `init()` side effects, no global mutable state (CLIENT-10).

What this phase does NOT deliver: `Client`, options, transport, endpoints, helpers (`Name`, `IsInRegion`, `Range`), retry, cache, observability, CLI, CI, docs. All of those depend on the type contract being stable.

</domain>

<decisions>
## Implementation Decisions

### Module path and packaging

- **D-01:** `go.mod` declares `module github.com/egeek-tech/go-openholidays`. This resolves PROJECT.md's deferred REL-04 module-path-owner decision for the entire library lifecycle (not just for Phase 1).
- **D-02:** Every `.go` file declares `package openholidays`. Repo `go-openholidays` → package `openholidays`, matching go-github / stripe-go convention.
- **D-03:** A dedicated `version.go` holds `const Version = "0.1.0"`. Single source of truth for Phase 2's `User-Agent: go-openholidays/<version>` and Phase 5's CLI `--version` flag. `-ldflags '-X github.com/egeek-tech/go-openholidays.Version=...'` injection works out of the box.
- **D-04:** Phase 1 ships the MIT `LICENSE` at the repo root from the first commit. README and other distribution artifacts remain in Phase 5.

### Date type

- **D-05:** `type Date struct { time.Time }` — wrapper struct that embeds `time.Time` so all stdlib methods are promoted. Every `Date` is normalized to **UTC midnight** at construction time (Pitfall TZ-1 mitigation).
- **D-06:** `UnmarshalJSON(b []byte) error` returns errors for both `null` and empty-string inputs (locks ROADMAP success criterion #1). The error is built with `fmt.Errorf("...: %w", errEmptyDate)` against an **unexported** `errEmptyDate` sentinel — kept out of the public sentinel list so the locked 5-sentinel surface is not expanded.
- **D-07:** `MarshalJSON()` always emits `"YYYY-MM-DD"`. Zero `Date{}` round-trips to `"0001-01-01"` — symmetric with `time.Time.MarshalJSON` semantics. Callers detect missing dates via `Date.IsZero()`.
- **D-08:** `Date.String() string` overrides the embedded `time.Time.String()` to return `Format("2006-01-02")`. Same shape as JSON, friendly for the CLI table output (Phase 5).
- **D-09:** `Date` defines its own `Equal`, `Before`, `After`, `Compare` methods that internally normalize both operands to UTC midnight before delegating to `time.Time`. These shadow the embedded methods so callers cannot accidentally compare a non-UTC-constructed Date.
- **D-10:** `func (d Date) DaysUntil(other Date) int` ships in Phase 1 — DST-correct inclusive day count (Pitfall TZ-2 mitigation). Phase 3's `Holiday.Days()` and `Holiday.Range()` call this.
- **D-11:** Constructors ship in Phase 1: `NewDate(year int, month time.Month, day int) Date` (always UTC midnight) and `ParseDate(s string) (Date, error)` (delegates to `UnmarshalJSON` semantics).
- **D-12:** `FuzzDateUnmarshal` ships in Phase 1 alongside the unit tests (overrides ROADMAP placement of fuzz in Phase 5). Pitfall JSON-3 mandates a fuzz target for every custom unmarshaler; not waiting four phases to surface regressions.

### Errors

- **D-13:** Exported sentinels in Phase 1: `ErrInvalidCountry`, `ErrInvalidLanguage`, `ErrDateRangeTooLarge`, `ErrEmptyResponse`, **`ErrInvalidDateRange`** (new — for `validFrom > validTo`). This expands ROADMAP success criterion #2 from 4 to 5 sentinels — see "Scope clarifications" below for rationale and the required Key Decisions entry.
- **D-14:** `*APIError` carries `{StatusCode int, Path string, Body []byte, Message string}`. `Message` is best-effort parsed from upstream JSON shape (`{"error": "..."}` / `{"detail": "..."}` / `{"title": "..."}`) by Phase 2's endpoint methods; empty string when unparseable. Brief minimum was 3 fields; `Message` is additive.
- **D-15:** `*APIError` implements `Is(target error) bool` so `errors.Is(err, &openholidays.APIError{StatusCode: 404})` matches by status. If `target.StatusCode == 0`, it matches any `*APIError` (i.e., "was this an API error?"). Idiomatic per `*os.PathError` and `*net.OpError`.
- **D-16:** `*APIError` does **not** implement `Unwrap()` — it is a leaf error type. Phase 2 endpoint methods return either a transport-level error (wrapped with `%w`) OR a freshly-constructed `*APIError`, never both.
- **D-17:** `APIError.Body` is capped at **4 KiB** when populated (Phase 2). 10 MiB cap applies only to successful response decode (Pitfall HTTP-4); error envelopes are bounded smaller.
- **D-18:** `(e *APIError) Error() string` returns `"openholidays: api error <status> at <path>: <message>"`. When `Message` is empty, the suffix is omitted, producing `"openholidays: api error <status> at <path>"`.
- **D-19:** Phase 1 ships the `APIError` type, `Error()`, `Is()`, and tests for those methods. Construction (reading `resp.Body`, parsing `Message`) lands in Phase 2 alongside the first endpoint, since Phase 1 has no `*http.Response` to draw from. Tests in Phase 1 construct `APIError` literals manually to verify `errors.As` round-trips per ROADMAP criterion #3.

### Validators

- **D-20:** `validateCountry(code string) (string, error)` — accepts any case, **canonicalizes to uppercase** via `strings.ToUpper`, then verifies the result is exactly 2 ASCII letters in `[A-Z]`. Returns the canonicalized form for the caller to send on the wire. Deviates from VALID-01's literal "2 uppercase ASCII letters" by accepting mixed-case input — see "Scope clarifications".
- **D-21:** `validateLanguage(code string) (string, error)` — accepts any case, **canonicalizes to lowercase** via `strings.ToLower`, verifies the result is exactly 2 ASCII letters in `[a-z]`. No allowlist — shape-only check (allowlist deferred indefinitely; the `Languages` endpoint is the source of truth for what the upstream supports).
  - **[REVERSED 2026-05-30, quick task 260530-dvc]** `validateLanguage` now canonicalizes to **UPPERCASE** via `strings.ToUpper`, not lowercase. Verified live: the OpenHolidays API is case-sensitive and a lowercase `languageIsoCode` silently returns English-only names. See `validate.go` and the matching `validateCountry` analog.
- **D-22:** `validateDateRange(from, to Date) error` — returns `ErrInvalidDateRange` (wrapped with %w + value context) when `from.After(to)`; returns `ErrDateRangeTooLarge` when `to.Before(from.AddDate(3, 0, 1))` is false (i.e., the window exceeds 3 calendar years inclusive). Uses calendar arithmetic via `AddDate`, not duration math, so leap years are handled correctly.
- **D-23:** Validator errors wrap their sentinel with %w and include the offending value: `fmt.Errorf("%w: %q", ErrInvalidCountry, code)`. Caller error messages look like `openholidays: invalid country code: "pl"`. Country/language/date values are not secrets — documenting the "no secrets in error messages" rule remains a global library invariant.

### Scope clarifications (deviations from ROADMAP literal wording)

- **CL-01:** ROADMAP success criterion #2 lists 4 sentinels; Phase 1 ships 5 (`ErrInvalidDateRange` added). Rationale: two semantically distinct failure modes (`from > to` vs `range > 3 years`) deserve two `errors.Is`-distinguishable identities. Must be recorded in PROJECT.md `Key Decisions` before Phase 1 closes.
- **CL-02:** VALID-01 says "2 uppercase ASCII letters; non-empty" — Phase 1 ships a case-insensitive validator that canonicalizes to uppercase. Wire format remains uppercase. Justification: ergonomic input parity with `validateLanguage`'s case-insensitive lowercase behavior. Must be recorded in PROJECT.md `Key Decisions`.
- **CL-03:** ROADMAP places fuzz (TEST-07) in Phase 5; Phase 1 ships `FuzzDateUnmarshal` early per Pitfall JSON-3 mandate. Other fuzz targets (`FuzzParseLocalizedText`, `FuzzUnmarshalHoliday`) still land in Phase 5.

### Claude's Discretion

The following are inferred from already-locked architecture and conventions; no need to re-ask:

- File layout per ARCHITECTURE.md: `types.go`, `date.go`, `errors.go`, `validate.go`, `doc.go`, `version.go`, plus `*_test.go` siblings.
- `package openholidays` for production files; `package openholidays` for in-package tests (access to unexported helpers); `package openholidays_test` for `example_test.go` (external-consumer view, deferred to Phase 5).
- testify v1 (assert + require) added under `require` in `go.mod`. Verify `go mod why github.com/stretchr/testify` reports only test imports.
- Every exported symbol gets a godoc comment starting with the symbol name (Rule from PROJECT.md constraints).
- All error message strings start with `"openholidays: "` (Go stdlib convention).

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project baseline (read first)
- `.planning/PROJECT.md` — what we're building, locked constraints, Key Decisions table.
- `.planning/REQUIREMENTS.md` — 82 v1 requirements with traceability (Phase 1 owns 14: TYPES × 5, ERR × 4, VALID × 4, CLIENT-10).
- `.planning/ROADMAP.md` §"Phase 1: Foundation" — goal + 5 success criteria that must be TRUE at phase end.
- `.planning/STATE.md` §"Key Decisions Locked In" — running ledger of decisions inherited from project-init.
- `.planning/codebase/CONVENTIONS.md` — Gold Project Rules (English-only, verify-or-ask, testify+one-test-per-prod-func+t.Run).

### Architecture and design (read before writing types)
- `.planning/research/ARCHITECTURE.md` §"Pattern 3: Custom Date Type Over Per-Field Unmarshal" — wrapper-struct rationale, JSON shape.
- `.planning/research/ARCHITECTURE.md` §"Pattern 5: Validation as Unexported Package Functions" — validator signatures, placement.
- `.planning/research/ARCHITECTURE.md` §"Pattern 6: Error Construction at the Method Layer, Sentinels at Package Level" — `*APIError` shape, `Is(target)` semantics, sentinel-error idiom.
- `.planning/research/ARCHITECTURE.md` §"Recommended Project Structure" — file layout that Phase 1 establishes.
- `.planning/research/STACK.md` §"Core Technologies" — Go 1.23 floor, stdlib-only baseline, testify approval.

### Pitfalls (read before writing any unmarshaler or validator)
- `.planning/research/PITFALLS.md` §"Pitfall JSON-1: Strict decoding by default" — lenient by default in Phase 2+; Phase 1 just needs to not enforce strictness on `Date.UnmarshalJSON`.
- `.planning/research/PITFALLS.md` §"Pitfall JSON-3: Custom UnmarshalJSON that ignores null or empty string" — mandatory pattern that `Date.UnmarshalJSON` follows.
- `.planning/research/PITFALLS.md` §"Pitfall JSON-4: time.Time zero value passing as a valid date" — explains why `null`/empty must return errors per ROADMAP criterion #1.
- `.planning/research/PITFALLS.md` §"Pitfall TZ-1: YYYY-MM-DD ambiguity — UTC vs local TZ" — locks UTC-midnight normalization.
- `.planning/research/PITFALLS.md` §"Pitfall TZ-2: DST off-by-one in date arithmetic" — drives `DaysUntil` implementation.
- `.planning/research/PITFALLS.md` §"Pitfall TZ-3: Treating a multi-day school holiday as a single date" — locks `Holiday` having both `StartDate` and `EndDate`.

### Upstream API surface (verify shape against this before writing types)
- `https://openholidaysapi.org/swagger/v1/swagger.json` — OpenAPI 3 spec. Researcher (Phase 1) should hit this live and confirm field names/nullability for `Holiday`, `Country`, `Language`, `Subdivision`, `LocalizedText`, `SubdivisionRef`. Specifically validate: `Holiday.Type` enum string casing (`"Public"` vs `"public"`), `comment`/`subdivisions`/`groups` nullable flags (per PITFALL JSON-2), `quality` field handling (not in spec but observed in POC responses).

### Gold Project Rules (apply everywhere)
- `CLAUDE.md` §"Project Rules (Gold)" — Rule 1 (English-only), Rule 2 (verify-or-ask), Rule 3 (testify + t.Run + one-test-per-prod-function).

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets

None — this is a greenfield package. The repo currently contains `go.mod` (module `holidays-poc`, Go 1.26.3) and `go.sum` from a removed POC; Phase 1 rewrites `go.mod` and deletes `go.sum`. No `.go` files exist in the repo as of this discussion.

### Established Patterns

- **English-only invariant** (CONVENTIONS.md Rule 1). Every identifier, comment, test name, error string is English. Exception: `testdata/` fixtures with real upstream responses (e.g., `"Wigilia Bożego Narodzenia"`).
- **testify-based tests** (CONVENTIONS.md Rule 3). One `TestXxx` per exported production function, every case in `t.Run`, `require` for preconditions, `assert` for verifications.

### Integration Points

- `go.mod` → consumed by every Go file and every CI step in later phases.
- Sentinel error identities (`ErrInvalidCountry`, …) → Phase 2 endpoint methods wrap with %w when validators reject; Phase 3 endpoint tests assert via `errors.Is`.
- `Date` type → embedded in every `Holiday.StartDate`/`Holiday.EndDate` field (Phase 3); Phase 3 `Holiday.Days()` calls `StartDate.DaysUntil(EndDate)`.
- `*APIError` type → constructed in Phase 2 endpoint methods on non-2xx responses; never reached by Phase 1 code.
- testify is added to `go.mod` `require` block in Phase 1; first import lands in Phase 1 test files (e.g., `date_test.go`).

</code_context>

<specifics>
## Specific Ideas

- The CLI (`cmd/ohcli`, Phase 5) and the live POC referenced in PROJECT.md will both consume `Date.String()` for table output — keep the `YYYY-MM-DD` format stable.
- `ParseDate` is intended to back the CLI `--year 2025` and `--from 2025-01-01` parsing, so it must accept the exact `YYYY-MM-DD` string the CLI flag receives. Trim/lowercase/etc. happens before `ParseDate` if needed — Phase 1's ParseDate is strict.
- The bumped 5-sentinel surface and the case-insensitive country validator are deliberate deviations from the literal text of ROADMAP / REQUIREMENTS. Phase 1's executor must add the Key Decisions entries before phase verification passes.

</specifics>

<deferred>
## Deferred Ideas

- **Language allowlist** (closed set of ISO 639-1 codes) — D-21 ships shape-only validation; allowlist enforcement deferred (likely never, since the `Languages` endpoint is the upstream source of truth).
- **`*APIError.Unwrap()` returning an internal cause** (D-16 rejected) — can be added in v0.2 non-breakingly if a real pattern surfaces.
- **Typed validation error struct** (D-23 rejected) — `*ValidationError{Field, Value, Reason}` was considered; sentinels + `%w` are the idiomatic answer for v0.1.0.
- **Exported `NewAPIError` constructor** — rejected as premature; can be added later if downstream consumers need to fabricate `*APIError` values for their own middleware/tests.
- **`Date.MarshalText`/`UnmarshalText`** for `database/sql` interop — out of scope for v0.1.0 OpenHolidays use case; flag for v0.2 if a consumer needs it.

</deferred>

---

*Phase: 1-Foundation*
*Context gathered: 2026-05-27*
