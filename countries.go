// Package openholidays — Countries endpoint method and supporting
// buildAPIError / parseAPIMessage helpers.
//
// This file ships the first end-to-end endpoint contract Phase 3 will mirror:
//
//   - Client.Countries(ctx) is the canonical shape every endpoint method
//     follows — nil-ctx guard → optional context.WithTimeout per c.timeout →
//     http.NewRequestWithContext → c.http.Do → defer drain-then-close →
//     status check → JSON decode → post-Decode sentinel-byte truncation gate
//     (ENDPT-01 + D-41 + D-42).
//   - buildAPIError reads up to 4 KiB of the upstream body (Phase 1 D-17) and
//     constructs *APIError with parsed Message (D-43) — TRANS-02 + Phase 1
//     APIError body-cap invariant.
//   - parseAPIMessage best-effort extracts a Message from RFC 7807
//     ProblemDetails (verified live 2026-05-27): priority detail → title →
//     error; empty string on unparseable or empty body.
//   - const maxResponseBytes = 10 << 20 holds the unconfigurable 10 MiB
//     decode cap (D-25); placement in countries.go per PATTERNS.md (only
//     Countries uses it today; Phase 3 endpoints will share the const from
//     here).
//
// Sentinel-byte truncation gate: io.LimitReader caps Decode at maxResponseBytes;
// after Decode returns nil, a single-byte Read on resp.Body detects whether
// the upstream still has bytes (i.e. the body exceeded the cap and Decode
// happened to finish on a valid JSON boundary). The result is wrapped via
// fmt.Errorf %w so errors.Is(err, ErrResponseTooLarge) holds through caller
// wraps (D-24 + RESEARCH.md Pitfall 5).

package openholidays

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// maxResponseBytes is the hard ceiling on any decoded response body (D-25).
// 10 MiB. Not configurable in v0.1.0 — PROJECT.md fixes the cap.
const maxResponseBytes = 10 << 20

// apiErrorBodyCap is the maximum number of upstream body bytes copied into
// APIError.Body (Phase 1 D-17). 4 KiB. The cap bounds the byte cost of
// echoing a hostile multi-MB error envelope into operator logs while still
// preserving enough context for diagnostics.
const apiErrorBodyCap = 4 << 10

// Countries fetches the list of supported countries from the upstream
// OpenHolidays API. Each returned Country carries an IsoCode, a per-language
// localized Name array (look up a specific language via Country.NameFor),
// and the country's OfficialLanguages list.
//
// Per-request timeout: when the Client was constructed with WithTimeout(d)
// and d > 0, Countries wraps ctx via context.WithTimeout(ctx, d) before
// dispatching. Cancellation of the caller's ctx interrupts the in-flight
// HTTP within ≤ 100 ms (CLIENT-09); errors.Is(err, context.Canceled) holds
// through the fmt.Errorf %w wrap returned on transport-level failures.
//
// Error handling:
//
//   - 4xx and 5xx upstream responses produce *APIError with the StatusCode,
//     a parsed Message (RFC 7807 ProblemDetails priority: detail → title →
//     error), and the raw response body capped at 4 KiB (Phase 1 D-17).
//     Use errors.As(err, &apiErr) to recover the populated value.
//   - 2xx with an empty body returns an error that errors.Is matches against
//     ErrEmptyResponse.
//   - Upstream responses exceeding the 10 MiB cap return an error that
//     errors.Is matches against ErrResponseTooLarge.
//   - JSON decode failures wrap the underlying error with the
//     "openholidays: decode /Countries: " prefix.
//
// Concurrent use: the Client is immutable after NewClient, so Countries is
// safe to call from any goroutine without external synchronization
// (CLIENT-07; mechanically asserted by TestClient_ConcurrentAccess under
// the race detector).
func (c *Client) Countries(ctx context.Context) ([]Country, error) {
	if ctx == nil {
		return nil, errors.New("openholidays: nil context")
	}
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/Countries", nil)
	if err != nil {
		return nil, fmt.Errorf("openholidays: build /Countries request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openholidays: GET /Countries: %w", err)
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
		return nil, buildAPIError(resp, "/Countries")
	}
	var countries []Country
	decoder := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes))
	if decodeErr := decoder.Decode(&countries); decodeErr != nil {
		if errors.Is(decodeErr, io.EOF) {
			return nil, fmt.Errorf("openholidays: empty /Countries response: %w", ErrEmptyResponse)
		}
		// Mid-truncation gate (RESEARCH.md Pitfall 5, option 2): the
		// LimitReader returns EOF at maxResponseBytes; Decode then surfaces
		// io.ErrUnexpectedEOF (or *json.SyntaxError) because the array's
		// closing `]` was never reached. If the underlying body still has
		// bytes, the truncation was caused by the size cap, not by garbage
		// JSON — prefer ErrResponseTooLarge so caller branching works
		// uniformly across boundary-truncation and mid-truncation cases.
		var one [1]byte
		if n, _ := resp.Body.Read(one[:]); n > 0 {
			return nil, fmt.Errorf("openholidays: response exceeded %d bytes: %w", maxResponseBytes, ErrResponseTooLarge)
		}
		return nil, fmt.Errorf("openholidays: decode /Countries: %w", decodeErr)
	}
	// Boundary-truncation gate (D-24 / RESEARCH.md Pitfall 5): if the
	// upstream sent more than maxResponseBytes and Decode happened to finish
	// on a valid JSON boundary, a single-byte Read on resp.Body returns
	// n > 0. Wrap via %w so errors.Is(err, ErrResponseTooLarge) holds.
	var one [1]byte
	if n, _ := resp.Body.Read(one[:]); n > 0 {
		return nil, fmt.Errorf("openholidays: response exceeded %d bytes: %w", maxResponseBytes, ErrResponseTooLarge)
	}
	return countries, nil
}

// buildAPIError constructs an *APIError from a non-2xx *http.Response. The
// upstream body is read via io.LimitReader so APIError.Body never exceeds
// apiErrorBodyCap (4 KiB, Phase 1 D-17). Message is parsed best-effort via
// parseAPIMessage; an unparseable body yields an empty Message and the
// Error() output omits the suffix.
//
// The drain-then-close defer in Countries handles closing resp.Body — this
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
