// Package openholidays — shared HTTP-and-decode pipeline used by every
// endpoint method.
//
// This file ships the single, generic helper doJSONGet[T any] that
// consolidates the Phase 2 D-41..D-45 + D-24 oversize-gate pipeline
// (originally inlined in countries.go) so each new endpoint method shrinks
// to validate → build query → doJSONGet → optional post-decode validate →
// return (D-62 / D-63). The buildAPIError and parseAPIMessage helpers and
// the maxResponseBytes / apiErrorBodyCap constants live here too — their
// natural home is the shared pipeline, not the Countries-specific endpoint
// file.

package openholidays

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// maxResponseBytes is the hard ceiling on any decoded response body (D-25).
// 10 MiB. Not configurable in v0.1.0 — PROJECT.md fixes the cap.
const maxResponseBytes = 10 << 20

// apiErrorBodyCap is the maximum number of upstream body bytes copied into
// APIError.Body (Phase 1 D-17). 4 KiB. The cap bounds the byte cost of
// echoing a hostile multi-MB error envelope into operator logs while still
// preserving enough context for diagnostics.
const apiErrorBodyCap = 4 << 10

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
// error-bearing helpers.
//
// Post-decode validation is the caller's responsibility — doJSONGet does
// NOT inspect the decoded value. PublicHolidays and SchoolHolidays will
// call a separate validateHolidays helper (D-65, lands in Plan 04) on the
// decode result before returning.
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
	// Retry loop wrapping c.http.Do(req) per D-77 + RESIL-05: retry lives
	// in the endpoint layer (NOT a RoundTripper) so caller-supplied
	// *http.Client retry middleware does NOT double-fire. The loop runs
	// exactly maxAttempts times when c.retry.maxAttempts > 0, else once
	// (retry disabled — D-74). shouldRetry / parseRetryAfter /
	// computeBackoff live in retry.go (Plan 03 Task 1). The decode path
	// and the 4xx/5xx error-build path below the loop run ONCE after the
	// loop exits with the final response.
	var (
		resp    *http.Response
		httpErr error
	)
	maxAttempts := 1
	if c.retry.maxAttempts > 0 {
		maxAttempts = c.retry.maxAttempts
	}
	for attempt := 0; attempt < maxAttempts; attempt++ {
		// Pitfall RETRY-3 + D-75: ctx.Err() at loop top so a caller
		// cancellation between attempts surfaces immediately as ctx.Err()
		// without another c.http.Do dispatch. Ctx errors are NEVER
		// retried (D-75).
		if ctxErr := ctx.Err(); ctxErr != nil {
			return zero, ctxErr
		}
		resp, httpErr = c.http.Do(req)
		if !shouldRetry(resp, httpErr) {
			break
		}
		// Last attempt — surface the error verbatim without sleeping.
		// The post-loop block wraps with retry-exhaustion context.
		if attempt == maxAttempts-1 {
			break
		}
		// Pitfall HTTP-3: drain+close the about-to-be-retried response
		// so its keep-alive connection is reusable (the deferred drain-
		// and-close below only handles the FINAL resp). LimitReader
		// bounds the drain at maxResponseBytes+1 (T-02-12).
		var retryAfter time.Duration
		if resp != nil {
			retryAfter, _ = parseRetryAfter(resp.Header.Get("Retry-After"), c.nowFunc())
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes+1))
			_ = resp.Body.Close()
			// Nil out resp so the post-loop drain-and-close defer never
			// double-closes a body whose underlying connection was
			// already returned to the pool. The next iteration will
			// overwrite resp with a fresh c.http.Do return.
			resp = nil
		}
		delay := computeBackoff(attempt, retryAfter, c.retry, c.rand)
		if sleepErr := c.sleepFunc(ctx, delay); sleepErr != nil {
			return zero, sleepErr // ctx.Err() on cancel during sleep
		}
	}
	if httpErr != nil {
		// Retry exhaustion (maxAttempts > 1): wrap with the explicit
		// retry-exhaustion prefix per D-77 so callers branching on
		// errors.Is(err, ErrEmptyResponse) / errors.As(err, &APIError)
		// still match via %w. When retry was disabled (maxAttempts == 1),
		// preserve the existing Phase 2 error-wrap shape verbatim.
		if maxAttempts > 1 {
			return zero, fmt.Errorf("openholidays: retry exhausted (%d attempts): %w", maxAttempts, httpErr)
		}
		return zero, fmt.Errorf("openholidays: GET %s: %w", path, httpErr)
	}
	defer func() {
		// Drain before close so the keep-alive connection can be reused
		// (PITFALLS HTTP-3). LimitReader bounds the drain at
		// maxResponseBytes+1 so a hostile infinite stream cannot block the
		// drain indefinitely (T-02-12).
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes+1))
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= 400 {
		return zero, buildAPIError(resp, path)
	}
	var out T
	limited := &io.LimitedReader{R: resp.Body, N: maxResponseBytes}
	decoder := json.NewDecoder(limited)
	if c.strict {
		decoder.DisallowUnknownFields()
	} // D-92: applied BEFORE Decode; runs on every call including cache hits (D-93).
	if decodeErr := decoder.Decode(&out); decodeErr != nil {
		if errors.Is(decodeErr, io.EOF) {
			return zero, fmt.Errorf("openholidays: empty %s response: %w", path, ErrEmptyResponse)
		}
		// Mid-truncation gate (RESEARCH.md Pitfall 5, option 2): when the
		// LimitReader exhausts its budget (limited.N == 0), the upstream
		// payload exceeded maxResponseBytes and Decode surfaces
		// io.ErrUnexpectedEOF (or *json.SyntaxError) because the closing
		// token was never reached. Prefer ErrResponseTooLarge so caller
		// branching works uniformly across boundary and mid-truncation
		// cases. Testing limited.N (not resp.Body.Read) avoids the CR-01
		// false positive where HTTP/2 chunked framing leaves stray
		// post-JSON bytes in resp.Body that have nothing to do with
		// overflow.
		if limited.N == 0 {
			return zero, fmt.Errorf("openholidays: response exceeded %d bytes: %w", maxResponseBytes, ErrResponseTooLarge)
		}
		return zero, fmt.Errorf("openholidays: decode %s: %w", path, decodeErr)
	}
	// Boundary-truncation gate (D-24 / RESEARCH.md Pitfall 5): use the
	// decoder's own More() to ask "is another JSON value waiting?" — this
	// correctly ignores RFC 8259 whitespace (newlines, spaces, tabs) that
	// servers commonly emit after the closing token, so trailing
	// whitespace in a separate HTTP/2 chunk no longer triggers a false
	// ErrResponseTooLarge (CR-01). When upstream genuinely sent more than
	// maxResponseBytes and Decode finished on a valid boundary, More()
	// returns true (another JSON value is starting) and the sentinel
	// fires.
	if decoder.More() {
		return zero, fmt.Errorf("openholidays: response exceeded %d bytes: %w", maxResponseBytes, ErrResponseTooLarge)
	}
	return out, nil
}

