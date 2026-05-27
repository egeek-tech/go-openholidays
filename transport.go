// Package openholidays — HTTP RoundTripper decorators.
//
// This file ships two unexported http.RoundTripper structs that compose
// Phase 2's HTTP middleware chain (D-29):
//
//	req → headerTransport → loggingTransport → underlying http.RoundTripper
//
// headerTransport injects the SDK's standard Accept and User-Agent headers,
// always cloning the inbound request via req.Clone(req.Context()) BEFORE any
// header mutation so the caller's *http.Request is never modified (D-30,
// Pitfall HTTP-2 — Header maps are shared across req.WithContext copies and
// mutating them would race with concurrent reuse of the same request).
//
// loggingTransport emits exactly one slog.LevelDebug record per round trip
// with the six OBS-02 fields (method, path, status, duration_ms, attempt,
// bytes_in). The response body is NEVER read inside RoundTrip (D-31, OBS-01,
// Pitfall OBS-1 — reading the body here would consume bytes before the
// endpoint decoder runs and would risk leaking payload data into operator
// logs at non-Debug levels via a downstream handler bug).
//
// Neither transport is exported. Endpoint methods consume the chain via the
// *http.Client returned by composeHTTPClient (Phase 2 plan 02 wires the
// chain into Client construction).

package openholidays

import (
	"log/slog"
	"net/http"
	"time"
)

// headerTransport injects the SDK's standard request headers when the caller
// did not supply them.
//
// Contract:
//
//   - Sets Accept to "application/json" when the inbound request lacks one.
//   - Sets User-Agent to h.userAgent (typically "go-openholidays/<Version>")
//     when the inbound request lacks one.
//   - Caller-supplied Accept and User-Agent values are preserved verbatim
//     (caller override wins per D-30 / TRANS-01).
//
// Implementation note: RoundTrip deep-copies the inbound request via
// req.Clone(req.Context()) BEFORE any header mutation. The http.RoundTripper
// contract (pkg.go.dev/net/http#RoundTripper) requires that RoundTrip not
// modify the request, and req.Header is a shared http.Header map across
// shallow copies (req.WithContext does NOT deep-copy it); only req.Clone
// produces a deep-copied Header map. Mutating the caller's req.Header would
// otherwise race with concurrent reuse of the same *http.Request (Pitfall
// HTTP-2).
type headerTransport struct {
	userAgent string
	next      http.RoundTripper
}

// RoundTrip clones the inbound request, sets the SDK's default Accept and
// User-Agent headers when absent, and delegates to the next RoundTripper.
// See the headerTransport godoc for the full contract.
func (h *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	reqCopy := req.Clone(req.Context())
	if reqCopy.Header.Get("Accept") == "" {
		reqCopy.Header.Set("Accept", "application/json")
	}
	if reqCopy.Header.Get("User-Agent") == "" {
		reqCopy.Header.Set("User-Agent", h.userAgent)
	}
	return h.next.RoundTrip(reqCopy)
}

// loggingTransport emits a single slog.LevelDebug record per HTTP round trip
// carrying the six OBS-02 structured fields.
//
// Fields (D-31 / OBS-02):
//
//   - method:      req.Method (e.g., "GET").
//   - path:        req.URL.Path (no host, no query — host is fixed per
//     Client; query is omitted to avoid leaking parameters at Debug).
//   - status:      resp.StatusCode, or -1 when resp is nil (network failure).
//   - duration_ms: time.Since(start).Milliseconds(), int64.
//   - attempt:     hardcoded to 1 in Phase 2. Phase 3's retry transport will
//     inject a per-attempt counter via a context value; loggingTransport
//     will then read it from req.Context().
//   - bytes_in:    resp.ContentLength when known. The value is -1 for
//     HTTP/2 chunked responses (which the live OpenHolidays API uses) and
//     for HTTP/1.1 Transfer-Encoding: chunked — both documented stdlib
//     semantics for "unknown length", forwarded unchanged (NOT coerced to
//     0). Returns -1 when resp is nil.
//
// Response body invariant (OBS-01, Pitfall OBS-1): RoundTrip MUST NOT call
// Read, io.ReadAll, or io.Copy on the response body. Doing so would consume
// bytes before the endpoint decoder runs and risks leaking payload data into
// operator logs via a downstream handler that elevates Debug records. The
// unit test transport_logging_test.go::TestLoggingTransport_RoundTrip
// mechanically asserts this via a trackedReader whose Read counter must
// remain at zero after RoundTrip.
//
// Level invariant (OBS-01): the record is emitted at slog.LevelDebug only.
// No other level appears in this file.
type loggingTransport struct {
	logger *slog.Logger
	next   http.RoundTripper
}

// RoundTrip delegates to the next RoundTripper, then emits exactly one
// Debug-level slog record with the six OBS-02 fields. See the
// loggingTransport godoc for the full contract.
func (l *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := l.next.RoundTrip(req)
	l.logger.LogAttrs(req.Context(), slog.LevelDebug, "openholidays http",
		slog.String("method", req.Method),
		slog.String("path", req.URL.Path),
		slog.Int("status", statusOf(resp)),
		slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		slog.Int("attempt", 1),
		slog.Int64("bytes_in", bytesIn(resp)),
	)
	return resp, err
}

// statusOf returns resp.StatusCode when resp != nil, else -1.
//
// Nil-safety rationale: on a network-level failure the next RoundTripper
// returns (nil, err). loggingTransport still emits its single record so the
// failure is observable; statusOf must therefore tolerate a nil response
// without panicking. The -1 sentinel is the documented "no status" value in
// the field — it cannot collide with any valid HTTP status code, which are
// all in [100, 599].
func statusOf(resp *http.Response) int {
	if resp == nil {
		return -1
	}
	return resp.StatusCode
}

// bytesIn returns resp.ContentLength when resp != nil, else -1.
//
// Nil-safety rationale: same as statusOf — a network-level failure produces
// a nil response, but loggingTransport still emits its record.
//
// HTTP/2 chunked semantics: stdlib net/http documents ContentLength as -1
// when the response uses chunked transfer encoding (HTTP/1.1 chunked) or
// HTTP/2 framing (which carries length per-frame and commonly omits a total
// Content-Length header). The live OpenHolidays API uses HTTP/2; operators
// should expect bytes_in == -1 on every successful call. bytesIn forwards
// the -1 unchanged rather than coercing to 0, because 0 would be
// indistinguishable from a genuinely zero-length response and would lose
// the diagnostic signal.
func bytesIn(resp *http.Response) int64 {
	if resp == nil {
		return -1
	}
	return resp.ContentLength
}
