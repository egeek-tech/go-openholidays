// Package openholidays — retry layer for the doJSONGet HTTP pipeline.
//
// This file declares the four building blocks of the Phase 4 retry layer
// per D-77:
//
//   - retryConfig (unexported struct: maxAttempts / baseDelay / maxWait)
//   - shouldRetry(resp, err) — pure predicate over the D-75 matrix
//   - parseRetryAfter(h, now) — RFC 7231 §7.1.1.1 parser (integer seconds
//     + RFC 1123 / RFC 850 / ANSI C asctime via http.ParseTime, plus the
//     Pitfall 9 negative-duration guard)
//   - computeBackoff(attempt, retryAfter, cfg, rnd) — full-jitter
//     exponential backoff (AWS canonical formula) with Retry-After
//     promotion, capped at cfg.maxWait
//
// Plus two named constants documenting the D-74 defaults:
//
//   - defaultBaseDelay     (250ms; baseDelay fallback when caller passes
//     a non-positive value)
//   - defaultMaxRetryWait  (60s; maxWait fallback consistent with the
//     AWS guidance summarized in Pitfall RETRY-2)
//
// Retry placement (RESIL-05): the loop that calls these helpers lives in
// request.go::doJSONGet (Plan 03 Task 3), NOT in a RoundTripper. Caller-
// supplied *http.Client retry middleware therefore does NOT double-fire.
// This file contains pure helpers only — no Client dependency, no HTTP
// dispatch — which keeps each function trivially unit-testable.
//
// Requirements covered by this layer: RESIL-01 (full-jitter exponential
// backoff), RESIL-02 (retryable-conditions matrix), RESIL-03 (Retry-After
// honoring), RESIL-04 (ctx-aware sleep — implemented in doJSONGet via
// Client.sleepFunc, not here), RESIL-05 (NOT a RoundTripper — enforced by
// retry_test.go::TestRetry_NotARoundTripper), and TEST-05 (deterministic
// clock — exercised by retry_test.go via the fakeClock helper from
// clock_test.go).
//
// No init() and no package-level vars — only consts. Keeps the CLIENT-10
// AST audit in internal_test.go green without modification to its
// allowlist (the audit flags package-level `var` declarations; named
// constants are exempt).

package openholidays

import (
	"context"
	"errors"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"syscall"
	"time"
)

// defaultBaseDelay is the fallback baseDelay applied by WithRetry when the
// caller supplies a non-positive value (D-74). 250ms is the FEATURES.md
// research recommendation: short enough to avoid noticeable user-facing
// delay on transient blips, long enough that the second attempt does not
// land in the same upstream rate-limit window as the first.
const defaultBaseDelay = 250 * time.Millisecond

// defaultMaxRetryWait is the fallback per-attempt sleep ceiling applied by
// WithRetry / WithMaxRetryWait when the caller supplies a non-positive
// value (D-74; Pitfall RETRY-2 AWS guidance). 60s bounds the worst-case
// per-attempt sleep even in the face of a hostile upstream returning
// Retry-After: 999999999 (threat T-04-05) — the cumulative budget is
// further bounded by the caller's ctx deadline.
const defaultMaxRetryWait = 60 * time.Second

// retryConfig is the unexported retry policy carried by Client.retry.
// Zero-value (maxAttempts == 0) means retry is disabled — opt-in for M1
// per D-74 and STATE.md.
//
// Fields:
//
//   - maxAttempts: maximum number of c.http.Do invocations inside the
//     retry loop. <=0 means disabled (the loop runs exactly once and
//     surfaces the first response/error verbatim).
//   - baseDelay: base unit for exponential backoff. The cap at attempt
//     N is baseDelay << N (clamped at maxWait); jitter is uniform in
//     [0, capped).
//   - maxWait: per-attempt sleep ceiling. Note this is NOT a cumulative
//     budget — five attempts with 60s cap can still take ~5 min total.
//     Consumers wanting a cumulative cap supply ctx.WithTimeout.
type retryConfig struct {
	maxAttempts int
	baseDelay   time.Duration
	maxWait     time.Duration
}