// buildAPIError constructs an *APIError from a non-2xx *http.Response. The
// upstream body is read via io.LimitReader so APIError.Body never exceeds
// apiErrorBodyCap (4 KiB, Phase 1 D-17). Message is parsed best-effort via
// parseAPIMessage; an unparseable body yields an empty Message and the
// Error() output omits the suffix.
//
// The drain-then-close defer in doJSONGet handles closing resp.Body — this
// helper only consumes (at most) the first 4 KiB.
func buildAPIError(resp *http.Response, path string) *APIError {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, apiErrorBodyCap))
	msg := parseAPIMessage(body)
	return &APIError{
		StatusCode: resp.StatusCode,
		Path:       path,
		Body:       body,
		Message:    msg,
	}
}

// validateHolidays runs the post-decode Holiday schema-drift checks
// mandated by D-65 / CL-12 / Pitfall JSON-4. The function returns nil when
// every Holiday in hs satisfies the invariants, or the first violation
// wrapped via fmt.Errorf %w against ErrMalformedResponse.
//
// Invariants enforced (per holiday):
//
//   - h.StartDate.IsZero() == false. A zero Date corresponds to the
//     time.Time zero value (Pitfall JSON-4) and would silently masquerade
//     as a valid 0001-01-01 calendar date downstream. Reject loudly here
//     so callers can branch with errors.Is(err, ErrMalformedResponse).
//   - h.EndDate.IsZero() == false. Same rationale as StartDate; the
//     upstream OpenAPI spec marks both as required so any zero EndDate
//     is upstream schema drift.
//   - h.EndDate.Before(h.StartDate) == false. Single-day holidays where
//     EndDate == StartDate are accepted (the upstream emits this shape
//     for every public holiday). A strictly-before EndDate would make
//     Holiday.Range yield zero items and Holiday.Days return a negative
//     count — both observable bugs for callers iterating returned
//     holidays.
//
// path is the request path (e.g. "/PublicHolidays" or "/SchoolHolidays").
// It is included in the wrapped error message so a multi-endpoint failure
// surfaces which endpoint produced the violation. The offending Holiday's
// ID is also echoed so a bug report against the upstream can reference the
// stable UUID rather than the human-readable name.
//
// Placement (RESEARCH.md Pattern 6): validateHolidays lives next to
// doJSONGet in request.go because it is conceptually post-decode validation
// owned by the response pipeline, not a Holiday-type method. Putting it on
// the Holiday method set would tempt future contributors to expose it (D-65
// is explicit that the function stays unexported in v0.x). The pointer
// iteration `h := &hs[i]` avoids per-iteration struct copies of the
// ~10-field Holiday value (rangeValCopy linter recommendation); the
// function does NOT mutate *h.
//
// TZ-2 / Pitfall JSON-4 are the two pitfalls this guard closes — both Dates
// are UTC-midnight by virtue of Date.UnmarshalJSON normalization, so the
// IsZero / Before checks are calendar-correct without further conversion.
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

// parseAPIMessage best-effort extracts a human-readable message from an
// upstream error body. The OpenHolidays API returns RFC 7807 ProblemDetails
// envelopes (verified live 2026-05-27); the priority order detail → title →
// error reflects the field most likely to carry the human-facing message:
//
//   - detail: per-occurrence narrative (most specific; preferred).
//   - title: generic class label (fallback when detail is absent).
//   - error: legacy shorthand observed on some 5xx responses (third-priority
//     fallback).
//
// Returns the empty string when the body is not valid JSON or when none of
// the three fields are populated.
func parseAPIMessage(body []byte) string {
	var env struct {
		Detail string `json:"detail"`
		Title  string `json:"title"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return ""
	}
	switch {
	case env.Detail != "":
		return env.Detail
	case env.Title != "":
		return env.Title
	case env.Error != "":
		return env.Error
	default:
		return ""
	}
}
