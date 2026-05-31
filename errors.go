// error surface.
//
// This file ships the seven exported sentinel error values plus the *APIError
// leaf type. Phase 1 shipped the original five sentinels; Phase 2 added
// ErrResponseTooLarge (CL-07); Phase 3 appends ErrMalformedResponse (CL-12,
// D-66) for post-decode Holiday schema-drift detection by validateHolidays
// in request.go.

package openholidays

import (
	"errors"
	"fmt"
)

// Sentinel errors. Callers should detect them via errors.Is through
// fmt.Errorf("...: %w", ...) wrappers.
//
// Sentinels are declared one-per-var (not grouped under a single var block)
// so each godoc comment lives at column 0 — required by the DOC-07 audit
// (Plan 05-07) and by Go's standard godoc rendering conventions, which treat
// a comment immediately preceding `var Name = ...` as that symbol's doc only
// when the comment begins at column 0.

// ErrInvalidCountry is returned for malformed country codes
// (not exactly two ASCII letters after canonicalization).
var ErrInvalidCountry = errors.New("openholidays: invalid country code")

// ErrInvalidLanguage is returned for malformed language codes
// (not exactly two ASCII letters after canonicalization).
var ErrInvalidLanguage = errors.New("openholidays: invalid language code")

// ErrDateRangeTooLarge is returned when the validFrom..validTo window
// spans more than 3 calendar years inclusive.
var ErrDateRangeTooLarge = errors.New("openholidays: date range too large")

// ErrInvalidDateRange is returned when validFrom is strictly after validTo.
var ErrInvalidDateRange = errors.New("openholidays: invalid date range")

// ErrEmptyResponse is returned when the upstream returns a 2xx with an
// empty body where a non-empty payload was required.
var ErrEmptyResponse = errors.New("openholidays: empty response body")

// ErrResponseTooLarge is returned when an upstream response exceeds the
// 10 MiB cap. Both boundary-truncation (Decode finishes on a valid JSON
// boundary, sentinel-byte read detects extra bytes) and mid-truncation
// (Decode surfaces [io.ErrUnexpectedEOF], sentinel-byte read confirms the
// body has more bytes) cases produce this sentinel — see RESEARCH.md
// Pitfall 5 and Plan 02-03 deviation 1.
var ErrResponseTooLarge = errors.New("openholidays: response too large")

// ErrMalformedResponse is returned whenever an upstream response body cannot be
// turned into valid, schema-conforming data. It unifies two failure modes under
// one sentinel so callers have a single "the response body was malformed" check:
//
//   - A body that is not decodable as the expected JSON shape — a syntax error
//     or type mismatch. The underlying *[json.SyntaxError] / *[json.UnmarshalTypeError]
//     remains recoverable via [errors.As].
//   - A structurally-decodable response that violates the Holiday post-decode
//     invariants checked by validateHolidays: StartDate non-zero, EndDate
//     non-zero, and EndDate not strictly before StartDate.
//
// In both cases the sentinel is wrapped via [fmt.Errorf] with the %w verb so
// [errors.Is](err, ErrMalformedResponse) holds through the endpoint method's
// caller-facing wrap. It still differentiates a malformed body from transport
// failures, *APIError 4xx/5xx responses, empty bodies (ErrEmptyResponse), and
// oversize bodies (ErrResponseTooLarge), and closes Pitfall JSON-4 (time.Time
// zero value masquerading as a valid Date).
var ErrMalformedResponse = errors.New("openholidays: malformed response")

// APIError represents a non-2xx response from the upstream API.
//
// Phase 1 ships the type, its Error method, and its Is method only;
// construction (reading resp.Body, parsing Message) lands in Phase 2
// alongside the first endpoint method.
//
// Callers match by status code with [errors.Is]:
//
//	if [errors.Is](err, &openholidays.APIError{StatusCode: 404}) { ... }
//
// The wildcard form (zero StatusCode) matches any *APIError, allowing
// callers to ask "was this an API error at all?":
//
//	if [errors.Is](err, &openholidays.APIError{}) { ... }
//
// Use [errors.As] to recover the populated value:
//
//	var apiErr *openholidays.APIError
//	if errors.As(err, &apiErr) { _ = apiErr.StatusCode }
type APIError struct {
	StatusCode int    // HTTP status code (>= 400 when populated by Phase 2)
	Path       string // Request path (e.g., "/PublicHolidays")
	Body       []byte // Raw response body. Phase 2 caps the populated length at 4 KiB.
	Message    string // Best-effort message parsed from upstream JSON; empty when unparseable.
}

// audit:ok 2026-05-30

// Error returns a human-readable description of the API error.
//
// When Message is empty:
//
//	openholidays: api error <status> at <path>
//
// Otherwise:
//
//	openholidays: api error <status> at <path>: <message>
//
// The Body field is intentionally omitted from the Error output so that raw
// upstream payloads never leak into operator-visible error strings.
func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("openholidays: api error %d at %s", e.StatusCode, e.Path)
	}
	return fmt.Sprintf("openholidays: api error %d at %s: %s", e.StatusCode, e.Path, e.Message)
}

// audit:ok 2026-05-30

// Is supports [errors.Is](err, &APIError{StatusCode: N}) status-code matching.
//
// Semantics:
//
//   - target is not *APIError: returns false.
//   - target.StatusCode == 0 (the wildcard): matches any *APIError, i.e.
//     "was this an API error at all?".
//   - target.StatusCode != 0: matches when e.StatusCode == target.StatusCode.
//
// The Path, Body, and Message fields on the target are intentionally ignored —
// they exist for diagnostics, not for matching. A future contributor extending
// Is to consider those fields would silently break callers that rely on
// status-only branching; the unit tests assert this guarantee.
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