// shouldRetry reports whether the supplied (resp, err) pair from a
// preceding c.http.Do(req) call qualifies for another attempt per the
// D-75 retryable-conditions matrix (RESIL-02). The function is pure —
// no Client dependency, no HTTP dispatch — so unit testing covers every
// branch with a single table-driven test (TestShouldRetry in retry_test.go).
//
// Matrix (D-75):
//
// When err != nil:
//   - net.Error with Timeout() == true (RESIL-02 explicit) → retry.
//   - errors.Is(err, syscall.ECONNRESET) (transport reset) → retry.
//   - any other error (including context.Canceled and
//     context.DeadlineExceeded) → do NOT retry. The endpoint-layer
//     retry loop in doJSONGet checks ctx.Err() at the top of the
//     iteration BEFORE calling shouldRetry, so by construction
//     shouldRetry should never see a raw ctx error here; the defensive
//     false return ensures correct behavior even if upstream changes
//     the ordering.
//
// When resp != nil and err == nil:
//   - HTTP statuses {408, 429, 500, 502, 503, 504} → retry. The
//     upstream OpenAPI spec only documents 400/500, but the set is
//     forward-defensive per Pitfall RETRY-1 (every modern HTTP client
//     retries this set).
//   - any other status (including 200, 201, 400, 401, 403, 404) → do
//     NOT retry.
//
// When both resp and err are nil (defensive — shouldn't happen but be
// explicit): do NOT retry. A nil-resp/nil-err pair signals a programming
// error upstream of shouldRetry; retrying it would loop forever.
//
// net.Error is an interface, so the err-check uses errors.As (interface
// assertion); errors.Is would unconditionally return false for any
// interface target. syscall.ECONNRESET is a sentinel value (per-OS
// integer constant), so the conn-reset check uses errors.Is per Go's
// stdlib convention. Both forms work uniformly on Linux / macOS /
// Windows because Go's syscall package abstracts the platform values.
//
// Ctx-error guard rationale: context.DeadlineExceeded implements
// net.Error with Timeout() == true (verified live against the Go
// stdlib), which would otherwise let the net.Error branch claim
// DeadlineExceeded as retryable — directly violating D-75 ("ctx
// errors NEVER retried"). The endpoint-layer retry loop's loop-top
// ctx.Err() check (Task 3 / Pitfall RETRY-3) catches ctx errors
// before they reach c.http.Do most of the time, but a *http.Client.Do
// can also surface ctx errors wrapped inside transport errors (e.g.
// when the timer fires during DNS resolution). The explicit guard
// below ensures shouldRetry is correct in isolation — making the
// pure-function contract testable and the retry loop trivially
// composable.
func shouldRetry(resp *http.Response, err error) bool {
	if err != nil {
		// D-75: ctx errors NEVER retried. Check this BEFORE the
		// net.Error branch because context.DeadlineExceeded
		// implements net.Error with Timeout() == true and would
		// otherwise be claimed as retryable.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return false
		}
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return true
		}
		if errors.Is(err, syscall.ECONNRESET) {
			return true
		}
		return false
	}
	if resp != nil {
		switch resp.StatusCode {
		case http.StatusRequestTimeout, // 408
			http.StatusTooManyRequests,     // 429
			http.StatusInternalServerError, // 500
			http.StatusBadGateway,          // 502
			http.StatusServiceUnavailable,  // 503
			http.StatusGatewayTimeout:      // 504
			return true
		}
		return false
	}
	return false
}

