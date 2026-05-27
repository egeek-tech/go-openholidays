# Phase 1: Foundation - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-05-27
**Phase:** 1-Foundation
**Areas discussed:** Module path resolution, Date type internal representation, APIError shape, Validation error vocabulary

---

## Module path resolution

### Q1 — Module path for go.mod

| Option | Description | Selected |
|--------|-------------|----------|
| `github.com/rtkocz/go-openholidays` | Personal GitHub namespace; cheap to publish; can be transferred later via rename + redirect. | |
| `github.com/PLACEHOLDER/go-openholidays` | Explicit placeholder; breaks `go install` until renamed. | |
| `go-openholidays` (no github prefix) | Local module name; hardest to publish later. | |
| `github.com/egeek-tech/go-openholidays` | Free-text response. | ✓ |

**User's choice:** `https://github.com/egeek-tech/go-openholidays` — normalized to `github.com/egeek-tech/go-openholidays` (Go module paths drop the scheme).
**Notes:** This resolves PROJECT.md's deferred REL-04 module-path-owner decision for the entire library lifecycle.

### Q2 — Package name in .go files

| Option | Description | Selected |
|--------|-------------|----------|
| `openholidays` | Strip `go-` prefix per Go convention (go-github / stripe-go). | ✓ |
| `goopenholidays` | Match repo verbatim minus hyphen. | |
| `holidays` | Shortest; risks identifier collision with rickar/cal etc. | |

**User's choice:** `openholidays`.

### Q3 — Version constant location

| Option | Description | Selected |
|--------|-------------|----------|
| `version.go` at root | Single source of truth; `-ldflags` injectable. | ✓ |
| Inline in `doc.go` | Co-locate with package overview. | |
| `internal/version` package | Hide from callers; adds indirection. | |

**User's choice:** `version.go` at root.

### Q4 — LICENSE timing

| Option | Description | Selected |
|--------|-------------|----------|
| Ship LICENSE in Phase 1 | MIT at root from commit #1. | ✓ |
| Defer LICENSE to Phase 5 | Strict ROADMAP adherence. | |
| LICENSE + minimal README stub | OSS hygiene + WIP marker. | |

**User's choice:** Ship LICENSE in Phase 1. README stays in Phase 5.

---

## Date type internal representation

### Q1 — Internal storage

| Option | Description | Selected |
|--------|-------------|----------|
| Embedded `time.Time`, normalized to UTC midnight | ARCHITECTURE Pattern 3; ergonomic; document comparison rule. | ✓ |
| Civil-style struct `{Year, Month, Day}` | Truly TZ-free; more code in helpers. | |
| `type Date = time.Time` alias | Loses ability to attach Date-specific methods. | |

**User's choice:** Embedded time.Time, normalized to UTC midnight.

### Q2 — UnmarshalJSON error vocabulary for null/empty

| Option | Description | Selected |
|--------|-------------|----------|
| Wrap unexported `errEmptyDate` with %w | Not exported; sentinel surface stays at 5. | ✓ |
| Return `*time.ParseError` directly | More stdlib-feeling but verbose. | |
| Promote to exported `ErrInvalidDate` | Expands locked 5-sentinel list. | |

**User's choice:** Wrap unexported errEmptyDate.

### Q3 — MarshalJSON for zero value

| Option | Description | Selected |
|--------|-------------|----------|
| Always emit `"YYYY-MM-DD"` | Round-trip exact; matches `time.Time.MarshalJSON`. | ✓ |
| Emit `null` for zero value | Asymmetric round-trip is surprising. | |
| Return error for zero value | Breaks naive `json.Marshal(holiday)`. | |

**User's choice:** Always emit "YYYY-MM-DD".

### Q4 — DST-correct day-counting helper location

| Option | Description | Selected |
|--------|-------------|----------|
| `Date.DaysUntil(other)` lives on Date | Reusable; Phase 3 wires it up. | ✓ |
| Inline in `Holiday.Days()` (Phase 3) | Phase 1 stays minimal. | |
| Skip — callers do their own math | Accepts Pitfall TZ-2 bug. | |

**User's choice:** Date.DaysUntil(other) on Date in date.go.

### Q5 — Comparison helpers

| Option | Description | Selected |
|--------|-------------|----------|
| Add `Date.Equal/Before/After/Compare` (shadow time.Time) | Defends against non-UTC-constructed Date. | ✓ |
| Rely on embedded `time.Time` methods | Less Phase 1 code; documentation-heavy. | |
| Defer to Phase 3 | Risk: ad-hoc normalization. | |

**User's choice:** Add own comparison helpers.

### Q6 — String() override

| Option | Description | Selected |
|--------|-------------|----------|
| Override to `YYYY-MM-DD` | Matches JSON; CLI table output. | ✓ |
| Inherit `time.Time.String()` | Verbose default form. | |

**User's choice:** Override to YYYY-MM-DD.

### Q7 — FuzzDateUnmarshal timing

| Option | Description | Selected |
|--------|-------------|----------|
| Ship `FuzzDateUnmarshal` in Phase 1 | Pitfall JSON-3 mandate. | ✓ |
| Defer all fuzz to Phase 5 | Strict ROADMAP adherence. | |

**User's choice:** Ship in Phase 1.

### Q8 — NewDate constructor

| Option | Description | Selected |
|--------|-------------|----------|
| Add `NewDate(y, m, d)` | Guarantees UTC midnight; ergonomic for tests. | ✓ |
| Use `Date{time.Date(...)}` literals | Less API surface; risks non-UTC location. | |
| Add `ParseDate(string) (Date, error)` instead | String-input analog. | |

**User's choice:** Add NewDate constructor.

### Q9 — ParseDate