// parseRetryAfter parses an HTTP Retry-After header per RFC 7231
// §7.1.1.1 and returns the corresponding sleep duration relative to the
// supplied now (D-76). The header accepts two forms in the wild:
//
//   - delta-seconds:    integer seconds (e.g. "120"), parsed by
//     strconv.Atoi. A non-negative integer returns (s*time.Second,
//     true).
//   - HTTP-date:        RFC 1123 / RFC 850 / ANSI C asctime (e.g.
//     "Wed, 21 Oct 2026 07:28:00 GMT"), parsed by http.ParseTime. A
//     parsed date strictly AFTER now returns (date - now, true).
//
// Pitfall 9 negative-duration guard: a parsed date at or BEFORE now is
// rejected with (0, false). A hostile or stale upstream returning a
// past-dated Retry-After would otherwise disable backoff and trigger a
// run-away retry loop (threat T-04-06).
//
// An empty header, a non-numeric non-date string, or a negative
// integer-seconds value all return (0, false). The boolean is the
// caller's signal to fall back to the jitter computation in
// computeBackoff.
//
// The function is pure — no allocation beyond stdlib internals, no
// Client dependency. Unit-tested by TestParseRetryAfter in retry_test.go.
func parseRetryAfter(h string, now time.Time) (time.Duration, bool) {
	if h == "" {
		return 0, false
	}
	if s, err := strconv.Atoi(h); err == nil && s >= 0 {
		return time.Duration(s) * time.Second, true
	}
	if t, err := http.ParseTime(h); err == nil {
		if d := t.Sub(now); d > 0 {
			return d, true
		}
	}
	return 0, false
}

// computeBackoff returns the per-attempt sleep duration applied between
// retry attempts per D-76 + RESIL-01. The formula composes three
// inputs:
//
//   - Exponential ceiling: capped := min(cfg.baseDelay << attempt,
//     cfg.maxWait). The bit-shift mirrors the AWS canonical formula
//     (base * 2^attempt) without the floating-point round-trip.
//   - Full jitter:        jitter := rand.Int64N(int64(capped)) cast
//     back to time.Duration. Uniform in [0, capped). RESIL-01
//     specifies "full jitter" (not equal jitter, not decorrelated
//     jitter) — AWS canonical, demonstrated to minimize fleet-wide
//     correlation (Pitfall RETRY-4).
//   - Retry-After promotion: when retryAfter > 0 and retryAfter >
//     jitter, retryAfter wins (caller-driven backoff overrides jitter).
//     The promotion is itself capped at cfg.maxWait so a hostile
//     Retry-After: 999999999 (threat T-04-05) cannot hold the request
//     for the lifetime of the process.
//
// The rnd argument is the per-Client *math/rand/v2.Rand seeded at
// NewClient time via newClientRand (D-78). Per-Client randomness
// prevents the thundering-herd failure mode where every instance in a
// fleet retries on identical jittered offsets after a shared upstream
// outage. Goroutine-safety of *math/rand/v2.Rand is established by the
// stdlib (math/rand/v2 docs).
//
// Defensive against a zero baseDelay: capped is forced to >=
// time.Millisecond so rand.Int64N never sees a non-positive argument
// (the stdlib panics on n <= 0). This defends against a future
// regression where WithRetry's baseDelay-fallback is removed; the
// helper still produces a finite, non-panicking sleep.
//
// min is a Go 1.21+ builtin and is safe across the project's Go 1.23
// floor (Constraints in PROJECT.md). Unit-tested by TestComputeBackoff
// (range checks) and TestComputeBackoff_HonorsRetryAfter (Retry-After
// promotion + cap) in retry_test.go.
func computeBackoff(attempt int, retryAfter time.Duration, cfg retryConfig, rnd *rand.Rand) time.Duration {
	capped := cfg.maxWait
	if exp := cfg.baseDelay << attempt; exp < capped {
		capped = exp
	}
	if capped <= 0 {
		capped = time.Millisecond
	}
	jitter := time.Duration(rnd.Int64N(int64(capped)))
	if retryAfter > 0 && retryAfter > jitter {
		return min(retryAfter, cfg.maxWait)
	}
	return jitter
}