| Option | Description | Selected |
|--------|-------------|----------|
| Ship `ParseDate` in Phase 1 | Backs CLI flag parsing (Phase 5). | ✓ |
| Defer to Phase 5 | Minimal Phase 1 surface. | |

**User's choice:** Ship ParseDate in Phase 1.

---

## APIError shape

### Q1 — Field set

| Option | Description | Selected |
|--------|-------------|----------|
| `{StatusCode, Path, Body, Message}` | Brief minimum + best-effort parsed message. | ✓ |
| `{StatusCode, Path, Body}` | Strict brief minimum. | |
| `{StatusCode, Path, Body, Message, RetryAfter}` | Premature bloat. | |

**User's choice:** {StatusCode, Path, Body, Message}.

### Q2 — Is(target) for status-code matching

| Option | Description | Selected |
|--------|-------------|----------|
| Yes — match by StatusCode if target.StatusCode != 0 | ARCHITECTURE Pattern 6 idiom. | ✓ |
| No — callers use errors.As only | Less API surface. | |

**User's choice:** Yes, implement Is() with status-code match.

### Q3 — Body cap

| Option | Description | Selected |
|--------|-------------|----------|
| 4 KiB | ARCHITECTURE Pattern 6 default. | ✓ |
| 64 KiB | Larger margin for verbose errors. | |
| 10 MiB | Excessive symmetry with success responses. | |

**User's choice:** 4 KiB cap.

### Q4 — Construction timing

| Option | Description | Selected |
|--------|-------------|----------|
| Type only in Phase 1; construction in Phase 2 | Phase 1 has no `*http.Response` to read. | ✓ |
| Unexported `newAPIError(resp, path)` in Phase 1 | Logic tested in Phase 1. | |
| Exported `NewAPIError` in Phase 1 | Premature export. | |

**User's choice:** Type only in Phase 1.

### Q5 — Error() format

| Option | Description | Selected |
|--------|-------------|----------|
| `"openholidays: api error <status> at <path>: <message>"` | ARCHITECTURE Pattern 6 idiom. | ✓ |
| Path-first compact form | Less conventional. | |
| JSON-shape error string | Unusual; breaks log grep. | |

**User's choice:** ARCHITECTURE Pattern 6 format.

### Q6 — Unwrap support

| Option | Description | Selected |
|--------|-------------|----------|
| No Unwrap (leaf type) | Phase 1 has no internal cause to wrap. | ✓ |
| Add unexported cause field | Premature flexibility. | |

**User's choice:** No Unwrap.

---

## Validation error vocabulary

### Q1 — Reporting validFrom > validTo

| Option | Description | Selected |
|--------|-------------|----------|
| Add `ErrInvalidDateRange` as 5th sentinel | Two distinct conditions, two identities. | ✓ |
| Reuse `ErrDateRangeTooLarge` for both | Stay literal to ROADMAP criterion #2. | |
| Hierarchy: `ErrDateRangeTooLarge` wraps `ErrInvalidDateRange` | Sentinel hierarchies are unusual in Go. | |

**User's choice:** Add ErrInvalidDateRange as 5th sentinel. Requires PROJECT.md Key Decisions entry (CL-01).

### Q2 — validateLanguage casing

| Option | Description | Selected |
|--------|-------------|----------|
| Strict lowercase | Symmetric with VALID-01 strict-uppercase. | |
| Case-insensitive, lowercase internally | Caller-friendly. | ✓ |
| Lowercase + allowlist | Premature for v0.1.0. | |

**User's choice:** Case-insensitive, lowercase internally.

### Q3 — validateCountry casing (consistency follow-up)

| Option | Description | Selected |
|--------|-------------|----------|
| Keep VALID-01 strict uppercase | Matches ISO 3166-1 spec. | |
| Case-insensitive, uppercase internally | Symmetric ergonomics with language. | ✓ |

**User's choice:** Case-insensitive, uppercase internally. Requires PROJECT.md Key Decisions entry (CL-02).

### Q4 — 3-year threshold semantics

| Option | Description | Selected |
|--------|-------------|----------|
| `validTo.Before(validFrom.AddDate(3, 0, 1))` | Calendar arithmetic; leap-year correct. | ✓ |
| `3*365*24h + 24h` duration | Leap-tolerant but arbitrary near boundaries. | |
| `3*365*24h` strict | Wrong near leap days. | |

**User's choice:** Calendar 3 years via AddDate.

### Q5 — Error message context

| Option | Description | Selected |
|--------|-------------|----------|
| Wrap with `fmt.Errorf("%w: %q", sentinel, value)` | Bad value visible for debugging. | ✓ |
| Sentinel only | Safer if values ever contained secrets. | |
| Typed `*ValidationError{Field, Value, Reason}` | More API surface to commit to. | |

**User's choice:** Wrap with %w + %q value context.

---

## Claude's Discretion

- File layout (`types.go`, `date.go`, `errors.go`, `validate.go`, `doc.go`, `version.go`) follows ARCHITECTURE.md verbatim.
- `package openholidays` for in-package tests; `package openholidays_test` reserved for `example_test.go` (Phase 5).
- testify added under `require` in `go.mod`; verify with `go mod why github.com/stretchr/testify`.
- Godoc comments start with the symbol name for every exported identifier.
- All error strings prefixed `"openholidays: "`.

## Deferred Ideas

- Language allowlist (closed set of ISO 639-1 codes) — Languages endpoint is upstream source of truth.
- `*APIError.Unwrap()` with internal cause field — can be added in v0.2 non-breakingly.
- Typed `*ValidationError` struct — sentinels + `%w` are the idiomatic answer.
- Exported `NewAPIError` constructor — add later if downstream consumers ask.
- `Date.MarshalText` / `UnmarshalText` for `database/sql` interop — flag for v0.2 if a consumer asks.
